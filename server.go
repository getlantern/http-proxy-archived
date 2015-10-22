package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/getlantern/measured"
	"github.com/gorilla/context"

	"./devicefilter"
	"./forward"
	"./httpconnect"
	"./profilter"
	"./tokenfilter"
	"./utils"
)

type Server struct {
	connectComponent      *httpconnect.HTTPConnectHandler
	lanternProComponent   *profilter.LanternProFilter
	tokenFilterComponent  *tokenfilter.TokenFilter
	deviceFilterComponent *devicefilter.DeviceFilter
	firstComponent        http.Handler

	listener net.Listener
	tls      bool

	numConnections int64
}

func NewServer(token string, logLevel utils.LogLevel) *Server {
	stdWriter := io.Writer(os.Stdout)

	// The following middleware architecture can be seen as a chain of
	// filters that is run from last to first.
	// Don't forget to check Oxy and Gorilla's handlers for middleware.

	// Handles Direct Proxying
	forwardHandler, _ := forward.New(
		nil,
		forward.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
	)

	// Handles HTTP CONNECT
	connectHandler, _ := httpconnect.New(
		forwardHandler,
		httpconnect.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
	)
	// Identifies Lantern Pro users (currently NOOP)
	lanternPro, _ := profilter.New(
		connectHandler,
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

	server := &Server{
		connectComponent:      connectHandler,
		lanternProComponent:   lanternPro,
		tokenFilterComponent:  tokenFilter,
		deviceFilterComponent: deviceFilter,
		firstComponent:        deviceFilter,
	}
	return server
}

func (s *Server) ServeHTTP(addr string, ready *chan bool) error {
	var err error
	if s.listener, err = net.Listen("tcp", addr); err != nil {
		return err
	}
	s.tls = false
	fmt.Printf("Listen http on %s\n", addr)
	return s.doServe(ready)
}

func (s *Server) ServeHTTPS(addr, keyfile, certfile string, ready *chan bool) error {
	var err error
	if s.listener, err = listenTLS(addr, keyfile, certfile); err != nil {
		return err
	}
	s.tls = true
	fmt.Printf("Listen http on %s\n", addr)
	return s.doServe(ready)
}

func (s *Server) doServe(ready *chan bool) error {
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
			s.firstComponent.ServeHTTP(w, req)
		})

	if ready != nil {
		*ready <- true
	}
	hs := http.Server{Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				atomic.AddInt64(&s.numConnections, 1)
			case http.StateActive:
				select {
				case q <- c:
				default:
					fmt.Print("Oops! the connection queue is full!\n")
				}
			case http.StateClosed:
				atomic.AddInt64(&s.numConnections, -1)
			}
		},
	}
	return hs.Serve(measured.Listener(s.listener, 10*time.Second))
}
