package proxyfilters

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/getlantern/proxy/v2/filters"
)

// RestrictConnectPorts restricts CONNECT requests to the given list of allowed
// ports and returns either a 400 error if the request is missing a port or a
// 403 error if the port is not allowed.
func RestrictConnectPorts(allowedPorts []int) filters.Filter {
	return filters.FilterFunc(func(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
		if req.Method != http.MethodConnect || len(allowedPorts) == 0 {
			return next(cs, req)
		}

		log.Tracef("Checking CONNECT tunnel to %s against allowed ports %v", req.Host, allowedPorts)
		_, portString, err := net.SplitHostPort(req.Host)
		if err != nil {
			// CONNECT request should always include port in req.Host.
			// Ref https://tools.ietf.org/html/rfc2817#section-5.2.
			return fail(cs, req, http.StatusBadRequest, "No port field in Request-URI / Host header")
		}

		port, err := strconv.Atoi(portString)
		if err != nil {
			return fail(cs, req, http.StatusBadRequest, fmt.Sprintf("Invalid port for %v: %v", req.Host, portString))
		}

		for _, p := range allowedPorts {
			if port == p {
				return next(cs, req)
			}
		}
		return fail(cs, req, http.StatusForbidden, fmt.Sprintf("Port not allowed for %v: %d", req.Host, port))
	})
}
