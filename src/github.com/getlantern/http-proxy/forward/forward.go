package forward

import (
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy/utils"
	"github.com/getlantern/idletiming"
)

var log = golog.LoggerFor("forward")

type Forwarder struct {
	errHandler   utils.ErrorHandler
	roundTripper http.RoundTripper
	rewriter     RequestRewriter
	next         http.Handler

	idleTimeout time.Duration
}

type optSetter func(f *Forwarder) error

func RoundTripper(r http.RoundTripper) optSetter {
	return func(f *Forwarder) error {
		f.roundTripper = r
		return nil
	}
}

type RequestRewriter interface {
	Rewrite(r *http.Request)
}

func Rewriter(r RequestRewriter) optSetter {
	return func(f *Forwarder) error {
		f.rewriter = r
		return nil
	}
}

func IdleTimeoutSetter(i time.Duration) optSetter {
	return func(f *Forwarder) error {
		f.idleTimeout = i
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*Forwarder, error) {
	var dialerFunc func(string, string) (net.Conn, error)

	var timeoutTransport http.RoundTripper = &http.Transport{
		Dial:                dialerFunc,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	f := &Forwarder{
		errHandler:   utils.DefaultHandler,
		roundTripper: timeoutTransport,
		next:         next,
		idleTimeout:  30,
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}
	if f.rewriter == nil {
		f.rewriter = &HeaderRewriter{
			TrustForwardHeader: true,
			Hostname:           "",
		}
	}

	dialerFunc = func(network, addr string) (conn net.Conn, err error) {
		conn, err = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial(network, addr)
		if err != nil {
			return
		}

		idleConn := idletiming.Conn(conn, f.idleTimeout, func() {
			if conn != nil {
				conn.Close()
			}
		})
		return idleConn, err
	}

	return f, nil
}

func (f *Forwarder) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	// Create a copy of the request suitable for our needs
	reqClone, err := f.cloneRequest(req, req.URL)
	if err != nil {
		log.Errorf("Error forwarding to %v, error: %v", req.Host, err)
		f.errHandler.ServeHTTP(w, req, err)
		return
	}
	f.rewriter.Rewrite(reqClone)

	// Forward the request and get a response
	start := time.Now().UTC()
	response, err := f.roundTripper.RoundTrip(reqClone)
	if err != nil {
		log.Errorf("Error forwarding to %v, error: %v", req.Host, err)
		f.errHandler.ServeHTTP(w, req, err)
		return
	}
	log.Debugf("Round trip: %v, code: %v, duration: %v",
		req.URL, response.StatusCode, time.Now().UTC().Sub(start))

	if log.IsTraceEnabled() {
		respStr, _ := httputil.DumpResponse(response, true)
		log.Tracef("Forward Middleware received response:\n%s", respStr)
	}

	// Forward the response to the origin
	copyHeadersForForwarding(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)

	// It became nil in a Co-Advisor test though the doc says it will never be nil
	if response.Body != nil {
		_, err = io.Copy(w, response.Body)
		if err != nil {
			log.Error(err)
		}

		response.Body.Close()
	}
}

func (f *Forwarder) cloneRequest(req *http.Request, u *url.URL) (*http.Request, error) {
	outReq := new(http.Request)
	// Beware, this will make a shallow copy. We have to copy all maps
	*outReq = *req

	outReq.Proto = "HTTP/1.1"
	outReq.ProtoMajor = 1
	outReq.ProtoMinor = 1
	// Overwrite close flag: keep persistent connection for the backend servers
	outReq.Close = false

	// Request Header
	outReq.Header = make(http.Header)
	copyHeadersForForwarding(outReq.Header, req.Header)

	// Request URL
	scheme := "http"
	outReq.URL = cloneURL(req.URL)
	outReq.URL.Scheme = scheme
	outReq.URL.Host = req.Host
	outReq.URL.Opaque = req.RequestURI
	// raw query is already included in RequestURI, so ignore it to avoid dupes
	outReq.URL.RawQuery = ""

	/*
		// Trailer support
		// We are forced to do this because Go's server won't allow us to read the trailers otherwise
		_, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			log.Errorf("Error: %v", err)
			return outReq, err
		}

		rcloser := ioutil.NopCloser(req.Body)
		outReq.Body = rcloser

		chunkedTransfer := false
		for _, enc := range req.TransferEncoding {
			if enc == "chunked" {
				chunkedTransfer = true
				break
			}
		}

		// Append Trailer
		if chunkedTransfer && len(req.Trailer) > 0 {
			outReq.Trailer = http.Header{}
			for k, vv := range req.Trailer {
				for _, v := range vv {
					outReq.Trailer.Add(k, v)
				}
			}
		}
	*/

	return outReq, nil
}
