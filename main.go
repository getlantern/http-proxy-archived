package main

import (
	"flag"
	"fmt"
	"os"
	"time"
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
	var err error

	_ = flag.CommandLine.Parse(os.Args[1:])
	if *help {
		flag.Usage()
		return
	}

	server := NewServer(*token)
	// Connect to Redis before initiating the server
	if err = connectRedis(); err != nil {
		fmt.Printf("Error connecting to Redis: %v,\nWARNING: NOT REPORTING TO REDIS\n", err)
		// panic(err)
	}

	// Start data collection
	server.lanternProComponent.ScanClientsSnapshot(
		upsertRedisEntry, time.Second,
	)
	if *https {
		err = server.ServeHTTPS(*addr, nil)
	} else {
		err = server.ServeHTTP(*addr, nil)
	}
	if err != nil {
		fmt.Printf("Error serving: %v\n", err)
	}
}
