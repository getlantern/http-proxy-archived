package main

import (
	"fmt"
	"strconv"

	"gopkg.in/redis.v3"

	"github.com/getlantern/golog"
	"github.com/getlantern/measured"
)

var log = golog.LoggerFor("main")

type redisReporter struct {
	rc *redis.Client
}

func NewRedisReporter(redisAddr string) (measured.Reporter, error) {
	rc := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	_, err := rc.Ping().Result()
	if err != nil {
		return nil, fmt.Errorf("Unable to ping redis server: %s", err)
	}
	return &redisReporter{rc}, nil
}

func (rp *redisReporter) Submit(s *measured.Stats) error {
	if s.Type != "stats" {
		return nil
	}
	key := s.Tags["client"]
	if key == "" {
		panic("empty key is not allowed")
	}
	bytesIn := s.Fields["bytesIn"].(uint64)
	bytesOut := s.Fields["bytesOut"].(uint64)
	// TODO: use INCRBY instead, as user can connect to multiple chained server
	// TODO: wrap two operations in transaction, or redis function
	err := rp.rc.HMSet("client:"+string(key),
		"bytesIn", strconv.FormatUint(bytesIn, 10),
		"bytesOut", strconv.FormatUint(bytesOut, 10)).Err()
	if err != nil {
		return fmt.Errorf("Error setting Redis key: %v\n", err)
	}
	// An auxiliary ordered set for aggregated bytesIn+bytesOut
	// Redis stores scores as float64
	err = rp.rc.ZAdd("client->bytes",
		redis.Z{
			float64(bytesIn + bytesOut),
			key,
		}).Err()
	if err != nil {
		return fmt.Errorf("Error setting Redis key: %v\n", err)
	}
	return nil
}
