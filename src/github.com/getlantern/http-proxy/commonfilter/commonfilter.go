package commonfilter

import (
	"net/http"

	"github.com/getlantern/http-proxy/utils"
)

type CommonFilter struct {
	log        utils.Logger
	errHandler utils.ErrorHandler
	next       http.Handler
}

type optSetter func(f *CommonFilter) error

func New(next http.Handler, setters ...optSetter) (*CommonFilter, error) {
	f := &CommonFilter{
		next:       next,
		errHandler: utils.DefaultHandler,
	}

	return f, nil
}

func (f *CommonFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f.ServeHTTP(w, req)
}
