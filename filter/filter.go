package filter

import (
	"fmt"
	"net/http"

	"github.com/getlantern/http-proxy/utils"
)

// Filter is a special http.Handler that returns true or false depending on
// whether subsequent handlers should continue.
type Filter interface {
	// ServeHTTP is like the function on http.Handler but also gets a context
	// which allows it to control the progress of the filter chain.
	Apply(w http.ResponseWriter, req *http.Request, ctx Context)
}

// Filters is a chain of filters that acts as an http.Handler.
type Filters interface {
	http.Handler

	// Creates a new chain by appending the given filters.
	Append(post ...Filter) Filters

	// Creates a new chain by prepending the given filters.
	Prepend(pre Filter) Filters
}

type chain []Filter

// Chain constructs a new chain of filters that executes the filters in order
// until it encounters a filter that returns false.
func Chain(filters ...Filter) Filters {
	return chain(filters)
}

func (c chain) Append(post ...Filter) Filters {
	return append(c, post...)
}

func (c chain) Prepend(pre Filter) Filters {
	result := make(chain, len(c)+1)
	result[0] = pre
	copy(result[1:], c)
	return result
}

func (c chain) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(c) == 0 {
		return
	}
	ctx := &context{w, req, c[1:]}
	c[0].Apply(w, req, ctx)
}

// Context is the context within which Filters execute
type Context interface {
	// Continue continues execution down the filter chain.
	Continue()

	// Fail fails execution of the current filter and stops processing the filter
	// chain.
	Fail(err error, msg string, args ...interface{})
}

type context struct {
	w         http.ResponseWriter
	req       *http.Request
	remaining []Filter
}

func (ctx context) Continue() {
	if len(ctx.remaining) == 0 {
		return
	}

	next := ctx.remaining[0]
	next.Apply(ctx.w, ctx.req, &context{ctx.w, ctx.req, ctx.remaining[1:]})
}

func (ctx context) Fail(err error, msg string, args ...interface{}) {
	utils.DefaultHandler.ServeHTTP(ctx.w, ctx.req, err, fmt.Sprintf(msg, args))
}

// Adapt adapts an existing http.Handler to the Filter interface.
func Adapt(handler http.Handler) Filter {
	return &wrapper{handler}
}

type wrapper struct {
	handler http.Handler
}

func (w *wrapper) Apply(resp http.ResponseWriter, req *http.Request, ctx Context) {
	w.handler.ServeHTTP(resp, req)
	ctx.Continue()
}
