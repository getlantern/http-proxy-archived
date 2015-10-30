package devicefilter

import (
	"net/http"
	"net/http/httputil"

	"github.com/gorilla/context"

	"github.com/getlantern/measured"
  "github.com/getlantern/http-proxy/utils"
)

const (
	deviceIdHeader = "X-Lantern-Device-Id"
)

type DeviceFilter struct {
	log  utils.Logger
	next http.Handler
}

type optSetter func(f *DeviceFilter) error

func Logger(l utils.Logger) optSetter {
	return func(f *DeviceFilter) error {
		f.log = l
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*DeviceFilter, error) {
	f := &DeviceFilter{
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

func (f *DeviceFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f.log.IsLevel(utils.DEBUG) {
		reqStr, _ := httputil.DumpRequest(req, true)
		f.log.Debugf("DeviceFilter Middleware received request:\n%s", reqStr)
	}

	lanternDeviceId := req.Header.Get(deviceIdHeader)

	if lanternDeviceId == "" {
		f.log.Debugf("No %s header found, respond 404 not found to %s\n", deviceIdHeader, req.RemoteAddr)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Attached the uid to connection to report stats to redis correctly
	// "conn" in context is previously attached in server.go
	key := []byte(lanternDeviceId)
	c := context.Get(req, "conn")
	c.(*measured.Conn).ID = string(key)

	req.Header.Del(deviceIdHeader)

	f.next.ServeHTTP(w, req)
}
