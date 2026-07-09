package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/repository"

	"golang.org/x/sync/singleflight"
)

const productCatalogTTL = 30 * time.Second

type catalogEntry struct {
	products  []*ent.Product
	expiresAt time.Time
}

type CatalogCache struct {
	mu      sync.RWMutex
	entries map[string]catalogEntry
	gen     atomic.Uint64
	sf      singleflight.Group
}

func NewCatalogCache() *CatalogCache {
	return &CatalogCache{entries: make(map[string]catalogEntry)}
}

func (c *CatalogCache) generation() uint64 {
	return c.gen.Load()
}

func (c *CatalogCache) invalidate() {
	c.gen.Add(1)
	c.mu.Lock()
	c.entries = make(map[string]catalogEntry)
	c.mu.Unlock()
}

func (c *CatalogCache) get(key string, gen uint64, now time.Time) ([]*ent.Product, bool) {
	if gen != c.generation() {
		return nil, false
	}
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || now.After(e.expiresAt) {
		return nil, false
	}
	return e.products, true
}

func (c *CatalogCache) set(key string, gen uint64, products []*ent.Product, expiresAt time.Time) {
	if gen != c.generation() {
		return
	}
	c.mu.Lock()
	c.entries[key] = catalogEntry{products: products, expiresAt: expiresAt}
	c.mu.Unlock()
}

// Entries and singleflight are tenant-keyed; no-tenant requests bypass the cache.
func (c *CatalogCache) load(ctx context.Context, key string, loader func(context.Context) ([]*ent.Product, error)) ([]*ent.Product, error) {
	tenantID, ok := repository.TenantIDFromContext(ctx)
	if !ok {
		return loader(ctx)
	}
	key = tenantID + ":" + key

	now := time.Now()
	gen := c.generation()
	if v, ok := c.get(key, gen, now); ok {
		return v, nil
	}
	v, err, _ := c.sf.Do(key, func() (any, error) {
		gen := c.generation()
		if v, ok := c.get(key, gen, time.Now()); ok {
			return v, nil
		}
		products, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		c.set(key, gen, products, time.Now().Add(productCatalogTTL))
		return products, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]*ent.Product), nil
}

func catalogKeyAll() string                 { return "all" }
func catalogKeyByCategory(id string) string { return "cat:" + id }
