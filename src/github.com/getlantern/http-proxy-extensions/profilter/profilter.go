// Lantern Pro middleware will identify Pro users and forward their requests
// immediately.  It will intercept non-Pro users and limit their total transfer

package profilter

import (
	"net/http"
	"net/http/httputil"

	"github.com/Workiva/go-datastructures/set"
	"github.com/getlantern/golog"
)

const (
	proTokenHeader = "X-Lantern-Pro-Token"
)

var log = golog.LoggerFor("profilter")

type LanternProFilter struct {
	next      http.Handler
	proTokens *set.Set
}

type optSetter func(f *LanternProFilter) error

func New(next http.Handler, setters ...optSetter) (*LanternProFilter, error) {
	f := &LanternProFilter{
		next:      next,
		proTokens: set.New(),
	}

	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f *LanternProFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("Lantern Pro Filter Middleware received request:\n%s", reqStr)
	}

	lanternProToken := req.Header.Get(proTokenHeader)
	req.Header.Del(proTokenHeader)
	if lanternProToken != "" {
		log.Tracef("Lantern Pro Token found")
	}

	// If a Pro token is found in the header, test if its valid and then let
	// the request pass.
	// If we ever want to block users above a threshold, do it here.
	// If we want to pass data along the request, we should use:
	// http://www.gorillatoolkit.org/pkg/context
	/*
		if lanternProToken != "" {
			if f.proTokens.Exists(lanternProToken) {
				f.next.ServeHTTP(w, req)
			} else {
				w.WriteHeader(http.StatusBadGateway)
			}
			return
		}
	*/
	f.next.ServeHTTP(w, req)
}
