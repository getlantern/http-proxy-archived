package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/context"

	"github.com/getlantern/measured"

	"./filters"
	"./forward"
	"./httpconnect"
	"./utils"
)

type Server struct {
	connectComponent     *httpconnect.HTTPConnectHandler
	lanternProComponent  *filters.LanternProFilter
	tokenFilterComponent *filters.TokenFilter
	uidFilterComponent   *filters.UIDFilter
	firstComponent       http.Handler

	listener net.Listener
	tls      bool
}

func NewServer(token string, logLevel utils.LogLevel) *Server {
	stdWriter := io.Writer(os.Stdout)
	logger := utils.NewTimeLogger(&stdWriter, logLevel)

	// The following middleware architecture can be seen as a chain of
	// filters that is run from last to first.
	// Don't forget to check Oxy and Gorilla's handlers for middleware.

	// Handles Direct Proxying
	forwardHandler, _ := forward.New(
		nil,
		forward.Logger(logger),
	)

	// Handles HTTP CONNECT
	connectHandler, _ := httpconnect.New(
		forwardHandler,
		httpconnect.Logger(logger),
	)
	// Identifies Lantern Pro users (currently NOOP)
	lanternPro := filters.NewProFilter(connectHandler, logger)
	// Returns a 404 to requests without the proper token.  Removes the
	// header before continuing.
	tokenFilter := filters.NewTokenFilter(
		lanternPro,
		logger,
		token,
	)
	// Extracts the user ID and attaches the matching client to the request
	// context.  Returns a 404 to requests without the UID.  Removes the
	// header before continuing.
	uidFilter := filters.NewUIDFilter(
		tokenFilter,
		logger,
	)

	server := &Server{
		connectComponent:     connectHandler,
		lanternProComponent:  lanternPro,
		tokenFilterComponent: tokenFilter,
		uidFilterComponent:   uidFilter,
		firstComponent:       uidFilter,
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
		ConnState: func(c net.Conn, s http.ConnState) {
			if s == http.StateActive {
				select {
				case q <- c:
				default:
					fmt.Print("Oops! the connection queue is full!\n")
				}
			}
		},
	}
	return hs.Serve(measured.Listener(s.listener, 10*time.Second))
}
