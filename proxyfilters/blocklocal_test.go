package proxyfilters

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockLocalBlocked(t *testing.T) {
	doTestBlockLocal(t, []string{"localhost"}, "http://127.0.0.1/index.html", http.StatusForbidden)
}

func TestBlockLocalException(t *testing.T) {
	doTestBlockLocal(t, []string{"localhost"}, "http://localhost/index.html", http.StatusOK)
}

func TestBlockLocalNotLocal(t *testing.T) {
	doTestBlockLocal(t, []string{"localhost"}, "http://example.com/index.html", http.StatusOK)
}

func doTestBlockLocal(t *testing.T, exceptions []string, urlStr string, expectedStatus int) {
	ctx := context.Background()
	next := func(ctx context.Context, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, nil
	}

	filter := BlockLocal(exceptions)
	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)
	log.Debug(req.Host)
	resp, _ := filter.Apply(ctx, req, next)
	assert.Equal(t, expectedStatus, resp.StatusCode)
}
