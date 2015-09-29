// Lantern Pro middleware will identify Pro users and forward their requests
// immediately.  It will intercept non-Pro users and limit their total transfer

package lanternpro

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Workiva/go-datastructures/set"
	"github.com/Workiva/go-datastructures/trie/ctrie"
	//"github.com/hashicorp/golang-lru"
)

type LanternProFilter struct {
	next           http.Handler
	proTokens      *set.Set
	clientRegistry *ctrie.Ctrie
	//clientCache, _ = lru.New(32) // 32 seems a reasonable number of concurrent users per server
}

type Client struct {
	Created     time.Time
	LastAccess  time.Time
	TransferIn  int64
	TransferOut int64
}

func New(next http.Handler) (*LanternProFilter, error) {
	return &LanternProFilter{
		next:           next,
		proTokens:      set.New(),
		clientRegistry: ctrie.New(nil),
	}, nil
}

func (f *LanternProFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	lanternUID := req.Header.Get("X-Lantern-UID")
	lanternProToken := req.Header.Get("X-Lantern-Pro-Token")

	// If a Pro token is found in the header, test if its valid and then let
	// the request pass
	if lanternProToken != "" {
		if f.proTokens.Exists(lanternProToken) {
			f.next.ServeHTTP(w, req)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
		return
	}

	// A UID must be provided always by the client
	if lanternUID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// If this point is reached, handle the request as a non-Pro user
	var client Client
	key := []byte(lanternUID)

	// Try first in the cache
	// TODO: Actually, leave optimizations for later
	/*
		if client, ok := clientCache.Get(key); ok {
			client.(*Client).LastAccess = time.Now()
			// TODO: numbytes
			clientRegistry.Insert(key, *(client.(*Client)))
			return
		} else {
			clientCache.Set(key, *client)
		}
	*/

	if val, ok := f.clientRegistry.Lookup(key); ok {
		client = val.(Client)
		client.LastAccess = time.Now()
		f.clientRegistry.Insert(key, client)
	} else {
		clientPtr := &Client{
			Created:     time.Now(),
			LastAccess:  time.Now(),
			TransferIn:  0,
			TransferOut: 0,
		}
		f.clientRegistry.Insert(key, *clientPtr)
	}
	f.intercept(key, client, w, req)

	//clientCache.Add(key, client)
}

func (f *LanternProFilter) GatherData(w io.Writer, period time.Duration) {
	go func() {
		for {
			time.Sleep(period)
			snapshot := f.clientRegistry.Snapshot()
			for entry := range snapshot.Iterator(nil) {
				fmt.Fprintf(w, "%s  In: %v, Out: %v\n",
					entry.Key,
					entry.Value.(Client).TransferIn,
					entry.Value.(Client).TransferOut)
			}
		}
	}()
}

func (f *LanternProFilter) intercept(key []byte, client Client, w http.ResponseWriter, req *http.Request) {
	var err error
	var wg sync.WaitGroup
	if req.Method == "CONNECT" {
		var clientConn net.Conn
		var connOut net.Conn

		respondOK(w, req)
		if clientConn, _, err = w.(http.Hijacker).Hijack(); err != nil {
			respondBadGateway(w, req, fmt.Sprintf("Unable to hijack connection: %s", err))
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
		wg.Add(1)
		go func() {
			n, _ := io.Copy(connOut, clientConn)
			atomic.AddInt64(&client.TransferIn, n)
			closeOnce.Do(closeConns)
			wg.Done()

		}()
		n, _ := io.Copy(clientConn, connOut)
		atomic.AddInt64(&client.TransferOut, n)
		closeOnce.Do(closeConns)
		fmt.Println("== CONNECT DONE ==")
	} else {
		f.next.ServeHTTP(w, req)
		// TODO: byte counting in this case
	}
	wg.Wait()
	f.clientRegistry.Insert(key, client)
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

func respondBadGateway(w io.Writer, req *http.Request, msgs ...string) {
	defer func() {
		if err := req.Body.Close(); err != nil {
			fmt.Printf("Error closing body of OK response: %s", err)
		}
	}()

	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	err := resp.Write(w)
	if err == nil {
		for _, msg := range msgs {
			if _, err = w.Write([]byte(msg)); err != nil {
				fmt.Printf("Error writing error to io.Writer: %s", err)
			}
		}
	}
}
