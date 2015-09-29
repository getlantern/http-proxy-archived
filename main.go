package main

import (
	"flag"
	"net"
	"net/http"
	"os"

	"./connectforward"
	"./lanternpro"
	"./tokenfilter"
)

var (
	help    = flag.Bool("help", false, "Get usage help")
	keyfile = flag.String("keyfile", "", "the cert key file name")
	https   = flag.Bool("https", false, "listen on https")
	addr    = flag.String("addr", ":8080", "the address to listen")
	token   = flag.String("token", "", "Lantern token")

	logTimestampFormat = "Jan 02 15:04:05.000"
)

func main() {
	_ = flag.CommandLine.Parse(os.Args[1:])
	if *help {
		flag.Usage()
		return
	}

	// The following middleware is run from last to first:
	var handler http.Handler

	// Handles CONNECT and direct proxying requests
	connectFwd, _ := connectforward.New()
	// Handles Lantern Pro users
	lanternPro, _ := lanternpro.New(connectFwd)
	if *token != "" {
		// Bounces back requests without the proper token
		tokenFilter, _ := tokenfilter.New(lanternPro, *token)
		handler = tokenFilter
	} else {
		handler = lanternPro
	}

	var l net.Listener
	var err error
	if *https {
		panic("TLS not implemted")
		l, err = listenTLS()
	} else {
		l, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		panic(err)
	}

	proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handler.ServeHTTP(w, req)
	})

	http.Serve(l, proxy)
}
