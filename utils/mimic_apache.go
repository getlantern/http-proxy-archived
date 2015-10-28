package utils

import (
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type apacheMimic struct {
	w   http.ResponseWriter
	req *http.Request
}

func MimicApache(w http.ResponseWriter, req *http.Request) {
	fmt.Printf("%+v", req.URL)
	m := apacheMimic{w, req}
	path := req.URL.Path
	switch {
	case len(path) == 0:
		m.ok()
	case path[len(path)-1] == '/':
		m.forbidden()
	case BAD_URIS[path]:
		m.internalServerError()
	case strings.ToLower(path) == "/cgi-bin/":
		m.notFound()
	case req.Method == "OPTIONS":
		m.options()
	case !KNOWN_METHODS[req.Method]:
		m.notImplemented()
	case !ALLOWED_METHODS[req.Method]:
		m.methodNotAllowed()
	case KNOWN_URIS[path]:
		m.ok()
	default:
		m.notFound()
	}
}

func (f *apacheMimic) ok() {
	f.w.Header().Set("Last-Modified", time.Now().String())
	f.w.Header().Set("ETag", newETag())
	f.w.Header().Set("Accept-Ranges", "bytes")
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Connection", "close")
	f.w.Header().Set("Content-Type", "text/html")
	f.writeBody(OK_BODY)
}

func (f *apacheMimic) options() {
	f.w.Header().Set("Allow", "GET,HEAD,POST,OPTIONS")
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Content-Type", "text/html")
	f.w.WriteHeader(http.StatusOK)
}

func (f *apacheMimic) forbidden() {
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Connection", "close")
	f.w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
	f.w.WriteHeader(http.StatusForbidden)
	f.writeBody(FORBIDDEN_BODY)
}

func (f *apacheMimic) notFound() {
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Connection", "close")
	f.w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
	f.w.WriteHeader(http.StatusNotFound)
	f.writeBody(NOT_FOUND_BODY)
}

func (f *apacheMimic) methodNotAllowed() {
	f.w.Header().Set("Allow", "GET,HEAD,POST,OPTIONS")
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
	f.w.WriteHeader(http.StatusMethodNotAllowed)
	f.writeBody(METHOD_NOT_ALLOWED_BODY)
}

func (f *apacheMimic) internalServerError() {
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Connection", "close")
	f.w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
	f.w.WriteHeader(http.StatusInternalServerError)
	f.writeBody(INTERNAL_SERVER_ERROR_BODY)
}

func (f *apacheMimic) notImplemented() {
	f.w.Header().Set("Allow", "GET,HEAD,POST,OPTIONS")
	f.w.Header().Set("Vary", "Accept-Encoding")
	f.w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
	f.w.Header().Set("Connection", "close")
	f.w.WriteHeader(http.StatusNotImplemented)
	f.writeBody(NOT_IMPLEMENTED_BODY)
}

func (f *apacheMimic) writeBody(fmtString string) {
	host, port, err := net.SplitHostPort(f.req.Host)
	if err != nil {
		fmt.Printf("Error %s", err)
		return
	}
	f.w.Write([]byte(fmt.Sprintf(fmtString, "", host, port)))

}

func newETag() string {
	const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	bytes := [20]byte{}
	rand.Read(bytes[:])
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes[:])
}

var KNOWN_URIS = map[string]bool{
	"/":           true,
	"/index":      true,
	"/index.html": true,
}
var BAD_URIS = map[string]bool{
	"/cgi-bin/php":  true,
	"/cgi-bin/php5": true,
}
var ALLOWED_METHODS = map[string]bool{
	"GET":     true,
	"HEAD":    true,
	"POST":    true,
	"OPTIONS": true,
}
var KNOWN_METHODS = map[string]bool{
	"BASELINE-CONTROL": true,
	"CHECKIN":          true,
	"CHECKOUT":         true,
	"CONNECT":          true,
	"COPY":             true,
	"DELETE":           true,
	"GET":              true,
	"HEAD":             true,
	"LABEL":            true,
	"LOCK":             true,
	"MERGE":            true,
	"MKACTIVITY":       true,
	"MKCOL":            true,
	"MKWORKSPACE":      true,
	"MOVE":             true,
	"OPTIONS":          true,
	"PATCH":            true,
	"POLL":             true,
	"POST":             true,
	"PROPFIND":         true,
	"PROPPATCH":        true,
	"PUT":              true,
	"REPORT":           true,
	"TRACE":            true,
	"UNCHECKOUT":       true,
	"UNLOCK":           true,
	"UPDATE":           true,
	"VERSION-CONTROL":  true,
}

const OK_BODY = `<html><body><h1>It works!</h1>
<p>This is the default web page for this server.</p>
<p>The web server software is running but no content has been added, yet.</p>
</body></html>`

const FORBIDDEN_BODY = `<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML 2.0//EN\">
<html><head>
<title>403 Forbidden</title>
</head><body>
<h1>Forbidden</h1>
<p>You don't have permission to access %1$s
on this server.</p>
<hr>
<address>Apache Server at %2$s Port %3$s</address>
</body></html>`

const NOT_FOUND_BODY = `<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML 2.0//EN\">
<html><head>
<title>404 Not Found</title>
</head><body>
<h1>Not Found</h1>
<p>The requested URL %1$s was not found on this server.</p>
<hr>
<address>Apache Server at %2$s Port %3$s</address>
</body></html>`

const METHOD_NOT_ALLOWED_BODY = `<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML 2.0//EN\">
<html><head>
<title>405 Method Not Allowed</title>
</head><body>
<h1>Method Not Allowed</h1>
<p>The requested method %1$s is not allowed for the URL %2$s.</p>
<hr>
<address>Apache Server at %3$s Port %4$s</address>
</body></html>`

const NOT_IMPLEMENTED_BODY = `<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML 2.0//EN\">
<html><head>
<title>501 Method Not Implemented</title>
</head><body>
<h1>Method Not Implemented</h1>
<p>%1$s to %2$s not supported.<br />
</p>
<hr>
<address>Apache Server at %3$s Port %4$s</address>
</body></html>`

const INTERNAL_SERVER_ERROR_BODY = `<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML 2.0//EN\">
<html><head>
<title>500 Internal Server Error</title>
</head><body>
<h1>Internal Server Error</h1>
<p>The server encountered an internal error or
misconfiguration and was unable to complete
your request.</p>
<p>Please contact the server administrator,
 webmaster@%1$s and inform them of the time the error occurred,
and anything you might have done that may have
caused the error.</p>
<p>More information about this error may be available
in the server error log.</p>
<hr>
<address>Apache Server at %1$s Port %2$s</address>
</body></html>`

/*func getApacheLikeURI(req http.Request) string {
        u := req.URL
	                // Strip duplicate leading slash like Apache
			                .replaceFirst("//", "/");
					        if ("/".equals(uri)) {
						            uri = "/index.html";
							            }
								            return uri;
									        }
*/
