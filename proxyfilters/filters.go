package proxyfilters

import (
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/filters"
)

var log = golog.LoggerFor("http-proxy.filters")

func fail(ctx filters.Context, req *http.Request, statusCode int, description string, params ...interface{}) (*http.Response, filters.Context, error) {
	log.Errorf("Filter fail: "+description, params...)
	return filters.Fail(ctx, req, statusCode, errors.New(description, params...))
}
