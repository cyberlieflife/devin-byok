package localapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"devin-byok/internal/config"
)

func (s *Server) registerExtraAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/metrics", s.handleAPIMetrics)
	mux.HandleFunc("/api/logs", s.handleAPILogs)
	mux.HandleFunc("/api/families", s.handleAPIFamilies)
}

func (s *Server) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, metricsSnapshot())
}

func (s *Server) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	snap := metricsSnapshot()
	writeJSON(w, map[string]any{"logs": snap["logs"]})
}

func (s *Server) handleAPIFamilies(w http.ResponseWriter, r *http.Request) {
	path := s.getConfigPath()
	switch r.Method {
	case http.MethodGet:
		cfg := s.GetConfig()
		writeJSON(w, map[string]any{"families": cfg.GroupModelsByFamily(), "default_model": cfg.DefaultModelID()})
	case http.MethodPost, http.MethodPut:
		var body config.FamilyUpsertInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if path == "" {
			http.Error(w, "no config path", 500)
			return
		}
		cfg, err := config.Load(path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		cfg.UpsertFamilyPresets(body)
		if err := config.Save(path, cfg); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.ReloadConfig(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "families": s.GetConfig().GroupModelsByFamily()})
	case http.MethodDelete:
		uid := strings.TrimSpace(r.URL.Query().Get("uid"))
		if uid == "" {
			http.Error(w, "uid required", 400)
			return
		}
		cfg, err := config.Load(path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// remove family + models
		fams := make([]config.FamilyConfig, 0)
		for _, f := range cfg.Upstream.Families {
			fu := f.UID
			if fu == "" {
				fu = config.SlugID(f.Label)
			}
			if fu != uid {
				fams = append(fams, f)
			}
		}
		cfg.Upstream.Families = fams
		ms := make([]config.ModelEntry, 0)
		for _, m := range cfg.Upstream.Models {
			mu := m.FamilyUID
			if mu == "" {
				mu = config.SlugID(m.Family)
			}
			if mu != uid {
				ms = append(ms, m)
			}
		}
		cfg.Upstream.Models = ms
		if err := config.Save(path, cfg); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = s.ReloadConfig()
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", 405)
	}
}


