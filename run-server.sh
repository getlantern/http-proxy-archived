#!/usr/bin/env sh

go run main.go server.go tls.go -addr=":1443" -https -token=111
