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
	log utils.Logger
	fwd *forward.Forwarder
}

type optSetter func(f *HTTPConnectForwarder) error

func Logger(l utils.Logger) optSetter {
	return func(f *HTTPConnectForwarder) error {
		f.log = l
		return nil
	}
}

func New(setters ...optSetter) (*HTTPConnectForwarder, error) {
	// TODO: connectforward should handle CONNECT and direct, and do the bytecounting (or split this last one if feasible/reasonable)
	fwd, err := forward.New(
		//forward.Logger(utils.NewTimeLogger()),
		forward.PassHostHeader(true),
	)
	if err != nil {
		return nil, err
	}

	f := &HTTPConnectForwarder{
		log: utils.NullLogger,
		fwd: fwd,
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f *HTTPConnectForwarder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reqStr, _ := httputil.DumpRequest(req, true)
	f.log.Debugf("HTTPConnectForwarder Middleware received request:\n%s", reqStr)

	var err error
	if req.Method == "CONNECT" {
		f.log.Infof("CONNECT proxying\n")
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
		f.log.Infof("Direct proxying\n")
		fmt.Println(req.Host)
		f.fwd.ServeHTTP(w, req)
	}
}
