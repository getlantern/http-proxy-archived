package filters

import (
	"context"
	"net"
	"net/http"
	"strconv"

	"github.com/getlantern/proxy/filters"
)

// RestrictConnectPorts restricts CONNECT requests to the given list of allowed
// ports.
func RestrictConnectPorts(allowedPorts []int) filters.Filter {
	fail := func(ctx context.Context, req *http.Request, statusCode int, description string) (*http.Response, error) {
		op := getOp(ctx)
		err := op.FailIf(log.Error(description))
		return filters.Fail(req, statusCode, err)
	}

	return filters.FilterFunc(func(ctx context.Context, req *http.Request, next filters.Next) (*http.Response, error) {
		if req.Method != http.MethodConnect || len(allowedPorts) == 0 {
			return next(ctx, req)
		}

		log.Tracef("Checking CONNECT tunnel to %s against allowed ports %v", req.Host, allowedPorts)
		_, portString, err := net.SplitHostPort(req.Host)
		if err != nil {
			// CONNECT request should always include port in req.Host.
			// Ref https://tools.ietf.org/html/rfc2817#section-5.2.
			return fail(ctx, req, http.StatusBadRequest, "No port field in Request-URI / Host header")
		}

		port, err := strconv.Atoi(portString)
		if err != nil {
			return fail(ctx, req, http.StatusBadRequest, "Invalid port")
		}

		for _, p := range allowedPorts {
			if port == p {
				return next(ctx, req)
			}
		}
		return fail(ctx, req, http.StatusForbidden, "Port not allowed")
	})
}
