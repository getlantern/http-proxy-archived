package proxyfilters

import (
	"net/http"
	"testing"

	"github.com/getlantern/proxy/filters"
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
	ctx := filters.BackgroundContext()
	next := func(ctx filters.Context, req *http.Request) (*http.Response, filters.Context, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, ctx, nil
	}

	filter := BlockLocal(exceptions)
	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)
	log.Debug(req.Host)
	resp, _, _ := filter.Apply(ctx, req, next)
	assert.Equal(t, expectedStatus, resp.StatusCode)
}
