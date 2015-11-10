package listeners

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/idletiming"
)

// Wrapped idleConnListener that generates the wrapped idleConn
type idleConnListener struct {
	net.Listener
	idleTimeout time.Duration
}

func NewIdleConnListener(l net.Listener, timeout time.Duration) net.Listener {
	return &idleConnListener{
		Listener:    l,
		idleTimeout: timeout,
	}
}

// Wrapped IdleTimingConn that supports OnState
type idleConn struct {
	StateAwareConn
	idletiming.IdleTimingConn
}

func (l *idleConnListener) Accept() (c net.Conn, err error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	iConn := idletiming.Conn(
		conn,
		l.idleTimeout,
		func() {
			conn.Close()
		},
	)

	sac, _ := conn.(StateAwareConn)
	return &idleConn{
		StateAwareConn: sac,
		IdleTimingConn: *iConn,
	}, err
}

func (c *idleConn) OnState(s http.ConnState) {
	if c.StateAwareConn != nil {
		c.StateAwareConn.OnState(s)
	}
}
