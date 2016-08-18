// Package ratelimiter provides a mechanism for limiting the rate at which
// clients can make requests to specific domains.
package ratelimiter

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy/filters"
	"github.com/hashicorp/golang-lru"
)

var log = golog.LoggerFor("ratelimiter")

type ratelimiter struct {
	hostPeriods          map[string]time.Duration
	hostAccessesByClient *lru.Cache
	mx                   sync.Mutex
}

// New creates a new rate limiting filter that only allows access to the hosts
// listed in the given hostPeriods, and limits the periodicity of requests to
// each host to the given duration. It limits the number of clients tracked to
// the the numClients with the most recent activity.
func New(numClients int, hostPeriods map[string]time.Duration) filters.Filter {
	if numClients <= 0 {
		numClients = 5000
	}
	// We can safely ignore the error, since the only thing that would cause an
	// error is numClients <= 0
	cache, _ := lru.New(numClients)
	return &ratelimiter{
		hostPeriods:          hostPeriods,
		hostAccessesByClient: cache,
	}
}

func (f *ratelimiter) Apply(resp http.ResponseWriter, req *http.Request, next filters.Next) error {
	err := f.verifyAllowed(resp, req, next)
	if err != nil {
		return fail(resp, err)
	}
	return next()
}

func (f *ratelimiter) verifyAllowed(resp http.ResponseWriter, req *http.Request, next filters.Next) error {
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	client, _, _ := net.SplitHostPort(req.RemoteAddr)
	now := time.Now()

	f.mx.Lock()
	defer f.mx.Unlock()
	period := f.hostPeriods[host]
	if period == 0 {
		return fmt.Errorf("Access to %v not allowed", host)
	}
	var hostAccesses map[string]time.Time
	_hostAccesses, found := f.hostAccessesByClient.Get(client)
	if found {
		hostAccesses = _hostAccesses.(map[string]time.Time)
	} else {
		hostAccesses = make(map[string]time.Time)
	}
	allowed := now.Sub(hostAccesses[host]) > period
	if allowed {
		hostAccesses[host] = now
		f.hostAccessesByClient.Add(client, hostAccesses)
	}
	if !allowed {
		return fmt.Errorf("Rate limit for %v exceeded", host)
	}

	return nil
}

func fail(resp http.ResponseWriter, err error) error {
	resp.WriteHeader(http.StatusForbidden)
	fmt.Fprint(resp, err)
	return filters.Stop()
}
