package main

import (
	"io"
	"net"
	"net/http"
	"os"

	"./forward"
	"./httpconnect"
	"./profilter"
	"./tokenfilter"
	"./utils"
)

type Server struct {
	connectComponent     *httpconnect.HTTPConnectHandler
	lanternProComponent  *profilter.LanternProFilter
	tokenFilterComponent *tokenfilter.TokenFilter
	firstComponent       http.Handler
	listener             net.Listener
	tls                  bool
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
	// Bounces back requests without the proper token
	tokenFilter, _ := tokenfilter.New(
		lanternPro,
		tokenfilter.TokenSetter(token),
		tokenfilter.Logger(utils.NewTimeLogger(&stdWriter, logLevel)),
	)

	server := &Server{
		connectComponent:     connectHandler,
		lanternProComponent:  lanternPro,
		tokenFilterComponent: tokenFilter,
		firstComponent:       tokenFilter,
	}
	return server
}

func (s *Server) ServeHTTP(addr string, ready *chan bool) error {
	var err error
	if s.listener, err = net.Listen("tcp", addr); err != nil {
		return err
	}

	// Set up server
	proxy := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			s.firstComponent.ServeHTTP(w, req)
		})

	if ready != nil {
		*ready <- true
	}
	s.tls = false
	return http.Serve(s.listener, proxy)
}

func (s *Server) ServeHTTPS(addr, keyfile, certfile string, ready *chan bool) error {
	var err error
	if s.listener, err = listenTLS(addr, keyfile, certfile); err != nil {
		return err
	}

	// Set up server
	proxy := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			s.firstComponent.ServeHTTP(w, req)
		})

	if ready != nil {
		*ready <- true
	}
	s.tls = true
	return http.Serve(s.listener, proxy)
}
