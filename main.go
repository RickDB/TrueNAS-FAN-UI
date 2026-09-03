package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed web/*
var webFS embed.FS

type App struct {
	fans *FanManager
	cfg  *ConfigStore
}

type apiStatus struct {
	Snapshot
	Profiles    map[string]int `json:"profiles"`
	LastProfile string         `json:"last_profile"`
	MinPercent  int            `json:"min_percent"`
	CPUModel    string         `json:"cpu_model,omitempty"`
	Now         time.Time      `json:"now"`
}


func detectCPUModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		if key == "model name" || key == "hardware" {
			if model := strings.TrimSpace(parts[1]); model != "" {
				return model
			}
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func (a *App) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap, err := a.fans.Scan()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	cfg := a.cfg.Snapshot()
	writeJSON(w, 200, apiStatus{
		Snapshot:    snap,
		Profiles:    cfg.Profiles,
		LastProfile: cfg.LastProfile,
		MinPercent:  cfg.MinPercent,
		CPUModel:    detectCPUModel(),
		Now:         time.Now(),
	})
}

func (a *App) setName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/fans/")
	id = strings.TrimSuffix(id, "/name")
	if id == "" {
		writeErr(w, 400, fmt.Errorf("missing fan id"))
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if _, err := a.fans.findFan(id); err != nil {
		writeErr(w, 404, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) > 80 {
		writeErr(w, 400, fmt.Errorf("name is too long"))
		return
	}
	if err := a.cfg.SetAlias(id, name); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) setPWM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/fans/")
	id = strings.TrimSuffix(id, "/pwm")
	var req struct {
		Percent int `json:"percent"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := a.fans.SetPercent(id, req.Percent); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) restoreFan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/fans/")
	id = strings.TrimSuffix(id, "/restore")
	if err := a.fans.Restore(id); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) setFanProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/fans/")
	id = strings.TrimSuffix(id, "/profile")
	if id == "" {
		writeErr(w, 400, fmt.Errorf("missing fan id"))
		return
	}
	if _, err := a.fans.findFan(id); err != nil {
		writeErr(w, 404, err)
		return
	}

	if r.Method == http.MethodDelete {
		if err := a.cfg.DeleteFanProfile(id); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}

	var req struct {
		Percent          int  `json:"percent"`
		RestoreOnStartup bool `json:"restore_on_startup"`
		ApplyNow         bool `json:"apply_now"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	cfg := a.cfg.Snapshot()
	if req.Percent < cfg.MinPercent || req.Percent > 100 {
		writeErr(w, 400, fmt.Errorf("percent must be between %d and 100", cfg.MinPercent))
		return
	}
	if req.ApplyNow {
		if err := a.fans.SetPercent(id, req.Percent); err != nil {
			writeErr(w, 400, err)
			return
		}
	}
	if err := a.cfg.SetFanProfile(id, req.Percent, req.RestoreOnStartup); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) fanRouter(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/name"):
		a.setName(w, r)
	case strings.HasSuffix(r.URL.Path, "/pwm"):
		a.setPWM(w, r)
	case strings.HasSuffix(r.URL.Path, "/restore"):
		a.restoreFan(w, r)
	case strings.HasSuffix(r.URL.Path, "/profile"):
		a.setFanProfile(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) profile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Profile string `json:"profile"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := a.fans.ApplyProfile(strings.ToLower(strings.TrimSpace(req.Profile))); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) applyStartupProfile() {
	cfg := a.cfg.Snapshot()
	profile := strings.ToLower(strings.TrimSpace(cfg.LastProfile))
	if profile == "" {
		log.Printf("startup: no saved fan profile; leaving hardware unchanged")
		return
	}

	percent, ok := cfg.Profiles[profile]
	if !ok {
		log.Printf("startup: saved profile %q no longer exists; leaving hardware unchanged", profile)
		return
	}

	log.Printf("startup: restoring saved fan profile %q at %d%%", profile, percent)
	if err := a.fans.ApplyProfile(profile); err != nil {
		log.Printf("startup: unable to restore fan profile %q: %v", profile, err)
		return
	}
	log.Printf("startup: profile %q restored successfully", profile)
}

func (a *App) restoreAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.fans.RestoreAll(); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func main() {
	root := getenv("SYSFS_ROOT", "/host/sys")
	cfgPath := getenv("CONFIG_PATH", "/config/config.json")
	listen := getenv("LISTEN_ADDR", ":8080")

	cfg, err := NewConfigStore(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	app := &App{cfg: cfg, fans: NewFanManager(root, cfg)}

	// Re-apply the last profile explicitly chosen by the user. If no profile
	// has ever been selected, or the saved profile is invalid/unavailable,
	// startup deliberately leaves the motherboard/kernel fan state untouched.
	app.applyStartupProfile()

	// Per-fan startup profiles run after the global profile so they can
	// intentionally override it for selected headers.
	if applied, errs := app.fans.ApplyStartupFanProfiles(); applied > 0 || len(errs) > 0 {
		log.Printf("startup: restored %d per-fan custom profile(s)", applied)
		for _, msg := range errs {
			log.Printf("startup: per-fan profile failed: %s", msg)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", app.status)
	mux.HandleFunc("/api/profile", app.profile)
	mux.HandleFunc("/api/restore", app.restoreAll)
	mux.HandleFunc("/api/fans/", app.fanRouter)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok\n")) })

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(static)))

	server := &http.Server{Addr: listen, Handler: logMiddleware(mux), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("Only Fans - TrueNAS Edition listening on %s (sysfs=%s config=%s)", listen, root, cfgPath)
	log.Fatal(server.ListenAndServe())
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
