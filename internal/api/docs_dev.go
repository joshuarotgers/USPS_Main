//go:build !embed_openapi

package api

import "os"

// openAPILoad loads the OpenAPI spec from the repo path (dev mode)
func openAPILoad() ([]byte, error) { return os.ReadFile("openapi/openapi.yaml") }
