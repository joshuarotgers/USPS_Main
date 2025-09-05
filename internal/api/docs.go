package api

import (
    "net/http"
    "os"
)

// openAPILoad loads the OpenAPI bytes (dev default reads from disk).
func openAPILoad() ([]byte, error) {
    return os.ReadFile("openapi/openapi.yaml")
}

// OpenAPIHandler serves the OpenAPI spec
func (s *Server) OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
    b, err := openAPILoad()
    if err != nil { writeProblem(w, 500, "OpenAPI not available", err.Error(), r.URL.Path); return }
    w.Header().Set("Content-Type", "application/yaml")
    w.WriteHeader(200)
    _, _ = w.Write(b)
}

// DocsHandler serves a minimal ReDoc page referencing /openapi.yaml
func (s *Server) DocsHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(200)
    _, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>API Docs</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <script src="https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"></script>
    </head><body>
    <redoc spec-url="/openapi.yaml"></redoc>
    </body></html>`))
}
