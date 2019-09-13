/*
2019 © Postgres.ai
*/

package util

import (
	"math"
	"time"
)

func EqualStringSlicesUnordered(x, y []string) bool {
	xMap := make(map[string]int)
	yMap := make(map[string]int)

	for _, xElem := range x {
		xMap[xElem]++
	}
	for _, yElem := range y {
		yMap[yElem]++
	}

	for xMapKey, xMapVal := range xMap {
		if yMap[xMapKey] != xMapVal {
			return false
		}
	}

	for yMapKey, yMapVal := range yMap {
		if xMap[yMapKey] != yMapVal {
			return false
		}
	}

	return true
}

func Unique(list []string) []string {
	keys := make(map[string]bool)
	uqList := []string{}
	for _, entry := range list {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			uqList = append(uqList, entry)
		}
	}
	return uqList
}

func Contains(list []string, s string) bool {
	for _, item := range list {
		if s == item {
			return true
		}
	}
	return false
}

func SecondsAgo(ts time.Time) uint {
	now := time.Now()
	return uint(math.Floor(now.Sub(ts).Seconds()))
}

func RunInterval(d time.Duration, fn func()) chan struct{} {
	ticker := time.NewTicker(d)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				fn()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	// Use `close(quit)` to stop interval execution.
	return quit
}
