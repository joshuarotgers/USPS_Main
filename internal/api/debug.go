package api

import (
    "encoding/json"
    "net/http"
    "os"
    "time"

    "gpsnav/internal/buildinfo"
)

func (s *Server) DebugJSON(w http.ResponseWriter, r *http.Request) {
    info := map[string]any{
        "build": buildinfo.Info(),
        "time":  time.Now().UTC().Format(time.RFC3339),
        "config": map[string]any{
            "PORT": os.Getenv("PORT"),
            "AUTH_MODE": os.Getenv("AUTH_MODE"),
            "ALLOW_ORIGINS": os.Getenv("ALLOW_ORIGINS"),
            "RATE_RPS": os.Getenv("RATE_RPS"),
            "RATE_BURST": os.Getenv("RATE_BURST"),
            "WEBHOOK_MAX_ATTEMPTS": os.Getenv("WEBHOOK_MAX_ATTEMPTS"),
            "HAS_DATABASE_URL": os.Getenv("DATABASE_URL") != "",
            "HAS_REDIS_URL": os.Getenv("REDIS_URL") != "",
        },
    }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(info)
}

