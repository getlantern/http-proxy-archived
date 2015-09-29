package main

import (
	//"../plugins"
	"fmt"
	"github.com/mailgun/oxy/forward"
	"net/http"
	"net/http/httputil"
)

func main() {
	fwd, _ := forward.New()
	//proFilter, _ := lanternpro.New()

	redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		dmp, err := httputil.DumpRequest(req, true)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(dmp))
		fwd.ServeHTTP(w, req)
	})

	s := &http.Server{
		Addr:    ":8080",
		Handler: redirect,
	}
	s.ListenAndServe()
}
