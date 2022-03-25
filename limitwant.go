package limitwant

import (
	"github.com/garyburd/redigo/redis"
	"github.com/rooftopj/limitwant/entry"
	"github.com/rooftopj/limitwant/limiter/slidingwindowlimiter"
)

const (
	SlidingWindowLimiterType = iota
	TokenBucketLimiterType
	LeakyBucketLimiterType
)

func NewLimitWant(config *entry.LimitWantConfig, redisCli redis.Conn) entry.Limiter {
	t := config.LimitType
	limitWant := &entry.LimitWant{config, redisCli}
	var res entry.Limiter
	switch t {
	case SlidingWindowLimiterType:
		res = slidingwindowlimiter.NewSlidingWindowLimiter(limitWant)
	case TokenBucketLimiterType:
		// TODO
	case LeakyBucketLimiterType:
		// TODO
	default:
		panic("Unknown limiter type! ")
	}
	return res
}

func SWLimitInfo(key string, freq, num int64) *entry.LimitInfo {
	return &entry.LimitInfo{
		LimitKey:  key,
		LimitFreq: freq,
		LimitNum:  num,
	}
}

func SWLimitConfig() *entry.LimitWantConfig {
	return &entry.LimitWantConfig{
		LimitType: SlidingWindowLimiterType,
	}
}
