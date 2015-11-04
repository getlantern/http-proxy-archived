package httpconnect

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy/utils"
	"github.com/getlantern/idletiming"
)

var log = golog.LoggerFor("tokenfilter")

type HTTPConnectHandler struct {
	errHandler utils.ErrorHandler
	next       http.Handler

	idleTimeout time.Duration
}

type optSetter func(f *HTTPConnectHandler) error

func IdleTimeoutSetter(i time.Duration) optSetter {
	return func(f *HTTPConnectHandler) error {
		f.idleTimeout = i
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*HTTPConnectHandler, error) {
	f := &HTTPConnectHandler{
		errHandler: utils.DefaultHandler,
		next:       next,
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f *HTTPConnectHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("HTTPConnectHandler Middleware received request:\n%s", reqStr)
	}

	// If the request is not HTTP CONNECT, pass along to the next handler
	if req.Method != "CONNECT" {
		f.next.ServeHTTP(w, req)
		return
	}

	log.Tracef("Proxying CONNECT request")

	f.intercept(w, req)
}

func (f *HTTPConnectHandler) intercept(w http.ResponseWriter, req *http.Request) (err error) {
	utils.RespondOK(w, req)

	clientConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		utils.RespondBadGateway(w, req, fmt.Sprintf("Unable to hijack connection: %s", err))
		return
	}
	connOutRaw, err := net.Dial("tcp", req.Host)
	if err != nil {
		return
	}
	connOut := idletiming.Conn(connOutRaw, f.idleTimeout, func() {
		if connOutRaw != nil {
			connOutRaw.Close()
		}
	})

	// Pipe data through CONNECT tunnel
	closeConns := func() {
		if clientConn != nil {
			if err := clientConn.Close(); err != nil {
				log.Errorf("Error closing the out connection: %s", err)
			}
		}
		if connOut != nil {
			if err := connOut.Close(); err != nil {
				log.Errorf("Error closing the client connection: %s", err)
			}
		}
	}
	var closeOnce sync.Once
	go func() {
		_, _ = io.Copy(connOut, clientConn)
		closeOnce.Do(closeConns)

	}()
	_, _ = io.Copy(clientConn, connOut)
	closeOnce.Do(closeConns)

	return
}
