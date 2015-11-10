package listeners

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/measured"
)

type measuredStateAwareConn struct {
	StateAware
	*measured.Conn
}

func (c measuredStateAwareConn) OnState(s http.ConnState) {}

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

func NewMeasuredListener(l net.Listener, reportInterval time.Duration) net.Listener {
	return stateAwareMeasuredListener{
		MeasuredListener: (measured.Listener(l, reportInterval)).(*measured.MeasuredListener),
	}
}
