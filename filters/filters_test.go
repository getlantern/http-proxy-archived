package filters

import (
	"bufio"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getlantern/proxy"
	"github.com/getlantern/proxy/filters"
	"github.com/stretchr/testify/assert"
)

const (
	expectedBody = "response body"
	badBody      = "bad body"
)

func TestPersistent(t *testing.T) {
	doTestFilter(t,
		filters.Join(DiscardInitialPersistentRequest, AddForwardedFor, RecordOp),
		func(send func(method string, headers http.Header, body string) error, recv func() (*http.Response, string, error)) {
			// Initial persistent request should be discarded
			err := send(http.MethodGet, http.Header{xLanternPersistent: []string{"true"}}, badBody)
			if !assert.NoError(t, err) {
				return
			}

			// This request should get reflected
			err = send(http.MethodGet, nil, expectedBody)
			if !assert.NoError(t, err) {
				return
			}
			resp, body, err := recv()
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.NotEmpty(t, resp.Header.Get("Reflected-X-Forwarded-For"))
			assert.Equal(t, expectedBody, body)
		})
}

func TestRestrictConnectPortsEmpty(t *testing.T) {
	doTestFilter(t,
		RestrictConnectPorts([]int{}),
		func(send func(method string, headers http.Header, body string) error, recv func() (*http.Response, string, error)) {
			err := send(http.MethodConnect, nil, "")
			if !assert.NoError(t, err) {
				return
			}
			resp, _, err := recv()
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
}

func TestRestrictConnectNonConnect(t *testing.T) {
	doTestFilter(t,
		RestrictConnectPorts([]int{9999999}),
		func(send func(method string, headers http.Header, body string) error, recv func() (*http.Response, string, error)) {
			err := send(http.MethodGet, nil, "")
			if !assert.NoError(t, err) {
				return
			}
			resp, _, err := recv()
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
}

func TestRestrictConnectPortDisallowed(t *testing.T) {
	doTestFilter(t,
		RestrictConnectPorts([]int{9999999}),
		func(send func(method string, headers http.Header, body string) error, recv func() (*http.Response, string, error)) {
			err := send(http.MethodConnect, nil, "")
			if !assert.NoError(t, err) {
				return
			}
			resp, _, err := recv()
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
}

func doTestFilter(t *testing.T, filter filters.Filter, run func(send func(method string, headers http.Header, body string) error, recv func() (*http.Response, string, error))) {
	s := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		for key, values := range req.Header {
			for _, value := range values {
				resp.Header().Add("Reflected-"+key, value)
			}
		}
		resp.WriteHeader(http.StatusOK)
		io.Copy(resp, req.Body)
	}))
	defer s.Close()
	originURL := "http://" + s.Listener.Addr().String()

	pl, err := net.Listen("tcp", "localhost:0")
	if !assert.NoError(t, err) {
		return
	}
	defer pl.Close()
	if !assert.NoError(t, err) {
		return
	}

	p := proxy.New(&proxy.Opts{
		Filter: filter,
	})
	go p.Serve(pl)

	conn, err := net.Dial("tcp", pl.Addr().String())
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	br := bufio.NewReader(conn)
	var req *http.Request

	send := func(method string, headers http.Header, bodyString string) error {
		body := ioutil.NopCloser(strings.NewReader(bodyString))
		req, _ = http.NewRequest(method, originURL, body)
		if err != nil {
			return err
		}
		if headers != nil {
			req.Header = headers
		}
		return req.Write(conn)
	}

	recv := func() (*http.Response, string, error) {
		resp, err := http.ReadResponse(br, req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}

		return resp, string(body), nil
	}

	run(send, recv)
}
