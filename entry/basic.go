package entry

import (
	"github.com/garyburd/redigo/redis"
)

type LimitInfo struct {
	LimitKey  string
	LimitFreq int64
	LimitNum  int64
}

type LimitWantConfig struct {
	LimitType uint8
}

type LimitWant struct {
	LimitWantConfig *LimitWantConfig
	RedisClient     redis.Conn
}

type Limiter interface {
	Take(info *LimitInfo) (bool, error)
}
