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
const current = "test-data/chained-server.raw"

type entry struct {
	method         string
	path           string
	withHostHeader bool
}

/*
For rationale behind these tests, refer following:
[1] https://github.com/getlantern/lantern/issues/1255#issuecomment-30658217
[2] https://github.com/getlantern/lantern-java/blob/master/src/main/java/org/lantern/proxy/GiveModeHttpFilters.java#L167
[3] https://github.com/apache/httpd/blob/2.4.17/server/core.c#L4379
*/
var candidates = []entry{
	// Without Host header, Apache will return 400
	{"GET", "/", false},
	{"GET", "/index.html", false},

	// 200 with default page
	{"GET", "/", true},
	{"GET", "/index.html", true},

	{"GET", "/icons/ubuntu-logo.png", true},

	// 404
	{"GET", "//cgi-bin/php", true},
	{"GET", "//cgi-bin/php5", true},
	{"GET", "//cgi-bin/php-cgi", true},
	{"GET", "//cgi-bin/php.cgi", true},
	{"GET", "//cgi-bin/php4", true},

	// multiple slashes
	{"GET", "///cgi-bin/php4", true},
	{"GET", "//cgi-bin//php4", true},

	{"GET", "/not-existed", true},
	{"GET", "/end-with-slash/", true},

	{"HEAD", "/", true},
	{"HEAD", "/index.html", true},
	{"HEAD", "/icons/ubuntu-logo.png", true},
	{"HEAD", "/not-existed", true},

	{"POST", "/", true},
	{"POST", "/index.html", true},
	{"POST", "/icons/ubuntu-logo.png", true},
	{"POST", "/not-existed", true},

	{"OPTIONS", "/", true},
	{"OPTIONS", "/index.html", true},
	{"OPTIONS", "/icons/ubuntu-logo.png", true},
	{"OPTIONS", "/not-existed", true},

	{"PUT", "/", true},
	{"PUT", "/index.html", true},
	{"PUT", "/icons/ubuntu-logo.png", true},
	{"PUT", "/not-existed", true},

	{"CONNECT", "/", true},
	{"CONNECT", "/index.html", true},
	{"CONNECT", "/icons/ubuntu-logo.png", true},
	{"CONNECT", "/not-existed", true},

	{"INVALID", "/", true},
	{"INVALID", "/index.html", true},
	{"INVALID", "/not-existed", true},
}

func TestMimicApache(t *testing.T) {
	s := proxy.NewServer("anytoken", 100000, 30*time.Second, false, false)
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
		conn, err := net.Dial("tcp", addr)
		if assert.NoError(t, err, "should connect") {
			defer conn.Close()
			req := c.method + " " + c.path + " HTTP/1.1\n"
			buf.WriteString(req)
			buf.WriteString("--------------------\n")
			if c.withHostHeader {
				req = req + "Host: " + addr + "\n"
			}
			_, err := conn.Write([]byte(req + "\n"))
			if assert.NoError(t, err, "should write") {
				_, err := io.Copy(&buf, conn)
				assert.NoError(t, err, "should copy")
				buf.WriteString("====================\n\n")
			}
		}
	}
	return &buf
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
