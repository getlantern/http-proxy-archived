package forward

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/getlantern/http-proxy/filters"
	"github.com/stretchr/testify/assert"
)

var (
	text = "Hello There"
)

func TestProxy(t *testing.T) {
	goodOrigin := buildOrigin(false)
	defer goodOrigin.Close()
	prematureCloser := buildOrigin(true)
	defer prematureCloser.Close()

	server := httptest.NewServer(filters.Join(
		New(&Options{IdleTimeout: 500 * time.Second}),
		filters.Adapt(http.NotFoundHandler())))
	defer server.Close()

	doTestProxy(t, goodOrigin, server, false)
	doTestProxy(t, goodOrigin, server, true)
	doTestProxy(t, prematureCloser, server, false)
	doTestProxy(t, prematureCloser, server, true)
}

func buildOrigin(closePrematurely bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		resp.Header().Set("Content-Length", fmt.Sprint(len(text)))
		resp.WriteHeader(200)
		if closePrematurely {
			conn, buffered, err := resp.(http.Hijacker).Hijack()
			if err == nil {
				buffered.Flush()
				conn.Write([]byte(text))
				conn.Close()
			}
		} else {
			resp.Write([]byte(text))
		}
	}))
}

func doTestProxy(t *testing.T, origin *httptest.Server, server *httptest.Server, disableKeepAlives bool) {
	u, _ := url.Parse(server.URL)
	client := &http.Client{Transport: &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return u, nil
		},
		DisableKeepAlives: disableKeepAlives,
	}}

	// Do a simple GET
	if !testGet(t, client, origin) {
		return
	}

	// Do another GET to test keepalive functionality
	if !testGet(t, client, origin) {
		return
	}

	// Forcibly close client connections and make sure we can still proxy
	origin.CloseClientConnections()
	testGet(t, client, origin)
}

func testGet(t *testing.T, client *http.Client, origin *httptest.Server) bool {
	resp, err := client.Get(origin.URL)
	if !assert.NoError(t, err) {
		return false
	}
	if !assert.Equal(t, http.StatusOK, resp.StatusCode) {
		return false
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if !assert.NoError(t, err) {
		return false
	}
	return assert.Equal(t, text, string(b))
}
