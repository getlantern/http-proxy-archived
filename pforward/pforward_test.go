package pforward

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/getlantern/http-proxy/filters"
	"github.com/stretchr/testify/assert"
)

const (
	proxyAuthorization = "Proxy-Authorization"

	fakeRequestHeader  = "X-Fake-Request-Header"
	fakeResponseHeader = "X-Fake-Response-Header"
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
		New(&Options{
			IdleTimeout: 500 * time.Second,
			OnRequest: func(req *http.Request) {
				req.Header.Set(fakeRequestHeader, "faker")
			},
			OnResponse: func(resp *http.Response, req *http.Request, responseNumber int) *http.Response {
				// Add fake response header
				resp.Header.Set(fakeResponseHeader, "fakeresp")
				return resp
			},
		}),
		filters.Adapt(http.NotFoundHandler())))
	defer server.Close()

	doTestProxy(t, goodOrigin, server, false)
	doTestProxy(t, goodOrigin, server, true)
	doTestProxy(t, prematureCloser, server, false)
	doTestProxy(t, prematureCloser, server, true)
}

func buildOrigin(closePrematurely bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		// Reflect the fake request header
		resp.Header().Set(fakeRequestHeader, req.Header.Get(fakeRequestHeader))
		resp.Header().Set("Content-Length", fmt.Sprint(len(text)))
		resp.Header().Set("Keep-Alive", "timeout=15, max=100")
		resp.Header().Set(proxyAuthorization, "hop-by-hop header that should be removed")
		if req.Header.Get("Proxy-Authorization") != "" {
			resp.WriteHeader(http.StatusBadRequest)
		} else {
			resp.WriteHeader(http.StatusOK)
		}
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
		Dial: func(network, addr string) (net.Conn, error) {
			conn, err := net.Dial("tcp", u.Host)
			if err == nil {
				initReq, reqErr := http.NewRequest("GET", fmt.Sprintf("http://%v", addr), nil)
				if reqErr != nil {
					return nil, fmt.Errorf("Unable to construct initial request: %v", reqErr)
				}
				initReq.Header.Set(XLanternPersistent, "true")
				writeErr := initReq.Write(conn)
				if writeErr != nil {
					return nil, fmt.Errorf("Unable to write initial request: %v", writeErr)
				}
			}
			return conn, err
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
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "faker", resp.Header.Get(fakeRequestHeader), "OnRequest should have been applied")
	assert.Equal(t, "fakeresp", resp.Header.Get(fakeResponseHeader), "OnResponse should have been applied")
	assert.Empty(t, resp.Header.Get("Proxy-Authorization"), "Hop-by-hop headers should have been removed")
	if resp != nil {
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if !assert.NoError(t, err) {
			return false
		}
		return assert.Equal(t, text, string(b))
	}
	return true
}
