package httpconnect

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"

	"../utils"
)

type HTTPConnectHandler struct {
	log        utils.Logger
	errHandler utils.ErrorHandler
	next       http.Handler
}

type optSetter func(f *HTTPConnectHandler) error

func Logger(l utils.Logger) optSetter {
	return func(f *HTTPConnectHandler) error {
		f.log = l
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*HTTPConnectHandler, error) {
	f := &HTTPConnectHandler{
		log:        utils.NullLogger,
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
	if f.log.IsLevel(utils.DEBUG) {
		reqStr, _ := httputil.DumpRequest(req, true)
		f.log.Debugf("HTTPConnectHandler Middleware received request:\n%s", reqStr)
	}

	// If the request is not HTTP CONNECT, pass along to the next handler
	if req.Method != "CONNECT" {
		f.next.ServeHTTP(w, req)
		return
	}

	f.log.Debugf("Proxying CONNECT request\n")
	f.intercept(w, req)
}

func (f *HTTPConnectHandler) intercept(w http.ResponseWriter, req *http.Request) (err error) {
	var clientConn net.Conn
	var connOut net.Conn

	utils.RespondOK(w, req)
	if clientConn, _, err = w.(http.Hijacker).Hijack(); err != nil {
		utils.RespondBadGateway(w, req, fmt.Sprintf("Unable to hijack connection: %s", err))
		return
	}
	connOut, err = net.Dial("tcp", req.Host)
	if err != nil {
		return
	}

	// Pipe data through CONNECT tunnel
	closeConns := func() {
		if clientConn != nil {
			if err := clientConn.Close(); err != nil {
				f.log.Errorf("Error closing the out connection: %s", err)
			}
		}
		if connOut != nil {
			if err := connOut.Close(); err != nil {
				f.log.Errorf("Error closing the client connection: %s", err)
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
