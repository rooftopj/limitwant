# ratelimit (Under development) [![](https://img.shields.io/badge/language-Go-blue)](https://go.dev/)


---
This package provides a Golang implementation of the Redis-based distributed rate limit algorithm, including Sliding Window, Token Bucket and Leaky Bucket.

---

## 1. Sliding Window (SW) Algorithm
This implementation uses `Redis` to count the requests in the current time window, and caches the amount of requests in the previous time window locally. If there is no data record in the cache, `SingleFilght` is used to obtain data from `Redis` and cache it. SingleFilght is cleared and the health-check of `Redis` is performed periodically in the background.

### SW's Example
```go
limitConfig := &entry.LimitWantConfig{LimitType: entry.SlidingWindowLimiterType}
redisCli, err := redis.Dial("tcp", "your Redis ip:port", redis.DialPassword("your Redis password"))
limiter := NewLimitWant(limitConfig, redisCli)

limitKey := "your limit key"
var limitFreq int64 = 10 // seconds
var limitCount int64 = 1000
limitInfo := &entry.LimitInfo{limitKey, limitFreq, limitCount} // 1000Requsets/10s
ok, err := limiter.Take(limitInfo)
if err != nil {
    panic(err)
    return
}
if ok {
    fmt.Println("the request passes")
} else {
    fmt.Println("the request is limited")
}