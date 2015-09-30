package main

import (
	"flag"
	"net"
	"net/http"
	"os"
	"time"

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
	debug   = flag.Bool("debug", false, "Produce debug output")
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
		panic("TLS support not implemented")
		l, err = listenTLS(*addr)
	} else {
		l, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		panic(err)
	}

	// Data gathering
	if err = connectRedis(); err != nil {
		panic(err)
	}
	lanternPro.ScanClientsSnapshot(upsertRedisEntry, time.Second)

	// Set up server
	proxy := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handler.ServeHTTP(w, req)
	})

	http.Serve(l, proxy)
}
