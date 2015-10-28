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
	"github.com/getlantern/measured"
	"github.com/getlantern/testify/assert"

	"./utils"
)

const (
	deviceId       = "1234-1234-1234-1234-1234-1234"
	validToken     = "6o0dToK3n"
	tunneledReq    = "GET / HTTP/1.1\r\n\r\n"
	targetResponse = "Fight for a Free Internet!"
)

var (
	httpProxy        *Server
	tlsProxy         *Server
	httpTargetServer *targetHandler
	httpTargetURL    string
	tlsTargetServer  *targetHandler
	tlsTargetURL     string

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
	var err error

	// Set up mock target servers
	httpTargetURL, httpTargetServer = newTargetHandler(targetResponse, false)
	defer httpTargetServer.Close()
	tlsTargetURL, tlsTargetServer = newTargetHandler(targetResponse, true)
	defer tlsTargetServer.Close()

	// Set up HTTP chained server
	httpProxy, err = setupNewHTTPServer(0, 30*time.Second)
	if err != nil {
		log.Println("Error starting proxy server")
		os.Exit(1)
	}
	log.Printf("Started HTTP proxy server at %s\n", httpProxy.listener.Addr().String())

	// Set up HTTPS chained server
	tlsProxy, err = setupNewHTTPSServer(0, 30*time.Second)
	if err != nil {
		log.Println("Error starting proxy server")
		os.Exit(1)
	}
	log.Printf("Started HTTPS proxy server at %s\n", tlsProxy.listener.Addr().String())

	os.Exit(m.Run())
}

// Keep this one first to avoid measuring previous connections
func TestReportStats(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"
	m := mockReporter{error: make(map[measured.Error]int)}
	measured.Start(100*time.Millisecond, &m)
	defer measured.Stop()
	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, deviceId)
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, len(m.error))
	assert.Equal(t, 1, len(m.traffic))
	t.Logf("%+v", m.error)
	t.Logf("%+v", m.traffic[0])
}

func TestMaxConnections(t *testing.T) {
	limitedServer, err := setupNewHTTPServer(5, 30*time.Second)
	if err != nil {
		log.Println("Error starting proxy server")
		t.FailNow()
	}

	//limitedServer.httpServer.SetKeepAlivesEnabled(false)
	okFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		var buf [400]byte
		_, err = conn.Read(buf[:])

		assert.Nil(t, err)

		time.Sleep(time.Millisecond * 100)
	}

	waitFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		var buf [400]byte
		_, err = conn.Read(buf[:])

		assert.NotNil(t, err)
	}

	for i := 0; i < 5; i++ {
		go testRoundTrip(t, limitedServer, httpTargetServer, okFn)
	}

	time.Sleep(time.Millisecond * 100)

	for i := 0; i < 5; i++ {
		go testRoundTrip(t, limitedServer, httpTargetServer, waitFn)
	}

	time.Sleep(time.Millisecond * 100)

	for i := 0; i < 5; i++ {
		go testRoundTrip(t, limitedServer, httpTargetServer, okFn)
	}
}

func TestIdleClientConnections(t *testing.T) {
	limitedServer, err := setupNewHTTPServer(0, 100*time.Millisecond)
	if err != nil {
		log.Println("Error starting proxy server")
		t.FailNow()
	}

	okFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		time.Sleep(time.Millisecond * 90)
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))

		var buf [400]byte
		_, err := conn.Read(buf[:])

		assert.Nil(t, err)
	}

	idleFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		time.Sleep(time.Millisecond * 110)
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))

		var buf [400]byte
		_, err := conn.Read(buf[:])

		assert.NotNil(t, err)
	}

	go testRoundTrip(t, limitedServer, httpTargetServer, okFn)
	testRoundTrip(t, limitedServer, httpTargetServer, idleFn)
}

// TODO: Since both client and target server idle timeouts are identical,
// we are just testing the combined behavior.  We probably can do that by
// creating a custom server that only sets one timeout at a time
func TestIdleTargetConnections(t *testing.T) {
	normalServer, err := setupNewHTTPServer(0, 30*time.Second)
	if err != nil {
		log.Println("Error starting proxy server")
		t.FailNow()
	}

	impatientServer, err := setupNewHTTPServer(0, 100*time.Millisecond)
	if err != nil {
		log.Println("Error starting proxy server")
		t.FailNow()
	}

	okForwardFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		var buf [400]byte
		_, err := conn.Read(buf[:])

		assert.Nil(t, err)
	}

	okConnectFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		conn.Write([]byte("CONNECT www.google.com HTTP/1.1\r\nHost: www.google.com\r\n\r\n"))
		var buf [400]byte
		_, err := conn.Read(buf[:])

		assert.Nil(t, err)
	}

	failForwardFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		var buf [400]byte
		conn.Read(buf[:])

		time.Sleep(150 * time.Millisecond)
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		_, err := conn.Read(buf[:])

		assert.NotNil(t, err)
	}

	failConnectFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		conn.Write([]byte("CONNECT www.google.com HTTP/1.1\r\nHost: www.google.com\r\n\r\n"))
		var buf [400]byte
		conn.Read(buf[:])

		time.Sleep(150 * time.Millisecond)
		conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		_, err := conn.Read(buf[:])

		assert.NotNil(t, err)
	}

	testRoundTrip(t, normalServer, httpTargetServer, okForwardFn)
	testRoundTrip(t, normalServer, httpTargetServer, okConnectFn)
	testRoundTrip(t, impatientServer, httpTargetServer, failForwardFn)
	testRoundTrip(t, impatientServer, httpTargetServer, failConnectFn)
}

