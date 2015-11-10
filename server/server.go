package server

import (
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/context"

	"github.com/getlantern/golog"

	"github.com/getlantern/http-proxy/listeners"
)

var (
	testingLocal = false
	log          = golog.LoggerFor("server")
)

type Server struct {
	handler       http.Handler
	httpServer    http.Server
	tls           bool
	moreListeners func(net.Listener) net.Listener
}

func NewServer(handler http.Handler) *Server {
	maxConns := uint64(0)
	if maxConns == 0 {
		maxConns = math.MaxUint64
	}

	server := &Server{
		handler: handler,
	}

	return server
}

func (s *Server) DecorateListener(f func(l net.Listener) net.Listener) {
	s.moreListeners = f
}

func (s *Server) ServeHTTP(addr string, chListenOn *chan string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.tls = false
	log.Debugf("Listen http on %s", addr)
	return s.doServe(listener, chListenOn)
}

func (s *Server) ServeHTTPS(addr, keyfile, certfile string, chListenOn *chan string) error {
	listener, err := listenTLS(addr, keyfile, certfile)
	if err != nil {
		return err
	}
	s.tls = true
	log.Debugf("Listen https on %s", addr)
	return s.doServe(listener, chListenOn)
}

// connBag is a just bag of connections. You can put a connection in and
// withdraw it afterwards, or purge it regardless it's withdrawed or not.
type connBag struct {
	mu sync.Mutex
	m  map[string]net.Conn
}

func (cb *connBag) Put(c net.Conn) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.m[c.RemoteAddr().String()] = c
}

func (cb *connBag) Withdraw(remoteAddr string) (c net.Conn) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	c = cb.m[remoteAddr]
	delete(cb.m, remoteAddr)
	return
}

func (cb *connBag) Purge(remoteAddr string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	// non-op if item doesn't exist
	delete(cb.m, remoteAddr)
}

func (s *Server) doServe(listener net.Listener, chListenOn *chan string) error {
	cb := connBag{m: make(map[string]net.Conn)}

	proxy := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			c := cb.Withdraw(req.RemoteAddr)
			context.Set(req, "conn", c)
			s.handler.ServeHTTP(w, req)
		})

	// preListener := preprocessor.NewListener(limListener)

	s.httpServer = http.Server{Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			c.(listeners.StateAware).OnState(state)
			switch state {
			case http.StateActive:
				cb.Put(c)
			case http.StateClosed:
				// When go server encounters abnormal request, it
				// will transit to StateClosed directly without
				// the handler being invoked, hence the connection
				// will not be withdrawed. Purge it in such case.
				cb.Purge(c.RemoteAddr().String())
			}
		},
	}

	var firstListener net.Listener
	limListener := listeners.NewLimitedListener(listener, 0, 30*time.Second)
	if true {
		firstListener = listeners.StateAwaredMeasuredListener(limListener, 30*time.Second)
	} else {
		firstListener = limListener
	}
	if s.moreListeners != nil {
		firstListener = s.moreListeners(firstListener)
	}

	addr := firstListener.Addr().String()
	s.httpServer.Addr = addr
	if chListenOn != nil {
		*chListenOn <- addr
	}

	return s.httpServer.Serve(firstListener)
}
