package main

import (
	"github.com/garyburd/redigo/redis"
	"limitwant/entry"
	"limitwant/limiter/slidingwindowlimiter"
)

func NewLimitWant(config *entry.LimitWantConfig, redisCli redis.Conn) entry.Limiter {
	t := config.LimitType
	limitWant := &entry.LimitWant{config, redisCli}
	var res entry.Limiter
	switch t {
	case entry.SlidingWindowLimiterType:
		res = slidingwindowlimiter.NewSlidingWindowLimiter(limitWant)
	case entry.TokenBucketLimiterType:
		// TODO
	case entry.LeakyBucketLimiterType:
		// TODO
	default:
		panic("Unknown limiter type! ")
	}
	return res
}
