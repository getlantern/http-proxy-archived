package httpconnect

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxy"

	"github.com/getlantern/http-proxy/buffers"
	"github.com/getlantern/http-proxy/filters"
)

var log = golog.LoggerFor("httpconnect")

type Options struct {
	IdleTimeout  time.Duration
	AllowedPorts []int
	Dialer       func(network, address string) (net.Conn, error)
}

type httpConnectHandler struct {
	*Options
	intercept proxy.Interceptor
}

func New(opts *Options) filters.Filter {
	if opts.Dialer == nil {
		opts.Dialer = func(network, address string) (net.Conn, error) {
			return net.DialTimeout(network, address, 10*time.Second)
		}
	}

	f := &httpConnectHandler{Options: opts}
	f.intercept = proxy.CONNECT(opts.IdleTimeout-5*time.Second, buffers.Pool(), true, f.dial)
	return f
}

func (f *httpConnectHandler) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, dialErr := f.Dialer(network, addr)
	if dialErr != nil {
		return nil, errors.New("Unable to dial %v: %v", addr, dialErr)
	}
	conn = idletiming.Conn(conn, f.IdleTimeout, nil)
	return conn, nil
}

func (f *httpConnectHandler) Apply(w http.ResponseWriter, req *http.Request, next filters.Next) error {
	if req.Method != "CONNECT" {
		return next()
	}

	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("httpConnectHandler Middleware received request:\n%s", reqStr)
	}

	op := ops.Begin("proxy_https")
	defer op.End()
	if f.portAllowed(op, w, req) {
		err := f.intercept(context.TODO(), w, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	}

	return filters.Stop()
}

func (f *httpConnectHandler) portAllowed(op ops.Op, w http.ResponseWriter, req *http.Request) bool {
	if len(f.AllowedPorts) == 0 {
		return true
	}
	log.Tracef("Checking CONNECT tunnel to %s against allowed ports %v", req.Host, f.AllowedPorts)
	_, portString, err := net.SplitHostPort(req.Host)
	if err != nil {
		// CONNECT request should always include port in req.Host.
		// Ref https://tools.ietf.org/html/rfc2817#section-5.2.
		f.ServeError(op, w, req, http.StatusBadRequest, "No port field in Request-URI / Host header")
		return false
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		f.ServeError(op, w, req, http.StatusBadRequest, "Invalid port")
		return false
	}

	for _, p := range f.AllowedPorts {
		if port == p {
			return true
		}
	}
	f.ServeError(op, w, req, http.StatusForbidden, "Port not allowed")
	return false
}

func (f *httpConnectHandler) ServeError(op ops.Op, w http.ResponseWriter, req *http.Request, statusCode int, reason interface{}) {
	log.Error(errorf(op, "Respond error to CONNECT request to %s: %d %v", req.Host, statusCode, reason))
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, "%v", reason)
}

func errorf(op ops.Op, msg string, args ...interface{}) error {
	return op.FailIf(fmt.Errorf(msg, args...))
}
