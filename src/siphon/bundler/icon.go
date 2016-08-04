package bundler

import (
	"bytes"
	"fmt"
	"image/png"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"
)

type IconData struct {
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	Height      int    `json:"height"`
	Width       int    `json:"width"`
	ImageFormat string `json:"image_format"`
}

func LoadIconData(name string, platform string,
	b []byte) (iconData *IconData, err error) {
	r := bytes.NewReader(b)
	image, err := png.DecodeConfig(r)
	if err != nil {
		// There was a problem decoding the image
		return nil, fmt.Errorf(`Unsupported icon %s detected.
			All icons must have a png extension.`, name)
	}
	return &IconData{Name: name, Platform: platform,
		Height: image.Height, Width: image.Width, ImageFormat: "png"}, nil
}

// Takes a project directory and returns a slice of IconData struct pointers
func GetIcons(d string, files map[string]string) (icons []*IconData,
	err error) {
	icons = []*IconData{}
	// Extract all icons that are listed and populate our iconData slice
	for name, _ := range files {
		// tmpFiles, _ := ioutil.ReadDir(d)
		// for _, f := range tmpFiles {
		// 	log.Printf("Files: %v", f)
		// }
		ext := path.Ext(name)
		// Iterate through the platform-specific directories
		for _, platform := range []string{"android", "ios"} {
			prefix := path.Join("publish", platform, "icons")
			if strings.HasPrefix(name, prefix) {
				if ext != ".png" {
					return nil, fmt.Errorf(`Unsupported icon %s detected.
						All icons must have a png extension.`, name)
				}
				fPath := path.Join(d, name)
				b, err := ioutil.ReadFile(fPath)
				if err != nil {
					return nil, err
				}

				// Load the iconData struct
				// The name of the icon here is the one relative to its directory
				// minus the extension
				iconExtName, _ := filepath.Rel(prefix, name)
				iconName := iconExtName[0 : len(iconExtName)-len(ext)]
				iData, err := LoadIconData(iconName, platform, b)
				if err != nil {
					return nil, err
				}
				icons = append(icons, iData)
			}
		}
	}
	return icons, nil
}
