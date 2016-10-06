package forward

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/ops"

	"github.com/getlantern/http-proxy/buffers"
	"github.com/getlantern/http-proxy/filters"
	"github.com/getlantern/http-proxy/utils"
)

var log = golog.LoggerFor("forward")

type Options struct {
	IdleTimeout time.Duration
	Rewriter    RequestRewriter
	Dialer      func(network, address string) (net.Conn, error)
}

type forwarder struct {
	*Options
}

type RequestRewriter interface {
	Rewrite(r *http.Request)
}

func New(opts *Options) filters.Filter {
	if opts == nil {
		opts = &Options{}
	}

	if opts.Rewriter == nil {
		opts.Rewriter = &HeaderRewriter{
			TrustForwardHeader: true,
			Hostname:           "",
		}
	}

	if opts.Dialer == nil {
		opts.Dialer = func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, time.Second*30)
		}
	}

	return &forwarder{opts}
}

func (f *forwarder) Apply(w http.ResponseWriter, req *http.Request, next filters.Next) error {
	op := ops.Begin("proxy_http")
	defer op.End()
	f.intercept(op, w, req)
	return filters.Stop()
}

func (f *forwarder) intercept(op ops.Op, w http.ResponseWriter, req *http.Request) (err error) {
	addr := req.Host
	_, _, err = net.SplitHostPort(addr)
	if err != nil {
		// Use default port
		addr = fmt.Sprintf("%v:%d", addr, 80)
	}
	clientConn, clientBuffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		desc := errorf(op, "Unable to hijack connection: %s", err)
		utils.RespondBadGateway(w, req, desc)
		return
	}
	originConnRaw, err := f.Dialer("tcp", addr)
	if err != nil {
		errorf(op, "Unable to dial %v: %v", addr, err)
		return
	}
	originConn := idletiming.Conn(originConnRaw, f.IdleTimeout, nil)

	// Pipe data through tunnel
	closeConns := func() {
		if clientConn != nil {
			if err := clientConn.Close(); err != nil {
				log.Tracef("Error closing the client connection: %s", err)
			}
		}
		if originConn != nil {
			if err := originConn.Close(); err != nil {
				log.Tracef("Error closing the origin connection: %s", err)
			}
		}
	}

	op.Go(func() {
		defer closeConns()
		buf := buffers.Get()
		defer buffers.Put(buf)
		var readErr error
		for {
			cloned := f.cloneRequest(req)
			writeErr := cloned.Write(originConn)
			if writeErr != nil {
				if isUnexpected(writeErr) {
					log.Debug(errorf(op, "Unable to write request to origin: %v", writeErr))
				}
				break
			}
			req, readErr = http.ReadRequest(clientBuffered.Reader)
			if readErr != nil {
				if isUnexpected(readErr) {
					log.Debug(errorf(op, "Unable to read next request from client: %v", readErr))
				}
				break
			}
		}
	})

	originBuffered := bufio.NewReader(originConn)
	for {
		resp, readErr := http.ReadResponse(originBuffered, nil)
		if readErr != nil {
			if isUnexpected(readErr) {
				log.Debug(errorf(op, "Unable to read from origin: %v", readErr))
			}
			break
		}
		writeErr := resp.Write(clientConn)
		if writeErr != nil {
			if isUnexpected(writeErr) {
				log.Debug(errorf(op, "Unable to write response to client: %v", writeErr))
			}
			break
		}
	}
	closeConns()

	return
}

func (f *forwarder) cloneRequest(req *http.Request) *http.Request {
	outReq := new(http.Request)
	// Beware, this will make a shallow copy. We have to copy all maps
	*outReq = *req

	outReq.Proto = "HTTP/1.1"
	outReq.ProtoMajor = 1
	outReq.ProtoMinor = 1
	// Overwrite close flag: keep persistent connection for the backend servers
	outReq.Close = false

	// Request Header
	outReq.Header = make(http.Header)
	copyHeadersForForwarding(outReq.Header, req.Header)
	// Ensure we have a HOST header (important for Go 1.6+ because http.Server
	// strips the HOST header from the inbound request)
	outReq.Header.Set("Host", req.Host)

	// Request URL
	outReq.URL = cloneURL(req.URL)
	// We know that is going to be HTTP always because HTTPS isn't forwarded.
	// We need to hardcode it here because req.URL.Scheme can be undefined, since
	// client request don't need to use absolute URIs
	outReq.URL.Scheme = "http"
	// We need to make sure the host is defined in the URL (not the actual URI)
	outReq.URL.Host = req.Host
	outReq.URL.RawQuery = req.URL.RawQuery
	outReq.Body = req.Body

	userAgent := req.UserAgent()
	if userAgent == "" {
		outReq.Header.Del("User-Agent")
	} else {
		outReq.Header.Set("User-Agent", userAgent)
	}

	return outReq
}

func isUnexpected(err error) bool {
	text := err.Error()
	return !strings.HasSuffix(text, "EOF") && !strings.Contains(text, "use of closed network connection") && !strings.Contains(text, "Use of idled network connection")
}

func errorf(op ops.Op, msg string, args ...interface{}) error {
	return op.FailIf(fmt.Errorf(msg, args...))
}
