#!/usr/bin/env sh

go run main.go server.go redis.go log.go tls.go -debug -https -token=111
