package lanternpro

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/mailgun/oxy/utils"
)

// ReqRewriter can alter request headers and body
type ReqRewriter interface {
	Rewrite(r *http.Request)
}

// Rewriter is responsible for removing hop-by-hop headers and setting forwarding headers
type HeaderRewriter struct {
	TrustForwardHeader bool
	Hostname           string
}

func (rw *HeaderRewriter) Rewrite(req *http.Request) {
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if rw.TrustForwardHeader {
			if prior, ok := req.Header[XForwardedFor]; ok {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
		}
		req.Header.Set(XForwardedFor, clientIP)
	}

	if xfp := req.Header.Get(XForwardedProto); xfp != "" && rw.TrustForwardHeader {
		req.Header.Set(XForwardedProto, xfp)
	} else if req.TLS != nil {
		req.Header.Set(XForwardedProto, "https")
	} else {
		req.Header.Set(XForwardedProto, "http")
	}

	if xfh := req.Header.Get(XForwardedHost); xfh != "" && rw.TrustForwardHeader {
		req.Header.Set(XForwardedHost, xfh)
	} else if req.Host != "" {
		req.Header.Set(XForwardedHost, req.Host)
	}

	if rw.Hostname != "" {
		req.Header.Set(XForwardedServer, rw.Hostname)
	}

	// Remove hop-by-hop headers to the backend.  Especially important is "Connection" because we want a persistent
	// connection, regardless of what the client sent to us.
	utils.RemoveHeaders(req.Header, HopHeaders...)
}

type optSetter func(f *LanternProPlugin) error

type LanternProPlugin struct {
	errHandler   utils.ErrorHandler
	roundTripper http.RoundTripper
	rewriter     ReqRewriter
	log          utils.Logger
}

func New(setters ...optSetter) (*LanternProPlugin, error) {
	p := &LanternProPlugin{}
	for _, s := range setters {
		if err := s(p); err != nil {
			return nil, err
		}
	}
	if p.roundTripper == nil {
		p.roundTripper = http.DefaultTransport
	}
	if p.rewriter == nil {
		h, err := os.Hostname()
		if err != nil {
			h = "localhost"
		}
		p.rewriter = &HeaderRewriter{TrustForwardHeader: true, Hostname: h}
	}
	if p.log == nil {
		p.log = utils.NullLogger
	}
	if p.errHandler == nil {
		p.errHandler = utils.DefaultHandler
	}
	return p, nil
}

func (f *LanternProPlugin) ServeHTTP(w http.ResponseWriter, req *http.Request) {
}
