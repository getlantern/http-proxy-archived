package forward

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/interceptor"
	"github.com/getlantern/ops"

	"github.com/getlantern/http-proxy/filters"
)

var log = golog.LoggerFor("forward")

const (
	xForwardedFor = "X-Forwarded-For"
)

type Options struct {
	IdleTimeout time.Duration
	Dialer      func(network, address string) (net.Conn, error)
}

type forwarder struct {
	*Options
	ic *interceptor.Interceptor
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

	f := &forwarder{Options: opts}
	f.ic = &interceptor.Interceptor{
		Dial:      f.dial,
		OnRequest: f.modifyRequest,
	}

	return f
}

func (f *forwarder) Apply(w http.ResponseWriter, req *http.Request, next filters.Next) error {
	op := ops.Begin("proxy_http")
	defer op.End()
	f.ic.Intercept(w, req, op, 80)
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
	return req
}
