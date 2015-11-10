package http_proxy

import (
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/context"

	"github.com/getlantern/golog"
	"github.com/getlantern/measured"

	// "github.com/getlantern/http-proxy-lantern/devicefilter"
	// "github.com/getlantern/http-proxy-lantern/preprocessor"
	// "github.com/getlantern/http-proxy-lantern/profilter"
	//"github.com/getlantern/http-proxy-lantern/tokenfilter"
	"github.com/getlantern/http-proxy/commonfilter"
	"github.com/getlantern/http-proxy/forward"
	"github.com/getlantern/http-proxy/httpconnect"
)

type measuredStateAwareConn struct {
	StateAware
	*measured.Conn
}

func (c measuredStateAwareConn) OnState(s http.ConnState) {
	if sc, ok := c.Conn.Conn.(StateAware); ok {
		sc.OnState(s)
	}
}

type stateAwareMeasuredListener struct {
	StateAware
	*measured.MeasuredListener
}

func (l stateAwareMeasuredListener) Accept() (c net.Conn, err error) {
	c, err = l.MeasuredListener.Accept()
	if err != nil {
		return nil, err
	}
	return measuredStateAwareConn{Conn: c.(*measured.Conn)}, err
}

func StateAwaredMeasuredListener(l net.Listener, reportInterval time.Duration) net.Listener {
	return stateAwareMeasuredListener{MeasuredListener: measured.Listener(l, reportInterval)}
}

var (
	log          = golog.LoggerFor("http-proxy")
	testingLocal = false
)

// StateAware is an interface that aware of HTTP state changes
type StateAware interface {
	OnState(s http.ConnState)
}

type Server struct {
	firstHandler  http.Handler
	httpServer    http.Server
	tls           bool
	enableReports bool
	maxConns      uint64
	idleTimeout   time.Duration

	moreListeners func(net.Listener) net.Listener
}

func DefaultHandlers(idleTimeout time.Duration) http.Handler {
	// The following middleware architecture can be seen as a chain of
	// filters that is run from last to first.
	// Don't forget to check Oxy and Gorilla's handlers for middleware.

	// Handles Direct Proxying
	forwardHandler, _ := forward.New(
		nil,
		forward.IdleTimeoutSetter(idleTimeout),
	)

	// Handles HTTP CONNECT
	connectHandler, _ := httpconnect.New(
		forwardHandler,
		httpconnect.IdleTimeoutSetter(idleTimeout),
	)

	// Catches any request before reaching the CONNECT middleware or
	// the forwarder
	commonFilter, _ := commonfilter.New(
		connectHandler,
		testingLocal,
	)
	return commonFilter
}

func NewServer(firstHandler http.Handler, maxConns uint64, idleTimeout time.Duration, enableReports bool) *Server {
	if maxConns == 0 {
		maxConns = math.MaxUint64
	}
	server := &Server{
		firstHandler:  firstHandler,
		enableReports: enableReports,
		maxConns:      maxConns,
		idleTimeout:   idleTimeout,
	}
	return server
}

func (s *Server) DecorateListener(f func(l net.Listener) net.Listener) {
	s.moreListeners = f
}

func (s *Server) ServeHTTP(addr string, chListenOn *chan string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.tls = false
	log.Debugf("Listen http on %s", addr)
	return s.doServe(listener, chListenOn)
}

func (s *Server) ServeHTTPS(addr, keyfile, certfile string, chListenOn *chan string) error {
	listener, err := listenTLS(addr, keyfile, certfile)
	if err != nil {
		return err
	}
	s.tls = true
	log.Debugf("Listen https on %s", addr)
	return s.doServe(listener, chListenOn)
}

// connBag is a just bag of connections. You can put a connection in and
// withdraw it afterwards, or purge it regardless it's withdrawed or not.
type connBag struct {
	mu sync.Mutex
	m  map[string]net.Conn
}

func (cb *connBag) Put(c net.Conn) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.m[c.RemoteAddr().String()] = c
}

func (cb *connBag) Withdraw(remoteAddr string) (c net.Conn) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	c = cb.m[remoteAddr]
	delete(cb.m, remoteAddr)
	return
}

func (cb *connBag) Purge(remoteAddr string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	// non-op if item doesn't exist
	delete(cb.m, remoteAddr)
}

func (s *Server) doServe(listener net.Listener, chListenOn *chan string) error {
	cb := connBag{m: make(map[string]net.Conn)}

	proxy := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			c := cb.Withdraw(req.RemoteAddr)
			context.Set(req, "conn", c)
			s.firstHandler.ServeHTTP(w, req)
		})

	// preListener := preprocessor.NewListener(limListener)

	s.httpServer = http.Server{Handler: proxy,
		ConnState: func(c net.Conn, state http.ConnState) {
			c.(StateAware).OnState(state)
			switch state {
			case http.StateActive:
				cb.Put(c)
			case http.StateClosed:
				// When go server encounters abnormal request, it
				// will transit to StateClosed directly without
				// the handler being invoked, hence the connection
				// will not be withdrawed. Purge it in such case.
				cb.Purge(c.RemoteAddr().String())
			}
		},
	}

	var firstListener net.Listener
	limListener := NewLimitedListener(listener, s.maxConns, s.idleTimeout)
	if s.enableReports {
		firstListener = StateAwaredMeasuredListener(limListener, 30*time.Second)
	} else {
		firstListener = limListener
	}
	if s.moreListeners != nil {
		firstListener = s.moreListeners(firstListener)
	}

	addr := firstListener.Addr().String()
	s.httpServer.Addr = addr
	if chListenOn != nil {
		*chListenOn <- addr
	}

	return s.httpServer.Serve(firstListener)
}
