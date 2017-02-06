package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy/listeners"
	"github.com/getlantern/tlsdefaults"
	"github.com/gorilla/context"
)

var (
	testingLocal = false
	log          = golog.LoggerFor("server")
)

type listenerGenerator func(net.Listener) net.Listener

// Server is an HTTP proxy server.
type Server struct {
	// Allow is a function that determines whether or not to allow connections
	// from the given IP address. If unspecified, all connections are allowed.
	Allow              func(string) bool
	handler            http.Handler
	listenerGenerators []listenerGenerator
}

// NewServer constructs a new HTTP proxy server using the given handler.
func NewServer(handler http.Handler) *Server {
	return &Server{
		handler: handler,
	}
}

func (s *Server) AddListenerWrappers(listenerGens ...listenerGenerator) {
	for _, g := range listenerGens {
		s.listenerGenerators = append(s.listenerGenerators, g)
	}
}

func (s *Server) ListenAndServeHTTP(addr string, readyCb func(addr string)) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Debugf("Listen http on %s", addr)
	return s.serve(s.wrapListenerIfNecessary(listener), readyCb)
}

func (s *Server) ListenAndServeHTTPS(addr, keyfile, certfile string, readyCb func(addr string)) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	listener, err := tlsdefaults.NewListener(s.wrapListenerIfNecessary(l), keyfile, certfile)
	if err != nil {
		return err
	}
	log.Debugf("Listen https on %s", addr)
	return s.serve(listener, readyCb)
}

func (s *Server) Serve(listener net.Listener, readyCb func(addr string)) error {
	return s.serve(s.wrapListenerIfNecessary(listener), readyCb)
}

func (s *Server) serve(listener net.Listener, readyCb func(addr string)) error {
	l := listeners.NewDefaultListener(listener)

	for _, wrap := range s.listenerGenerators {
		l = wrap(l)
	}

	if readyCb != nil {
		readyCb(l.Addr().String())
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			return fmt.Errorf("Unable to accept connection: %v", err)
		}
		go s.proxy(conn)
	}
}

func (s *Server) proxy(conn net.Conn) {
	defer conn.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)) // todo: pool these to avoid allocations
	for {
		req, err := http.ReadRequest(rw.Reader)
		if err != nil {
			if err != io.EOF {
				log.Errorf("Unable to read request: %v", err)
				rw.Write([]byte("HTTP/1.1 400 Bad Request\r\n"))
				rw.Flush()
			}
			return
		}

		resp := newResponseWriter(conn, rw)
		context.Set(req, "conn", conn)
		s.handler.ServeHTTP(resp, req)
		err = resp.flush()
		context.Clear(req)
		if err != nil {
			log.Errorf("Error flushing response: %v", err)
			return
		}
		if req.Header.Get("Connection") == "Close" {
			conn.Close()
			return
		}
		if resp.hijacked {
			return
		}
	}
}

func (s *Server) wrapListenerIfNecessary(l net.Listener) net.Listener {
	if s.Allow != nil {
		log.Debug("Wrapping listener with Allow")
		return &allowinglistener{l, s.Allow}
	}
	return l
}

type allowinglistener struct {
	wrapped net.Listener
	allow   func(string) bool
}

func (l *allowinglistener) Accept() (net.Conn, error) {
	conn, err := l.wrapped.Accept()
	if err != nil {
		return conn, err
	}

	ip := ""
	remoteAddr := conn.RemoteAddr()
	switch addr := remoteAddr.(type) {
	case *net.TCPAddr:
		ip = addr.IP.String()
	case *net.UDPAddr:
		ip = addr.IP.String()
	default:
		log.Errorf("Remote addr %v is of unknown type %v, unable to determine IP", remoteAddr, reflect.TypeOf(remoteAddr))
		return conn, err
	}
	if !l.allow(ip) {
		conn.Close()
		// Note - we don't return an error, because that causes http.Server to stop
		// serving.
	}

	return conn, err
}

func (l *allowinglistener) Close() error {
	return l.wrapped.Close()
}

func (l *allowinglistener) Addr() net.Addr {
	return l.wrapped.Addr()
}
