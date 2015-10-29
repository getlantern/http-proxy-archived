package main

import (
	"bytes"
	"net"
	"testing"

	"github.com/getlantern/testify/assert"

	"./utils"
)

/*type request struct {
path string
method string
}*/

/*
For rationale behind these tests, refer following:
[1] https://github.com/getlantern/lantern/issues/1255#issuecomment-30658217
[2] https://github.com/getlantern/lantern-java/blob/master/src/main/java/org/lantern/proxy/GiveModeHttpFilters.java#L167
[3] https://github.com/apache/httpd/blob/2.4.17/server/core.c#L4379
*/
var candidates = []string{
	"",
	"/",
	"/index",
	"/index.html",

	"//cgi-bin/php",
	"//cgi-bin/php5",
	"//cgi-bin/php-cgi",
	"//cgi-bin/php.cgi",
	"//cgi-bin/php4",

	"/not-existed",
}

func TestMimicApache(t *testing.T) {
	s := NewServer("anytoken", 100000, 30, false, utils.QUIET)
	chListenOn := make(chan net.Addr)
	go func() {
		err := s.ServeHTTP(":0", &chListenOn)
		assert.NoError(t, err, "should start chained server")
	}()
	addr := (<-chListenOn).String()
	var buf bytes.Buffer
	var tmp [400]byte
	for _, path := range candidates {
		conn, err := net.Dial("tcp", addr)
		if assert.NoError(t, err, "should connect") {
			defer conn.Close()
			_, err := conn.Write([]byte("GET " + path + " HTTP/1.1\n\n"))
			if assert.NoError(t, err, "should write") {
				n, err := conn.Read(tmp[:])
				assert.NoError(t, err, "should read")
				buf.Write(tmp[:n])
			}
		}
	}
}
