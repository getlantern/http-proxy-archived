package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
)

// responseWriter implements http.ResponseWriter and http.Hijacker
type responseWriter struct {
	conn     net.Conn
	rw       *bufio.ReadWriter
	resp     *http.Response
	pipe     io.WriteCloser
	errCh    chan error
	hijacked bool
}

func newResponseWriter(conn net.Conn, rw *bufio.ReadWriter) *responseWriter {
	return &responseWriter{
		conn:  conn,
		rw:    rw,
		resp:  newResponse(),
		errCh: make(chan error),
	}
}

func (resp *responseWriter) Header() http.Header {
	return resp.resp.Header
}

func (resp *responseWriter) Write(p []byte) (int, error) {
	if resp.pipe == nil {
		pr, pw := io.Pipe()
		resp.pipe = pw
		resp.resp.Body = pr
		go func() {
			tee := io.MultiWriter(resp.rw, os.Stdout)
			err := resp.resp.Write(tee)
			resp.errCh <- err
		}()
	}
	return resp.pipe.Write(p)
}

func (resp *responseWriter) WriteHeader(statusCode int) {
	resp.resp.StatusCode = statusCode
}

func (resp *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if resp.hijacked {
		return nil, nil, http.ErrHijacked
	}
	resp.hijacked = true
	return resp.conn, resp.rw, nil
}

func (resp *responseWriter) flush() (err error) {
	if !resp.hijacked {
		if resp.pipe != nil {
			resp.pipe.Close()
			err = <-resp.errCh
		}
		if err == nil {
			resp.rw.Write([]byte("\r\n\r\n"))
			err = resp.rw.Flush()
			resp.resp = newResponse()
			resp.pipe = nil
		}
	}

	return
}

func newResponse() *http.Response {
	header := make(http.Header)
	resp := &http.Response{
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     header,
	}
	resp.TransferEncoding = append(resp.TransferEncoding, "chunked")
	return resp
}
