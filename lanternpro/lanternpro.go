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
	f.next.ServeHTTP(w, req)
}
