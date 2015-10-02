#!/usr/bin/env sh

go run main.go server.go redis.go tls.go -debug -https -token=111
