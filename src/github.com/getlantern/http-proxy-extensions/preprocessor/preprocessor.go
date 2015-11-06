package preprocessor

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy-extensions/mimic"
)

// StatefulConn is a type of connection that changes it's internal state when its
// SetState() is called
type StatefulConn interface {
	SetState(s http.ConnState)
}

var log = golog.LoggerFor("preprocessor")

type listener struct {
	net.Listener
}

func NewListener(l net.Listener) *listener {
	listener := &listener{l}
	return listener
}

func (sl *listener) Accept() (net.Conn, error) {
	c, err := sl.Listener.Accept()
	return &conn{Conn: c, newRequest: 1}, err
}

type conn struct {
	net.Conn
	// ready to handle a new http request when == 1
	newRequest uint32
}

func (c *conn) SetState(s http.ConnState) {
	if s == http.StateIdle {
		atomic.StoreUint32(&c.newRequest, 1)
	}
}

func (c *conn) Read(p []byte) (n int, err error) {
	if atomic.SwapUint32(&c.newRequest, 0) == 0 {
		return c.Conn.Read(p)
	}
	// TODO: user sync.Pool to avoid allocating memory for each request
	var buf bytes.Buffer
	r := io.TeeReader(c.Conn, &buf)
	n, err = r.Read(p)
	if err != nil {
		return
	}
	// we assume a single read will read full http header, which is not the case
	_, e := http.ReadRequest(bufio.NewReader(&buf))
	if e != nil {
		// do nothing for network errors. ref (c *conn) serve() in net/http/server.go
		if e == io.EOF {
		} else if neterr, ok := e.(net.Error); ok && neterr.Timeout() {
		} else {
			log.Debugf("Error read request from %s: %s", c.RemoteAddr().String(), e)
			mimic.MimicApacheOnInvalidRequest(c.Conn)
			return 0, errors.New("Bad request")
		}
	}
	return
}
