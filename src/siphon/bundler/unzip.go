// Derived from: https://github.com/pivotal-golang/archiver
// (but fixed to disallow malicious paths in the zip file.)

package bundler

import (
	"archive/zip"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// Unzip extracts a .zip file at `src` into a directory `dest`.
func Unzip(src string, dest string) error {
	files, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer files.Close()

	for _, file := range files.File {
		err = func() error {
			readCloser, err := file.Open()
			if err != nil {
				return err
			}
			defer readCloser.Close()

			return extractFile(file, dest, readCloser)
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractFile(file *zip.File, dest string, input io.Reader) error {
	// Security checks
	name := file.Name
	if strings.Contains(name, "..") {
		return errors.New("Bad zip listing name: " + file.Name)
	}

	name = filepath.Clean(strings.Replace(name, "../", "", -1)) // security
	filePath := filepath.Join(dest, name)
	fileInfo := file.FileInfo()

	if fileInfo.IsDir() {
		err := os.MkdirAll(filePath, fileInfo.Mode())
		if err != nil {
			return err
		}
	} else {
		err := os.MkdirAll(filepath.Dir(filePath), 0755)
		if err != nil {
			return err
		}

		if fileInfo.Mode()&os.ModeSymlink != 0 {
			linkName, err := ioutil.ReadAll(input)
			if err != nil {
				return err
			}
			return os.Symlink(string(linkName), filePath)
		}

		fileCopy, err := os.OpenFile(
			filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fileInfo.Mode())
		if err != nil {
			return err
		}
		defer fileCopy.Close()

		_, err = io.Copy(fileCopy, input)
		if err != nil {
			return err
		}
	}

	return nil
}
