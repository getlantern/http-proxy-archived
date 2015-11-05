package main

import (
	"math"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/context"

	"github.com/getlantern/measured"

	// "github.com/getlantern/http-proxy-extensions/devicefilter"
	"github.com/getlantern/http-proxy-extensions/mimic"
	// "github.com/getlantern/http-proxy-extensions/profilter"
	"github.com/getlantern/http-proxy-extensions/tokenfilter"
	"github.com/getlantern/http-proxy/commonfilter"
	"github.com/getlantern/http-proxy/forward"
	"github.com/getlantern/http-proxy/httpconnect"
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
		maxConns = math.MaxInt64
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

func (s *Server) doServe(listener net.Listener, chListenOn *chan string) error {
	// A dirty trick to associate a connection with the http.Request it
	// contains. In "net/http/server.go", handler will be called
	// immediately after ConnState changed to StateActive, so it's safe to
	// loop through all elements in a channel to find a match remote addr.
	q := make(chan net.Conn, 10)

	proxy := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			for c := range q {
				if c.RemoteAddr().String() == req.RemoteAddr {
					context.Set(req, "conn", c)
					break
				} else {
					q <- c
				}
			}
			s.firstHandler.ServeHTTP(w, req)
		})

	limListener := newLimitedListener(listener, &s.numConns, s.idleTimeout)

	if s.enableReports {
		mListener := measured.Listener(limListener, 30*time.Second)
		s.listener = mListener
	} else {
		s.listener = limListener
	}

	s.httpServer = http.Server{Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				if atomic.LoadUint64(&s.numConns) >= s.maxConns {
					limListener.Stop()
				} else if limListener.IsStopped() {
					limListener.Restart()
				}
			case http.StateActive:
				select {
				case q <- c:
				default:
					log.Error("Oops! the connection queue is full!")
				}
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
