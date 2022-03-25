package slidingwindowlimiter

import (
	"context"
	"errors"
	"fmt"
	"github.com/bluele/gcache"
	"github.com/garyburd/redigo/redis"
	"github.com/rooftopj/limitwant/entry"
	"golang.org/x/sync/singleflight"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	WindowSize           = 5
	LRUSize              = 5000
	LimitFlagSize        = 3000
	MGETTimeOut          = 1
	IncrBy               = 1
	HealthCheckThreshold = 10
	BGTaskInterval       = 5
	SegeSize             = 10000
	IncrScript           = `local num=redis.call('incrby',KEYS[1],ARGV[1])
  if tonumber(num)==1 then
        redis.call('pexpire',KEYS[1],ARGV[3])
        return 0
  elseif tonumber(num)>tonumber(ARGV[2]) then
		redis.call('decrby',KEYS[1],ARGV[1])
        return 1
  else
        return 0
  end`
)

type SlidingWindowLimiter struct {
	turn               bool
	incrScript         *redis.Script
	lru                gcache.Cache
	limitFlag          gcache.Cache
	limitWant          *entry.LimitWant
	rwLock             sync.RWMutex
	sfg                singleflight.Group
	sfgDeleteSet       *sync.Map
	sfgDeleteSetRecord *sync.Map
	redisErrCnt        *int64
}

func NewSlidingWindowLimiter(limitWant *entry.LimitWant) *SlidingWindowLimiter {
	redisErrCnt := new(int64)
	*redisErrCnt = 0
	sw := &SlidingWindowLimiter{
		turn:               false,
		incrScript:         redis.NewScript(1, IncrScript),
		lru:                gcache.New(LRUSize).LRU().Build(),
		limitFlag:          gcache.New(LimitFlagSize).LRU().Build(),
		limitWant:          limitWant,
		rwLock:             sync.RWMutex{},
		sfg:                singleflight.Group{},
		sfgDeleteSet:       &sync.Map{},
		sfgDeleteSetRecord: &sync.Map{},
		redisErrCnt:        redisErrCnt,
	}

	go sw.cleanSingleFlight()
	go sw.redisHealthCheck()

	return sw
}

func (s *SlidingWindowLimiter) cleanSingleFlight() {
	ticker := time.NewTicker(BGTaskInterval * time.Second)
	for range ticker.C {
		if s.turn {
			s.sfgDeleteSetRecord.Range(func(key, value interface{}) bool {
				s.sfg.Forget(key.(string))
				return true
			})
			s.turn = false
		} else {
			s.rwLock.Lock()
			s.sfgDeleteSetRecord = s.sfgDeleteSet
			s.sfgDeleteSet = &sync.Map{}
			s.rwLock.Unlock()
			s.turn = true
		}
	}
}

func (s *SlidingWindowLimiter) redisHealthCheck() {
	ticker := time.NewTicker(BGTaskInterval * time.Second)
	for range ticker.C {
		if !s.ifRedisReady() {
			_, err := s.limitWant.RedisClient.Do("ping")
			if err == nil {
				atomic.StoreInt64(s.redisErrCnt, 0)
			}
		}
	}
}

func (s *SlidingWindowLimiter) Take(info *entry.LimitInfo) (bool, error) {
	if !s.ifRedisReady() {
		return false, errors.New("Redis not healthy!")
	}

	interval := (info.LimitFreq*1000-1)/WindowSize + 1
	count := (int64)(float64(info.LimitNum) * float64(interval) / (float64(info.LimitFreq*1000) / float64(WindowSize)))
	freq := interval * WindowSize
	key := info.LimitKey
	segeCnt := max(count/(freq/1000)/SegeSize, 1)
	var segeIdx int64 = 0

	if segeCnt > 1 {
		segeIdx = rand.Int63n(segeCnt)
		if segeIdx == segeCnt-1 {
			count = count - (segeCnt-1)*(count/segeCnt)
		} else {
			count = count / segeCnt
		}
	}

	key = fmt.Sprintf("%s-sege:%d", key, segeIdx)

	return s.acquirePermit(key, freq, interval, count)
}

func (s *SlidingWindowLimiter) ifRedisReady() bool {
	return atomic.LoadInt64(s.redisErrCnt) <= HealthCheckThreshold
}

func (s *SlidingWindowLimiter) acquirePermit(key string, freq, interval, count int64) (bool, error) {
	var all int64 = 0
	now := time.Now().UnixNano() / 1e6
	idx := now / interval
	redisKey := fmt.Sprintf("%s-idx:%d", key, idx)

	flag, flagErr := s.limitFlag.Get(key)
	if flagErr != gcache.KeyNotFoundError && flagErr != nil {
		return false, flagErr
	}
	if flagErr == nil && flag.(int64) == idx {
		return false, nil
	}

	needRedis := make([]interface{}, 0)
	for i := idx - WindowSize + 1; i < idx; i++ {
		windowKey := fmt.Sprintf("%s-idx:%d", key, i)
		get, LRUError := s.lru.Get(windowKey)
		if LRUError == nil {
			all += get.(int64)
		} else if LRUError == gcache.KeyNotFoundError {
			needRedis = append(needRedis, windowKey)
		} else {
			return false, LRUError
		}
	}

	if all >= count {
		s.limitFlag.SetWithExpire(key, idx, time.Duration(freq)*time.Millisecond)
		return false, nil
	}

	if len(needRedis) > 0 {
		results := s.singleflightCache(freq, redisKey, needRedis)
		ctx, _ := context.WithTimeout(context.Background(), MGETTimeOut*time.Second)
		select {
		case r := <-results:
			if r.Err != nil {
				atomic.AddInt64(s.redisErrCnt, 1)
				return false, r.Err
			}
			vals := r.Val.([]int64)
			for _, v := range vals {
				all += v
			}
		case <-ctx.Done():
			return false, errors.New("Redis mget timeout!")
		}
	}

	if all >= count {
		s.limitFlag.SetWithExpire(key, idx, time.Duration(freq)*time.Millisecond)
		return false, nil
	}

	permit := count - all
	val, err := redis.Int64(s.incrScript.Do(s.limitWant.RedisClient, redisKey, IncrBy, permit, 2*freq))
	if err != nil {
		atomic.AddInt64(s.redisErrCnt, 1)
		return false, err
	}

	if val == 1 {
		s.limitFlag.SetWithExpire(key, idx, time.Duration(freq)*time.Millisecond)
		return false, nil
	}

	return true, nil
}

func (s *SlidingWindowLimiter) singleflightCache(freq int64, redisKey string, needRedis []interface{}) <-chan singleflight.Result {
	results := s.sfg.DoChan(redisKey, func() (interface{}, error) {
		vals, err := redis.Int64s(s.limitWant.RedisClient.Do("mget", needRedis...))
		if err == nil {
			s.rwLock.RLock()
			s.sfgDeleteSet.Store(redisKey, struct{}{})
			s.rwLock.RUnlock()
			for i, v := range vals {
				s.lru.SetWithExpire(needRedis[i], v, time.Duration(2*freq)*time.Millisecond)
			}
		}
		return vals, err
	})
	return results
}

func max(a, b int64) int64 {
	if a > b {
		return a
	} else {
		return b
	}
}
