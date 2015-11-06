/*
Package mimic mimics popular web servers to keep the server from being detected
Currently it only mimics Apache web server.
*/

package mimic

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"text/template"
	"time"
)

type apacheMimic struct {
	conn net.Conn
	req  *http.Request
	path string
}

const timeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

var (
	Host         string
	Port         string
	lastModified = time.Now().Format(timeFormat)
	etag         = makeETag()
)

func MimicApacheOnInvalidRequest(c net.Conn) {
	defer c.Close()
	m := apacheMimic{c, nil, ""}
	m.writeError(nil, badRequestBody)
}

// MimicApache mimics the behaviour of an unconfigured Apache web server 2.4.7
// (the one installed by 'apt-get install apache2') running on Ubuntu 14.04.
// Set 'Host' and 'Port' before calling it.
func MimicApache(w http.ResponseWriter, req *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		panic("fail to hijack, should not happen")
	}
	defer conn.Close()
	path := req.URL.Path
	// remove extra leading slash
	if len(path) > 0 && path[0] == '/' {
		i := 1
		for ; i < len(path) && path[i] == '/'; i++ {
		}
		path = path[i-1:]
	}
	m := apacheMimic{conn, req, path}
	if req.Host == "" {
		m.writeError(badRequestHeader, badRequestBody)
		return
	}
	switch req.Method {
	case "CONNECT":
		m.writeError(badRequestHeader, badRequestBody)
	case "GET", "POST":
		switch path {
		case "/", "/index.html":
			m.ok(indexHeader, indexDotHTML)
		case "/icons/ubuntu-logo.png":
			m.ok(logoHeader, ubuntuLogo)
		default:
			m.writeError(notFoundHeader, notFoundBody)
		}
	case "HEAD":
		switch path {
		case "/", "/index.html":
			m.ok(indexHeader, nil)
		case "/icons/ubuntu-logo.png":
			m.ok(logoHeader, nil)
		default:
			m.writeError(notFoundHeaderWhenHead, nil)
		}
	case "OPTIONS":
		switch path {
		case "/", "/index.html":
			m.ok(optionsHeader, nil)
		case "/icons/ubuntu-logo.png":
			m.ok(optionsHeaderOfLogo, nil)
		default:
			m.writeError(optionsHeaderWhenNotFound, nil)
		}
	default:
		if m.path == "/" {
			m.path = "/index.html"
		}
		switch {
		case !KNOWN_METHODS[req.Method]:
			m.writeError(notImplementedHeader, notImplementedBody)
		case !ALLOWED_METHODS[req.Method]:
			m.writeError(methodNotAllowedHeader, methodNotAllowedBody)
		case KNOWN_URIS[path]:
			//m.ok()
		default:
			m.writeError(notFoundHeader, notFoundBody)
		}
	}
}

func (f *apacheMimic) ok(header *template.Template, body []byte) {
	var buf bytes.Buffer
	err := header.Execute(&buf, f.collectVars())
	if err != nil {
		panic(fmt.Sprintf("execute template err: %s", err))
	}
	// ignore any errors writing back to connection
	_, _ = buf.WriteTo(f.conn)
	_, _ = f.conn.Write(body)
}

func (f *apacheMimic) writeError(header, body *template.Template) {
	var headBuf bytes.Buffer
	var bodyBuf bytes.Buffer
	vars := f.collectVars()
	if body != nil {
		err := body.Execute(&bodyBuf, vars)
		if err != nil {
			panic(fmt.Sprintf("execute template err: %s", err))
		}
		vars.ContentLength = bodyBuf.Len()
	}
	if header != nil {
		err := header.Execute(&headBuf, vars)
		if err != nil {
			panic(fmt.Sprintf("execute template err: %s", err))
		}
	}
	// ignore any errors writing back to connection
	_, _ = headBuf.WriteTo(f.conn)
	_, _ = bodyBuf.WriteTo(f.conn)
}

type vars struct {
	Date, LastModified, ETag, Path, Host, Port string
	ContentType                                string
	ContentLength                              int
}

func (f *apacheMimic) collectVars() *vars {
	return &vars{
		Date:         time.Now().Format(timeFormat),
		LastModified: lastModified,
		ETag:         etag,
		Path:         f.path,
		Host:         Host,
		Port:         Port,
	}
}

func makeETag() string {
	const alphanum = "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := [17]byte{}
	rand.Read(bytes[:])
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	bytes[4] = '-'
	return string(bytes[:])
}

