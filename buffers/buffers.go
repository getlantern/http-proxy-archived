// Package buffers provides shared byte buffers based on connmux
package buffers

import (
	"github.com/getlantern/connmux"
)

const (
	maxBuffers = 5000
)

var (
	pool = connmux.NewBufferPool(maxBuffers)
)

// Pool gets the byte pool
func Pool() connmux.BufferPool {
	return pool
}

// Get gets a byte buffer from the pool
func Get() []byte {
	return pool.Get()
}

// Put returns a byte buffer to the pool
func Put(b []byte) {
	pool.Put(b)
}
