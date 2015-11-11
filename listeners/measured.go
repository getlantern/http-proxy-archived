package listeners

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/measured"
)

// Wrapped stateAwareMeasuredListener that genrates the wrapped stateAwareMeasuredConn
type stateAwareMeasuredListener struct {
	measured.MeasuredListener
}

func NewMeasuredListener(l net.Listener, reportInterval time.Duration) net.Listener {
	return &stateAwareMeasuredListener{
		MeasuredListener: *measured.Listener(l, reportInterval),
	}
}

func (l *stateAwareMeasuredListener) Accept() (c net.Conn, err error) {
	c, err = l.MeasuredListener.Accept()
	if err != nil {
		return nil, err
	}
	sac, _ := c.(*measured.Conn).Conn.(StateAwareConn)
	return &stateAwareMeasuredConn{
		StateAwareConn: sac,
		Conn:           *c.(*measured.Conn),
	}, err
}

// Wrapped MeasuredConn that supports OnState
type stateAwareMeasuredConn struct {
	StateAwareConn
	measured.Conn
}

func (c *stateAwareMeasuredConn) OnState(s http.ConnState) {
	if c.StateAwareConn != nil {
		c.StateAwareConn.OnState(s)
	}
}
