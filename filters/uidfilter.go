package filters

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

func NewUIDFilter(next http.Handler, log utils.Logger) *UIDFilter {
	return &UIDFilter{
		log:  log,
		next: next,
	}
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

	// Get the client and attach it as request context
	key := []byte(lanternUID)
	c := context.Get(req, "conn")
	c.(*measured.Conn).ID = string(key)

	req.Header.Del(uIDHeader)

	f.next.ServeHTTP(w, req)
}
