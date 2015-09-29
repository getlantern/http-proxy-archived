package lanternpro

import (
	"fmt"
	"io"
	"net/http"
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
	Created    time.Time
	LastAccess time.Time
	NumBytes   uint64
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
		f.clientRegistry.Insert(key,
			Client{
				Created:    time.Now(),
				LastAccess: time.Now(),
				NumBytes:   0,
			})
	}
	f.next.ServeHTTP(w, req)

	//clientCache.Add(key, client)
}

func (f *LanternProFilter) GatherData(w io.Writer, period time.Duration) {
	go func() {
		for {
			time.Sleep(period)
			snapshot := f.clientRegistry.Snapshot()
			for entry := range snapshot.Iterator(nil) {
				fmt.Fprintf(w, "%s: %v\n", entry.Key, entry.Value.(Client).NumBytes)
			}
		}
	}()
}
