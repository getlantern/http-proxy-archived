package connectforward

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/mailgun/oxy/forward"
)

type HTTPConnectForwarder struct {
	fwd *forward.Forwarder
}

func New() (*HTTPConnectForwarder, error) {
	fwd, err := forward.New()
	if err != nil {
		return nil, err
	}
	return &HTTPConnectForwarder{
		fwd: fwd,
	}, nil
}

func (p *HTTPConnectForwarder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var err error
	if req.Method == "CONNECT" {
		var clientConn net.Conn
		var connOut net.Conn

		respondOK(w, req)
		if clientConn, _, err = w.(http.Hijacker).Hijack(); err != nil {
			respondBadGateway(w, fmt.Sprintf("Unable to hijack connection: %s", err))
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
		fmt.Println("== CONNECT DONE ==")
	} else {
		p.fwd.ServeHTTP(w, req)
		fmt.Println("== DIRECT PROXYING DONE ==")
	}
}

func respondOK(writer io.Writer, req *http.Request) error {
	defer func() {
		if err := req.Body.Close(); err != nil {
			fmt.Printf("Error closing body of OK response: %s", err)
		}
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
	}

	return resp.Write(writer)
}

func respondBadGateway(w io.Writer, msg string) {
	fmt.Printf("Responding BadGateway: %v", msg)
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	err := resp.Write(w)
	if err == nil {
		if _, err = w.Write([]byte(msg)); err != nil {
			fmt.Printf("Error writing error to io.Writer: %s", err)
		}
	}
}
