package service

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type giftDeduper struct {
	mu        sync.Mutex
	ttl       time.Duration
	seenUntil map[string]time.Time

	opCount        atomic.Uint64
	duplicateCount atomic.Uint64
}

func newGiftDeduper(ttl time.Duration) *giftDeduper {
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}
	return &giftDeduper{
		ttl:       ttl,
		seenUntil: make(map[string]time.Time, 1024),
	}
}

// MarkIfNew 返回 true 表示首次出现；false 表示命中去重窗口。
func (d *giftDeduper) MarkIfNew(key string) bool {
	if key == "" {
		return true
	}

	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	if expireAt, ok := d.seenUntil[key]; ok && now.Before(expireAt) {
		totalDup := d.duplicateCount.Add(1)
		if totalDup == 1 || totalDup%50 == 0 {
			log.Printf("礼物去重命中: duplicate_total=%d key=%s", totalDup, key)
		}
		return false
	}

	d.seenUntil[key] = now.Add(d.ttl)

	ops := d.opCount.Add(1)
	if ops%200 == 0 || len(d.seenUntil) > 50000 {
		d.sweepExpired(now)
	}

	return true
}

func (d *giftDeduper) sweepExpired(now time.Time) {
	for k, expireAt := range d.seenUntil {
		if !now.Before(expireAt) {
			delete(d.seenUntil, k)
		}
	}
}
