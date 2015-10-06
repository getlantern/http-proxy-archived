package utils

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"gopkg.in/redis.v3"
)

const (
	bytesInField  = "bytesIn"
	bytesOutField = "bytesOut"
)

var (
	redisClient *redis.Client
)

// connectRedis will connect to the database and make sure we can ping
func ConnectRedis() error {
	redisAddr := os.Getenv("REDIS_PRODUCTION_URL")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	redisClient = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err := redisClient.Ping().Result()
	return err
}

// upsertRedisEntry must be thread-safe
func UpsertRedisEntry(key []byte, client *Client) {
	// TODO: use "lastAccess" field to avoid updating the unchanged fields
	/*
		if *debug {
			fmt.Printf("%s  In: %v, Out: %v\n",
				key,
				atomic.LoadInt64(&client.BytesIn),
				atomic.LoadInt64(&client.BytesOut))
		}
	*/
	var err error
	// We are not supposed to be updating a user concurrently, since it's
	// going to be assigned to one server only.  We are just being cautious
	// here
	bytesIn := atomic.LoadInt64(&client.BytesIn)
	bytesOut := atomic.LoadInt64(&client.BytesOut)
	err = redisClient.HMSet("client:"+string(key),
		bytesInField, strconv.FormatInt(bytesIn, 10),
		bytesOutField, strconv.FormatInt(bytesOut, 10)).Err()
	if err != nil {
		fmt.Printf("Error setting Redis key: %v\n", err)
	}
	// An auxiliary ordered set for aggregated bytesIn+bytesOut
	// Redis stores scores as float64
	err = redisClient.ZAdd("client->bytes",
		redis.Z{
			float64(bytesIn + bytesOut),
			string(key),
		}).Err()
	if err != nil {
		fmt.Printf("Error setting Redis key: %v\n", err)
	}
}

func getRedisEntry(key []byte) (*Client, bool) {
	hmget := redisClient.HMGet("client:"+string(key), bytesInField, bytesOutField)
	result, err := hmget.Result()
	if err != nil {
		return nil, false
	}
	var bytesIn int64
	if result[0] != nil {
		bytesIn, _ = strconv.ParseInt(result[0].(string), 10, 64)
	}
	var bytesOut int64
	if result[1] != nil {
		bytesOut, _ = strconv.ParseInt(result[1].(string), 10, 64)
	}

	return &Client{
		Created:    time.Now(),
		LastAccess: time.Now(),
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
	}, true
}
