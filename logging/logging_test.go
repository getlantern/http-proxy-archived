package logging

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type BadWriter struct{}
type GoodWriter struct{ counter int }

func (w *BadWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("Fail intentionally")
}

func (w *GoodWriter) Write(p []byte) (int, error) {
	w.counter = len(p)
	return w.counter, nil
}

func TestNonStopWriter(t *testing.T) {
	b, g := BadWriter{}, GoodWriter{}
	ns := NonStopWriter(&b, &g)
	ns.Write([]byte("1234"))
	assert.Equal(t, 4, g.counter, "Should write to all writers even when error encountered")
}
