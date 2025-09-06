package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	yaml "gopkg.in/yaml.v3"
)

// OpenAPIHandler serves the OpenAPI spec
func (s *Server) OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	b, err := openAPILoad()
	if err != nil {
		writeProblem(w, 500, "OpenAPI not available", err.Error(), r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(200)
	_, _ = w.Write(b)
}

// DocsHandler serves a minimal ReDoc page referencing /openapi.yaml
func (s *Server) DocsHandler(w http.ResponseWriter, r *http.Request) {
	// Load OpenAPI YAML and inline as JSON for Redoc.init to avoid fetch/CORS issues
	data, err := openAPILoad()
	if err != nil {
		writeProblem(w, 500, "OpenAPI not available", err.Error(), r.URL.Path)
		return
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		writeProblem(w, 500, "OpenAPI parse failed", err.Error(), r.URL.Path)
		return
	}
	js, err := json.Marshal(obj)
	if err != nil {
		writeProblem(w, 500, "OpenAPI marshal failed", err.Error(), r.URL.Path)
		return
	}
	b64 := base64.StdEncoding.EncodeToString(js)
	html := `<!DOCTYPE html><html lang="en"><head><title>API Docs</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>body{margin:0;padding:0;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif}</style>
    <script src="/static/redoc.standalone.js"></script>
    </head><body>
    <div id="redoc"></div>
    <script>const spec=JSON.parse(atob('` + b64 + `')); Redoc.init(spec, {}, document.getElementById('redoc'));</script>
    <noscript><p style="padding:1rem">Docs require JavaScript. You can download the spec at <a href="/openapi.yaml">/openapi.yaml</a>.</p></noscript>
    </body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(html))
}
