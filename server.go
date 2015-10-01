package main

import (
	"net"
	"net/http"

	"./connectforward"
	"./lanternpro"
	"./tokenfilter"
)

type Server struct {
	forwarderComponent   *connectforward.HTTPConnectForwarder
	lanternProComponent  *lanternpro.LanternProFilter
	tokenFilterComponent *tokenfilter.TokenFilter
	handler              http.Handler
	listener             net.Listener
}

func NewServer(token string) *Server {
	// The following middleware is run from last to first:
	var handler http.Handler

	// Handles CONNECT and direct proxying requests
	connectFwd, _ := connectforward.New()
	// Handles Lantern Pro users
	lanternPro, _ := lanternpro.New(connectFwd)
	var tokenFilter *tokenfilter.TokenFilter
	if token != "" {
		// Bounces back requests without the proper token
		tokenFilter, _ = tokenfilter.New(lanternPro, token)
		handler = tokenFilter
	} else {
		handler = lanternPro
	}

	server := &Server{
		forwarderComponent:   connectFwd,
		lanternProComponent:  lanternPro,
		tokenFilterComponent: tokenFilter,
		handler:              handler,
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
			s.handler.ServeHTTP(w, req)
		})

	if ready != nil {
		*ready <- true
	}
	return http.Serve(s.listener, proxy)
}

func (s *Server) ServeHTTPS(addr string, ready *chan bool) error {
	var err error
	if s.listener, err = listenTLS(addr); err != nil {
		return err
	}

	// Set up server
	proxy := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			s.handler.ServeHTTP(w, req)
		})

	if ready != nil {
		*ready <- true
	}
	return http.Serve(s.listener, proxy)
}
