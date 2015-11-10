package server

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/getlantern/idletiming"
)

type limitedListener struct {
	net.Listener

	numConns    *uint64
	idleTimeout time.Duration

	stopped int32
	stop    chan bool
	restart chan bool
}

func newLimitedListener(l net.Listener, numConns *uint64, idleTimeout time.Duration) *limitedListener {
	listener := &limitedListener{
		Listener:    l,
		stop:        make(chan bool, 1),
		restart:     make(chan bool),
		numConns:    numConns,
		idleTimeout: idleTimeout,
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

	atomic.AddUint64(sl.numConns, 1)
	log.Tracef("Accepted a new connection, now we have %v in total", *sl.numConns)
	lc := &LimitedConn{
		counter: sl.numConns,
	}
	lc.Conn = idletiming.Conn(conn, sl.idleTimeout, func() {
		lc.Close()
	})

	return lc, err
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
	counter *uint64
	closed  uint32
}

func (c *LimitedConn) Close() (err error) {
	if atomic.SwapUint32(&c.closed, 1) == 1 {
		return errors.New("network connection already closed")
	}

	// Substract 1 by adding the two-complement of -1
	atomic.AddUint64(c.counter, ^uint64(0))
	log.Tracef("Closed a connection and left %v remaining", *c.counter)
	return c.Conn.Close()
}
