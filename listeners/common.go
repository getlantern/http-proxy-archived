package listeners

import (
	"net/http"
)

// StateAware is an interface that aware of HTTP state changes
type StateAware interface {
	OnState(s http.ConnState)
}
