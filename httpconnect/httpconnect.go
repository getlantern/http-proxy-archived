package httpconnect

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"sync/atomic"

	"../utils"
)

const (
	uidHeader = "X-Lantern-UID"
)

type HTTPConnectHandler struct {
	log  utils.Logger
	next http.Handler
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
		log:  utils.NullLogger,
		next: next,
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f *HTTPConnectHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reqStr, _ := httputil.DumpRequest(req, true)
	f.log.Debugf("HTTPConnectHandler Middleware received request:\n%s", reqStr)

	lanternUID := req.Header.Get(uidHeader)
	req.Header.Del(uidHeader)

	// A UID must be provided always by the client.  Respond 404 otherwise.
	if lanternUID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// If the request is not HTTP CONNECT, pass along to the next handler
	if req.Method != "CONNECT" {
		f.next.ServeHTTP(w, req)
		return
	}

	f.log.Debugf("Proxying CONNECT request\n")

	key := []byte(lanternUID)
	f.intercept(key, utils.GetClient(key), w, req)
}

func (f *HTTPConnectHandler) intercept(key []byte, atomicClient atomic.Value, w http.ResponseWriter, req *http.Request) (err error) {
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
		n, _ := io.Copy(connOut, clientConn)

		client := atomicClient.Load().(*utils.Client)
		atomic.AddInt64(&client.BytesOut, n)

		closeOnce.Do(closeConns)

	}()
	n, _ := io.Copy(clientConn, connOut)

	client := atomicClient.Load().(*utils.Client)
	atomic.AddInt64(&client.BytesIn, n)

	closeOnce.Do(closeConns)

	return
}
