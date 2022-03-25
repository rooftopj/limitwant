package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
)

func main() {
	swExample()
}

func swExample() {
	limitConfig := SWLimitConfig()
	redisCli, err := redis.Dial("tcp", "your Redis ip:port", redis.DialPassword("your Redis password"))
	limiter := NewLimitWant(limitConfig, redisCli)

	limitKey := "your limit key"
	var limitFreq int64 = 10 // seconds
	var limitNum int64 = 1000
	limitInfo := SWLimitInfo(limitKey, limitFreq, limitNum) // 1000Requsets/10s
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
}
