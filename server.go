package main

import (
	"math"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/context"

	"github.com/getlantern/measured"

	// "github.com/getlantern/http-proxy-lantern/devicefilter"
	"github.com/getlantern/http-proxy-lantern/mimic"
	"github.com/getlantern/http-proxy-lantern/preprocessor"
	// "github.com/getlantern/http-proxy-lantern/profilter"
	"github.com/getlantern/http-proxy-lantern/tokenfilter"
	"github.com/getlantern/http-proxy/commonfilter"
	"github.com/getlantern/http-proxy/forward"
	"github.com/getlantern/http-proxy/httpconnect"
)

var (
	testingLocal = false
)

type Server struct {
	firstHandler http.Handler
	httpServer   http.Server
	tls          bool

	listener net.Listener

	maxConns uint64
	numConns uint64

	idleTimeout time.Duration

	enableReports bool
}

func NewServer(token string, maxConns uint64, idleTimeout time.Duration, enableFilters, enableReports bool) *Server {
	if maxConns == 0 {
		maxConns = math.MaxUint64
	}

	// The following middleware architecture can be seen as a chain of
	// filters that is run from last to first.
	// Don't forget to check Oxy and Gorilla's handlers for middleware.

	// Handles Direct Proxying
	forwardHandler, _ := forward.New(
		nil,
		forward.IdleTimeoutSetter(idleTimeout),
	)

	// Handles HTTP CONNECT
	connectHandler, _ := httpconnect.New(
		forwardHandler,
		httpconnect.IdleTimeoutSetter(idleTimeout),
	)

	// Catches any request before reaching the CONNECT middleware or
	// the forwarder
	commonFilter, _ := commonfilter.New(
		connectHandler,
		testingLocal,
	)

	var firstHandler http.Handler
	if !enableFilters {
		firstHandler = commonFilter
	} else {
		// Temporarily remove deviceFilter and lanternPro.  These need changes in the client
		// that will come after the proxy is well tested.
		/*
			// Identifies Lantern Pro users (currently NOOP)
			lanternPro, _ := profilter.New(
				commonFilter,
				profilter.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
			)
			// Returns a 404 to requests without the proper token.  Removes the
			// header before continuing.
			tokenFilter, _ := tokenfilter.New(
				lanternPro,
				tokenfilter.TokenSetter(token),
				tokenfilter.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
			)
			// Extracts the user ID and attaches the matching client to the request
			// context.  Returns a 404 to requests without the UID.  Removes the
			// header before continuing.
			deviceFilter, _ := devicefilter.New(
				tokenFilter,
				devicefilter.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
			)
			firstHandler = deviceFilter
		*/
		tokenFilter, _ := tokenfilter.New(
			commonFilter,
			tokenfilter.TokenSetter(token),
		)
		firstHandler = tokenFilter
	}

	server := &Server{
		firstHandler:  firstHandler,
		maxConns:      maxConns,
		numConns:      0,
		idleTimeout:   idleTimeout,
		enableReports: enableReports,
	}
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
			s.firstHandler.ServeHTTP(w, req)
		})

	limListener := newLimitedListener(listener, &s.numConns, s.idleTimeout)
	preListener := preprocessor.NewListener(limListener)

	if s.enableReports {
		mListener := measured.Listener(preListener, 30*time.Second)
		s.listener = mListener
	} else {
		s.listener = preListener
	}

	s.httpServer = http.Server{Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			if sc, ok := c.(preprocessor.StatefulConn); ok {
				sc.SetState(state)
			}
			switch state {
			case http.StateNew:
				if atomic.LoadUint64(&s.numConns) >= s.maxConns {
					log.Tracef("numConns %v >= maxConns %v, stop accepting new connections", s.numConns, s.maxConns)
					limListener.Stop()
				} else if limListener.IsStopped() {
					log.Tracef("numConns %v < maxConns %v, accept new connections again", s.numConns, s.maxConns)
					limListener.Restart()
				}
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
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic("should not happen")
	}
	mimic.Host = host
	mimic.Port = port
	if chListenOn != nil {
		*chListenOn <- addr
	}

	return s.httpServer.Serve(s.listener)
}
