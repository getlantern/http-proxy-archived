package ratelimiter

import (
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

func (f *ratelimiter) Apply(w http.ResponseWriter, req *http.Request, next filters.Next) error {
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	f.mx.Lock()
	period := f.hostPeriods[host]
	if period == 0 {
		f.mx.Unlock()
		return filters.Fail("Access to %v not allowed", host)
	}
	clientAddr := req.RemoteAddr
	var hostAccesses map[string]time.Time
	_hostAccesses, found := f.hostAccessesByClient.Get(clientAddr)
	if found {
		hostAccesses = _hostAccesses.(map[string]time.Time)
	} else {
		hostAccesses = make(map[string]time.Time)
	}
	now := time.Now()
	allowed := now.Sub(hostAccesses[host]) > period
	hostAccesses[host] = now
	f.hostAccessesByClient.Add(clientAddr, hostAccesses)
	f.mx.Unlock()
	if !allowed {
		return filters.Fail("Rate limit for %v exceeded", host)
	}
	return next()
}
