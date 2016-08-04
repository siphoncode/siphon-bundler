package bundler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/context"
)

// HashesResponse represents an app's files (names mapped to SHA-256 hashes).
type HashesResponse struct {
	Files map[string]string `json:"hashes"`
}

// MakeJSONHashes returns the JSON bytes representation of the current
// hashes stored for the given App ID.
func MakeJSONHashes(appID string) (HashesResponse, error) {
	db := OpenDB()
	defer db.Close()

	files, err := GetFiles(db, appID, "")
	if err != nil {
		return HashesResponse{}, err
	}
	obj := HashesResponse{files}
	return obj, nil
}

// Writes a JSON response containing the SHA-256 hashes that we
// currently have stored for the specified app.
func writeHashesResponse(w http.ResponseWriter, appID string) {
	w.Header().Set("Content-Type", "application/json")
	hashesResponse, err := MakeJSONHashes(appID)
	if err != nil {
		log.Printf("Failed to fetch hashes: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}
	b, err := json.Marshal(hashesResponse)
	if err != nil {
		log.Printf("Failed to serialize hashes: %v", err)
		http.Error(w, "Internal error.", 500)
		return
	}
	w.Write(b)
}

// protectHash returns true if the map `files` (i.e. a listing of current
// files associated with an app) contains one or more hashes equal to `hash`
// excluding cases where that hash is mapped to the name given in `name`,
// because that's the file we're deleting.
func protectHash(files map[string]string, name string, hash string) bool {
	for k, v := range files {
		if v == hash && k != name {
			return true
		}
	}
	return false
}

type pushHandler struct {
	request  *http.Request
	response http.ResponseWriter

	appID   string
	userID  string
	db      *sql.DB
	cache   *Cache
	archive *Archive
	dirty   bool // signals that we need to generate a new bundle footer

	// Metadata (i.e. contents of Siphonfile)
	metadata      *Metadata
	icons         []*IconData
	metadataDirty bool // true indicates that a PUT needs to happen
}

func newPushHandler(w http.ResponseWriter, r *http.Request,
	appID string, userID string) (
	h *pushHandler, err error) {
	cache, err := NewCache(appID, "")
	if err != nil {
		return nil, err
	}
	return &pushHandler{
		request:       r,
		response:      w,
		appID:         appID,
		userID:        userID,
		cache:         cache,
		dirty:         false,
		metadataDirty: false,
	}, nil
}

func (h *pushHandler) remove(names []string) error {
	for _, name := range names {
		h.log("--> " + name) // log progress to the user
		// We need the current hash we have stored for this file
		files, _ := GetFiles(h.db, h.appID, "")
		hash, ok := files[name]
		if !ok || hash == "" {
			return fmt.Errorf("No hash for file removal: %s", name)
		}
		// Delete this hash from S3/memcache, but importantly only if we
		// do not have another file in this app with the same hash!
		if !protectHash(files, name, hash) {
			if err := h.cache.Delete(hash); err != nil {
				return err
			}
		}
		// Delete the file row locally
		if err := DeleteFile(h.db, h.appID, "", name); err != nil {
			return err
		}
		h.dirty = true
	}
	return nil
}

// If add == false, then we will UPDATE the file row, not INSERT a new one.
func (h *pushHandler) update(names []string, add bool) error {
	for _, name := range names {
		h.log("--> " + name) // log progress to the user
		// Retrieve the file content from the archive for this name
		hash := h.archive.GetHash(name)
		b, err := h.archive.GetContent(name)
		if hash == "" {
			return fmt.Errorf("Hash not found for name: %s", name)
		} else if err != nil {
			return fmt.Errorf("Get content for hash %s failed: %v", hash, err)
		}
		// Write the file to S3 and memcache
		if err := h.cache.Set(hash, b); err != nil {
			return err
		}
		// Add or update the file row in our local database
		if add {
			err = AddFile(h.db, h.appID, "", name, hash)
		} else {
			err = UpdateFile(h.db, h.appID, "", name, hash)
		}
		if err != nil {
			return err
		}
		h.dirty = true
	}
	return nil
}

func (h *pushHandler) log(s string) {
	BufferLine(h.response, s)
}

func (h *pushHandler) decompress(r *http.Request) error {
	b, err := ioutil.ReadAll(r.Body) // we must read before we write
	h.log("Decompressing...")
	archive := NewArchive()
	err = archive.Decompress(b)
	if err != nil {
		archive.Close()
		return err
	}
	h.archive = archive
	return nil
}

// For when the error message is not suitable for displaying to the user,
// instead we show 'Internal error' to them.
func (h *pushHandler) internalError(err error, debug string) {
	log.Printf("[pushHandler() error] %s: %v [type=%T]", debug, err, err)
	http.Error(h.response, "Internal error.", 500)
}

// For when the error message is appropriate to show to the user.
func (h *pushHandler) expectedError(err error) {
	log.Printf("[pushHandler() user-facing error] %v [type=%T]", err, err)
	http.Error(h.response, "[ERROR] "+err.Error(), 500)
}

// putMetadata initiates the PUT request to Django to change
// certain metadata (e.g. the base_version). This should only be called
// after a footer has been successfully generated.
func (h *pushHandler) putMetadata() error {
	// Prepare the request
	endpoint := fmt.Sprintf("https://%s/api/v1/apps/%s",
		os.Getenv("WEB_HOST"), h.appID)
	v := url.Values{}
	v.Set("base_version", h.metadata.BaseVersion)
	v.Set("display_name", h.metadata.DisplayName)
	v.Set("facebook_app_id", h.metadata.FacebookAppID)
	v.Set("app_store_name", h.metadata.IOS.StoreName)
	v.Set("play_store_name", h.metadata.Android.StoreName)
	v.Set("app_store_language", h.metadata.IOS.Language)
	v.Set("play_store_language", h.metadata.Android.Language)

	iconsList, err := json.Marshal(h.icons)
	if err != nil {
		log.Printf("[putMetadata() error marshalling icons] %v", err)
		return errors.New("Internal error while updating app metadata.")
	}

	v.Set("icons", string(iconsList))

	// If we're testing, print the put content and return nil
	if os.Getenv("SIPHON_ENV") == "testing" {
		log.Printf("User icons: %v", string(iconsList))
		log.Printf("PUT data: %v", v.Encode())
		return nil
	}

	req, err := http.NewRequest("PUT", endpoint, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Authentication headers
	token := context.Get(h.request, HandshakeTokenKey).(string)
	signature := context.Get(h.request, HandshakeSignatureKey).(string)
	req.Header.Set("X-Siphon-Handshake-Token", url.QueryEscape(token))
	req.Header.Set("X-Siphon-Handshake-Signature", url.QueryEscape(signature))

	// Fire off the request
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		log.Printf("[putMetadata() strange put error] %v", err)
		return errors.New("Internal error while updating app metadata.")
	}

	// Anything other than an HTTP 200 response indicates an issue.
	if response.StatusCode != 200 {
		defer response.Body.Close()
		s, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Printf("[putMetadata() read error] %v [HTTP %v]", err,
				response.StatusCode)
			return errors.New("Internal error while updating app metadata.")
		}
		return fmt.Errorf("Problem updating metadata: %s [%v]", s,
			response.StatusCode)
	}
	// If we got this far, the PUT was all good.
	return nil
}

func (h *pushHandler) loadMetaData() error {
	h.log("Checking your Siphonfile...")
	// First try to load it from the archive (it's only present if it changed)
	b, err := h.archive.GetMetadata()
	if err != nil {
		log.Printf("[Archive.GetMetadata() error] %v", err)
		return errors.New("Problem loading Siphonfile from the archive.")
	}
	// If the metadata was loaded from the archive (i.e. it has chnaged),
	// then mark it as dirty so it gets PUT later.
	if b != nil {
		h.metadataDirty = true
	}
	// If it wasn't in the archive, we need to load it from the cache
	if b == nil {
		// We need the SHA-256 hash to look it up with
		hash, err := GetFile(h.db, h.appID, "", MetadataName)
		if err != nil {
			log.Printf("[GetFile() metadata error] %v", err)
			return errors.New("Problem loading Siphonfile from the cache.")
		}
		res, err := h.cache.Get(hash)
		if err != nil {
			log.Printf("[Cache.Get() metadata error] %v, hash=%s", err, hash)
			return errors.New("Problem loading Siphonfile from the cache.")
		}
		b = res
	}
	// Parse the JSON and check for issues
	metadata, err := ParseMetadata(b)
	if err != nil {
		return err
	}
	// We're good to go
	h.metadata = metadata
	return nil
}

func (h *pushHandler) handle(r *http.Request) {
	// Decompress the payload
	if err := h.decompress(r); err != nil {
		h.internalError(err, "decompress()")
		return
	}
	defer h.archive.Close() // clean up the archive's temporary directory

	// Load and check the Siphonfile metadata from the archive or cache
	err := h.loadMetaData()
	if err != nil {
		h.expectedError(err)
		return
	}

	// Compare the archive's listing to our current hashes for this app
	files, _ := GetFiles(h.db, h.appID, "")
	comp, err := h.archive.Compare(files)
	if err != nil {
		h.internalError(err, "archive.Compare()")
		return
	}

	// Process additions
	if len(comp.added) > 0 {
		h.log("Adding files...")
		if err := h.update(comp.added, true); err != nil {
			h.internalError(err, "update() add=true")
			return
		}
	}

	// Process changes
	if len(comp.changed) > 0 {
		h.log("Updating files...")
		if err := h.update(comp.changed, false); err != nil {
			h.internalError(err, "update() add=false")
			return
		}
	}

	// Process removals
	if len(comp.removed) > 0 {
		h.log("Removing deleted files...")
		if err := h.remove(comp.removed); err != nil {
			h.internalError(err, "remove()")
			return
		}
	}

	// Copy all the app's files into a temp directory. We use these
	// files to check icon data and create the bundle footer.
	cache, err := NewCache(h.appID, "")
	if err != nil {
		h.expectedError(err)
		return
	}

	if !h.dirty && !h.metadataDirty {
		h.log("No changes detected.")
		return
	}

	files, err = GetFiles(h.db, h.appID, "")
	d, err := FilesToTemp(files, h.db, h.archive, cache)
	if err != nil {
		h.internalError(err, "FilesToTemp()")
		return
	}

	icons, err := GetIcons(d, files)
	if err != nil {
		Cleanup(d)
		h.expectedError(err)
		return
	}

	h.icons = icons

	// Generate new bundle footers if we got this far
	h.log("Building diffs...")
	f, err := MakeBundleFooters(d, h.metadata.BaseVersion)
	if err != nil {
		// Clean up our temp dir
		Cleanup(d)
		h.expectedError(err)
		return
	}
	Cleanup(d)
	// f, err := MakeBundleFooters(h.db, h.appID, "", h.archive,
	// 	h.metadata.BaseVersion)
	// if err != nil {
	// 	h.expectedError(err)
	// 	return
	// }

	// The bundle footers were generated successfully, so now we can PUT
	// the Siphonfile metadata to Django (e.g. base_version). We do this
	// regardless of whether the metadata has changed, so that
	// 'last pushed ...' always gets bumped in the user's dashboard.
	// if os.Getenv("SIPHON_ENV") != "testing" {
	// 	if err := h.putMetadata(); err != nil {
	// 		h.expectedError(err)
	// 		return
	// 	}
	// }

	if err := h.putMetadata(); err != nil {
		h.expectedError(err)
		return
	}

	// Write the footers to S3 (and memcache) and clean up the footers if we
	// encounter any errors
	if f.IOS != "" {
		ftr, err := ioutil.ReadFile(f.IOS)
		if err != nil {
			h.internalError(err, "ReadFile()")
			CleanupFooters(f)
			return
		}
		if err := h.cache.SetBundleFooter(ftr, BundleFooterFile("ios")); err != nil {
			h.internalError(err, "SetBundleFooter()")
			CleanupFooters(f)
			return
		}
	}

	if f.Android != "" {
		ftr, err := ioutil.ReadFile(f.Android)
		if err != nil {
			h.internalError(err, "ReadFile()")
			CleanupFooters(f)
			return
		}
		if err := h.cache.SetBundleFooter(ftr, BundleFooterFile("android")); err != nil {
			h.internalError(err, "SetBundleFooter()")
			CleanupFooters(f)
			return
		}
	}
	h.log("Done.")

	CleanupFooters(f)

	// Post an app update notification (fails silently)
	PostAppUpdated(h.appID, h.userID)
}

// Push handles a response for the /push route
func Push(w http.ResponseWriter, r *http.Request) {
	// Note that a client can only push development files, not files
	// for a submission (for now)
	appID := context.Get(r, AppIDKey).(string)
	userID := context.Get(r, UserIDKey).(string)

	if r.Method == "GET" {
		writeHashesResponse(w, appID)
	} else if r.Method == "POST" {
		// defer to pushHandler() to service this request
		h, err := newPushHandler(w, r, appID, userID)
		if err != nil {
			http.Error(w, "Internal error.", 500)
			return
		}
		db := OpenDB() // we create/assign like this so we can use defer
		defer db.Close()
		h.db = db
		h.handle(r)
	} else {
		http.Error(w, "Expected GET or POST.", 500)
	}
}
