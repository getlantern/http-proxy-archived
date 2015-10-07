package utils

import (
	"sync/atomic"
	"time"

	//"github.com/hashicorp/golang-lru"
	"github.com/Workiva/go-datastructures/trie/ctrie"
)

type Client struct {
	Created    time.Time
	LastAccess time.Time
	BytesIn    int64
	BytesOut   int64
}

type Key int

// Used for request contexts (attaching the client structure to the request)
const ClientKey Key = 0

var (
	clientRegistry *ctrie.Ctrie
	// Client Cache to avoid hitting the clientRegistry when possible
	// clientCache, _ = lru.New(32) // 32 seems a reasonable number of concurrent users per server
)

func init() {
	clientRegistry = ctrie.New(nil)
}

// ScanClientsSnapshot will run a fn over each client on a specific snapshot in
// time.  It will do it periodically given the second argument.
// Note that the provided function must operate on the clients concurrently
// with other routines.
func ScanClientsSnapshot(fn func([]byte, *Client), period time.Duration) {
	go func() {
		for {
			time.Sleep(period)
			snapshot := clientRegistry.Snapshot()
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

func GetClient(key []byte) atomic.Value {
	var client *Client

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

	if val, ok := clientRegistry.Lookup(key); ok {
		client = val.(*Client)
		//client.LastAccess = time.Now()
		//f.clientRegistry.Insert(key, client)
	} else {
		// First try to retrieve it from Redis
		if redisClient != nil {
			client, ok = getRedisEntry(key)
		}
		if client == nil {
			client = &Client{
				Created:    time.Now(),
				LastAccess: time.Now(),
				BytesIn:    0,
				BytesOut:   0,
			}
		}
		clientRegistry.Insert(key, client)
	}
	var atomicClient atomic.Value
	atomicClient.Store(client)
	//clientCache.Add(key, client)

	return atomicClient
}
