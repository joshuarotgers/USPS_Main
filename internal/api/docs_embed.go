//go:build embed_openapi

package api

import _ "embed"

//go:embed ../../openapi/openapi.yaml
var openAPIEmbedded []byte

func openAPILoad() ([]byte, error) { return openAPIEmbedded, nil }

