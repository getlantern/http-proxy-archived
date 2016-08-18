package ratelimiter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimit(t *testing.T) {
	rateLimiter := New(2, map[string]time.Duration{
		"www.google.com":   50 * time.Millisecond,
		"www.facebook.com": 50 * time.Millisecond,
	})

	next := func() error { return nil }

	google, _ := http.NewRequest("GET", "https://www.google.com", nil)
	facebook, _ := http.NewRequest("GET", "https://www.facebook.com", nil)
	twitter, _ := http.NewRequest("GET", "https://www.twitter.com", nil)

	port := 10000
	resp := &httptest.ResponseRecorder{}
	test := func(expectSuccess bool, desc string) {
		port++
		for _, client := range []string{"a", "b"} {
			google.RemoteAddr = fmt.Sprintf("%v:%d", client, port)
			facebook.RemoteAddr = fmt.Sprintf("%v:%d", client, port)
			twitter.RemoteAddr = fmt.Sprintf("%v:%d", client, port)
			if expectSuccess {
				resp = &httptest.ResponseRecorder{}
				rateLimiter.Apply(resp, google, next)
				assert.NotEqual(t, http.StatusForbidden, resp.Code, "Request from client %v to google should have succeeded: %v", client, desc)
				resp = &httptest.ResponseRecorder{}
				rateLimiter.Apply(resp, facebook, next)
				assert.NotEqual(t, http.StatusForbidden, resp.Code, "Request from client %v to facebook should have succeeded: %v", client, desc)
				resp = &httptest.ResponseRecorder{}
				rateLimiter.Apply(resp, twitter, next)
				assert.Equal(t, http.StatusForbidden, resp.Code, "Request from client %v to twitter should have failed: %v", client, desc)
			} else {
				resp = &httptest.ResponseRecorder{}
				rateLimiter.Apply(resp, google, next)
				assert.Equal(t, http.StatusForbidden, resp.Code, "Request from client %v to google should have failed: %v", client, desc)
				resp = &httptest.ResponseRecorder{}
				rateLimiter.Apply(resp, facebook, next)
				assert.Equal(t, http.StatusForbidden, resp.Code, "Request from client %v to facebook should have failed: %v", client, desc)
				resp = &httptest.ResponseRecorder{}
				rateLimiter.Apply(resp, twitter, next)
				assert.Equal(t, http.StatusForbidden, resp.Code, "Request from client %v to twitter should have failed: %v", desc)
			}
		}
	}

	test(true, "1st Request")
	test(false, "2nd Request")

	// Age the others out of the LRU
	google.RemoteAddr = "c"
	resp = &httptest.ResponseRecorder{}
	rateLimiter.Apply(resp, google, next)
	google.RemoteAddr = "d"
	resp = &httptest.ResponseRecorder{}
	rateLimiter.Apply(resp, google, next)

	test(true, "3rd Request")
	test(false, "4th Request")

	// Wait
	time.Sleep(100 * time.Millisecond)
	test(true, "5th Request")
}
