package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/getlantern/keyman"
	"github.com/getlantern/tlsdefaults"
)

var (
	tenYearsFromToday = time.Now().AddDate(10, 0, 0)
	processStart      = time.Now()
)

func listenTLS(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("Unable to split host and port for %v: %v\n", addr, err)
	}
	ctx := CertContext{
		PKFile:         "key.pem",
		ServerCertFile: "cert.pem",
	}
	err = ctx.InitServerCert(host)
	if err != nil {
		return nil, fmt.Errorf("Unable to init server cert: %s\n", err)
	}

	tlsConfig := tlsdefaults.Server()
	cert, err := tls.LoadX509KeyPair(ctx.ServerCertFile, ctx.PKFile)
	if err != nil {
		return nil, fmt.Errorf("Unable to load certificate and key from %s and %s: %s\n", ctx.ServerCertFile, ctx.PKFile, err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to listen for tls connections at %s: %s\n", addr, err)
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
			fmt.Printf("Creating new PK at: %s\n", ctx.PKFile)
			if ctx.PK, err = keyman.GeneratePK(2048); err != nil {
				return
			}
			if err = ctx.PK.WriteToFile(ctx.PKFile); err != nil {
				return fmt.Errorf("Unable to save private key: %s\n", err)
			}
		} else {
			return fmt.Errorf("Unable to read private key, even though it exists: %s\n", err)
		}
	}

	fmt.Printf("Creating new server cert at: %s\n", ctx.ServerCertFile)
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
