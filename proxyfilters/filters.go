package proxyfilters

import (
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/v2/filters"
)

var log = golog.LoggerFor("http-proxy.filters")

func fail(cs *filters.ConnectionState, req *http.Request, statusCode int, description string, params ...interface{}) (*http.Response, *filters.ConnectionState, error) {
	log.Errorf("Filter fail: "+description, params...)
	return filters.Fail(cs, req, statusCode, errors.New(description, params...))
}
