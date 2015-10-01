package main

import (
	"crypto/tls"
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

	"github.com/getlantern/keyman"
	"github.com/getlantern/testify/assert"
)

const (
	clientUID      = "1234-1234-1234-1234-1234-1234"
	validToken     = "6o0dToK3n"
	tunneledReq    = "GET / HTTP/1.1\r\n\r\n"
	targetResponse = "Fight for a Free Internet!"
)

var (
	targetURL    *url.URL
	proxyAddr    net.Addr
	tlsProxyAddr net.Addr

	serverCertificate *keyman.Certificate
	// TODO: this should be imported from tlsdefaults package, but is not being
	// exported there.
	preferredCipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	}
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Set up mock target servers
	defer stopMockServers()
	site, _ := newMockServer(targetResponse)
	targetURL, _ = url.Parse(site)
	log.Printf("Started target site at %s\n", targetURL)

	// Set up HTTP chained server
	httpServer, err := setUpNewHTTPServer()
	if err != nil {
		log.Println("Error starting proxy server")
		os.Exit(1)
	}
	proxyAddr = httpServer.listener.Addr()
	log.Printf("Started proxy server at %s\n", proxyAddr.String())

	// Set up HTTPS chained server
	tlsServer, err := setUpNewHTTPSServer()
	if err != nil {
		log.Println("Error starting proxy server")
		os.Exit(1)
	}
	tlsProxyAddr = tlsServer.listener.Addr()
	log.Printf("Started proxy server at %s\n", tlsProxyAddr.String())

	os.Exit(m.Run())
}

// No X-Lantern-Auth-Token -> 404
func TestConnectNoToken(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-UID: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	proxies := []struct {
		protocol string
		addr     string
	}{
		{"http", proxyAddr.String()},
		{"https", tlsProxyAddr.String()},
	}
	for _, proxy := range proxies {
		var conn net.Conn
		var err error
		if proxy.protocol == "http" {
			conn, err = net.Dial("tcp", proxy.addr)
		} else if proxy.protocol == "https" {
			x509cert := serverCertificate.X509()
			tlsConn, err := tls.Dial("tcp", proxy.addr, &tls.Config{
				CipherSuites:       preferredCipherSuites,
				InsecureSkipVerify: true,
			})
			if !assert.NoError(t, err, "should dial proxy server") {
				t.FailNow()
			}
			conn = tlsConn
			if !tlsConn.ConnectionState().PeerCertificates[0].Equal(x509cert) {
				if err := tlsConn.Close(); err != nil {
					log.Printf("Error closing chained server connection: %s\n", err)
				}
				t.Fatal("Server's certificate didn't match expected")
			}

		} else {
			t.Fatal("Unknown protocol")
		}

		defer func() {
			assert.NoError(t, conn.Close(), "should close connection")
		}()

		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, clientUID)
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
}

// Bad X-Lantern-Auth-Token -> 404
func TestConnectBadToken(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\nX-Lantern-UID: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	conn, err := net.Dial("tcp", proxyAddr.String())
	if !assert.NoError(t, err, "should dial proxy server") {
		t.FailNow()
	}
	defer func() {
		assert.NoError(t, conn.Close(), "should close connection")
	}()

	req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, "B4dT0k3n", clientUID)
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

// No X-Lantern-UID -> 404
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

// X-Lantern-Auth-Token + X-Lantern-UID -> 200 OK <- Tunneled request -> 200 OK
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

	req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, validToken, clientUID)
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

	_, err = conn.Write([]byte(tunneledReq))
	if !assert.NoError(t, err, "should write tunneled data") {
		t.FailNow()
	}

	_, err = conn.Read(buf[:])
	assert.Contains(t, string(buf[:]), targetResponse, "should read tunneled response")
}

func setUpNewHTTPServer() (*Server, error) {
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

func setUpNewHTTPSServer() (*Server, error) {
	s := NewServer(validToken)
	var err error
	ready := make(chan bool)
	go func(err *error) {
		if *err = s.ServeHTTPS("localhost:0", "key.pem", "cert.pem", &ready); err != nil {
			fmt.Println("Unable to serve: %s", err)
		}
	}(&err)
	<-ready
	if err != nil {
		return nil, err
	}
	serverCertificate, err = keyman.LoadCertificateFromFile("cert.pem")
	return s, err
}

//
// Mock server
// Emulating locally a target site for testing tunnels
//

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
