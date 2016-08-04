package bundler

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
)

// Keys for gorilla/context
const AppIDKey int = 1000
const UserIDKey int = 1001
const SubmissionIDKey int = 1002
const HandshakeTokenKey int = 1003
const HandshakeSignatureKey int = 1004

type handshake struct {
	Action       string `json:"action"`
	AppID        string `json:"app_id"`
	UserID       string `json:"user_id"`
	SubmissionID string `json:"submission_id"`
}

func decryptToken(token string) (decryptedTkn *handshake, err error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(decodedBytes, &decryptedTkn)
	if err != nil {
		return nil, err
	}
	return decryptedTkn, nil
}

func verifyToken(token string, signature string) bool {
	// We always allow it through for tests
	if os.Getenv("SIPHON_ENV") == "testing" {
		return true
	}

	b, err := ioutil.ReadFile("/code/.keys/handshake/handshake.pub")
	if err != nil {
		log.Printf("Problem opening handshake key file: %v", err)
		return false
	}

	block, _ := pem.Decode(b)
	re, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Printf("Problem parsing handshake key file: %v", err)
		return false
	}
	key := re.(*rsa.PublicKey)

	h := sha256.New()
	s, _ := base64.StdEncoding.DecodeString(token)
	h.Write(s)
	hashed := h.Sum(nil)

	err = rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed, []byte(signature))
	if err != nil {
		log.Printf("VerifyPKCS1v15() error: %v", err)
		return false
	}
	return true // if we got this far, the signature is valid.
}

// AuthMiddleware checks for a valid "handshake_token" GET parameter in the
// request and fails the request if needed.
// See: http://elithrar.github.io/article/map-string-interface/
func AuthMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Save our gorilla/context vars here
		vars := mux.Vars(r)
		appID := vars["app_id"]
		context.Set(r, AppIDKey, appID)

		// Extract the handshake parameters and persist them for later use
		r.ParseForm()
		token := r.FormValue("handshake_token")
		signature := r.FormValue("handshake_signature")
		context.Set(r, HandshakeTokenKey, token)
		context.Set(r, HandshakeSignatureKey, signature)

		// Verify the handshake
		obj, err := decryptToken(token)
		if err != nil || !verifyToken(token, signature) {
			http.Error(w, "Missing or invalid handshake_token.",
				http.StatusUnauthorized)
			return
		}

		// Verify the "action" matches the endpoint.
		validActions := []string{"push", "pull", "submit"}
		foundAction := false
		for _, action := range validActions {
			if obj.Action == action {
				path := fmt.Sprintf("/v1/%s/", action)
				if !strings.HasPrefix(r.URL.Path, path) {
					http.Error(w, "Unauthorized action for this endpoint.",
						http.StatusUnauthorized)
					return
				}
				foundAction = true
				break
			}
		}
		if !foundAction {
			http.Error(w, "Missing action.",
				http.StatusUnauthorized)
			return
		}

		if obj.SubmissionID != "" {
			// This is a production handshake, the user_id should not be
			// set in the handshake.
			if obj.UserID != "" {
				http.Error(w, "Malformed handshake_token.",
					http.StatusUnauthorized)
				return
			}
			context.Set(r, SubmissionIDKey, obj.SubmissionID)
		} else {
			// Otherwise assume this is a development handshake, in which case
			// the "user_id" should be set.
			if obj.UserID == "" {
				http.Error(w, "Malformed handshake_token.",
					http.StatusUnauthorized)
				return
			}
			context.Set(r, UserIDKey, obj.UserID)
		}

		// The handshake must match the requested app ID (i.e. the one in
		// the endpoint URL itself).
		if obj.AppID != appID {
			http.Error(w, "The handshake_token does not match.",
				http.StatusUnauthorized)
			return
		}

		// If we got this far then the user is authorized
		h.ServeHTTP(w, r)
	}
}
