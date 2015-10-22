package uidfilter

import (
	"net/http"
	"net/http/httputil"

	"github.com/gorilla/context"

	"github.com/getlantern/measured"

	"../utils"
)

const (
	uIDHeader = "X-Lantern-UID"
)

type UIDFilter struct {
	log  utils.Logger
	next http.Handler
}

type optSetter func(f *UIDFilter) error

func Logger(l utils.Logger) optSetter {
	return func(f *UIDFilter) error {
		f.log = l
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*UIDFilter, error) {
	f := &UIDFilter{
		log:  utils.NullLogger,
		next: next,
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f *UIDFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f.log.IsLevel(utils.DEBUG) {
		reqStr, _ := httputil.DumpRequest(req, true)
		f.log.Debugf("UIDFilter Middleware received request:\n%s", reqStr)
	}

	lanternUID := req.Header.Get(uIDHeader)

	// An UID must be provided always by the client.  Respond 404 otherwise.
	if lanternUID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Attached the uid to connection to report stats to redis correctly
	// "conn" in context is previously attached in server.go
	key := []byte(lanternUID)
	c := context.Get(req, "conn")
	c.(*measured.Conn).ID = string(key)

	req.Header.Del(uIDHeader)

	f.next.ServeHTTP(w, req)
}
