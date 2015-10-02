package main

import (
	"io"
	"net"
	"net/http"
	"os"

	"./connectforward"
	"./lanternpro"
	"./tokenfilter"
	"./utils"
)

type Server struct {
	forwarderComponent   *connectforward.HTTPConnectForwarder
	lanternProComponent  *lanternpro.LanternProFilter
	tokenFilterComponent *tokenfilter.TokenFilter
	firstComponent       http.Handler
	listener             net.Listener
}

func NewServer(token string) *Server {
	stdWriter := io.Writer(os.Stdout)
	// The following middleware is run from last to first:
	// Handles CONNECT and direct proxying requests
	connectFwd, _ := connectforward.New(
		connectforward.Logger(utils.NewTimeLogger(&stdWriter, utils.DEBUG)),
	)
	// Handles Lantern Pro users
	lanternPro, _ := lanternpro.New(
		connectFwd,
		lanternpro.Logger(utils.NewTimeLogger(&stdWriter, utils.DEBUG)),
	)
	// Bounces back requests without the proper token
	tokenFilter, _ := tokenfilter.New(
		lanternPro,
		tokenfilter.TokenSetter(token),
		tokenfilter.Logger(utils.NewTimeLogger(&stdWriter, utils.DEBUG)),
	)

	server := &Server{
		forwarderComponent:   connectFwd,
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
