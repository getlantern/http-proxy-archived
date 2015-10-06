package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"./utils"
)

var (
	help     = flag.Bool("help", false, "Get usage help")
	keyfile  = flag.String("key", "", "Private key file name")
	certfile = flag.String("cert", "", "Certificate file name")
	https    = flag.Bool("https", false, "Use TLS for client to proxy communication")
	addr     = flag.String("addr", ":8080", "Address to listen")
	token    = flag.String("token", "", "Lantern token")
	debug    = flag.Bool("debug", false, "Produce debug output")
)

func main() {
	var err error

	_ = flag.CommandLine.Parse(os.Args[1:])
	if *help {
		flag.Usage()
		return
	}

	var logLevel utils.LogLevel
	if *debug {
		logLevel = utils.DEBUG
	} else {
		logLevel = utils.ERROR
	}
	server := NewServer(*token, logLevel)
	// Connect to Redis before initiating the server
	if err = connectRedis(); err != nil {
		fmt.Printf("Error connecting to Redis: %v,\nWARNING: NOT REPORTING TO REDIS\n", err)
		// panic(err)
	}

	// Start data collection
	utils.ScanClientsSnapshot(
		upsertRedisEntry, 2*time.Second,
	)
	if *https {
		err = server.ServeHTTPS(*addr, *keyfile, *certfile, nil)
	} else {
		err = server.ServeHTTP(*addr, nil)
	}
	if err != nil {
		fmt.Printf("Error serving: %v\n", err)
	}
}
