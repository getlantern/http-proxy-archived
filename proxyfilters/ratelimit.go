package proxyfilters

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/proxy/v2/filters"
	lru "github.com/hashicorp/golang-lru"
)

// RateLimit restricts access to only specific hosts and limits the rate at
// which clients (identified by IP address) are allowed to access thoses hosts.
func RateLimit(numClients int, hostPeriods map[string]time.Duration) filters.Filter {
	if numClients <= 0 {
		numClients = 5000
	}
	hostAccessesByClient, _ := lru.New(numClients)
	var mx sync.Mutex

	return filters.FilterFunc(func(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
		host, _, err := net.SplitHostPort(req.Host)
		if err != nil {
			host = req.Host
		}
		client, _, _ := net.SplitHostPort(req.RemoteAddr)
		now := time.Now()

		mx.Lock()
		defer mx.Unlock()
		period := hostPeriods[host]
		if period == 0 {
			return fail(cs, req, http.StatusForbidden, "Access to %v not allowed", host)
		}
		var hostAccesses map[string]time.Time
		_hostAccesses, found := hostAccessesByClient.Get(client)
		if found {
			hostAccesses = _hostAccesses.(map[string]time.Time)
		} else {
			hostAccesses = make(map[string]time.Time)
		}
		allowed := now.Sub(hostAccesses[host]) > period
		if allowed {
			hostAccesses[host] = now
			hostAccessesByClient.Add(client, hostAccesses)
		}
		if !allowed {
			return fail(cs, req, http.StatusForbidden, "Rate limit for %v exceeded", host)
		}

		return next(cs, req)
	})
}
