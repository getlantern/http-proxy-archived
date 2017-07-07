package server

import (
	"context"
	"net"
	"reflect"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxy"
	"github.com/getlantern/proxy/filters"
	"github.com/getlantern/tlsdefaults"

	"github.com/getlantern/http-proxy/buffers"
	"github.com/getlantern/http-proxy/listeners"
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
	proxy              proxy.Proxy
	listenerGenerators []listenerGenerator
}

// NewServer constructs a new HTTP proxy server using the given handler.
func NewServer(idleTimeout time.Duration, filter filters.Filter) *Server {
	return &Server{
		proxy: proxy.New(&proxy.Opts{
			Filter:             filter,
			IdleTimeout:        idleTimeout,
			BufferSource:       buffers.Pool(),
			OKWaitsForUpstream: true,
		}),
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
			return errors.New("Error accepting: %v", err)
		}
		go s.proxy.Handle(context.Background(), conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	err := s.proxy.Handle(context.Background(), conn)
	if err != nil {
		log.Errorf("Error handling connection: %v", err)
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
