package listeners

import (
	"errors"
	"math"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("listeners")
)

type limitedListener struct {
	net.Listener

	maxConns    uint64
	numConns    uint64
	idleTimeout time.Duration

	stopped int32
	stop    chan bool
	restart chan bool
}

func NewLimitedListener(l net.Listener, maxConns uint64) net.Listener {
	if maxConns <= 0 {
		maxConns = math.MaxUint64
	}

	listener := &limitedListener{
		Listener:    l,
		stop:        make(chan bool, 1),
		restart:     make(chan bool),
		maxConns:    maxConns,
		idleTimeout: 30 * time.Second,
	}

	return listener
}

func (sl *limitedListener) Accept() (net.Conn, error) {
	select {
	case <-sl.stop:
		<-sl.restart
	default:
	}

	conn, err := sl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	atomic.AddUint64(&sl.numConns, 1)
	log.Tracef("Accepted a new connection, %v in total now, %v max allowed", sl.numConns, sl.maxConns)

	return &LimitedConn{Conn: conn, listener: sl}, err
}

func (sl *limitedListener) IsStopped() bool {
	return atomic.LoadInt32(&sl.stopped) == 1
}

func (sl *limitedListener) Stop() {
	if !sl.IsStopped() {
		sl.stop <- true
		atomic.StoreInt32(&sl.stopped, 1)
	}
}

func (sl *limitedListener) Restart() {
	if sl.IsStopped() {
		sl.restart <- true
		atomic.StoreInt32(&sl.stopped, 0)
	}
}

type LimitedConn struct {
	net.Conn
	listener *limitedListener
	closed   uint32
}

func (c *LimitedConn) OnState(s http.ConnState) {
	l := c.listener
	log.Tracef("OnState(%s), numConns = %v, maxConns = %v", s, l.numConns, l.maxConns)

	if s != http.StateNew {
		return
	}

	if atomic.LoadUint64(&l.numConns) >= l.maxConns {
		log.Tracef("numConns %v >= maxConns %v, stop accepting new connections", l.numConns, l.maxConns)
		l.Stop()
	} else if l.IsStopped() {
		log.Tracef("numConns %v < maxConns %v, accept new connections again", l.numConns, l.maxConns)
		l.Restart()
	}
}

func (c *LimitedConn) Close() (err error) {
	if atomic.SwapUint32(&c.closed, 1) == 1 {
		return errors.New("network connection already closed")
	}

	// Substract 1 by adding the two-complement of -1
	atomic.AddUint64(&c.listener.numConns, ^uint64(0))
	log.Tracef("Closed a connection and left %v remaining", c.listener.numConns)
	return c.Conn.Close()
}
