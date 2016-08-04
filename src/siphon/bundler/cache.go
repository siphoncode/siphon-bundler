package bundler

import (
	"fmt"
	"log"
	"os"

	"github.com/bradfitz/gomemcache/memcache"
)

// Cache manages the persistence of any files associated with an app, including
// source files and the bundle footers we generate with them. When calling
// Get(), it first tries to find that key in memcache, but if that fails it
// defers to S3. Calling Set() updates both memcache and S3.
type Cache struct {
	appID        string
	submissionID string
	mc           *memcache.Client
	s3           *S3Wrapper
}

// BundleFooterFile returns the key for a bundle footer for a given platform
func BundleFooterFile(platform string) string {
	return fmt.Sprintf("bundle-footer-%s", platform)
}

// NewCache wraps memcache and S3. It uses `appID` and `submissionID` to
// prefix it's keys internally. An empty `submissionID` means we're dealing
// with development files.
func NewCache(appID string, submissionID string) (c *Cache, err error) {
	var mc *memcache.Client
	if os.Getenv("SIPHON_ENV") != "testing" {
		host := os.Getenv("MEMCACHED_BUNDLER_PORT_11211_TCP_ADDR")
		mc = memcache.New(host + ":11211")
	}
	s3 := NewS3Wrapper()
	if err := s3.Open(); err != nil {
		return nil, err
	}
	return &Cache{appID: appID, submissionID: submissionID, mc: mc, s3: s3}, nil
}

// Returns the full key name, appropriately prefixed with appID/submissionID.
func (c *Cache) prefixed(key string) string {
	if c.submissionID == "" {
		return fmt.Sprintf("%s/%s", c.appID, key)
	}
	return fmt.Sprintf("%s/%s/%s", c.appID, c.submissionID, key)
}

// Cache() uses this to fetch from S3 when a key isn't present in
// memcached. On getting the result, it persists it in memcache to avoid
// future misses. Note that the key should already be prefixed.
func (c *Cache) cacheMiss(k string) (b []byte, err error) {
	log.Printf("[cache-miss %s]", k)
	b, err = c.s3.GetKey(k)
	if err != nil {
		return nil, err
	}
	// Persist the result to memcache
	if c.mc != nil {
		if err := c.mc.Set(&memcache.Item{Key: k, Value: b}); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// Get tries to gets the the key in memcache, or defers to S3.
func (c *Cache) Get(key string) (b []byte, err error) {
	k := c.prefixed(key)
	if c.mc != nil {
		item, err := c.mc.Get(k)
		if err == memcache.ErrCacheMiss {
			return c.cacheMiss(k)
		} else if err != nil {
			return nil, err
		}
		// Otherwise, great, we found the key in memcache so return its value
		//log.Printf("[cache-hit %s]", k)
		return item.Value, nil
	}
	return c.cacheMiss(k)
}

func (c *Cache) GetBundleFooter(name string) (b []byte, err error) {
	return c.Get(name)
}

func (c *Cache) SetBundleFooter(b []byte, name string) error {
	return c.Set(name, b)
}

// Set writes the key to S3 and memcache.
func (c *Cache) Set(key string, b []byte) error {
	k := c.prefixed(key)
	log.Printf("[cache-set %s]", k)
	// Store it in S3 first
	if err := c.s3.WriteKey(k, b); err != nil {
		log.Printf("[Cache.Set() S3 error] %v", err)
		return err
	}
	// Then add/update the key in memcache
	if c.mc != nil {
		if err := c.mc.Set(&memcache.Item{Key: k, Value: b}); err != nil {
			log.Printf("[Cache.Set() memcached error] %v", err)
			return err
		}
	}
	return nil
}

// Delete removes the key in both S3 and memcache.
func (c *Cache) Delete(key string) error {
	k := c.prefixed(key)
	log.Printf("[cache-delete %s]", k)
	// Delete it from S3 first
	if err := c.s3.DeleteKey(k); err != nil {
		return err
	}
	// Then from memcache (note that we only care about an error if it's
	// not a ErrCacheMiss, which is thrown when the key is not present.)
	if c.mc != nil {
		err := c.mc.Delete(k)
		if err != nil && err != memcache.ErrCacheMiss {
			return err
		}
	}
	return nil
}
