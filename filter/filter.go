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
	ServeHTTP(w http.ResponseWriter, req *http.Request) (ok bool, err error, errdesc string)
}

type FilterChain interface {
	http.Handler

	// Creates a new FilterChain by appending the given filters.
	And(filters ...Filter) FilterChain
}

// filterChain is a chain of filters that implements the http.Handler
// interface.
type filterChain struct {
	filters []Filter
}

// Chain constructs a new chain of filters that executes the filters in order
// until it encounters a filter that returns false.
func Chain(filters ...Filter) http.Handler {
	return &filterChain{filters}
}

func (c *filterChain) And(filters ...Filter) FilterChain {
	return &filterChain{append(c.filters, filters...)}
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

func (chain *filterChain) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, filter := range chain.filters {
		ok, err, desc := filter.ServeHTTP(w, req)
		if err != nil {
			utils.DefaultHandler.ServeHTTP(w, req, err, desc)
		} else if !ok {
			// Interrupt chain
			return
		}
	}
}

// Adapt adapts an existing http.Handler to the Filter interface.
func Adapt(handler http.Handler) Filter {
	return &wrapper{handler}
}

type wrapper struct {
	handler http.Handler
}

func (w *wrapper) ServeHTTP(resp http.ResponseWriter, req *http.Request) (bool, error, string) {
	w.handler.ServeHTTP(resp, req)
	return Continue()
}
