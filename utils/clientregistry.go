package utils

import (
	"time"

	"github.com/Workiva/go-datastructures/trie/ctrie"
)

type Client struct {
	Created    time.Time
	LastAccess time.Time
	BytesIn    int64
	BytesOut   int64
}

var (
	ClientRegistry *ctrie.Ctrie
)

func init() {
	ClientRegistry = ctrie.New(nil)
}

// ScanClientsSnapshot will run a fn over each client on a specific snapshot in
// time.  It will do it periodically given the second argument.
// Note that the provided function must operate on the clients concurrently
// with other routines.
func ScanClientsSnapshot(fn func([]byte, *Client), period time.Duration) {
	go func() {
		for {
			time.Sleep(period)
			snapshot := ClientRegistry.Snapshot()
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
