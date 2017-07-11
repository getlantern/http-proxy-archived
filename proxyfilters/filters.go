package proxyfilters

import (
	"context"
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/filters"
)

var log = golog.LoggerFor("http-proxy.filters")

func fail(ctx context.Context, req *http.Request, statusCode int, description string, params ...interface{}) (*http.Response, context.Context, error) {
	return filters.Fail(ctx, req, statusCode, errors.New(description, params...))
}
