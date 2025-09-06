package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// VerifyHMAC checks an HMAC-SHA256 signature over the raw body using the shared secret.
func VerifyHMAC(secret string, body []byte, provided string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	b, err := hex.DecodeString(provided)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, b)
}

// SignHMAC returns lowercase hex of HMAC-SHA256 for use in headers
func SignHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("%x", mac.Sum(nil))
}
