package bundler

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

// BufferLine writes a line into a streamed HTTP response.
func BufferLine(w http.ResponseWriter, msg string) {
	fmt.Fprintf(w, "siphon: %s\n", msg)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// FilterStringSlice takes a slice of strings and a function and
// returns a slice for which the function evaluates true on the string
func FilterStringSlice(slc []string, f func(string) bool) []string {
	var filt []string
	for _, s := range slc {
		if f(s) {
			filt = append(filt, s)
		}
	}
	return filt
}

func Cleanup(d string) {
	if err := os.RemoveAll(d); err != nil {
		log.Printf("(Ignored) Cleanup failed: %v", err)
	}
}

// FilesToTemp takes a map of {fileName: hash, ...} pairs, *sql.DB, *Archive
// and *Cache and writes the files to a temp directory. The path of the
// directory is returned
func FilesToTemp(files map[string]string, db *sql.DB,
	archive *Archive, cache *Cache) (dir string, err error) {
	// Copy all of the app's files into a new temporary directory
	d, _ := ioutil.TempDir("", "project-path")
	for name, hash := range files {
		var fb []byte
		// Try to grab from archive directory first
		if archive != nil {
			fb, _ = archive.GetContent(name)
		}
		// If that failed, try to get it from memcache/S3
		if fb == nil {
			fb, err = cache.Get(hash)
			if err != nil {
				Cleanup(d)
				return "", err
			}
		}
		// Write the file to our temporary directory
		p := path.Join(d, name)
		os.MkdirAll(filepath.Dir(p), 0700) // make any intermediate dirs
		if ioutil.WriteFile(p, fb, 0700); err != nil {
			Cleanup(d)
			return "", err
		}
	}
	return d, nil
}
