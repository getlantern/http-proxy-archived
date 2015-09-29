package tokenfilter

import (
	"net/http"
)

type TokenFilter struct {
	next  http.Handler
	token string
}

func New(next http.Handler, token string) (*TokenFilter, error) {
	return &TokenFilter{
		next:  next,
		token: token,
	}, nil
}

func (f *TokenFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	token := req.Header.Get("X-Lantern-Auth-Token")
	if token == "" || token != f.token {
		w.WriteHeader(http.StatusNotFound)
	} else {
		f.next.ServeHTTP(w, req)
	}
}
