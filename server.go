package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/mailgun/oxy/forward"
)

// Server provides the upstream side of a chained proxy setup. It can be run as
// a standalone HTTP server using Serve() or plugged into an existing HTTP
// server as an http.Handler.
type Server struct {
	// Dial: function for dialing destination
	Dial    func(network, address string) (net.Conn, error)
	fwd     *forward.Forwarder
	Checker Checker
}

// Checker provides a way to check the http request
type Checker func(req *http.Request) error

// Serve provides a convenience function for starting an HTTP server using this
// Server as the Handler.
func (s *Server) Serve(l net.Listener) error {
	s.fwd, _ = forward.New()
	server := http.Server{
		Handler: s,
	}
	return server.Serve(l)
}

// ServeHTTP implements the method from http.Handler.
func (s *Server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.Debugf("Got request to %s", req.Host)
	if s.Checker != nil {
		if err := s.Checker(req); err != nil {
			log.Debugf("Checker failed %s: %s", req.Host, err)
			s.mimicApache(resp, req)
			return
		}
	}
	if req.Method == "CONNECT" {
		s.handleCONNECT(resp, req)
		return
	}
	s.fwd.ServeHTTP(resp, req)
}

func (s *Server) handleCONNECT(resp http.ResponseWriter, req *http.Request) {
	address := req.Host
	log.Debugf("dialing target server %s", address)
	connOut, err := s.Dial("tcp", address)
	if err != nil {
		resp.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(resp, "Unable to dial %s : %s", address, err)
		return
	}

	defer closeConnection(connOut)
	if _, err := resp.Write([]byte("CONNECT OK")); err != nil {
		log.Debugf("Fail to respond CONNECT OK for %s : %s", address, err)
		return
	}

	hj := resp.(http.Hijacker)
	connIn, _, err := hj.Hijack()
	if err != nil {
		log.Errorf("Unable to hijack connection: %s", err)
		return
	}
	defer closeConnection(connIn)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		if _, err := io.Copy(connOut, connIn); err != nil {
			log.Errorf("Unable to pipe in->out: %v", err)
		}
		wg.Done()
	}()
	go func() {
		if _, err := io.Copy(connIn, connOut); err != nil {
			log.Errorf("Unable to pipe out->in: %v", err)
		}
		wg.Done()
	}()
	wg.Wait()
}

func (s *Server) mimicApache(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusNotFound)
}

func closeConnection(conn net.Conn) {
	if err := conn.Close(); err != nil {
		log.Errorf("Unable to close connection: %v", err)
	}
}
