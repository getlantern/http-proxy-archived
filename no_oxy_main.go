package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/keyman"
	"github.com/getlantern/tlsdefaults"
)

var (
	help    = flag.Bool("help", false, "Get usage help")
	keyfile = flag.String("keyfile", "", "the cert key file name")
	addr    = flag.String("addr", ":8080", "the address to listen")
	https   = flag.Bool("https", false, "listen on https")

	// Points in time, mostly used for generating certificates
	tenYearsFromToday  = time.Now().AddDate(10, 0, 0)
	processStart       = time.Now()
	logTimestampFormat = "Jan 02 15:04:05.000"

	log = golog.LoggerFor("server")
)

func main() {
	_ = flag.CommandLine.Parse(os.Args[1:])
	if *help {
		flag.Usage()
		return
	}
	golog.SetOutputs(&timestamped{os.Stderr}, &timestamped{os.Stdout})

	var l net.Listener
	var err error
	if *https {
		l, err = listenTLS()
	} else {
		l, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Debugf("Listen at %s", *addr)

	server := Server{Dial: net.Dial, Checker: checkAuthToken}
	server.Serve(l)
}

func checkAuthToken(req *http.Request) (err error) {
	if req.Header.Get("X-Lantern-Auth-Token") == "" {
		err = errors.New("No X-Lantern-Auth-Token provided")
	}
	return
}

func listenTLS() (net.Listener, error) {
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return nil, fmt.Errorf("Unable to split host and port for %v: %v", *addr, err)
	}
	ctx := CertContext{
		PKFile:         "key.pem",
		ServerCertFile: "cert.pem",
	}
	err = ctx.InitServerCert(host)
	if err != nil {
		return nil, fmt.Errorf("Unable to init server cert: %s", err)
	}

	tlsConfig := tlsdefaults.Server()
	cert, err := tls.LoadX509KeyPair(ctx.ServerCertFile, ctx.PKFile)
	if err != nil {
		return nil, fmt.Errorf("Unable to load certificate and key from %s and %s: %s", ctx.ServerCertFile, ctx.PKFile, err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	listener, err := tls.Listen("tcp", *addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to listen for tls connections at %s: %s", *addr, err)
	}

	return listener, err
}

// CertContext encapsulates the certificates used by a Server
type CertContext struct {
	PKFile         string
	ServerCertFile string
	PK             *keyman.PrivateKey
	ServerCert     *keyman.Certificate
}

// InitServerCert initializes a PK + cert for use by a server proxy, signed by
// the CA certificate.  We always generate a new certificate just in case.
func (ctx *CertContext) InitServerCert(host string) (err error) {
	if ctx.PK, err = keyman.LoadPKFromFile(ctx.PKFile); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("Creating new PK at: %s", ctx.PKFile)
			if ctx.PK, err = keyman.GeneratePK(2048); err != nil {
				return
			}
			if err = ctx.PK.WriteToFile(ctx.PKFile); err != nil {
				return fmt.Errorf("Unable to save private key: %s", err)
			}
		} else {
			return fmt.Errorf("Unable to read private key, even though it exists: %s", err)
		}
	}

	log.Debugf("Creating new server cert at: %s", ctx.ServerCertFile)
	ctx.ServerCert, err = ctx.PK.TLSCertificateFor("Lantern", host, tenYearsFromToday, true, nil)
	if err != nil {
		return
	}
	err = ctx.ServerCert.WriteToFile(ctx.ServerCertFile)
	if err != nil {
		return
	}
	return nil
}

type timestamped struct {
	w io.Writer
}

// timestamped adds a timestamp to the beginning of log lines
func (t *timestamped) Write(buf []byte) (n int, err error) {
	ts := time.Now()
	runningSecs := ts.Sub(processStart).Seconds()
	secs := int(math.Mod(runningSecs, 60))
	mins := int(runningSecs / 60)
	return fmt.Fprintf(t.w, "%s - %dm%ds %s", ts.In(time.UTC).Format(logTimestampFormat), mins, secs, buf)
}
