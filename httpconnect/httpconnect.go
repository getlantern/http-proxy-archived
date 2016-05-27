package httpconnect

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/idletiming"

	"github.com/getlantern/http-proxy/buffers"
	"github.com/getlantern/http-proxy/filter"
	"github.com/getlantern/http-proxy/utils"
)

var log = golog.LoggerFor("httpconnect")

type Options struct {
	IdleTimeout  time.Duration
	AllowedPorts []int
}

type httpConnectHandler struct {
	*Options
}

func AllowedPortsFromCSV(csv string) ([]int, error) {
	fields := strings.Split(csv, ",")
	ports := make([]int, len(fields))
	for i, f := range fields {
		p, err := strconv.Atoi(f)
		if err != nil {
			return nil, err
		}
		ports[i] = p
	}
	return ports, nil
}

func New(opts *Options) filter.Filter {
	return &httpConnectHandler{opts}
}

func (f *httpConnectHandler) Apply(w http.ResponseWriter, req *http.Request) (bool, error, string) {
	if req.Method != "CONNECT" {
		// Fall through
		return filter.Continue()
	}

	if log.IsTraceEnabled() {
		reqStr, _ := httputil.DumpRequest(req, true)
		log.Tracef("httpConnectHandler Middleware received request:\n%s", reqStr)
	}

	if f.portAllowed(w, req) {
		f.intercept(w, req)
	}

	return filter.Stop()
}

func (f *httpConnectHandler) portAllowed(w http.ResponseWriter, req *http.Request) bool {
	if len(f.AllowedPorts) == 0 {
		return true
	}
	log.Tracef("Checking CONNECT tunnel to %s against allowed ports %v", req.Host, f.AllowedPorts)
	_, portString, err := net.SplitHostPort(req.Host)
	if err != nil {
		// CONNECT request should always include port in req.Host.
		// Ref https://tools.ietf.org/html/rfc2817#section-5.2.
		f.ServeError(w, req, http.StatusBadRequest, "No port field in Request-URI / Host header")
		return false
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		f.ServeError(w, req, http.StatusBadRequest, "Invalid port")
		return false
	}

	for _, p := range f.AllowedPorts {
		if port == p {
			return true
		}
	}
	f.ServeError(w, req, http.StatusForbidden, "Port not allowed")
	return false
}

func (f *httpConnectHandler) intercept(w http.ResponseWriter, req *http.Request) {
	utils.RespondOK(w, req)

	clientConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		utils.RespondBadGateway(w, req, fmt.Sprintf("Unable to hijack connection: %s", err))
		return
	}
	connOutRaw, err := net.DialTimeout("tcp", req.Host, 10*time.Second)
	if err != nil {
		return
	}
	connOut := idletiming.Conn(connOutRaw, f.IdleTimeout, func() {
		if connOutRaw != nil {
			connOutRaw.Close()
		}
	})

	// Pipe data through CONNECT tunnel
	closeConns := func() {
		if clientConn != nil {
			if err := clientConn.Close(); err != nil {
				log.Debugf("Error closing the out connection: %s", err)
			}
		}
		if connOut != nil {
			if err := connOut.Close(); err != nil {
				log.Debugf("Error closing the client connection: %s", err)
			}
		}
	}
	var closeOnce sync.Once
	go func() {
		buf := buffers.Get()
		defer buffers.Put(buf)
		if _, err := io.CopyBuffer(connOut, clientConn, buf); err != nil {
			log.Debug(err)
		}
		closeOnce.Do(closeConns)

	}()
	buf := buffers.Get()
	defer buffers.Put(buf)
	if _, err := io.CopyBuffer(clientConn, connOut, buf); err != nil {
		log.Debug(err)
	}
	closeOnce.Do(closeConns)

	return
}

func (f *httpConnectHandler) ServeError(w http.ResponseWriter, req *http.Request, statusCode int, reason string) {
	log.Debugf("Respond error to CONNECT request to %s: %d %s", req.Host, statusCode, reason)
	w.WriteHeader(statusCode)
	w.Write([]byte(reason))
}
