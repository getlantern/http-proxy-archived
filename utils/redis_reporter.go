package utils

import (
	"fmt"

	"gopkg.in/redis.v3"

	"github.com/getlantern/golog"
	"github.com/getlantern/measured"
)

var log = golog.LoggerFor("main")

type redisReporter struct {
	redisClient *redis.Client
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

func (rp *redisReporter) ReportError(s map[*measured.Error]int) error {
	return nil
}
func (rp *redisReporter) ReportLatency(s []*measured.LatencyTracker) error {
	return nil
}
func (rp *redisReporter) ReportTraffic(tt []*measured.TrafficTracker) error {
	for _, t := range tt {
		key := t.ID
		if key == "" {
			panic("empty key is not allowed")
		}
		tx := rp.redisClient.Multi()

		_, err := tx.Exec(func() error {
			err := tx.HIncrBy("client:"+string(key), "bytesIn", int64(t.LastIn)).Err()
			if err != nil {
				return err
			}
			err = tx.HIncrBy("client:"+string(key), "bytesOut", int64(t.LastOut)).Err()
			if err != nil {
				return err
			}
			// An auxiliary ordered set for aggregated bytesIn+bytesOut
			// Redis stores scores as float64
			err = tx.ZAdd("client->bytes",
				redis.Z{
					float64(t.TotalIn + t.TotalOut),
					key,
				}).Err()
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("Error in MULTI command: %v\n", err)
		}

		tx.Close()
	}
	return nil
}
