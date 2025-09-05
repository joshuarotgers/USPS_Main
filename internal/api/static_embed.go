//go:build embed_openapi

package api

import (
    _ "embed"
    "net/http"
)

//go:embed embedded/redoc.standalone.js
var redocJS []byte
//go:embed embedded/swagger-ui-bundle.js
var swaggerBundle []byte
//go:embed embedded/swagger-ui-standalone-preset.js
var swaggerPreset []byte
//go:embed embedded/swagger-ui.css
var swaggerCSS []byte
//go:embed embedded/driver.html
var driverHTML []byte
//go:embed embedded/driver.js
var driverJS []byte
//go:embed embedded/driver.css
var driverCSS []byte

// StaticHandler serves embedded static assets (Redoc)
func (s *Server) StaticHandler(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/static/redoc.standalone.js":
        w.Header().Set("Content-Type", "application/javascript")
        w.WriteHeader(200)
        _, _ = w.Write(redocJS)
    case "/static/swagger-ui-bundle.js":
        w.Header().Set("Content-Type", "application/javascript")
        w.WriteHeader(200); _, _ = w.Write(swaggerBundle)
    case "/static/swagger-ui-standalone-preset.js":
        w.Header().Set("Content-Type", "application/javascript")
        w.WriteHeader(200); _, _ = w.Write(swaggerPreset)
    case "/static/swagger-ui.css":
        w.Header().Set("Content-Type", "text/css")
        w.WriteHeader(200); _, _ = w.Write(swaggerCSS)
    case "/static/driver.js":
        w.Header().Set("Content-Type", "application/javascript")
        w.WriteHeader(200); _, _ = w.Write(driverJS)
    case "/static/driver.css":
        w.Header().Set("Content-Type", "text/css")
        w.WriteHeader(200); _, _ = w.Write(driverCSS)
    case "/app", "/app/":
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(200); _, _ = w.Write(driverHTML)
    default:
        http.NotFound(w, r)
    }
}
