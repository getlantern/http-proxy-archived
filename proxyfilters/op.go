package proxyfilters

import (
	stderrors "errors"
	"net"
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxy/v2/filters"
)

// RecordOp records the proxy_http op.
var RecordOp = filters.FilterFunc(func(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	name := "proxy_http"
	if req.Method == http.MethodConnect {
		name += "s"
	}
	op := ops.Begin(name)
	resp, nextCtx, err := next(cs, req)
	if err != nil {
		op.FailIf(err)
		logFilterError(err)
	}
	op.End()
	return resp, nextCtx, err
})

func logFilterError(err error) {
	var (
		opErr  *net.OpError
		dnsErr *net.DNSError
	)
	switch {
	case stderrors.As(err, &opErr) && opErr.Timeout():
		// Network timeouts are out of our control and create noise in StackDriver.
		log.Debugf("%s timeout error: %v", opErr.Op, err)
		return
	case stderrors.As(err, &dnsErr):
		// DNS errors are out of our control and create noise in StackDriver.
		log.Debugf("DNS error: %v", err)
		return
	}
	if e, ok := err.(errors.Error); ok {
		// Filters are called recursively. We log only the root to reduce stack trace noise.
		err = e.RootCause()
	}
	log.Error(err)
}
