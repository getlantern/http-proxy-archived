package main

import (
	"flag"
	"os"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/measured"

	"github.com/getlantern/http-proxy/logging"
	"github.com/getlantern/http-proxy/utils"
)

var (
	help          = flag.Bool("help", false, "Get usage help")
	keyfile       = flag.String("key", "", "Private key file name")
	certfile      = flag.String("cert", "", "Certificate file name")
	https         = flag.Bool("https", false, "Use TLS for client to proxy communication")
	addr          = flag.String("addr", ":8080", "Address to listen")
	maxConns      = flag.Uint64("maxconns", 0, "Max number of simultaneous connections allowed connections")
	idleClose     = flag.Uint64("idleclose", 30, "Time in seconds that an idle connection will be allowed before closing it")
	token         = flag.String("token", "", "Lantern token")
	enableFilters = flag.Bool("enablefilters", false, "Enable Lantern-specific filters")
	enableReports = flag.Bool("enablereports", false, "Enable stats reporting")
	debug         = flag.Bool("debug", false, "Produce debug output")
	logglyToken   = flag.String("logglytoken", "", "Token used to report to loggly.com, not reporting if empty")

	log = golog.LoggerFor("main")
)

func main() {
	var err error

	_ = flag.CommandLine.Parse(os.Args[1:])
	if *help {
		flag.Usage()
		return
	}
	// TODO: use real parameters
	err = logging.Init("instanceid", "version", "releasedate", *logglyToken)
	if err != nil {
		log.Error(err)
	}

	if *enableReports {
		redisAddr := os.Getenv("REDIS_PRODUCTION_URL")
		if redisAddr == "" {
			redisAddr = "127.0.0.1:6379"
		}
		rp, err := utils.NewRedisReporter(redisAddr)
		if err != nil {
			log.Errorf("Error connecting to redis: %v", err)
		} else {
			measured.Start(20*time.Second, rp)
			defer measured.Stop()
		}
	}

	server := NewServer(
		*token,
		*maxConns,
		time.Duration(*idleClose)*time.Second,
		*enableFilters,
	)
	if *https {
		err = server.ServeHTTPS(*addr, *keyfile, *certfile, nil)
	} else {
		err = server.ServeHTTP(*addr, nil)
	}
	if err != nil {
		log.Errorf("Error serving: %v", err)
	}
}
