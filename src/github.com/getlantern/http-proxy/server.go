package main

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/gorilla/context"

	"github.com/getlantern/measured"

	"github.com/getlantern/http-proxy-extensions/devicefilter"
	"github.com/getlantern/http-proxy-extensions/mimic"
	"github.com/getlantern/http-proxy-extensions/profilter"
	"github.com/getlantern/http-proxy-extensions/tokenfilter"
	"github.com/getlantern/http-proxy/commonfilter"
	"github.com/getlantern/http-proxy/forward"
	"github.com/getlantern/http-proxy/httpconnect"
	"github.com/getlantern/http-proxy/utils"
)

type Server struct {
	firstHandler http.Handler
	httpServer   http.Server
	tls          bool

	listener net.Listener

	maxConns uint64
	numConns uint64

	idleTimeout time.Duration
}

func NewServer(token string, maxConns uint64, idleTimeout time.Duration, disableFilters bool, logLevel utils.LogLevel) *Server {
	stdWriter := io.Writer(os.Stdout)

	if maxConns == 0 {
		maxConns = math.MaxInt64
	}

	// The following middleware architecture can be seen as a chain of
	// filters that is run from last to first.
	// Don't forget to check Oxy and Gorilla's handlers for middleware.

	// Handles Direct Proxying
	forwardHandler, _ := forward.New(
		nil,
		forward.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
		forward.IdleTimeoutSetter(idleTimeout),
	)

	// Handles HTTP CONNECT
	connectHandler, _ := httpconnect.New(
		forwardHandler,
		httpconnect.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
		httpconnect.IdleTimeoutSetter(idleTimeout),
	)

	// Catches any request before reaching the CONNECT middleware or
	// the forwarder
	commonFilter, _ := commonfilter.New(connectHandler)

	var firstHandler http.Handler
	if disableFilters {
		firstHandler = commonFilter
	} else {
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
	}

	server := &Server{
		firstHandler: firstHandler,
		maxConns:     maxConns,
		idleTimeout:  idleTimeout,
	}
	return server
}

func (s *Server) ServeHTTP(addr string, chListenOn *chan string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.tls = false
	fmt.Printf("Listen http on %s\n", addr)
	return s.doServe(listener, chListenOn)
}

func (s *Server) ServeHTTPS(addr, keyfile, certfile string, chListenOn *chan string) error {
	listener, err := listenTLS(addr, keyfile, certfile)
	if err != nil {
		return err
	}
	s.tls = true
	fmt.Printf("Listen http on %s\n", addr)
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
	mListener := measured.Listener(limListener, 30*time.Second)
	s.listener = mListener

	s.httpServer = http.Server{Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			if state == http.StateActive {
				select {
				case q <- c:
				default:
					fmt.Print("Oops! the connection queue is full!\n")
				}
			}

			if atomic.LoadUint64(&s.numConns) >= s.maxConns {
				limListener.Stop()
			} else if limListener.IsStopped() {
				limListener.Restart()
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
