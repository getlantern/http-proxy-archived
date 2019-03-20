package proxyfilters

import (
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxy/filters"
)

type ctxKey string

const opKey = ctxKey("op")

func getOp(ctx filters.Context) ops.Op {
	return ctx.Value(opKey).(ops.Op)
}

// RecordOp records the proxy_http op.
var RecordOp = filters.FilterFunc(func(ctx filters.Context, req *http.Request, next filters.Next) (*http.Response, filters.Context, error) {
	name := "proxy_http"
	if req.Method == http.MethodConnect {
		name += "s"
	}
	op := ops.Begin(name)
	ctx = ctx.WithValue(opKey, op)
	resp, nextCtx, err := next(ctx, req)
	if err != nil {
		op.FailIf(err)
		// Dumping stack trace is useful but in this case, it would create tons
		// of noises as the filters are called recursively.
		if e, ok := err.(errors.Error); ok {
			log.Error(e.RootCause())
		} else {
			log.Error(err)
		}
	}
	op.End()
	return resp, nextCtx, err
})
