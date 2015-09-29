package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"

	"../connectforward"
)

func main() {
	connectFwd, _ := connectforward.New()

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		dmp, err := httputil.DumpRequest(req, true)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(dmp))
		connectFwd.ServeHTTP(w, req)

	})

	s := &http.Server{
		Addr:    ":8080",
		Handler: redirect,
	}
	s.ListenAndServe()
}
