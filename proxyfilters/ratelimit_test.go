package proxyfilters

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/getlantern/proxy/v2/filters"
	"github.com/stretchr/testify/assert"
)

func TestRateLimit(t *testing.T) {
	rateLimiter := RateLimit(2, map[string]time.Duration{
		"www.google.com":   50 * time.Millisecond,
		"www.facebook.com": 50 * time.Millisecond,
	})

	next := func(cs *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, cs, nil
	}

	google, _ := http.NewRequest("GET", "https://www.google.com", nil)
	facebook, _ := http.NewRequest("GET", "https://www.facebook.com", nil)
	twitter, _ := http.NewRequest("GET", "https://www.twitter.com", nil)

	googleState := filters.NewConnectionState(google, nil, nil)
	facebookState := filters.NewConnectionState(facebook, nil, nil)
	twitterState := filters.NewConnectionState(twitter, nil, nil)

	port := 10000
	test := func(expectSuccess bool, desc string) {
		port++
		for _, client := range []string{"a", "b"} {
			google.RemoteAddr = fmt.Sprintf("%v:%d", client, port)
			facebook.RemoteAddr = fmt.Sprintf("%v:%d", client, port)
			twitter.RemoteAddr = fmt.Sprintf("%v:%d", client, port)
			if expectSuccess {
				resp, _, _ := rateLimiter.Apply(googleState, google, next)
				assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Request from client %v to google should have succeeded: %v", client, desc)
				resp, _, _ = rateLimiter.Apply(facebookState, facebook, next)
				assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Request from client %v to facebook should have succeeded: %v", client, desc)
				resp, _, _ = rateLimiter.Apply(twitterState, twitter, next)
				assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Request from client %v to twitter should have failed: %v", client, desc)
			} else {
				resp, _, _ := rateLimiter.Apply(googleState, google, next)
				assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Request from client %v to google should have failed: %v", client, desc)
				resp, _, _ = rateLimiter.Apply(facebookState, facebook, next)
				assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Request from client %v to facebook should have failed: %v", client, desc)
				resp, _, _ = rateLimiter.Apply(twitterState, twitter, next)
				assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Request from client %v to twitter should have failed: %v", desc)
			}
		}
	}

	test(true, "1st Request")
	test(false, "2nd Request")

	// Age the others out of the LRU
	google.RemoteAddr = "c"
	rateLimiter.Apply(googleState, google, next)
	google.RemoteAddr = "d"
	rateLimiter.Apply(googleState, google, next)

	test(true, "3rd Request")
	test(false, "4th Request")

	// Wait
	time.Sleep(100 * time.Millisecond)
	test(true, "5th Request")
}
