package proxyfilters

import (
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/proxy/filters"
)

// BlockLocal blocks attempted accesses to localhost unless they're one of the
// listed exceptions.
func BlockLocal(exceptions []string) filters.Filter {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Errorf("Error enumerating local addresses: %v\n", err)
	}

	localIPs := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		str := a.String()
		idx := strings.Index(str, "/")
		if idx != -1 {
			str = str[:idx]
		}
		ip := net.ParseIP(str)
		localIPs = append(localIPs, ip)
	}

	isException := func(host string) bool {
		for _, exception := range exceptions {
			if strings.EqualFold(host, exception) {
				// This is okay, allow it
				return true
			}
		}
		return false
	}

	return filters.FilterFunc(func(ctx filters.Context, req *http.Request, next filters.Next) (*http.Response, filters.Context, error) {
		if isException(req.URL.Host) {
			return next(ctx, req)
		}

		host, _, err := net.SplitHostPort(req.URL.Host)
		if err != nil {
			// host didn't have a port, thus splitting didn't work
			host = req.URL.Host
		}

		ipAddr, err := net.ResolveIPAddr("ip", host)

		// If there was an error resolving is probably because it wasn't an address
		// in the form host or host:port
		if err == nil {
			if ipAddr.IP.IsLoopback() {
				return fail(ctx, req, http.StatusForbidden, "%v requested loopback address %v (%v)", req.RemoteAddr, req.Host, ipAddr)
			}
			for _, localIP := range localIPs {
				if ipAddr.IP.Equal(localIP) {
					return fail(ctx, req, http.StatusForbidden, "%v requested local address %v (%v)", req.RemoteAddr, req.Host, ipAddr)
				}
			}
		}

		return next(ctx, req)
	})
}
