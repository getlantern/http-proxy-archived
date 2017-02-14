// Package buffers provides shared byte buffers based on lampshade
package buffers

import (
	"github.com/getlantern/lampshade"
)

const (
	maxBufferBytes = 100 * 1024 * 1024
)

var (
	pool = lampshade.NewBufferPool(maxBufferBytes)
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
