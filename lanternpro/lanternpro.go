// Lantern Pro middleware will identify Pro users and forward their requests
// immediately.  It will intercept non-Pro users and limit their total transfer

package lanternpro

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Workiva/go-datastructures/set"
	"github.com/Workiva/go-datastructures/trie/ctrie"
	//"github.com/hashicorp/golang-lru"

	"../utils"
)

const (
	uidHeader = "X-Lantern-UID"
)

type LanternProFilter struct {
	debug          bool
	next           http.Handler
	proTokens      *set.Set
	clientRegistry *ctrie.Ctrie
	//clientCache, _ = lru.New(32) // 32 seems a reasonable number of concurrent users per server
}

type Client struct {
	Created    time.Time
	LastAccess time.Time
	BytesIn    int64
	BytesOut   int64
}

func New(next http.Handler) (*LanternProFilter, error) {
	return &LanternProFilter{
		debug:          true,
		next:           next,
		proTokens:      set.New(),
		clientRegistry: ctrie.New(nil),
	}, nil
}

func (f *LanternProFilter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f.debug {
		fmt.Println("Lantern Pro Middleware received request:")
		reqStr, _ := httputil.DumpRequest(req, true)
		fmt.Printf(string(reqStr))
	}
	lanternUID := req.Header.Get(uidHeader)
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
	var client *Client
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
		client = val.(*Client)
		//client.LastAccess = time.Now()
		//f.clientRegistry.Insert(key, client)
	} else {
		client = &Client{
			Created:    time.Now(),
			LastAccess: time.Now(),
			BytesIn:    0,
			BytesOut:   0,
		}
		f.clientRegistry.Insert(key, client)
	}
	var atomicClient atomic.Value
	atomicClient.Store(client)
	f.intercept(key, atomicClient, w, req)

	//clientCache.Add(key, client)
}

// ScanClientsSnapshot will run a fn over each client on a specific snapshot in
// time.  It will do it periodically given the second argument.
// Note that the provided function must operate on the clients concurrently
// with other routines.
func (f *LanternProFilter) ScanClientsSnapshot(fn func([]byte, *Client), period time.Duration) {
	go func() {
		for {
			time.Sleep(period)
			snapshot := f.clientRegistry.Snapshot()
			// Note: Remember that if the snapshot is not going to
			// be fully iterated, it will leak.  A cancelling chanel
			// needs to be used
			for entry := range snapshot.Iterator(nil) {
				client := entry.Value.(*Client)
				fn(entry.Key, client)
			}
		}
	}()
}

func (f *LanternProFilter) intercept(key []byte, atomicClient atomic.Value, w http.ResponseWriter, req *http.Request) (err error) {
	if req.Method == "CONNECT" {
		var clientConn net.Conn
		var connOut net.Conn

		utils.RespondOK(w, req)
		if clientConn, _, err = w.(http.Hijacker).Hijack(); err != nil {
			utils.RespondBadGateway(w, req, fmt.Sprintf("Unable to hijack connection: %s", err))
			return
		}
		if connOut, err = net.Dial("tcp", req.Host); err != nil {
			return
		}
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
			n, _ := io.Copy(connOut, clientConn)

			client := atomicClient.Load().(*Client)
			atomic.AddInt64(&client.BytesIn, n)

			closeOnce.Do(closeConns)

		}()
		n, _ := io.Copy(clientConn, connOut)

		client := atomicClient.Load().(*Client)
		atomic.AddInt64(&client.BytesOut, n)

		closeOnce.Do(closeConns)
	} else {
		req.Header.Del(uidHeader)
		f.next.ServeHTTP(w, req)

		// HERE!!!

		// TODO: byte counting in this case (by using custom response writer and inspecting req)
	}
	return
}
