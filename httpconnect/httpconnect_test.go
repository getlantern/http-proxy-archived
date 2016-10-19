package httpconnect

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/getlantern/http-proxy/filters"
	"github.com/stretchr/testify/assert"
)

func TestFilterTunnelPorts(t *testing.T) {
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hi"))
	}))
	origin.StartTLS()
	defer origin.Close()
	ou, _ := url.Parse(origin.URL)
	_, _port, _ := net.SplitHostPort(ou.Host)
	port, _ := strconv.Atoi(_port)

	server := httptest.NewServer(filters.Join(
		New(&Options{AllowedPorts: []int{port, 443}, IdleTimeout: 30 * time.Second}),
		filters.Adapt(http.NotFoundHandler())))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := http.Client{Transport: &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return u, nil
		},
		DisableKeepAlives: true,
	}}

	req, _ := http.NewRequest("CONNECT", "https://site.com:abc", nil)
	resp, _ := client.Do(req)
	assert.Nil(t, resp, "CONNECT request with non-integer port should fail with 400")

	req, _ = http.NewRequest("GET", "https://www.google.com/humans.txt", nil)
	resp, err := client.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "CONNECT request to allowed port should succeed")

	req, _ = http.NewRequest("CONNECT", fmt.Sprintf("https://site.com:%d", (port-1)), nil)
	resp, _ = client.Do(req)
	assert.Nil(t, resp, "CONNECT request to disallowed port should fail with 403")
}
