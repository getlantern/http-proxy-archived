package commonfilter

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/http-proxy/utils"
)

type CommonFilter struct {
	log        utils.Logger
	errHandler utils.ErrorHandler
	next       http.Handler

	localIPs []net.IP
}

type optSetter func(f *CommonFilter) error

func Logger(l utils.Logger) optSetter {
	return func(f *CommonFilter) error {
		f.log = l
		return nil
	}
}

func New(next http.Handler, setters ...optSetter) (*CommonFilter, error) {
	f := &CommonFilter{
		next:       next,
		log:        utils.NullLogger,
		errHandler: utils.DefaultHandler,
	}

	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Print(fmt.Errorf("Error enumerating local addresses: %+\n", err))
	}
	for _, a := range addrs {
		str := a.String()
		idx := strings.Index(str, "/")
		if idx != -1 {
			str = str[:idx]
		}
		ip := net.ParseIP(str)
		f.localIPs = append(f.localIPs, ip)
	}

	return f, nil
}

func (f *CommonFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reqAddr, err := net.ResolveTCPAddr("tcp", req.URL.Host)

	// If there was an error resolving is probably because it wasn't an address
	// in the form localhost:port
	if err == nil {
		for _, ip := range f.localIPs {
			if reqAddr.IP.Equal(ip) {
				f.errHandler.ServeHTTP(w, req, err)
				return
			}
		}

	}

	f.next.ServeHTTP(w, req)
}
