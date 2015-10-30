#!/usr/bin/env sh
go run main.go server.go tls.go limitedlistener.go -token=111 $@
