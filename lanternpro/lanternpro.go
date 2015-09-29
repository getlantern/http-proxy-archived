package lanternpro

import (
	"net/http"
)

type LanternProFilter struct {
	next http.Handler
}

func New(next http.Handler) (*LanternProFilter, error) {
	return &LanternProFilter{
		next: next,
	}, nil
}

func (f *LanternProFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	token := req.Header.Get("X-Lantern-Auth-Token")
	if token == "" && req.Method == "CONNECT" {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	f.next.ServeHTTP(w, req)
}