var KNOWN_URIS = map[string]bool{
	"/":           true,
	"/index":      true,
	"/index.html": true,
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

var indexHeader = template.Must(template.New("index").Parse("HTTP/1.1 200 OK\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Last-Modified: {{.LastModified}}\r\n" +
	"ETag: \"{{.ETag}}\"\r\n" +
	"Accept-Ranges: bytes\r\n" +
	"Content-Length: 11510\r\n" +
	"Vary: Accept-Encoding\r\n" +
	"Content-Type: text/html\r\n\r\n"))

var logoHeader = template.Must(template.New("logo").Parse("HTTP/1.1 200 OK\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Last-Modified: {{.LastModified}}\r\n" +
	"ETag: \"{{.ETag}}\"\r\n" +
	"Accept-Ranges: bytes\r\n" +
	"Content-Length: 3404\r\n" +
	"Content-Type: image/png\r\n\r\n"))

var notFoundHeader = template.Must(template.New("notFoundHeader").Parse("HTTP/1.1 404 Not Found\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Content-Length: {{.ContentLength}}\r\n" +
	"Content-Type: text/html; charset=iso-8859-1\r\n\r\n"))

var notFoundHeaderWhenHead = template.Must(template.New("notFoundHeaderWhenHead").Parse("HTTP/1.1 404 Not Found\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Content-Type: text/html; charset=iso-8859-1\r\n\r\n"))

var notFoundBody = template.Must(template.New("notFoundBody").Parse(`<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>404 Not Found</title>
</head><body>
<h1>Not Found</h1>
<p>The requested URL {{.Path}} was not found on this server.</p>
<hr>
<address>Apache/2.4.7 (Ubuntu) Server at {{.Host}} Port {{.Port}}</address>
</body></html>
`))

var badRequestHeader = template.Must(template.New("notFound").Parse("HTTP/1.1 400 Bad Request\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Content-Length: {{.ContentLength}}\r\n" +
	"Connection: close\r\n" +
	"Content-Type: text/html; charset=iso-8859-1\r\n\r\n"))
var badRequestBody = template.Must(template.New("notFound").Parse(
	`<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>400 Bad Request</title>
</head><body>
<h1>Bad Request</h1>
<p>Your browser sent a request that this server could not understand.<br />
</p>
<hr>
<address>Apache/2.4.7 (Ubuntu) Server at {{.Host}} Port {{.Port}}</address>
</body></html>
`))

var optionsHeader = template.Must(template.New("optionsHeader").Parse("HTTP/1.1 200 OK\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Allow: GET,HEAD,POST,OPTIONS\r\n" +
	"Content-Length: {{.ContentLength}}\r\n" +
	"Content-Type: text/html\r\n\r\n"))

var optionsHeaderWhenNotFound = template.Must(template.New("optionsHeaderWhenNotFound").Parse("HTTP/1.1 200 OK\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Allow: GET,HEAD,POST,OPTIONS\r\n" +
	"Content-Length: {{.ContentLength}}\r\n\r\n"))

var optionsHeaderOfLogo = template.Must(template.New("optionsHeaderOfLogo").Parse("HTTP/1.1 200 OK\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Allow: GET,HEAD,POST,OPTIONS\r\n" +
	"Content-Length: {{.ContentLength}}\r\n" +
	"Content-Type: image/png\r\n\r\n"))

var methodNotAllowedHeader = template.Must(template.New("methodNotAllowedHeader").Parse("HTTP/1.1 405 Method Not Allowed\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Allow: GET,HEAD,POST,OPTIONS\r\n" +
	"Content-Length: {{.ContentLength}}\r\n" +
	"Content-Type: text/html; charset=iso-8859-1\r\n\r\n"))

var methodNotAllowedBody = template.Must(template.New("methodNotAllowed").Parse(
	`<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>405 Method Not Allowed</title>
</head><body>
<h1>Method Not Allowed</h1>
<p>The requested method PUT is not allowed for the URL {{.Path}}.</p>
<hr>
<address>Apache/2.4.7 (Ubuntu) Server at {{.Host}} Port {{.Port}}</address>
</body></html>
`))

var notImplementedHeader = template.Must(template.New("notImplementedHeader").Parse("HTTP/1.1 501 Not Implemented\r\n" +
	"Date: {{.Date}}\r\n" +
	"Server: Apache/2.4.7 (Ubuntu)\r\n" +
	"Allow: GET,HEAD,POST,OPTIONS\r\n" +
	"Content-Length: {{.ContentLength}}\r\n" +
	"Connection: close\r\n" +
	"Content-Type: text/html; charset=iso-8859-1\r\n\r\n"))

var notImplementedBody = template.Must(template.New("notImplementedBody").Parse(
	`<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>501 Not Implemented</title>
</head><body>
<h1>Not Implemented</h1>
<p>INVALID to {{.Path}} not supported.<br />
</p>
<hr>
<address>Apache/2.4.7 (Ubuntu) Server at {{.Host}} Port {{.Port}}</address>
</body></html>
`))
