package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"

	"../connectforward"
	"../lanternpro"
)

func main() {
	// Handles CONNECT and direct proxying requests
	connectFwd, _ := connectforward.New()
	// Processes requests before passing them to HTTPConnectForwarder
	lanternPro, _ := lanternpro.New(connectFwd)

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		dmp, err := httputil.DumpRequest(req, true)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(dmp))
		lanternPro.ServeHTTP(w, req)
	})

	s := &http.Server{
		Addr:    ":8080",
		Handler: redirect,
	}
	s.ListenAndServe()
}
