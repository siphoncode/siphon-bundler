package bundler

import (
	"database/sql"
	"errors"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/context"
)

type submitHandler struct {
	request      *http.Request
	response     http.ResponseWriter
	db           *sql.DB
	appID        string
	submissionID string
	metadata     *Metadata

	devCache        *Cache
	submissionCache *Cache
}

func newSubmitHandler(w http.ResponseWriter, r *http.Request,
	appID string, submissionID string) (
	h *submitHandler, err error) {

	// We take advantage of the abstractions provided by our Cache class
	// to handle the copying of files in S3, but note that it only writes
	// to S3 and memcached, not to postgres (we do that later).
	devCache, err := NewCache(appID, "")
	submissionCache, err := NewCache(appID, submissionID)
	if err != nil {
		return nil, err
	}
	return &submitHandler{
		request:         r,
		response:        w,
		appID:           appID,
		submissionID:    submissionID,
		devCache:        devCache,
		submissionCache: submissionCache,
	}, nil
}

func (h *submitHandler) internalError(err error, debug string) {
	log.Printf("[submitHandler() error] %s: %v [type=%T]", debug, err, err)
	http.Error(h.response, "Internal error.", 500)
}

// Generates new bundle footers and stores them against this submission ID in
// S3. You should only call this after the files have been copied in S3.
func (h *submitHandler) makeBundleFooter(platform string) error {
	// Note that because the file rows have not be copied to the submission_id
	// namespace in postgres yet, we need to run this against the app files,
	// which is fine because they're identical.
	f, err := MakeBundleFootersTmp(h.db, h.appID, "", nil, h.metadata.BaseVersion)

	if err != nil {
		return err
	}

	// Write the footer to s3/cache and remove the temporary ones created
	// by the packager
	if platform == "ios" && f.IOS != "" {
		ftr, err := ioutil.ReadFile(f.IOS)
		if err != nil {
			CleanupFooters(f)
			return err
		}
		if err := h.submissionCache.SetBundleFooter(ftr, BundleFooterFile(platform)); err != nil {
			CleanupFooters(f)
			return err
		}

	} else if platform == "android" && f.Android != "" {
		ftr, err := ioutil.ReadFile(f.Android)
		if err != nil {
			CleanupFooters(f)
			return err
		}
		if err := h.submissionCache.SetBundleFooter(ftr, "bundle-footer"); err != nil {
			CleanupFooters(f)
			return err
		}
	} else {
		CleanupFooters(f)
		return errors.New("Problem building footer for platform.")
	}

	CleanupFooters(f)
	return nil
}

// Retrieves the metadata (i.e. Siphonfile) stored for this app, because we
// need the "base_version" to generate a bundle footer. You must only call
// this after the files have been copied in S3.
func (h *submitHandler) loadMetaData() error {
	// We need the SHA-256 hash of the Siphonfile so that we can grab
	// it from the cache. Note that we can't do this query against the
	// submission_id because the postgres rows haven't been copied yet.
	// But that's OK because the hash is identical.
	hash, err := GetFile(h.db, h.appID, "", MetadataName)
	if err != nil {
		log.Printf("[makeBundleFooter() GetFile error] %v", err)
		return errors.New("Problem loading Siphonfile from the cache.")
	}
	b, err := h.submissionCache.Get(hash)
	if err != nil {
		log.Printf("[makeBundleFooter() cache error] %v, hash=%s", err, hash)
		return errors.New("Problem loading Siphonfile from the cache.")
	}
	// Parses the JSON and check for issues
	metadata, err := ParseMetadata(b)
	if err != nil {
		return err
	}
	h.metadata = metadata
	return nil
}

func (h *submitHandler) copyFiles() error {
	// Copy the files to a submission namespace in S3 (as a side effect,
	// also copies it in memcache).
	files, err := GetFiles(h.db, h.appID, "")
	if err != nil {
		log.Printf("[copyFiles() get files error]: %v", err)
		return errors.New("Problem copying files for the snapshot")
	}
	for _, hash := range files {
		b, err := h.devCache.Get(hash)
		if err != nil {
			log.Printf("[copyFiles() cache retrieve error]: %v", err)
			return errors.New("Problem copying files for the snapshot")
		}
		err = h.submissionCache.Set(hash, b)
		if err != nil {
			log.Printf("[copyFiles() cache error]: %v", err)
			return errors.New("Problem copying files for the snapshot")
		}
	}
	return nil
}

func (h *submitHandler) handle(r *http.Request) {
	// Copy a snapshot of the files in S3 first.
	if err := h.copyFiles(); err != nil {
		h.internalError(err, "copyFiles()")
		return
	}

	// Then grab the metadata (we need it for "base_version", which is
	// needed to generate the bundle footer).
	if err := h.loadMetaData(); err != nil {
		h.internalError(err, "loadMetaData()")
		return
	}

	// Now generate a brand new bundle footer. Note that we have to do this
	// here, rather than simply doing a blind copy, because /push is not
	// properly atomic yet and the app source files may not match the bundle
	// footer that's stored.
	platform := r.PostFormValue("platform")
	if platform == "" {
		platform = "ios"
	}
	if err := h.makeBundleFooter(platform); err != nil {
		h.internalError(err, "makeBundleFooters()")
		return
	}

	// If we got this far, all is good so copy the rows in postgres.
	if err := MakeSnapshot(h.db, h.appID, h.submissionID); err != nil {
		h.internalError(err, "MakeSnapshot()")
		return
	}
}

// Submit handles the response for the /submit route, which is used to
// make a snapshot of an app's current file listing, namespaced to a given
// submission ID (one that is generated by the caller).
func Submit(w http.ResponseWriter, r *http.Request) {
	// Extract the required parameters. Note that `submission_id` comes from
	// the POST payload.
	appID := context.Get(r, AppIDKey).(string)
	submissionID := r.PostFormValue("submission_id")
	if appID == "" || submissionID == "" {
		http.Error(w, "App ID and submission ID are required.",
			http.StatusBadRequest)
		return
	}

	db := OpenDB() // we create/assign like this so we can use defer
	defer db.Close()

	// Fail if this submission ID already exists in the database.
	exists, err := SubmissionExists(db, submissionID)
	if err != nil {
		http.Error(w, "Internal error.", 500)
		return
	} else if exists {
		http.Error(w, "Submission ID already exists.",
			http.StatusBadRequest)
		return
	}

	// Verify that the given app ID exists.
	exists, err = AppExists(db, appID)
	if err != nil {
		http.Error(w, "Internal error.", 500)
		return
	} else if !exists {
		http.Error(w, "App ID does not exist.",
			http.StatusBadRequest)
		return
	}

	// Ensure that the submission ID in the payload matches the one
	// in the handshake.
	if context.Get(r, SubmissionIDKey) != submissionID {
		http.Error(w, "Submission ID does not match the handshake.",
			http.StatusBadRequest)
		return
	}

	h, err := newSubmitHandler(w, r, appID, submissionID)
	if err != nil {
		http.Error(w, "Internal error.", 500)
		return
	}
	h.db = db
	h.handle(r)
}
