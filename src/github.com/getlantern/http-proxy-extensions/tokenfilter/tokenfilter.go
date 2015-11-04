package tokenfilter

import (
	"net/http"
	"net/http/httputil"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy-extensions/mimic"
)

const (
	tokenHeader = "X-Lantern-Auth-Token"
)

var log = golog.LoggerFor("tokenfilter")

type TokenFilter struct {
	next  http.Handler
	token string
}

type optSetter func(f *TokenFilter) error

func TokenSetter(token string) optSetter {
	return func(f *TokenFilter) error {
		f.token = token
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*TokenFilter, error) {
	f := &TokenFilter{
		next:  next,
		token: "",
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f *TokenFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("Token Filter Middleware received request:\n%s", reqStr)
	}

	token := req.Header.Get(tokenHeader)
	if f.token != "" && (token == "" || token != f.token) {
		log.Debugf("Token from %s doesn't match, mimicking apache", req.RemoteAddr)
		mimic.MimicApache(w, req)
	} else {
		req.Header.Del(tokenHeader)
		f.next.ServeHTTP(w, req)
	}
}
