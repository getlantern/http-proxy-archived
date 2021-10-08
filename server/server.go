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
	"github.com/getlantern/ops"
	"github.com/getlantern/proxy/v2"
	"github.com/getlantern/proxy/v2/filters"
	"github.com/getlantern/tlsdefaults"

	"github.com/getlantern/http-proxy/listeners"
)

var (
	testingLocal = false
	log          = golog.LoggerFor("server")
)

// A ListenerGenerator generates a new listener from an existing one.
type ListenerGenerator func(net.Listener) net.Listener

// Opts are used to configure a Server
type Opts struct {
	IdleTimeout  time.Duration
	BufferSource proxy.BufferSource
	Filter       filters.Filter
	Dial         proxy.DialFunc

	// OKDoesNotWaitForUpstream can be set to true in order to immediately return
	// OK to CONNECT requests.
	OKDoesNotWaitForUpstream bool

	// OnError provides a callback that's invoked if the proxy encounters an
	// error while proxying for the given client connection.
	OnError func(conn net.Conn, err error)

	// OnAcceptError is called when the server fails to accept a connection.
	// If the error is fatal and should halt server operations, this callback
	// should return an error. That error will be returned by functions like
	// Serve, ListenAndServeHTTP, etc. If this callback returns nil, the
	// server will carry on.
	//
	// Temporary network errors (errors of type net.Error for which Temporary()
	// returns true) will not trigger this callback.
	OnAcceptError func(err error) (fatalErr error)
}

// Server is an HTTP proxy server.
type Server struct {
	// Allow is a function that determines whether or not to allow connections
	// from the given IP address. If unspecified, all connections are allowed.
	Allow              func(string) bool
	proxy              proxy.Proxy
	listenerGenerators []ListenerGenerator
	onError            func(conn net.Conn, err error)
	onAcceptError      func(err error) (fatalErr error)
}

// New constructs a new HTTP proxy server using the given options
func New(opts *Opts) *Server {
	p, _ := proxy.New(&proxy.Opts{
		IdleTimeout:         opts.IdleTimeout,
		Dial:                opts.Dial,
		Filter:              opts.Filter,
		BufferSource:        opts.BufferSource,
		OKWaitsForUpstream:  !opts.OKDoesNotWaitForUpstream,
		OKSendsServerTiming: true,
		OnError: func(_ *filters.ConnectionState, req *http.Request, read bool, err error) *http.Response {
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
	})

	if opts.OnError == nil {
		opts.OnError = func(conn net.Conn, err error) {}
	}
	if opts.OnAcceptError == nil {
		opts.OnAcceptError = func(err error) (fatalErr error) { return err }
	}
	return &Server{
		proxy:         p,
		onError:       opts.OnError,
		onAcceptError: opts.OnAcceptError,
	}
}

func (s *Server) AddListenerWrappers(listenerGens ...ListenerGenerator) {
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
			} else if fatalErr := s.onAcceptError(err); fatalErr != nil {
				return fatalErr
			}
			continue
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
	go s.doHandle(conn, isWrapConn, wrapConn)
}

func (s *Server) doHandle(conn net.Conn, isWrapConn bool, wrapConn listeners.WrapConn) {
	clientIP := ""
	remoteAddr := conn.RemoteAddr()
	if remoteAddr != nil {
		clientIP, _, _ = net.SplitHostPort(remoteAddr.String())
	}
	op := ops.Begin("http_proxy_handle").Set("client_ip", clientIP)
	defer op.End()

	defer func() {
		p := recover()
		if p != nil {
			err := log.Errorf("Caught panic handling connection from %v: %v", conn.RemoteAddr(), p)
			if op != nil {
				op.FailIf(err)
			}
			safeClose(conn)
		}
	}()

	err := s.proxy.Handle(context.Background(), conn, conn)
	if err != nil {
		op.FailIf(errors.New("Error handling connection from %v: %v", conn.RemoteAddr(), err))
		s.onError(conn, err)
	}
	if isWrapConn {
		wrapConn.OnState(http.StateClosed)
	}
}

func safeClose(conn net.Conn) {
	defer func() {
		p := recover()
		if p != nil {
			log.Errorf("Panic on closing connection from %v: %v", conn.RemoteAddr(), p)
		}
	}()

	conn.Close()
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
