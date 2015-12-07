package forward

import (
	"errors"
	"net/http"
	"testing"

	"github.com/getlantern/testify/assert"
)

type mockRT struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m mockRT) RoundTrip(r *http.Request) (*http.Response, error) {
	return m.roundTrip(r)
}

type emptyRW struct {
}

func (m emptyRW) Header() http.Header {
	return http.Header{}
}

func (m emptyRW) Write([]byte) (int, error) {
	return 0, nil
}

func (m emptyRW) WriteHeader(int) {
}

// Regression for https://github.com/getlantern/http-proxy/issues/70
func TestCloneRequest(t *testing.T) {
	const rawPath = "/%E4%B8%9C%E6%96%B9Project"
	const url = "http://zh.moegirl.org" + rawPath
	rt := mockRT{func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, rawPath, r.URL.Opaque, "should not alter the path")
		return nil, errors.New("intentionally fail")
	}}
	fwd, _ := New(nil, RoundTripper(rt))
	req, _ := http.NewRequest("GET", url, nil)
	fwd.ServeHTTP(emptyRW{}, req)
}
