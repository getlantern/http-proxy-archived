package utils

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

func (rp *redisReporter) ReportError(s *measured.Error) error {
	return nil
}
func (rp *redisReporter) ReportLatency(s *measured.Latency) error {
	return nil
}
func (rp *redisReporter) ReportTraffic(s *measured.Traffic) error {
	key := s.ID
	if key == "" {
		panic("empty key is not allowed")
	}
	// TODO: use INCRBY instead, as user can connect to multiple chained server
	// TODO: wrap two operations in transaction, or redis function
	err := rp.rc.HMSet("client:"+string(key),
		"bytesIn", strconv.FormatUint(s.BytesIn, 10),
		"bytesOut", strconv.FormatUint(s.BytesOut, 10)).Err()
	if err != nil {
		return fmt.Errorf("Error setting Redis key: %v\n", err)
	}
	// An auxiliary ordered set for aggregated bytesIn+bytesOut
	// Redis stores scores as float64
	err = rp.rc.ZAdd("client->bytes",
		redis.Z{
			float64(s.BytesIn + s.BytesOut),
			key,
		}).Err()
	if err != nil {
		return fmt.Errorf("Error setting Redis key: %v\n", err)
	}
	return nil
}
