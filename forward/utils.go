package forward

import (
	"net/http"
	"net/url"
)

// cloneURL provides update safe copy by avoiding shallow copying User field
func cloneURL(i *url.URL) *url.URL {
	out := *i
	if i.User != nil {
		out.User = &(*i.User)
	}
	return &out
}

// copyHeaders copies http headers from source to destination.  It does not
// overide, but adds multiple headers
func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		switch k {
		// Skip hop-by-hop headers, ref section 13.5.1 of http://www.ietf.org/rfc/rfc2616.txt
		case "Connection":
			continue
		case "Keep-Alive":
			continue
		case "Proxy-Authenticate":
			continue
		case "Proxy-Authorization":
			continue
		case "TE":
			continue
		case "Trailers":
			continue
		case "Transfer-Encoding":
			continue
		case "Upgrade":
			continue
		default:
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

// removeHeaders removes the header with the given names from the headers map
func removeHeaders(headers http.Header, names ...string) {
	for _, h := range names {
		headers.Del(h)
	}
}
