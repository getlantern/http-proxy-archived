package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/getlantern/testify/assert"
)

var servers []*httptest.Server

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
	servers = append(servers, s)
	return s.URL, &m
}

func stopMockServers() {
	for _, s := range servers {
		s.Close()
	}
}

func TestCONNECT(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	tunneledReq := "GET / HTTP/1.1\r\n\r\n"
	connectResp := "HTTP/1.1 200 OK\r\n"
	tunneledResp := "holla!\n"
	defer stopMockServers()
	site, _ := newMockServer("holla!")
	u, _ := url.Parse(site)
	log.Printf("Started target site at %s\n", u)

	addr, _ := startServer(t)
	log.Printf("Started proxy server at %s\n", addr)

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
	log.Println("TestGET")
	reqTemplate := "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	resp := "HTTP/1.1 200 OK\r\n"
	content := "holla!\n"
	defer stopMockServers()
	site, _ := newMockServer(content)
	u, _ := url.Parse(site)
	log.Printf("Started target site at %s\n", u)

	addr, _ := startServer(t)
	log.Printf("Started proxy server at %s\n", addr)

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
	log.Println("TestGET")
	reqTemplate := "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n"
	resp := "HTTP/1.1 404 Not Found\r\n"
	content := "holla!\n"
	defer stopMockServers()
	site, _ := newMockServer(content)
	u, _ := url.Parse(site)
	log.Printf("Started target site at %s\n", u)

	addr, s := startServer(t)
	s.Checker = func(req *http.Request) error {
		return errors.New("dsafda")
	}
	log.Printf("Started proxy server at %s\n", addr)

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

func startServer(t *testing.T) (addr net.Addr, s *Server) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	s = &Server{
		Dial: net.Dial,
	}
	go func() {
		err := s.Serve(l)
		if err != nil {
			t.Fatalf("Unable to serve: %s", err)
		}
	}()

	return l.Addr(), s
}
