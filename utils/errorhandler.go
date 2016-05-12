package utils

import (
	"io"
	"net"
	"net/http"

	"github.com/getlantern/golog"
)

type ErrorHandler interface {
	ServeHTTP(w http.ResponseWriter, req *http.Request, err error, desc string)
}

var (
	log = golog.LoggerFor("errorhandler")

	DefaultHandler ErrorHandler = &StdHandler{}
)

type StdHandler struct {
}

func (e *StdHandler) ServeHTTP(w http.ResponseWriter, req *http.Request, err error, desc string) {
	statusCode := http.StatusInternalServerError
	if e, ok := err.(net.Error); ok {
		if e.Timeout() {
			statusCode = http.StatusGatewayTimeout
		} else {
			statusCode = http.StatusBadGateway
		}
	} else if err == io.EOF {
		statusCode = http.StatusBadGateway
	}
	log.Errorf("Responding with %d due to %v: %v", statusCode, err, desc)
	w.WriteHeader(statusCode)
	w.Write([]byte(http.StatusText(statusCode)))
}

type ErrorHandlerFunc func(http.ResponseWriter, *http.Request, error)

// ServeHTTP calls f(w, r).
func (f ErrorHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request, err error) {
	f(w, r, err)
}
