package proxyfilters

import (
	"net/http"

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
		log.Error(op.FailIf(err))
	}
	op.End()
	return resp, nextCtx, err
})
