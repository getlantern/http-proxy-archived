#!/usr/bin/env sh

go run main.go server.go redis.go tls.go -addr=":1443" -https -token=111
