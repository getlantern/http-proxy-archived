package proxyfilters

import (
	"net/http"
	"testing"

	"github.com/getlantern/proxy/v2/filters"
	"github.com/stretchr/testify/assert"
)

func TestBlockLocalBlocked(t *testing.T) {
	doTestBlockLocal(t, []string{"localhost"}, "http://127.0.0.1/index.html", http.StatusForbidden)
}

func TestBlockLocalException(t *testing.T) {
	doTestBlockLocal(t, []string{"localhost"}, "http://localhost/index.html", http.StatusOK)
}

func TestBlockLocalExceptionWithPort(t *testing.T) {
	doTestBlockLocal(t, []string{"127.0.0.1:7300"}, "http://127.0.0.1:7300/index.html", http.StatusOK)
}

func TestBlockLocalNotLocal(t *testing.T) {
	doTestBlockLocal(t, []string{"localhost"}, "http://example.com/index.html", http.StatusOK)
}

func doTestBlockLocal(t *testing.T, exceptions []string, urlStr string, expectedStatus int) {
	next := func(cs *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, cs, nil
	}

	filter := BlockLocal(exceptions)
	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)
	log.Debug(req.Host)
	cs := filters.NewConnectionState(req, nil, nil)
	resp, _, _ := filter.Apply(cs, req, next)
	assert.Equal(t, expectedStatus, resp.StatusCode)
}
