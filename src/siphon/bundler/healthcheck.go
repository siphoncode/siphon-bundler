package bundler

import (
	"fmt"
	"net/http"
	"os"
)

// Healthcheck is for Uptime Robot to check real uptime of the bundler.
func Healthcheck() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")

		hash := "9ad6ddd80b42ed82849ebfc0933cfeb3a1f08f3d2bbfb77cde1fc15d" +
			"631a0d43"
		appID := ""
		if os.Getenv("SIPHON_ENV") == "production" {
			appID = "FIqjdggmex"
		} else {
			appID = "vezfRWaSSD"
		}
		key := appID + "/" + hash

		// Make sure hashes exist.
		hashesResponse, err := MakeJSONHashes(appID)
		if err != nil {
			fmt.Fprintf(w, "Hashes error: %s", err)
			return
		}
		if len(hashesResponse.Files) < 1 {
			fmt.Fprint(w, "Hashes error: zero found")
			return
		}

		// Check S3.
		s3 := NewS3Wrapper()
		if err := s3.Open(); err != nil {
			fmt.Fprintf(w, "S3 Open() error: %s", err)
			return
		}
		b, err := s3.GetKey(key)
		if err != nil {
			fmt.Fprintf(w, "S3 GetKey() error: %s", err)
			return
		}
		if len(b) < 1 {
			fmt.Fprint(w, "S3 GetKey() error: empty response")
			return
		}

		// Write to the cache.
		cache, err := NewCache(appID, "")
		err = cache.Set(key, b)
		if err != nil {
			fmt.Fprintf(w, "Cache.Set() error: %s", err)
			return
		}

		// Read from the cache.
		b2, err := cache.Get(key)
		if err != nil {
			fmt.Fprintf(w, "Cache.Get() error: %s", err)
			return
		}
		if len(b2) != len(b) {
			fmt.Fprint(w, "Cache.Get() error: lengths do not match")
			return
		}

		// If we got this far, we're all good.
		fmt.Fprint(w, "Passed.")
	})
}
