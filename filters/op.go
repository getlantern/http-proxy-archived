package filters

import (
	"context"
	"net/http"

	"github.com/getlantern/ops"
	"github.com/getlantern/proxy/filters"
)

// RecordOp records the proxy_http op.
var RecordOp = filters.FilterFunc(func(ctx context.Context, req *http.Request, next filters.Next) (*http.Response, error) {
	op := ops.Begin("proxy_http")
	resp, err := next(ctx, req)
	op.FailIf(err)
	op.End()
	return resp, err
})
