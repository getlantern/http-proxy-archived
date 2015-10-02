package connectforward

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/mailgun/oxy/forward"

	"../utils"
)

type HTTPConnectForwarder struct {
	debug bool
	fwd   *forward.Forwarder
}

func New() (*HTTPConnectForwarder, error) {
	fwd, err := forward.New(
		//forward.Logger(utils.NewTimeLogger()),
		forward.PassHostHeader(true),
	)
	if err != nil {
		return nil, err
	}
	return &HTTPConnectForwarder{
		debug: true,
		fwd:   fwd,
	}, nil
}

func (p *HTTPConnectForwarder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if p.debug {
		fmt.Println("HTTPConnectForwarder Middleware received request:")
		reqStr, _ := httputil.DumpRequest(req, true)
		fmt.Printf(string(reqStr))
	}

	var err error
	if req.Method == "CONNECT" {
		if p.debug {
			fmt.Println("CONNECT proxying")
		}
		var clientConn net.Conn
		var connOut net.Conn

		utils.RespondOK(w, req)
		if clientConn, _, err = w.(http.Hijacker).Hijack(); err != nil {
			utils.RespondBadGateway(w, req, fmt.Sprintf("Unable to hijack connection: %s", err))
			return
		}
		connOut, err = net.Dial("tcp", req.Host)
		// Pipe data through CONNECT tunnel
		closeConns := func() {
			if clientConn != nil {
				if err := clientConn.Close(); err != nil {
					fmt.Printf("Error closing the out connection: %s", err)
				}
			}
			if connOut != nil {
				if err := connOut.Close(); err != nil {
					fmt.Printf("Error closing the client connection: %s", err)
				}
			}
		}
		var closeOnce sync.Once
		go func() {
			_, _ = io.Copy(connOut, clientConn)
			closeOnce.Do(closeConns)
		}()
		_, _ = io.Copy(clientConn, connOut)
		closeOnce.Do(closeConns)
	} else {
		if p.debug {
			fmt.Println("Direct proxying")
		}
		fmt.Println(req.Host)
		p.fwd.ServeHTTP(w, req)
	}
}
