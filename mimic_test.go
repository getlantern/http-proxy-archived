package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/getlantern/testify/assert"

	"./utils"
)

const recorded = "test-data/apache-2.4.7-ubuntu14.04.rec"
const fetched = "test-data/chained-server.rec"

type entry struct {
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
	{"", false},
	{"/", false},
	{"/index.html", false},
	{"/not-existed", true},

	// 200 with default page
	{"", true},
	{"/", true},
	{"/index.html", true},

	// 404
	{"//cgi-bin/php", true},
	{"//cgi-bin/php5", true},
	{"//cgi-bin/php-cgi", true},
	{"//cgi-bin/php.cgi", true},
	{"//cgi-bin/php4", true},

	{"/not-existed", true},
}

func TestMimicApache(t *testing.T) {
	s := NewServer("anytoken", 100000, 30, false, utils.QUIET)
	chListenOn := make(chan net.Addr)
	go func() {
		err := s.ServeHTTP(":0", &chListenOn)
		assert.NoError(t, err, "should start chained server")
	}()
	addr := (<-chListenOn).String()
	buf := request(t, addr)
	ioutil.WriteFile(fetched, buf.Bytes(), os.ModePerm)
	compare(t, buf, recorded)
}

func TestRealApache(t *testing.T) {
	t.Skip("comment out this line when you want to record http traffic against real apache server")
	addr := "128.199.100.121:80"
	buf := request(t, addr)
	ioutil.WriteFile(recorded, buf.Bytes(), os.ModePerm)
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
		assert.NoError(t, err, "should run diff")
		t.Log(string(b))
		if "" != string(b) {
			t.Error("should run diff")
			t.Fail()
		}
	}
}

func request(t *testing.T, addr string) *bytes.Buffer {
	var buf bytes.Buffer
	for _, c := range candidates {
		conn, err := net.Dial("tcp", addr)
		if assert.NoError(t, err, "should connect") {
			defer conn.Close()
			req := "GET " + c.path + " HTTP/1.1\n"
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
	s := buf.String()
	exps := []*regexp.Regexp{
		// Date: ... GMT
		regexp.MustCompile(`[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT`),
		// Last-Modified: ... CST
		regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{9} \+\d{4} CST`),
		// ETag: "..."
		regexp.MustCompile(`"\w{4}-\w{13}"`),
		// Apache/2.4.7 (Ubuntu) Server at 128.199.100.121 Port 80
		regexp.MustCompile(`Apache\/\d\.\d\.\d+ \(Ubuntu\) Server at \d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3} Port \d{2,}`),
	}
	for _, e := range exps {
		s = e.ReplaceAllString(s, "<PLACEHOLDER>")
	}
	return bytes.NewBufferString(s)
}
