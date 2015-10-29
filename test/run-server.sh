#!/usr/bin/env sh
go run main.go server.go tls.go limitedlistener.go -addr=":8080" -token=111 $@
