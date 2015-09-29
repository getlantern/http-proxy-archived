package chained

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/getlantern/testify/assert"
)

func TestCONNECT(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	tunneledReq := "GET / HTTP/1.1\r\n\r\n"
	connectResp := "HTTP/1.1 200 OK\r\n"
	tunneledResp := "holla!\n"
	defer stopMockServers()
	site, _ := newMockServer("holla!")
	u, _ := url.Parse(site)
	log.Debugf("Started target site at %s", u)

	addr, _ := startServer(t)
	log.Debugf("Started proxy server at %s", addr)

	con, err := net.Dial("tcp", addr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		return
	}
	defer func() {
		assert.NoError(t, con.Close(), "should close connection")
	}()

	req := fmt.Sprintf(connectReq, u.Host, u.Host)
	_, err = con.Write([]byte(req))
	if !assert.NoError(t, err, "should write CONNECT request") {
		return
	}

	var buf [400]byte
	_, err = con.Read(buf[:])
	if !assert.Contains(t, string(buf[:]), connectResp, "should get 200 OK for CONNECT") {
		return
	}

	_, err = con.Write([]byte(tunneledReq))
	if !assert.NoError(t, err, "should write tunneled data") {
		return
	}

	_, err = con.Read(buf[:])
	assert.Contains(t, string(buf[:]), tunneledResp, "should read tunneled response")
}

func TestGET(t *testing.T) {
	log.Debugf("TestGET")
	reqTemplate := "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	resp := "HTTP/1.1 200 OK\r\n"
	content := "holla!\n"
	defer stopMockServers()
	site, _ := newMockServer(content)
	u, _ := url.Parse(site)
	log.Debugf("Started target site at %s", u)

	addr, _ := startServer(t)
	log.Debugf("Started proxy server at %s", addr)

	con, err := net.Dial("tcp", addr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		return
	}
	defer func() {
		assert.NoError(t, con.Close(), "should close connection")
	}()

	req := fmt.Sprintf(reqTemplate, u, u.Host)
	_, err = con.Write([]byte(req))
	if !assert.NoError(t, err, "should write request") {
		return
	}

	var buf [400]byte
	_, err = con.Read(buf[:])
	if !assert.Contains(t, string(buf[:]), resp, "should get 200 OK for GET") {
		return
	}
	assert.Contains(t, string(buf[:]), content, "should read content")
}

func TestAuth(t *testing.T) {
	log.Debugf("TestGET")
	reqTemplate := "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	resp := "HTTP/1.1 404 Not Found\r\n"
	content := "holla!\n"
	defer stopMockServers()
	site, _ := newMockServer(content)
	u, _ := url.Parse(site)
	log.Debugf("Started target site at %s", u)

	addr, s := startServer(t)
	s.Checker = func(req *http.Request) error {
		return errors.New("dsafda")
	}
	log.Debugf("Started proxy server at %s", addr)

	con, err := net.Dial("tcp", addr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		return
	}
	defer func() {
		assert.NoError(t, con.Close(), "should close connection")
	}()

	req := fmt.Sprintf(reqTemplate, u, u.Host)
	_, err = con.Write([]byte(req))
	if !assert.NoError(t, err, "should write request") {
		return
	}

	var buf [400]byte
	_, err = con.Read(buf[:])
	assert.Contains(t, string(buf[:]), resp, "should get 404 Not Found")
}
