package listeners

import (
	"net"
	"net/http"
)

// WrapConnEmbeddable can be embedded along net.Conn or not
type WrapConnEmbeddable interface {
	OnState(s http.ConnState)
	ControlMessage(msgType string, data interface{})
}

// WrapConn is an interface that describes a connection that an be wrapped and
// wrap other connections.  It responds to connection changes with OnState, and
// allows control messages with ControlMessage (for things like modify the
// connection at the wrapper level).
// It is important that these functions, when defined, pass the arguments
// to the wrapped connections.
type WrapConn interface {
	net.Conn

	// Additional functionality
	OnState(s http.ConnState)
	ControlMessage(msgType string, data interface{})
	Wrapped() net.Conn
}