// No X-Lantern-Auth-Token -> 404
func TestConnectNoToken(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, deviceId)
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)

	testRoundTrip(t, httpProxy, tlsTargetServer, testFn)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFn)
}

// Bad X-Lantern-Auth-Token -> 404
func TestConnectBadToken(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, "B4dT0k3n", deviceId)
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)

	testRoundTrip(t, httpProxy, tlsTargetServer, testFn)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFn)
}

// No X-Lantern-Device-Id -> 404
func TestConnectNoDevice(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)

	testRoundTrip(t, httpProxy, tlsTargetServer, testFn)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFn)
}

// X-Lantern-Auth-Token + X-Lantern-Device-Id -> 200 OK <- Tunneled request -> 200 OK
func TestConnectOK(t *testing.T) {
	connectReq := "CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	connectResp := "HTTP/1.1 200 OK\r\n"

	testHTTP := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, validToken, deviceId)
		t.Log("\n" + req)
		_, err := conn.Write([]byte(req))
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

		buf = [400]byte{}
		_, err = conn.Read(buf[:])
		assert.Contains(t, string(buf[:]), targetResponse, "should read tunneled response")
	}

	testTLS := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, validToken, deviceId)
		t.Log("\n" + req)
		_, err := conn.Write([]byte(req))
		if !assert.NoError(t, err, "should write CONNECT request") {
			t.FailNow()
		}

		var buf [400]byte
		_, err = conn.Read(buf[:])
		if !assert.Contains(t, string(buf[:]), connectResp,
			"should get 200 OK") {
			t.FailNow()
		}

		// HTTPS-Tunneled HTTPS
		tunnConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
		})
		tunnConn.Handshake()

		_, err = tunnConn.Write([]byte(tunneledReq))
		if !assert.NoError(t, err, "should write tunneled data") {
			t.FailNow()
		}

		buf = [400]byte{}
		_, err = tunnConn.Read(buf[:])
		assert.Contains(t, string(buf[:]), targetResponse, "should read tunneled response")
	}

	testRoundTrip(t, httpProxy, httpTargetServer, testHTTP)
	testRoundTrip(t, tlsProxy, httpTargetServer, testHTTP)

	testRoundTrip(t, httpProxy, tlsTargetServer, testTLS)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testTLS)
}

// No X-Lantern-Auth-Token -> 404
func TestDirectNoToken(t *testing.T) {
	connectReq := "GET /%s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, deviceId)
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)

	testRoundTrip(t, httpProxy, tlsTargetServer, testFn)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFn)
}

// Bad X-Lantern-Auth-Token -> 404
func TestDirectBadToken(t *testing.T) {
	connectReq := "GET /%s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
		req := fmt.Sprintf(connectReq, targetURL.Host, targetURL.Host, "B4dT0k3n", deviceId)
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)

	testRoundTrip(t, httpProxy, tlsTargetServer, testFn)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFn)
}

// No X-Lantern-Device-Id -> 404
func TestDirectNoDevice(t *testing.T) {
	connectReq := "GET /%s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\n\r\n"
	connectResp := "HTTP/1.1 404 Not Found\r\n"

	testFn := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		var err error
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

	testRoundTrip(t, httpProxy, httpTargetServer, testFn)
	testRoundTrip(t, tlsProxy, httpTargetServer, testFn)

	testRoundTrip(t, httpProxy, tlsTargetServer, testFn)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFn)
}

