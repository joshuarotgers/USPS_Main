// Package auth provides JWT verification helpers.
package auth

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Verifier validates JWTs and extracts tenant/role claims.
// Supports modes: dev (no verify), hmac (HS256), jwks (RS256 from JWKS URL).
type Verifier struct {
	Mode        string
	HMACSecret  []byte
	JWKSURL     string
	TenantClaim string
	RoleClaim   string
	DriverClaim string
	http        *http.Client
	mu          sync.RWMutex
	jwks        jwks
	lastFetch   time.Time
	cacheTTL    time.Duration
}

type jwks struct {
	Keys []jwk `json:"keys"`
}
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

type Principal struct {
	Tenant   string
	Role     string
	DriverID string
}

func NewVerifierFromEnv() *Verifier {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_MODE")))
	if mode == "" {
		mode = "dev"
	}
	v := &Verifier{
		Mode:        mode,
		HMACSecret:  []byte(os.Getenv("AUTH_HMAC_SECRET")),
		JWKSURL:     os.Getenv("AUTH_JWKS_URL"),
		TenantClaim: envOr("AUTH_TENANT_CLAIM", "tenant"),
		RoleClaim:   envOr("AUTH_ROLE_CLAIM", "role"),
		DriverClaim: envOr("AUTH_DRIVER_CLAIM", "sub"),
		http:        &http.Client{Timeout: 5 * time.Second},
		cacheTTL:    10 * time.Minute,
	}
	return v
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func (v *Verifier) Verify(token string) (Principal, error) {
	if v.Mode == "dev" {
		// token format: tenant:role
		parts := strings.Split(token, ":")
		if len(parts) >= 2 {
			return Principal{Tenant: parts[0], Role: parts[1]}, nil
		}
		return Principal{}, errors.New("invalid dev token; expected tenant:role")
	}
	// split token
	segs := strings.Split(token, ".")
	if len(segs) != 3 {
		return Principal{}, errors.New("invalid JWT")
	}
	headerJSON, err := b64urlDecode(segs[0])
	if err != nil {
		return Principal{}, err
	}
	payloadJSON, err := b64urlDecode(segs[1])
	if err != nil {
		return Principal{}, err
	}
	sig, err := b64urlDecode(segs[2])
	if err != nil {
		return Principal{}, err
	}
	var hdr map[string]any
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return Principal{}, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return Principal{}, err
	}
	alg, _ := hdr["alg"].(string)
	kid, _ := hdr["kid"].(string)
	signingInput := []byte(segs[0] + "." + segs[1])
	switch v.Mode {
	case "hmac":
		if alg != "HS256" {
			return Principal{}, errors.New("unsupported alg for hmac")
		}
		mac := hmac.New(sha256.New, v.HMACSecret)
		mac.Write(signingInput)
		if !hmac.Equal(mac.Sum(nil), sig) {
			return Principal{}, errors.New("bad signature")
		}
	case "jwks":
		if alg != "RS256" {
			return Principal{}, errors.New("unsupported alg for jwks")
		}
		pub, err := v.getRSAPublicKey(kid)
		if err != nil {
			return Principal{}, err
		}
		h := sha256.Sum256(signingInput)
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig); err != nil {
			return Principal{}, errors.New("bad signature")
		}
	default:
		return Principal{}, errors.New("unsupported auth mode")
	}
	// Extract principal
	tenant, _ := claims[v.TenantClaim].(string)
	role, _ := claims[v.RoleClaim].(string)
	driver, _ := claims[v.DriverClaim].(string)
	if tenant == "" {
		return Principal{}, errors.New("missing tenant claim")
	}
	if role == "" {
		role = "user"
	}
	return Principal{Tenant: tenant, Role: strings.ToLower(role), DriverID: driver}, nil
}

func b64urlDecode(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// get RSAPublicKey from JWKS cache/fetch
func (v *Verifier) getRSAPublicKey(kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	cached := v.jwks
	stale := time.Since(v.lastFetch) > v.cacheTTL
	v.mu.RUnlock()
	if len(cached.Keys) == 0 || stale {
		if err := v.fetchJWKS(); err != nil {
			return nil, err
		}
		v.mu.RLock()
		cached = v.jwks
		v.mu.RUnlock()
	}
	for _, k := range cached.Keys {
		if k.Kid == kid && strings.EqualFold(k.Kty, "RSA") {
			nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				return nil, err
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				return nil, err
			}
			e := big.NewInt(0)
			// exponent may be 3 or 65537; e is big-endian
			e.SetInt64(int64(bytesToInt(eBytes)))
			n := new(big.Int).SetBytes(nBytes)
			return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
		}
	}
	return nil, errors.New("kid not found in JWKS")
}

func bytesToInt(b []byte) int {
	// small helper; for typical e bytes like 0x010001
	var x int
	for _, v := range b {
		x = (x << 8) | int(v)
	}
	return x
}

func (v *Verifier) fetchJWKS() error {
	if v.JWKSURL == "" {
		return errors.New("AUTH_JWKS_URL not set")
	}
	req, _ := http.NewRequest(http.MethodGet, v.JWKSURL, nil)
	resp, err := v.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var j jwks
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return err
	}
	v.mu.Lock()
	v.jwks = j
	v.lastFetch = time.Now()
	v.mu.Unlock()
	return nil
}

// nothing further
