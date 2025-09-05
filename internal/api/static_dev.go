//go:build !embed_openapi

package api

import (
    "net/http"
    "os"
    "path/filepath"
)

// StaticHandler serves static assets from ./static in dev, if present
func (s *Server) StaticHandler(w http.ResponseWriter, r *http.Request) {
    base := "static"
    name := filepath.Base(r.URL.Path)
    switch name {
    case "redoc.standalone.js", "swagger-ui-bundle.js", "swagger-ui-standalone-preset.js", "swagger-ui.css":
        p := filepath.Join(base, name)
        if _, err := os.Stat(p); err != nil { http.NotFound(w, r); return }
        http.ServeFile(w, r, p)
    case "driver.js", "driver.css":
        p := filepath.Join(base, name)
        if _, err := os.Stat(p); err != nil { http.NotFound(w, r); return }
        http.ServeFile(w, r, p)
    default:
        if r.URL.Path == "/app" || r.URL.Path == "/app/" {
            p := filepath.Join(base, "driver.html")
            if _, err := os.Stat(p); err != nil { http.NotFound(w, r); return }
            http.ServeFile(w, r, p); return
        }
        http.NotFound(w, r)
    }
}
