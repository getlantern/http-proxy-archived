package filters

import (
	"net/http"
	"net/http/httputil"

	"../utils"
)

const (
	tokenHeader = "X-Lantern-Auth-Token"
)

type TokenFilter struct {
	log   utils.Logger
	next  http.Handler
	token string
}

func NewTokenFilter(next http.Handler, log utils.Logger, token string) *TokenFilter {
	return &TokenFilter{
		log:   log,
		next:  next,
		token: token,
	}
}

func (f *TokenFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reqStr, _ := httputil.DumpRequest(req, true)
	f.log.Debugf("Token Filter Middleware received request:\n%s", reqStr)

	token := req.Header.Get(tokenHeader)
	if token == "" || token != f.token {
		w.WriteHeader(http.StatusNotFound)
	} else {
		req.Header.Del(tokenHeader)
		f.next.ServeHTTP(w, req)
	}
}
