// Package pforward provides HTTP forwarding using persistent connections,
// maintaining a 1 to 1 relationship between downstream and upstream connections.
// Clients wishing to take advantage of this capability need to send an initial
// GET request (analogous to a CONNECT request) that includes the desired host
// and the HTTP header "X-Lantern-Persistent: true".
package pforward

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/interceptor"
	"github.com/getlantern/ops"

	"github.com/getlantern/http-proxy/filters"
)

var log = golog.LoggerFor("pforward")

const (
	// XLanternPersistent is the X-Lantern-Persistent header that indicates
	// persistent connections are to be used.
	XLanternPersistent = "X-Lantern-Persistent"
	xForwardedFor      = "X-Forwarded-For"
)

type Options struct {
	Force       bool // set to true to use this without the initial http Request
	IdleTimeout time.Duration
	Dialer      func(network, address string) (net.Conn, error)
	OnRequest   func(req *http.Request)
	OnResponse  func(resp *http.Response, req *http.Request, responseNumber int) *http.Response
}

type forwarder struct {
	*Options
	ic interceptor.Interceptor
}

func New(opts *Options) filters.Filter {
	if opts == nil {
		opts = &Options{}
	}

	if opts.Dialer == nil {
		opts.Dialer = func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, time.Second*30)
		}
	}

	if opts.OnRequest == nil {
		opts.OnRequest = func(req *http.Request) {}
	}

	f := &forwarder{Options: opts}
	f.ic = interceptor.New(&interceptor.Opts{
		Dial:       f.dial,
		OnRequest:  f.modifyRequest,
		OnResponse: f.OnResponse,
	})

	return f
}

func (f *forwarder) Apply(w http.ResponseWriter, req *http.Request, next filters.Next) error {
	if !f.Force {
		persistentAllowed, _ := strconv.ParseBool(req.Header.Get(XLanternPersistent))
		if !persistentAllowed {
			return next()
		}
	}

	op := ops.Begin("proxy_http")
	defer op.End()
	f.ic.Intercept(w, req, f.Force, op, 80)
	return filters.Stop()
}

func (f *forwarder) dial(initialReq *http.Request, addr string, port int) (conn net.Conn, pipe bool, err error) {
	pipe = false
	conn, dialErr := f.Dialer("tcp", addr)
	if dialErr != nil {
		err = errors.New("Unable to dial %v: %v", addr, dialErr)
		return
	}
	conn = idletiming.Conn(conn, f.IdleTimeout, nil)
	return
}

func (f *forwarder) modifyRequest(req *http.Request) *http.Request {
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if prior, ok := req.Header[xForwardedFor]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set(xForwardedFor, clientIP)
	}
	f.OnRequest(req)
	return req
}
