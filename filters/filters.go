package filters

import (
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/filters"
)

var log = golog.LoggerFor("http-proxy.filters")

func fail(req *http.Request, statusCode int, description string, params ...interface{}) (*http.Response, error) {
	return filters.Fail(req, statusCode, errors.New(description, params...))
}
