package main

import (
	"io"
	"net"
	"net/http"
	"os"

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
}

func NewServer(token string) *Server {
	stdWriter := io.Writer(os.Stdout)

	// The following middleware is run from last to first
	// Don't forget to check Oxy and Gorilla's handlers for middleware.

	// Handles HTTP CONNECT
	connectHandler, _ := httpconnect.New(
		nil,
		httpconnect.Logger(utils.NewTimeLogger(&stdWriter, utils.DEBUG)),
	)
	// Handles Lantern Pro users
	lanternPro, _ := profilter.New(
		connectHandler,
		profilter.Logger(utils.NewTimeLogger(&stdWriter, utils.DEBUG)),
	)
	// Bounces back requests without the proper token
	tokenFilter, _ := tokenfilter.New(
		lanternPro,
		tokenfilter.TokenSetter(token),
		tokenfilter.Logger(utils.NewTimeLogger(&stdWriter, utils.DEBUG)),
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
	return http.Serve(s.listener, proxy)
}
