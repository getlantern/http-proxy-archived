package main

import (
	"flag"
	"os"
	"time"

	"github.com/getlantern/golog"

	"github.com/getlantern/http-proxy/commonfilter"
	"github.com/getlantern/http-proxy/forward"
	"github.com/getlantern/http-proxy/httpconnect"
	"github.com/getlantern/http-proxy/logging"
	"github.com/getlantern/http-proxy/server"
)

var (
	testingLocal = false
	log          = golog.LoggerFor("main")

	help      = flag.Bool("help", false, "Get usage help")
	keyfile   = flag.String("key", "", "Private key file name")
	certfile  = flag.String("cert", "", "Certificate file name")
	https     = flag.Bool("https", false, "Use TLS for client to proxy communication")
	addr      = flag.String("addr", ":8080", "Address to listen")
	maxConns  = flag.Uint64("maxconns", 0, "Max number of simultaneous connections allowed connections")
	idleClose = flag.Uint64("idleclose", 30, "Time in seconds that an idle connection will be allowed before closing it")
)

func main() {
	var err error

	_ = flag.CommandLine.Parse(os.Args[1:])
	if *help {
		flag.Usage()
		return
	}

	// TODO: use real parameters
	err = logging.Init("instanceid", "version", "releasedate", "")
	if err != nil {
		log.Error(err)
	}

	forwarder, err := forward.New(nil, forward.IdleTimeoutSetter(time.Duration(*idleClose)*time.Second))
	if err != nil {
		log.Error(err)
	}

	httpConnect, err := httpconnect.New(forwarder, httpconnect.IdleTimeoutSetter(time.Duration(*idleClose)*time.Second))
	if err != nil {
		log.Error(err)
	}

	commonHandler, err := commonfilter.New(httpConnect, testingLocal)
	if err != nil {
		log.Error(err)
	}

	srv := server.NewServer(commonHandler)

	if *https {
		err = srv.ServeHTTPS(*addr, *keyfile, *certfile, nil)
	} else {
		err = srv.ServeHTTP(*addr, nil)
	}
	if err != nil {
		log.Errorf("Error serving: %v", err)
	}
}
