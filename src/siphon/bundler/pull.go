package bundler

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gorilla/context"
)

const assetsListingFile string = "assets-listing"
const assetsDir string = "__siphon_assets" // dir written to in the zip
var assetsLike []string = []string{"%.png", "%.jpg", "%.jpeg",
																 "%.gif", "%.psd", "%.svg", "%.webp"}

var errAppEmpty = errors.New("This app has not been pushed yet.")

type pullArchive struct {
	appID        string
	submissionID string
	tempDir      string
	assetFiles   map[string]string
	cache        *Cache
}

func newPullArchive(appID string, submissionID string) (pa *pullArchive,
	err error) {
	// Make our temporary directory
	d, err := ioutil.TempDir("", "pull-archive")
	if err != nil {
		return nil, err
	}
	cache, err := NewCache(appID, submissionID)
	if err != nil {
		return nil, err
	}
	a := pullArchive{appID: appID, submissionID: submissionID,
		tempDir: d, cache: cache}
	return &a, nil
}

func (a *pullArchive) getAssetFiles(db *sql.DB) error {
	f, err := GetSliceFilteredFiles(db, a.appID, a.submissionID, assetsLike)
	if err != nil {
		return err
	}
	a.assetFiles = f
	return nil
}

func (a *pullArchive) writeAssetsListing() error {
	// Write out the assets-listing file into our temporary directory
	f, _ := os.Create(path.Join(a.tempDir, assetsListingFile))
	defer f.Close()

	for name := range a.assetFiles {
		// The original path (e.g. "components/bananas.png") is prefixed with
		// "images/<path>" to match the client's __siphon_assets/ directory
		fil := path.Join("images", name) + "\n"
		if _, err := f.WriteString(fil); err != nil {
			return err
		}
	}
	return nil
}

// WriteAssets adds any assets that the client does not have (or assets that
// have changed) into the __siphon_assets/images/ directory in this archive. It
// compares the given `assetHashes` with those stored by the server.
func (a *pullArchive) writeAssets(assetHashes map[string]string) error {
	for name, hash := range a.assetFiles {
		// If the client does not have this asset (i.e. the name is not
		// present in the hashes they sent us) or the client's SHA-256 hash
		// for a file differs from our's, then we will write it to the zip.
		prefixedName := path.Join("images", name) // as it appears in hashes
		clientSha, ok := assetHashes[prefixedName]
		if !ok || clientSha != hash {
			b, err := a.cache.Get(hash)
			if err != nil {
				return err
			}
			p := path.Join(a.tempDir, assetsDir, prefixedName)

			// Make intermediate dirs then write the asset fil
			if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				log.Printf("[writeAssets() MkdirAll error] %v", err)
				return err
			}
			if err = ioutil.WriteFile(p, b, 0700); err != nil {
				log.Printf("[writeAssets() WriteFile error] %v", err)
				return err
			}
		}
	}
	return nil
}

func (a *pullArchive) writeBundleFooter(platform string) error {
	b, err := a.cache.GetBundleFooter(BundleFooterFile(platform))

	// If we encounter an error, then we check for an old-style bundle footer
	// (user may have pushed their app before Android support)
	if err != nil {
		b, err = a.cache.GetBundleFooter("bundle-footer")
		if err != nil {
			return err
		}
	}

	p := path.Join(a.tempDir, "bundle-footer")
	if err = ioutil.WriteFile(p, b, 0600); err != nil {
		return err
	}
	return nil
}

func (a *pullArchive) writeZip(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/zip")
	zw := zip.NewWriter(w)
	defer zw.Close()
	err := filepath.Walk(a.tempDir, func(path string, info os.FileInfo,
		err error) error {
		if !info.IsDir() {
			p := strings.Replace(path, a.tempDir+"/", "", 1)
			f, err := zw.Create(p)
			src, err := os.Open(path)
			_, err = io.Copy(f, src)
			src.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (a *pullArchive) assertNotEmpty(db *sql.DB) error {
	f, err := GetFiles(db, a.appID, a.submissionID)
	if err != nil {
		return err
	} else if len(f) < 1 {
		return errAppEmpty
	}
	return nil
}

func (a *pullArchive) close() {
	// Clean up our temporary directory
	if err := os.RemoveAll(a.tempDir); err != nil {
		log.Printf("(Ignored) pullArchive() cleanup failed: %v", err)
	}
}

func parseAssetHashes(r *http.Request) (assetHashes map[string]string,
	err error) {
	b, err := ioutil.ReadAll(r.Body)
	var obj struct {
		AssetHashes map[string]string `json:"asset_hashes"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	} else if obj.AssetHashes == nil {
		return nil, fmt.Errorf("Empty struct found for AssetHashes.")
	}
	return obj.AssetHashes, nil
}

// Pull handles a response for the /pull route
func Pull(w http.ResponseWriter, r *http.Request) {
	assetHashes, err := parseAssetHashes(r)
	if err != nil {
		log.Printf("Error decoding /pull JSON: %v", err)
		http.Error(w, "Malformed payload.", 500)
		return
	}

	// Optionally extract a submission ID from the GET parameters. If it's
	// present it means this is a production pull. In this case we need
	// to assert that the ID in the GET parameter matches the one in the
	// handshake (which, if present, AuthMiddleware would have stored in
	// a context variable).
	r.ParseForm()
	submissionID := r.FormValue("submission_id")
	if submissionID != "" && submissionID != context.Get(r, SubmissionIDKey) {
		http.Error(w, "Submission ID does not match the handshake.",
			http.StatusUnauthorized)
		return
	}

	appID := context.Get(r, AppIDKey).(string)
	platform := r.FormValue("platform")
	if platform == "" {
		platform = "ios"
	}

	archive, err := newPullArchive(appID, submissionID)
	defer archive.close()

	// Open up a postgres connection
	db := OpenDB()
	defer db.Close()

	// Make sure this app has files, i.e. it has been pushed
	if err := archive.assertNotEmpty(db); err != nil {
		if err == errAppEmpty {
			http.Error(w, err.Error(), 400)
		} else {
			log.Printf("assertNotEmpty() error: %v", err)
			http.Error(w, "Internal error.", 500)
		}
		return
	}

	// Grab our currently stored asset names (also, it's an error to pull
	// an app if no files have been pushed yet).
	if err := archive.getAssetFiles(db); err != nil {
		// Otherwise it's some unexpected error
		log.Printf("getAssetFiles() error: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}

	// Write the assets listing file
	if err = archive.writeAssetsListing(); err != nil {
		log.Printf("WriteAssetsListing() error: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}

	// Write the differing assets
	if err = archive.writeAssets(assetHashes); err != nil {
		log.Printf("WriteAssets() error: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}

	// Write the bundle footer
	if err = archive.writeBundleFooter(platform); err != nil {
		log.Printf("WriteBundleFooter() error: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}

	// Write the response back to the client
	if err = archive.writeZip(w); err != nil {
		log.Printf("WriteZip() error: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}
}
