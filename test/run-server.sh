#!/usr/bin/env sh
PROXY_DIR="src/github.com/getlantern/http-proxy"
go run $PROXY_DIR/http_proxy.go $PROXY_DIR/server.go $PROXY_DIR/tls.go $PROXY_DIR/limitedlistener.go -addr=":8080" -token=111 $@
