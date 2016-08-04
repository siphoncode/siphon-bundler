package bundler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// FooterPath holds packager json output
type FooterPath struct {
	IOS     string `json:"ios"`
	Android string `json:"android"`
}

// CleanupFooters takes a FooterPath struct and erases the appropriate
// directory (the footers are both saved in the same tmp directory)
func CleanupFooters(f *FooterPath) {
	if f.IOS != "" && strings.Contains(f.IOS, "siphon-packager-tmp-") {
		d := filepath.Dir(f.IOS)
		if !strings.Contains(d, "siphon-packager-tmp-") {
			log.Printf("(Ignored) footer cleanup failed: Invalid directory")
			return
		}
		if err := os.RemoveAll(d); err != nil {
			log.Printf("(Ignored) footer cleanup failed: %v", err)
		}
		return
	}

	if f.Android != "" {
		d := filepath.Dir(f.Android)
		if !strings.Contains(f.Android, "siphon-packager-tmp-") {
			log.Printf("(Ignored) footer cleanup failed: Invalid directory")
			return
		}
		if err := os.RemoveAll(d); err != nil {
			log.Printf("(Ignored) footer cleanup failed: %v", err)
		}
		return
	}
}

func cleanup(d string) {
	if err := os.RemoveAll(d); err != nil {
		log.Printf("(Ignored) MakeBundleFooter() cleanup failed: %v", err)
	}
}

func MakeBundleFooters(d string, baseVersion string) (f *FooterPath,
	err error) {
	// Build the footer and cleanup
	if os.Getenv("SIPHON_ENV") == "testing" {
		// Create dummy footer files
		tmpDir, err := ioutil.TempDir("", "siphon-packager-tmp-")
		iosFooter := path.Join(tmpDir, "bundle-footer-ios")
		androidFooter := path.Join(tmpDir, "bundle-footer-android")
		txt := "Dummy bundle footer for testing."
		os.MkdirAll(filepath.Dir(iosFooter), 0600) // make any intermediate dirs
		if ioutil.WriteFile(iosFooter, []byte(txt), 0600); err != nil {
			cleanup(tmpDir)
			return nil, err
		}
		if ioutil.WriteFile(androidFooter, []byte(txt), 0600); err != nil {
			cleanup(tmpDir)
			return nil, err
		}
		f = &FooterPath{IOS: iosFooter, Android: androidFooter}
	} else {
		b, err := packager(d, baseVersion)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(b, &f); err != nil {
			return nil, err
		}
	}
	// Return the FooterPath struct pointer
	return f, nil
}

// MakeBundleFootersTmp spins up a temporary directory and writes all of the app
// files there. If `archive` is not nil, it will attempt to grab the files
// from there first (much faster).
// (convenenience function for the same purpose as above)
func MakeBundleFootersTmp(db *sql.DB, appID string, submissionID string,
	archive *Archive, baseVersion string) (f *FooterPath, err error) {
	// We need the cache for files that do not exist in the Archive (which
	// is probably  most of them).
	cache, err := NewCache(appID, submissionID)
	if err != nil {
		return nil, err
	}
	// Get the very latest files for this app
	files, err := GetFiles(db, appID, submissionID)
	if err != nil {
		return nil, err
	}

	d, err := FilesToTemp(files, db, archive, cache)
	if err != nil {
		return nil, err
	}

	f, err = MakeBundleFooters(d, baseVersion)
	if err != nil {
		Cleanup(d)
		return nil, err
	}
	Cleanup(d)
	return f, nil
}

// Only generates the footer for now
func packager(projectPath string, baseVersion string) (b []byte, err error) {
	prefix := "source $NVM_DIR/nvm.sh && "
	c := fmt.Sprintf("siphon-packager.py --footer --project-path %s "+
		"--base-version %s --minify", projectPath, baseVersion)
	log.Printf("PACKAGING: %s", c)
	cmd := exec.Command("bash", "-c", prefix+c)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		s := stderr.String()
		log.Printf("makeBundleFooters() err: %v, stderr: %s", err, s)
		return nil, errors.New(s)
	}
	return stdout.Bytes(), nil
}
