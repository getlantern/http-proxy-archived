// Package buffers provides shared byte buffers based on lampshade
package buffers

import (
	"github.com/getlantern/lampshade"
)

const (
	maxBuffers = 5000
)

var (
	pool = lampshade.NewBufferPool(maxBuffers)
)

// Pool gets the byte pool
func Pool() lampshade.BufferPool {
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
