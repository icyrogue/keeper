package cachemanager

import (
	"sync"
	"time"

	"github.com/icyrogue/ye-keeper/internal/jsonmodels"
	"github.com/zpatrick/go-cache"
	"golang.org/x/net/context"
)

type response = jsonmodels.JSONResponse

type cacheManager struct {
	queue map[string]bool
	cache *cache.Cache
	mtx   sync.RWMutex
}

func New() *cacheManager {
	return &cacheManager{
		queue: make(map[string]bool),
		cache: cache.New(),
	}
}

func (c *cacheManager) Check(name string) bool {
	c.mtx.RLock()

	//If value is in cache, return true
	if c.queue[name] {
		c.mtx.RUnlock()
		return true
	}
	c.mtx.RUnlock()
	c.mtx.Lock()
	defer c.mtx.Unlock()

	//if value hasnt yet been cached, return false but set
	//queue value to true for other requests with the same name
	c.queue[name] = true
	return false
}

func (c *cacheManager) Store(name string, data []response) {
	c.cache.Set(name, data)
	c.mtx.RLock()
	if c.queue[name] {
		c.mtx.RUnlock()
		return
	}
	c.mtx.RUnlock()
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.queue[name] = true
}

func (c *cacheManager) Get(ctx context.Context, name string) chan []response {
	output := make(chan []response)
	c.mtx.RLock()
	ticker := time.NewTicker(time.Second * 2)
	var timeout int
	go func() {
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case <-ticker.C:
				v, ok := c.cache.GetOK(name)
				if ok {
					output <- v.([]response) //TODO: add custom type
					break loop
				}
				timeout++
				if timeout >= 10 {
					c.mtx.Lock()
					defer c.mtx.Unlock()
					c.queue[name] = false
					break loop
				}
			}
		}
		close(output)
	}()
	return output
}
