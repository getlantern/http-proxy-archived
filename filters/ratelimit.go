package filters

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/proxy/filters"
	"github.com/hashicorp/golang-lru"
)

// RestrictConnectPorts restricts CONNECT requests to the given list of allowed
// ports and returns either a 400 error if the request is missing a port or a
// 403 error if the port is not allowed.
func RateLimit(numClients int, hostPeriods map[string]time.Duration) filters.Filter {
	if numClients <= 0 {
		numClients = 5000
	}
	hostAccessesByClient, _ := lru.New(numClients)
	var mx sync.Mutex

	return filters.FilterFunc(func(ctx context.Context, req *http.Request, next filters.Next) (*http.Response, error) {
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
			return fail(req, http.StatusForbidden, "Access to %v not allowed", host)
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
			return fail(req, http.StatusForbidden, "Rate limit for %v exceeded", host)
		}

		return next(ctx, req)
	})
}
