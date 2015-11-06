package mimic

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/getlantern/testify/assert"

	proxy "github.com/getlantern/http-proxy"
)

const target = "test-data/apache-2.4.7-ubuntu14.04.raw"
const template = "test-data/apache-2.4.7-ubuntu14.04.tpl"
const current = "test-data/http-proxy.raw"

type entry struct {
	method string
	path   string
}

/*
For rationale behind these tests, refer following:
[1] https://github.com/getlantern/lantern/issues/1255#issuecomment-30658217
[2] https://github.com/getlantern/lantern-java/blob/master/src/main/java/org/lantern/proxy/GiveModeHttpFilters.java#L167
[3] https://github.com/apache/httpd/blob/2.4.17/server/core.c#L4379
*/
var candidates = []entry{
	// 200 with default page
	{"GET", "/"},
	{"GET", "/index.html"},

	{"GET", "/icons/ubuntu-logo.png"},

	// 404
	{"GET", "//cgi-bin/php"},
	{"GET", "//cgi-bin/php5"},
	{"GET", "//cgi-bin/php-cgi"},
	{"GET", "//cgi-bin/php.cgi"},
	{"GET", "//cgi-bin/php4"},

	// multiple slashes
	{"GET", "///cgi-bin/php4"},
	{"GET", "//cgi-bin//php4"},

	{"GET", "/not-existed"},
	{"GET", "/end-with-slash/"},

	{"HEAD", "/"},
	{"HEAD", "/index.html"},
	{"HEAD", "/icons/ubuntu-logo.png"},
	{"HEAD", "/not-existed"},

	{"POST", "/"},
	{"POST", "/index.html"},
	{"POST", "/icons/ubuntu-logo.png"},
	{"POST", "/not-existed"},

	{"OPTIONS", "/"},
	{"OPTIONS", "/index.html"},
	{"OPTIONS", "/icons/ubuntu-logo.png"},
	{"OPTIONS", "/not-existed"},

	{"PUT", "/"},
	{"PUT", "/index.html"},
	{"PUT", "/icons/ubuntu-logo.png"},
	{"PUT", "/not-existed"},

	{"CONNECT", "/"},
	{"CONNECT", "/index.html"},
	{"CONNECT", "/icons/ubuntu-logo.png"},
	{"CONNECT", "/not-existed"},

	{"INVALID", "/"},
	{"INVALID", "/index.html"},
	{"INVALID", "/not-existed"},
}

type entryWithHeaders struct {
	method         string
	path           string
	withHostHeader bool
	headers        []string
}

var invalidRequests = []entryWithHeaders{
	// Without Host header, Apache will return 400
	{"GET", "/", false, []string{}},
	{"GET", "/index.html", false, []string{}},

	// invalid request handled by go server
	{"GET", "", false, []string{}},
	{"GET", "/", true, []string{"User-Agent: xxx", "User-Agent: xxx"}},
}

func TestMimicApache(t *testing.T) {
	s := proxy.NewServer("anytoken", 100000, 30*time.Second, true, false)
	chListenOn := make(chan string)
	go func() {
		err := s.ServeHTTP(":0", &chListenOn)
		assert.NoError(t, err, "should start chained server")
	}()
	buf := request(t, <-chListenOn)
	ioutil.WriteFile(current, buf.Bytes(), os.ModePerm)
	compare(t, normalize(buf), template)
}

func TestRealApache(t *testing.T) {
	t.Skip("comment out this line and run 'go test -run RealApache' when you want to record http traffic against real apache server")
	addr := "128.199.100.121:80"
	buf := request(t, addr)
	ioutil.WriteFile(target, buf.Bytes(), os.ModePerm)
	ioutil.WriteFile(template, normalize(buf).Bytes(), os.ModePerm)
}

func compare(t *testing.T, buf *bytes.Buffer, file string) {
	c := exec.Command("diff", "-a", "-", file)
	in, err := c.StdinPipe()
	if assert.NoError(t, err, "should gain stdin pipe") {
		go func() {
			buf.WriteTo(in)
			in.Close()
		}()
		b, err := c.CombinedOutput()
		assert.NoError(t, err, "should not different from an real apache")
		t.Log(string(b))
	}
}

func request(t *testing.T, addr string) *bytes.Buffer {
	var buf bytes.Buffer
	for _, c := range candidates {
		requestItem(t, &buf, addr, c.method, c.path, []string{"Host: " + addr})
	}
	for _, r := range invalidRequests {
		if r.withHostHeader {
			r.headers = append(r.headers, "Host: "+addr)
		}
		requestItem(t, &buf, addr, "GET", r.path, r.headers)
	}
	return &buf
}

func requestItem(t *testing.T, buf *bytes.Buffer, addr, method, path string, headers []string) {
	conn, err := net.Dial("tcp", addr)
	if assert.NoError(t, err, "should connect") {
		defer conn.Close()
		req := method + " " + path + " HTTP/1.1\n"
		buf.WriteString(req)
		buf.WriteString("--------------------\n")
		for _, h := range headers {
			req = req + h + "\n"
		}
		_, err := conn.Write([]byte(req + "\n"))
		if assert.NoError(t, err, "should write") {
			_, err := io.Copy(buf, conn)
			assert.NoError(t, err, "should copy")
			buf.WriteString("====================\n\n")
		}
	}
}

func normalize(in *bytes.Buffer) *bytes.Buffer {
	s := in.String()
	exps := []struct {
		regexp *regexp.Regexp
		with   string
	}{
		// Date: ... GMT
		{regexp.MustCompile(`[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT`), "<GMT>"},
		// ETag: "xxxx-xxxxxxxxxxxxx"
		{regexp.MustCompile(`ETag: "\w{3,4}-\w{12,13}"`), `ETag: "<ETAG>"`},
		// Apache/2.4.7 (Ubuntu) Server at 128.199.100.121 Port 80
		{regexp.MustCompile(`Apache\/\d\.\d\.\d+ \(Ubuntu\) Server at \S+ Port \d{2,}`), "Apache/<VERSION> (Ubuntu) Server at <Host> Port <Port>"},
		// Content-Length: 1243
		{regexp.MustCompile(`Content-Length: \d+`), "Content-Length: <...>"},
	}
	for _, e := range exps {
		s = e.regexp.ReplaceAllString(s, e.with)
	}
	return bytes.NewBufferString(s)
}
