package forward

import (
	"net"
	"net/http"
	"strings"
)

const (
	XForwardedProto  = "X-Forwarded-Proto"
	XForwardedFor    = "X-Forwarded-For"
	XForwardedHost   = "X-Forwarded-Host"
	XForwardedServer = "X-Forwarded-Server"
	ContentLength    = "Content-Length"
)

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
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

	// In principle we don't want to add the Proto and Host headers
	/*
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
	*/

	if rw.Hostname != "" {
		req.Header.Set(XForwardedServer, rw.Hostname)
	}

	// Remove hop-by-hop headers to the backend.  Especially important is
	// "Connection" because we want a persistent connection, regardless
	// of what the client sent to us.
	removeHeaders(req.Header, hopHeaders...)
}
