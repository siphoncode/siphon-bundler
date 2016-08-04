package bundler

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
)

// The file in an app directory where we expect to find the metadata
const MetadataName string = "Siphonfile"

type PlatformMetadata struct {
	Language  string `json:"language"`
	StoreName string `json:"store_name"`
}

// Metadata represents the parsed content of a Siphonfile and Publish
// directory
type Metadata struct {
	BaseVersion   string           `json:"base_version"`
	DisplayName   string           `json:"display_name"`
	FacebookAppID string           `json:"facebook_app_id"`
	IOS           PlatformMetadata `json:"ios"`
	Android       PlatformMetadata `json:"android"`
}

// ParseMetadata loads a raw Siphonfile, parses it from JSON and checks
// its values. It raises an error if (a) the JSON was malformed, (b) there
// are unknown keys or (c) one of the values is invalid
func ParseMetadata(b []byte) (metadata *Metadata, err error) {
	var m Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("[ParseMetadata() error] %v", err)
		return nil, fmt.Errorf("The format of your %s is invalid. "+
			"Please check the documentation.", MetadataName)
	}
	// Validate the "base_version" key
	if m.BaseVersion == "" {
		return nil, fmt.Errorf(`The "base_version" key in your %s is empty, `+
			`it should be a string like "0.1".`,
			MetadataName)
	}
	if _, err := strconv.ParseFloat(m.BaseVersion, 64); err != nil {
		return nil, fmt.Errorf(`The "base_version" key in your %s is not in `+
			`the correct format, it should be a string like "0.1".`,
			MetadataName)
	}
	// Validate the optional "display_name" key
	if len(m.DisplayName) > 32 {
		return nil, fmt.Errorf(`The "display_name" key in your %s is too `+
			`long. The maximum is 32 characters.`,
			MetadataName)
	}

	// Validate the optional "facebook_app_id" key. Anything longer than
	// 32 chars is definitely erroneous (although it doesn't seem like there's
	// a specific length).
	if len(m.FacebookAppID) > 32 {
		return nil, fmt.Errorf(`The "facebook_app_id" key in your %s is too `+
			`long. You can find the app ID in your app's dashboard at `+
			`https://developers.facebook.com`,
			MetadataName)
	}

	// Validate the optional iOS metadata
	if len(m.IOS.StoreName) > 255 {
		return nil, fmt.Errorf(`The iOS "store_name" key in your %s is too `+
			`long. The maximum is 255 characters.`,
			MetadataName)
	}
	if len(m.IOS.Language) > 7 {
		return nil, fmt.Errorf(`The iOS "language" key in your %s is too `+
			`long. The maximum is 7 characters.`,
			MetadataName)
	}

	// Validate the optional Android metadata
	if len(m.Android.StoreName) > 30 {
		return nil, fmt.Errorf(`The Android "store_name" key in your %s is too `+
			`long. The maximum is 30 characters.`,
			MetadataName)
	}
	if len(m.Android.Language) > 7 {
		return nil, fmt.Errorf(`The Android "language" key in your %s is too `+
			`long. The maximum is 7 characters.`,
			MetadataName)
	}

	return &m, nil
}
