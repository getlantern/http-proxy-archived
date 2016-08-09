package ratelimiter

import (
	"net/http"
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

	test := func(succeed bool, desc string) {
		for _, client := range []string{"a", "b"} {
			google.RemoteAddr = client
			facebook.RemoteAddr = client
			twitter.RemoteAddr = client
			if succeed {
				err := rateLimiter.Apply(nil, google, next)
				assert.NoError(t, err, "Request from client %v to google should have succeeded: %v", client, desc)
				err = rateLimiter.Apply(nil, facebook, next)
				assert.NoError(t, err, "Request from client %v to facebook should have succeeded: %v", client, desc)
				err = rateLimiter.Apply(nil, twitter, next)
				assert.Error(t, err, "Request from client %v to twitter should have failed: %v", client, desc)
			} else {
				err := rateLimiter.Apply(nil, google, next)
				assert.Error(t, err, "Request from client %v to google should have failed: %v", client, desc)
				err = rateLimiter.Apply(nil, facebook, next)
				assert.Error(t, err, "Request from client %v to facebook should have failed: %v", client, desc)
				err = rateLimiter.Apply(nil, twitter, next)
				assert.Error(t, err, "Request from client %v to twitter should have failed: %v", desc)
			}
		}
	}

	test(true, "1st Request")
	test(false, "2nd Request")

	// Age the others out of the LRU
	google.RemoteAddr = "c"
	rateLimiter.Apply(nil, google, next)
	google.RemoteAddr = "d"
	rateLimiter.Apply(nil, google, next)

	test(true, "3st Request")
	test(false, "4nd Request")

	// Wait
	time.Sleep(100 * time.Millisecond)
	test(true, "5st Request")
}