// X-Lantern-Auth-Token + X-Lantern-Device-Id -> Forward
func TestDirectOK(t *testing.T) {
	reqTempl := "GET /%s HTTP/1.1\r\nHost: %s\r\nX-Lantern-Auth-Token: %s\r\nX-Lantern-Device-Id: %s\r\n\r\n"
	failResp := "HTTP/1.1 500 Internal Server Error\r\n"

	testOk := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		req := fmt.Sprintf(reqTempl, targetURL.Path, targetURL.Host, validToken, deviceId)
		t.Log("\n" + req)
		_, err := conn.Write([]byte(req))
		if !assert.NoError(t, err, "should write GET request") {
			t.FailNow()
		}

		buf := [400]byte{}
		_, err = conn.Read(buf[:])
		assert.Contains(t, string(buf[:]), targetResponse, "should read tunneled response")

	}

	testFail := func(conn net.Conn, proxy *Server, targetURL *url.URL) {
		req := fmt.Sprintf(reqTempl, targetURL.Path, targetURL.Host, validToken, deviceId)
		t.Log("\n" + req)
		_, err := conn.Write([]byte(req))
		if !assert.NoError(t, err, "should write GET request") {
			t.FailNow()
		}

		buf := [400]byte{}
		_, err = conn.Read(buf[:])
		t.Log("\n" + string(buf[:]))

		assert.Contains(t, string(buf[:]), failResp, "should respond with 500 Internal Server Error")

	}

	testRoundTrip(t, httpProxy, httpTargetServer, testOk)
	testRoundTrip(t, tlsProxy, httpTargetServer, testOk)

	// HTTPS can't be tunneled using Direct Proxying, as redirections
	// require a TLS handshake between the proxy and the target
	testRoundTrip(t, httpProxy, tlsTargetServer, testFail)
	testRoundTrip(t, tlsProxy, tlsTargetServer, testFail)
}

//
// Auxiliary functions
//

func testRoundTrip(t *testing.T, proxy *Server, target *targetHandler, checkerFn func(conn net.Conn, proxy *Server, targetURL *url.URL)) {
	var conn net.Conn
	var err error

	addr := proxy.listener.Addr().String()
	if !proxy.tls {
		conn, err = net.Dial("tcp", addr)
		fmt.Printf("%s -> %s (via HTTP) -> %s\n", conn.LocalAddr().String(), addr, target.server.URL)
		if !assert.NoError(t, err, "should dial proxy server") {
			t.FailNow()
		}
	} else {
		var tlsConn *tls.Conn
		x509cert := serverCertificate.X509()
		tlsConn, err = tls.Dial("tcp", addr, &tls.Config{
			CipherSuites:       preferredCipherSuites,
			InsecureSkipVerify: true,
		})
		fmt.Printf("%s -> %s (via HTTPS) -> %s\n", tlsConn.LocalAddr().String(), addr, target.server.URL)
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
	}
	defer func() {
		assert.NoError(t, conn.Close(), "should close connection")
	}()

	url, _ := url.Parse(target.server.URL)
	checkerFn(conn, proxy, url)
}

//
// Proxy server
//

type proxy struct {
	protocol string
	addr     string
}

func setupNewHTTPServer(maxConns uint64, idleTimeout time.Duration) (*Server, error) {
	s := NewServer(validToken, maxConns, idleTimeout, false, utils.QUIET)
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

func setupNewHTTPSServer(maxConns uint64, idleTimeout time.Duration) (*Server, error) {
	s := NewServer(validToken, maxConns, idleTimeout, false, utils.QUIET)
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
// Mock target server
// Emulating locally a target site for testing tunnels
//

type targetHandler struct {
	writer func(w http.ResponseWriter)
	server *httptest.Server
}

func (m *targetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.writer(w)
}

func (m *targetHandler) Raw(msg string) {
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

func (m *targetHandler) Msg(msg string) {
	m.writer = func(w http.ResponseWriter) {
		w.Header()["Content-Length"] = []string{strconv.Itoa(len(msg))}
		_, _ = w.Write([]byte(msg))
		w.(http.Flusher).Flush()
	}
}

func (m *targetHandler) Timeout(d time.Duration, msg string) {
	m.writer = func(w http.ResponseWriter) {
		time.Sleep(d)
		w.Header()["Content-Length"] = []string{strconv.Itoa(len(msg))}
		_, _ = w.Write([]byte(msg))
		w.(http.Flusher).Flush()
	}
}

func (m *targetHandler) Close() {
	m.Close()
}

func newTargetHandler(msg string, tls bool) (string, *targetHandler) {
	m := targetHandler{}
	m.Msg(msg)
	if tls {
		m.server = httptest.NewTLSServer(&m)
	} else {
		m.server = httptest.NewServer(&m)
	}
	log.Printf("Started target site at %v\n", m.server.URL)
	return m.server.URL, &m
}

//
//
// Mock Redis reporter
//

type mockReporter struct {
	error   map[measured.Error]int
	latency []*measured.LatencyTracker
	traffic []*measured.TrafficTracker
}

func (nr *mockReporter) ReportError(e map[*measured.Error]int) error {
	for k, v := range e {
		nr.error[*k] = nr.error[*k] + v
	}
	return nil
}

func (nr *mockReporter) ReportLatency(l []*measured.LatencyTracker) error {
	nr.latency = append(nr.latency, l...)
	return nil
}

func (nr *mockReporter) ReportTraffic(t []*measured.TrafficTracker) error {
	nr.traffic = append(nr.traffic, t...)
	return nil
}
