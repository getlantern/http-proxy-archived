package main

import (
	"fmt"
	"net"
	"sync/atomic"
)

type limitedListener struct {
	net.Listener

	numConns *uint64

	stopped int32
	stop    chan bool
	restart chan bool
}

func newLimitedListener(l net.Listener, numConns *uint64) *limitedListener {
	listener := &limitedListener{
		Listener: l,
		stop:     make(chan bool, 1),
		restart:  make(chan bool),
		numConns: numConns,
	}

	return listener
}

func (sl *limitedListener) Accept() (net.Conn, error) {
	select {
	case <-sl.stop:
		fmt.Println("STOPPED")
		<-sl.restart
	default:
	}

	fmt.Println("ACCEPTING")
	conn, err := sl.Listener.Accept()
	atomic.AddUint64(sl.numConns, 1)
	fmt.Println("Accepted connection number: ", atomic.LoadUint64(sl.numConns))
	return &LimitedConn{
		Conn:    conn,
		counter: sl.numConns,
	}, err
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
}

/*
func (mc *LimitedConn) Read(b []byte) (n int, err error) {
	return mc.Conn.Read(b)
}

func (mc *LimitedConn) Write(b []byte) (n int, err error) {
	return mc.Conn.Write(b)
}
*/

func (c *LimitedConn) Close() (err error) {
	// Substract 1 by adding the two-complement of -1
	atomic.AddUint64(c.counter, ^uint64(0))
	return c.Conn.Close()
}
