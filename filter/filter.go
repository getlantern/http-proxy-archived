package filter

import (
	"fmt"
	"net/http"

	"github.com/getlantern/http-proxy/utils"
)

// Filter is a special http.Handler that returns true or false depending on
// whether subsequent handlers should continue.
type Filter interface {
	// ServeHTTP is like the function on http.Handler but also returns true or
	// false depending on whether subsequent handlers should continue. If an error
	// occurred, ServeHTTP should return the original error plus a description
	// for logging purposes.
	Apply(w http.ResponseWriter, req *http.Request) (ok bool, err error, errdesc string)
}

// Filters is a chain of filters that acts as an http.Handler and a Filter.
type Filters interface {
	http.Handler
	Filter

	// Creates a new Filters by appending the given filters.
	And(filters ...Filter) Filters
}

type chain []Filter

// Chain constructs a new chain of filters that executes the filters in order
// until it encounters a filter that returns false.
func Chain(filters ...Filter) Filters {
	return chain(filters)
}

func (c chain) And(filters ...Filter) Filters {
	return append(c, filters...)
}

func (c chain) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c.Apply(w, req)
}

func (c chain) Apply(w http.ResponseWriter, req *http.Request) (ok bool, err error, desc string) {
	for _, filter := range c {
		ok, err, desc = filter.Apply(w, req)
		if err != nil {
			utils.DefaultHandler.ServeHTTP(w, req, err, desc)
		} else if !ok {
			// Interrupt chain
			return
		}
	}

	return
}

// Continue is a convenience method for indicating that we should continue down
// filter chain.
func Continue() (bool, error, string) {
	return true, nil, ""
}

// Stop is a convenience method for indicating that we should stop processing
// the filter chain, but not due to an error.
func Stop() (bool, error, string) {
	return false, nil, ""
}

// Fail is a convenience method for failing and not continuing down filter
// chain.
func Fail(err error, msg string, args ...interface{}) (bool, error, string) {
	return false, err, fmt.Sprintf(msg, args)
}

// Adapt adapts an existing http.Handler to the Filter interface.
func Adapt(handler http.Handler) Filter {
	return &wrapper{handler}
}

type wrapper struct {
	handler http.Handler
}

func (w *wrapper) Apply(resp http.ResponseWriter, req *http.Request) (bool, error, string) {
	w.handler.ServeHTTP(resp, req)
	return Continue()
}
