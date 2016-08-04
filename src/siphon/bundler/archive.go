package bundler

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
)

const diffsDir string = "diffs"
const listingFile string = "listing.json"

// Archive encapsulates an unzipped POST zip archive
type Archive struct {
	tempDir string
	listing map[string]string // name => SHA-256 hash (from the client)
}

// ArchiveComparison represents the differences between the files that *we*
// have for an app vs. the files that the client has reported that it has.
type ArchiveComparison struct {
	added   []string
	changed []string
	removed []string
}

// Lazily loads and parses the listing file
func (a *Archive) parseListingFile() error {
	if a.listing != nil {
		return nil
	}
	p := path.Join(a.tempDir, listingFile)
	f, err := os.Open(p)
	defer f.Close()
	b, err := ioutil.ReadAll(f)

	l := map[string]string{}
	err = json.Unmarshal(b, &l)
	if err != nil {
		return err
	}

	// Security/sanity check
	for name, sha := range l {
		if strings.Contains(name, "../") {
			return errors.New("Bad listing name: " + name)
		}
		if sha == "" {
			return errors.New("Bad listing SHA: " + sha)
		}
	}

	a.listing = l
	return nil
}

func (a *Archive) diffPath(name string) string {
	return path.Join(a.tempDir, diffsDir, name)
}

func (a *Archive) diffExists(name string) bool {
	p := a.diffPath(name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}
	return true
}

// GetMetadata returns the contents of this app's Siphonfile if it exists,
// or returns nil if no Siphonfile was sent in the archive (i.e. it did not
// change).
func (a *Archive) GetMetadata() (b []byte, err error) {
	// If there's no Siphonfile in diffs/ then don't try to read it.
	if !a.diffExists(MetadataName) {
		return nil, nil
	}
	// If we got this far then the Siphonfile was included, so read it
	b, err = a.GetContent(MetadataName)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Compare does a comparison of each name->hash in `files` to our internal
// listing and returns new/changed files that need writing and removed files
// that need deleting.
func (a *Archive) Compare(files map[string]string) (
	comparison *ArchiveComparison, err error) {
	err = a.parseListingFile() // lazy
	if err != nil {
		return nil, err
	}

	added := []string{}
	changed := []string{}
	removed := []string{}

	// Compute additions (paths that exist in listing but not in our current
	// hashes) and changes (paths in the listing that we do have, but the SHA
	// is different)
	for name, sha := range a.listing {
		serverSha, ok := files[name]
		if !ok {
			added = append(added, name)
		} else if sha != serverSha {
			changed = append(changed, name)
		}
	}

	// Verify that a file in /diffs exists for each of our additions/changes
	for _, name := range append(added, changed...) {
		if !a.diffExists(name) {
			log.Printf("Could not find %s in diffs/ dir.", name)
			return nil, errors.New("Listing does not match diffs/ in archive.")
		}
	}

	// Removals (path that exist in our current hashes, but not in the listing)
	for name := range files {
		_, ok := a.listing[name]
		if !ok {
			removed = append(removed, name)
		}
	}

	return &ArchiveComparison{added, changed, removed}, nil
}

// GetHash returns a SHA-256 for a file name that should exist in this
// archive, or "" if no hash could be found.
func (a *Archive) GetHash(name string) string {
	h, ok := a.listing[name]
	if !ok {
		return ""
	}
	return h
}

// GetContent returns the raw file content from /diffs for the given name.
func (a *Archive) GetContent(name string) (b []byte, err error) {
	p := a.diffPath(name)
	b, err = ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Decompress takes a zip archive sent as a POST payload and unzips it
// for further processing.
func (a *Archive) Decompress(b []byte) error {
	// Create the temporary dir
	d, err := ioutil.TempDir("", "bundler-archive")
	if err != nil {
		log.Printf("Failed to create temporary directory: %v", err)
		return errors.New("Failed to decompress the payload.")
	}
	a.tempDir = d
	// Write out the zip archive there
	src := path.Join(a.tempDir, "archive.zip")
	err = ioutil.WriteFile(src, b, 0600)

	// Unzip it into the temp directory
	err = Unzip(src, a.tempDir)
	if err != nil {
		log.Printf("Failed to extract zip archive: %v", err)
		return errors.New("Failed to decompress the payload.")
	}
	return nil
}

// Close cleans up internal storage of the archive.
func (a *Archive) Close() {
	// Delete the temporary directory
	err := os.RemoveAll(a.tempDir)
	if err != nil {
		log.Printf("Failed to remove diffs dir %s (ignored).", a.tempDir)
	}
}

// NewArchive creates a new archive. Caller is responsible for calling
// Archive.Close() so that the internal temporary directory is removed.
func NewArchive() *Archive {
	return &Archive{}
}
