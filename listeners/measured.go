package listeners

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/measured"
)

// Wrap listener
type stateAwareMeasuredListener struct {
	*measured.MeasuredListener
}

func (l stateAwareMeasuredListener) Accept() (c net.Conn, err error) {
	c, err = l.MeasuredListener.Accept()
	if err != nil {
		return nil, err
	}
	return stateAwareMeasuredConn{Conn: c.(*measured.Conn)}, err
}

func NewMeasuredListener(l net.Listener, reportInterval time.Duration) net.Listener {
	return stateAwareMeasuredListener{
		MeasuredListener: measured.Listener(l, reportInterval),
	}
}

// Wrap connection
type stateAwareMeasuredConn struct {
	*measured.Conn
}

func (c stateAwareMeasuredConn) OnState(s http.ConnState) {}
