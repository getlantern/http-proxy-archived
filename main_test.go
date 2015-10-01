package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/getlantern/testify/assert"
)

const (
	validToken = "mytoken"
)

var (
	targetURL *url.URL
	proxyAddr net.Addr
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Set up mock target servers
	defer stopMockServers()
	site, _ := newMockServer("holla!")
	targetURL, _ = url.Parse(site)
	log.Printf("Started target site at %s\n", targetURL)

	// Set up chained server
	s, err := setUpNewServer()
	if err != nil {
		log.Println("Error starting proxy server")
		os.Exit(1)
	}
	proxyAddr = s.listener.Addr()
	log.Printf("Started proxy server at %s\n", proxyAddr.String())

	os.Exit(m.Run())
}

func TestConnectNoToken(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	conn, err := net.Dial("tcp", proxyAddr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		t.FailNow()
	}
	defer func() {
		assert.NoError(t, conn.Close(), "should close connection")
	}()

	req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host)
	t.Log("\n" + req)
	_, err = conn.Write([]byte(req))
	if !assert.NoError(t, err, "should write CONNECT request") {
		t.FailNow()
	}

	var buf [400]byte
	_, err = conn.Read(buf[:])
	if !assert.Contains(t, string(buf[:]), connectResp,
		"should get 404 Not Found because no token was provided") {
		t.FailNow()
	}
}

func TestConnectNoUID(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	conn, err := net.Dial("tcp", proxyAddr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		t.FailNow()
	}
	defer func() {
		assert.NoError(t, conn.Close(), "should close connection")
	}()

	req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, validToken)
	t.Log("\n" + req)
	_, err = conn.Write([]byte(req))
	if !assert.NoError(t, err, "should write CONNECT request") {
		t.FailNow()
	}

	var buf [400]byte
	_, err = conn.Read(buf[:])
	if !assert.Contains(t, string(buf[:]), connectResp,
		"should get 404 Not Found because no token was provided") {
		t.FailNow()
	}
}

func TestConnectOK(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\nX-Lantern-UID: %s\r\n\r\n"
	connectResp := "HTTP/1.1 200 OK\r\n"

	conn, err := net.Dial("tcp", proxyAddr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		t.FailNow()
	}
	defer func() {
		assert.NoError(t, conn.Close(), "should close connection")
	}()

	req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, validToken, "1234-1234-1234-1243-1234-1234")
	t.Log("\n" + req)
	_, err = conn.Write([]byte(req))
	if !assert.NoError(t, err, "should write CONNECT request") {
		t.FailNow()
	}

	var buf [400]byte
	_, err = conn.Read(buf[:])
	if !assert.Contains(t, string(buf[:]), connectResp,
		"should get 200 OK") {
		t.FailNow()
	}
}

func setUpNewServer() (*Server, error) {
	s := NewServer(validToken)
	var err error
	ready := make(chan bool)
	go func(err *error) {
		if *err = s.ServeHTTP("localhost:0", &ready); err != nil {
			fmt.Println("Unable to serve: %s", err)
		}
	}(&err)
	<-ready
	return s, err
}

// Mock servers are useful for emulating locally a target site for testing tunnels

var mockServers []*httptest.Server

type mockHandler struct {
	writer func(w http.ResponseWriter)
}

func (m *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.writer(w)
}

func (m *mockHandler) Raw(msg string) {
	m.writer = func(w http.ResponseWriter) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		if _, err := conn.Write([]byte(msg)); err != nil {
			log.Printf("Unable to write to connection: %v\n", err)
		}
		if err := conn.Close(); err != nil {
			log.Printf("Unable to close connection: %v\n", err)
		}
	}
}

func (m *mockHandler) Msg(msg string) {
	m.writer = func(w http.ResponseWriter) {
		w.Header()["Content-Length"] = []string{strconv.Itoa(len(msg))}
		_, _ = w.Write([]byte(msg))
		w.(http.Flusher).Flush()
	}
}

func (m *mockHandler) Timeout(d time.Duration, msg string) {
	m.writer = func(w http.ResponseWriter) {
		time.Sleep(d)
		w.Header()["Content-Length"] = []string{strconv.Itoa(len(msg))}
		_, _ = w.Write([]byte(msg))
		w.(http.Flusher).Flush()
	}
}

func newMockServer(msg string) (string, *mockHandler) {
	m := mockHandler{nil}
	m.Msg(msg)
	s := httptest.NewServer(&m)
	mockServers = append(mockServers, s)
	return s.URL, &m
}

func stopMockServers() {
	for _, s := range mockServers {
		s.Close()
	}
}
