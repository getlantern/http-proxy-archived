package server

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"reflect"
	"strings"
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

// Opts are used to configure a Server
type Opts struct {
	IdleTimeout time.Duration
	Filter      filters.Filter
	Dial        proxy.DialFunc
}

// Server is an HTTP proxy server.
type Server struct {
	// Allow is a function that determines whether or not to allow connections
	// from the given IP address. If unspecified, all connections are allowed.
	Allow              func(string) bool
	proxy              proxy.Proxy
	listenerGenerators []listenerGenerator
}

// New constructs a new HTTP proxy server using the given options
func New(opts *Opts) *Server {
	return &Server{
		proxy: proxy.New(&proxy.Opts{
			IdleTimeout:        opts.IdleTimeout,
			Dial:               opts.Dial,
			Filter:             opts.Filter,
			BufferSource:       buffers.Pool(),
			OKWaitsForUpstream: true,
			OnError: func(ctx filters.Context, req *http.Request, read bool, err error) *http.Response {
				status := http.StatusBadGateway
				if read {
					status = http.StatusBadRequest
				}
				return &http.Response{
					Request:    req,
					StatusCode: status,
					Body:       ioutil.NopCloser(strings.NewReader(err.Error())),
				}
			},
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

	var tempDelay time.Duration // how long to sleep on accept failure
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				// delay code based on net/http.Server
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Errorf("http: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return errors.New("Error accepting: %v", err)
		}
		tempDelay = 0
		s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	wrapConn, isWrapConn := conn.(listeners.WrapConn)
	if isWrapConn {
		wrapConn.OnState(http.StateNew)
	}
	go func() {
		err := s.proxy.Handle(context.Background(), conn, conn)
		if err != nil {
			log.Errorf("Error handling connection: %v", err)
		}
		if isWrapConn {
			wrapConn.OnState(http.StateClosed)
		}

	}()
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
