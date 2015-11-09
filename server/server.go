package server

import (
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/context"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("server")
)

type Server struct {
	httpServer http.Server
	tls        bool

	listener net.Listener
	handler  http.Handler
}

func NewServer(handler http.Handler) *Server {
	server := &Server{}
	return server
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

	s.listener = listener
	/*
		limListener := newLimitedListener(listener, &s.numConns, time.Duration(30)*time.Second)
		preListener := preprocessor.NewListener(limListener)

		if s.enableReports {
			mListener := measured.Listener(preListener, 30*time.Second)
			s.listener = mListener
		} else {
			s.listener = preListener
		}
	*/
	s.httpServer = http.Server{
		Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				/*
					if atomic.LoadUint64(&s.numConns) >= s.maxConns {
						limListener.Stop()
					} else if limListener.IsStopped() {
						limListener.Restart()
					}
				*/
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

	addr := s.listener.Addr().String()
	/*
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			panic("should not happen")
		}
	*/

	if chListenOn != nil {
		*chListenOn <- addr
	}

	return s.httpServer.Serve(s.listener)
}
