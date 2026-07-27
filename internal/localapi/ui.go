package localapi

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/desktop"
	"devin-byok/internal/devin"
	"devin-byok/internal/logx"
	"devin-byok/internal/version"
	"devin-byok/internal/update"
	"devin-byok/internal/lsinstall"
	"devin-byok/internal/upstream/openai"
)

//go:embed ui/*
var uiFS embed.FS

func (s *Server) registerUIRoutes(mux *http.ServeMux) {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		// fallback empty
		sub = uiFS
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})
	// UI 静态资源禁用缓存，避免 GUI/浏览器一直吃到旧版页面
	mux.Handle("/ui/", http.StripPrefix("/ui/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		fileServer.ServeHTTP(w, r)
	})))
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/api/config", s.handleAPIConfig)
	mux.HandleFunc("/api/test-upstream", s.handleAPITestUpstream)
	mux.HandleFunc("/api/control/", s.handleAPIControl)
	mux.HandleFunc("/api/desktop", s.handleAPIDesktop)
	mux.HandleFunc("/api/version", s.handleAPIVersion)
	mux.HandleFunc("/api/update/check", s.handleAPIUpdateCheck)
	mux.HandleFunc("/api/update/apply", s.handleAPIUpdateApply)
	mux.HandleFunc("/api/update/progress", s.handleAPIUpdateProgress)
	s.registerExtraAPI(mux)
}

func (s *Server) getConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfgPath
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()
	wrapper := false
	real := filepath.Join(cfg.Devin.InstallDir, "resources", "app", "extensions", "windsurf", "bin", "language_server_windows_x64.real.exe")
	if _, err := os.Stat(real); err == nil {
		wrapper = true
	}
	writeJSON(w, map[string]any{
		"ok":            true,
		"time":          time.Now().Format(time.RFC3339),
		"portal":        cfg.Server.PublicBase,
		"api":           cfg.Server.PublicBase + cfg.APIBasePath(),
		"default_model": cfg.DefaultModelID(),
		"models":        cfg.ModelList(),
		"pure_local":    cfg.Features.PureLocal,
		"stream":        cfg.Features.EnableStream,
		"cascade_tools": cfg.Features.EnableCascadeTools,
		"tools_mode":    cfg.ToolsMode(),
		"tools_timeout": cfg.ResolveChatTimeoutSec(true),
		"wrapper":       wrapper,
		"upstream":      config.NormalizeChatCompletionsURL(cfg.Upstream.BaseURL),
		"config_path":   s.getConfigPath(),
		"cache_enabled":  cfg.Cache.Enabled,
		"cache_ttl_sec":  cfg.Cache.TTLSec,
	})
}

func (s *Server) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.GetConfig()
		writeJSON(w, map[string]any{
			"base_url":             cfg.Upstream.BaseURL,
			"api_key_masked":       config.MaskAPIKey(cfg.Upstream.APIKey),
			"api_key_set":          strings.TrimSpace(cfg.Upstream.APIKey) != "",
			"model":                cfg.Upstream.Model,
			"timeout_sec":          cfg.Upstream.TimeoutSec,
			"tools_mode":           cfg.ToolsMode(),
			"tools_timeout_sec":    cfg.Tools.TimeoutSec,
			"enable_stream":        cfg.Features.EnableStream,
			"enable_cascade_tools": cfg.Features.EnableCascadeTools,
			"pure_local":           cfg.Features.PureLocal,
			"enable_deepwiki":      cfg.Features.EnableDeepWiki,
			"enable_codemap":       cfg.Features.EnableCodeMap,
			"deepwiki_model":            cfg.Features.DeepWikiModel,
			"codemap_model":             cfg.Features.CodeMapModel,
			"codemap_fast_model":        cfg.Features.CodeMapFastModel,
			"codemap_smart_model":       cfg.Features.CodeMapSmartModel,
			"deepwiki_model_resolved":   cfg.FeatureModelID("deepwiki"),
			"codemap_model_resolved":    cfg.FeatureModelID("codemap"),
			"codemap_fast_model_resolved":  cfg.FeatureModelID("codemap_fast"),
			"codemap_smart_model_resolved": cfg.FeatureModelID("codemap_smart"),
			"default_model":        cfg.DefaultModelID(),
			"models":               cfg.ModelList(),
			"config_path":          s.getConfigPath(),
			"update_enabled":       cfg.Update.Enabled,
			"update_auto_apply":    cfg.Update.AutoApply,
			"update_repo":          cfg.Update.Repo,
		})
	case http.MethodPut, http.MethodPost:
		var patch config.GUIPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, "invalid json: "+err.Error(), 400)
			return
		}
		path := s.getConfigPath()
		if path == "" {
			http.Error(w, "config path not set", 500)
			return
		}
		cfg, err := config.Load(path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if patch.APIKey != nil && strings.TrimSpace(*patch.APIKey) == "" {
			patch.APIKey = nil
		}
		cfg.ApplyGUIPatch(patch)
		if err := config.Save(path, cfg); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.ReloadConfig(); err != nil {
			http.Error(w, "saved but reload failed: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "已保存并热重载"})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) handleAPITestUpstream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	cfg := s.GetConfig()
	// 用第一个 Family 的供应商做探测（不再依赖全局默认 model 名）
	upCfg := cfg.Upstream
	if fams := cfg.GroupModelsByFamily(); len(fams) > 0 {
		f0 := fams[0]
		if f0.BaseURL != "" {
			upCfg.BaseURL = f0.BaseURL
		}
		// 需要明文 key：从配置重载
		if raw, err := config.Load(s.getConfigPath()); err == nil {
			for _, fc := range raw.Upstream.Families {
				fu := fc.UID
				if fu == "" {
					fu = config.SlugID(fc.Label)
				}
				if fu == f0.UID {
					if fc.APIKey != "" {
						upCfg.APIKey = fc.APIKey
					}
					if fc.UpstreamModel != "" {
						upCfg.Model = fc.UpstreamModel
					}
					if fc.BaseURL != "" {
						upCfg.BaseURL = fc.BaseURL
					}
					break
				}
			}
		}
	}
	c := openai.New(upCfg)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	text, err := c.Ping(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "text": text})
}

func (s *Server) handleAPIControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/control/"), "/")
	cfg := s.GetConfig()
	switch action {
	case "start":
		// 单 GUI / 内嵌模式：服务已在本进程，仅 apply portal
		if _, err := devin.ApplyPortal(cfg.Server.PublicBase, cfg.Devin.PortalURLKeys); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": "apply 失败: " + err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "服务已在运行（已 apply）"})
	case "install-wrapper", "install_wrapper":
		meta, err := lsinstall.Install(cfg.Devin.InstallDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "wrapper 已安装", "target": meta.Target, "real": meta.Real})
	case "uninstall-wrapper", "uninstall_wrapper":
		if err := lsinstall.Uninstall(cfg.Devin.InstallDir); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "wrapper 已卸载还原"})
	case "stop":
		// 保存计数、restore settings、清空日志
		MetricsSave()
		MetricsClearLogs()
		if _, err := devin.RestorePortal(); err != nil {
			logx.Warnf("control stop restore: %v", err)
		} else {
			logx.Infof("control stop restore ok")
		}
		writeJSON(w, map[string]any{"ok": true, "message": "已保存计数、restore settings 并停止（日志已清空）"})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// 仅独立 CLI serve 进程在 stop 时退出；GUI 内嵌由 nativeStop 关监听
		exe, _ := os.Executable()
		base := strings.ToLower(filepath.Base(exe))
		if base == "devin-byok.exe" {
			go func() {
				time.Sleep(150 * time.Millisecond)
				os.Exit(0)
			}()
		}
		return
	default:
		http.Error(w, "unknown action", 404)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}


func (s *Server) handleAPIDesktop(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		p := desktop.LoadPrefs()
		// 以磁盘 Startup 脚本为准同步 autostart 状态
		p.Autostart = desktop.AutostartEnabled()
		writeJSON(w, map[string]any{
			"autostart":         p.Autostart,
			"start_minimized":   p.StartMinimized,
			"minimize_to_tray":  p.MinimizeToTray,
			"autostart_path":    desktop.AutostartPath(),
		})
	case http.MethodPut, http.MethodPost:
		var body struct {
			Autostart       *bool `json:"autostart"`
			StartMinimized  *bool `json:"start_minimized"`
			MinimizeToTray  *bool `json:"minimize_to_tray"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), 400)
			return
		}
		p := desktop.LoadPrefs()
		if body.StartMinimized != nil {
			p.StartMinimized = *body.StartMinimized
		}
		if body.MinimizeToTray != nil {
			p.MinimizeToTray = *body.MinimizeToTray
		}
		if body.Autostart != nil {
			p.Autostart = *body.Autostart
			cli := filepath.Join(filepath.Dir(mustExe()), "devin-byok.exe")
			if _, err := os.Stat(cli); err != nil {
				// serve 进程自身可能是 devin-byok.exe
				cli = mustExe()
			}
			if err := desktop.SetAutostart(*body.Autostart, filepath.Dir(cli), cli); err != nil {
				http.Error(w, "autostart: "+err.Error(), 500)
				return
			}
		}
		if err := desktop.SavePrefs(p); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "prefs": p, "autostart": desktop.AutostartEnabled()})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func mustExe() string {
	exe, err := os.Executable()
	if err != nil {
		return "devin-byok.exe"
	}
	return exe
}



func (s *Server) handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"version":    version.Version,
		"build_time": version.BuildTime,
		"name":       "devin-byok",
	})
}

func (s *Server) updateCfg() update.Config {
	cfg := s.GetConfig()
	return update.Config{
		Enabled:       cfg.Update.Enabled,
		Repo:          cfg.Update.Repo,
		AssetContains: cfg.Update.AssetContains,
		AutoApply:     cfg.Update.AutoApply,
		CheckURL:      cfg.Update.CheckURL,
	}
}

func (s *Server) handleAPIUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	res := update.Check(ctx, s.updateCfg(), version.Version)
	writeJSON(w, res)
}

func (s *Server) handleAPIUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", 405)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ucfg := s.updateCfg()
	if !ucfg.Enabled {
		writeJSON(w, map[string]any{"ok": false, "message": "update disabled in config"})
		return
	}
	dir := update.DefaultInstallDir()
	exe := mustExe()
	if strings.Contains(strings.ToLower(filepath.Base(exe)), "devin-byok") {
		dir = filepath.Dir(exe)
	}
	res, err := update.DownloadAndSchedule(ctx, ucfg, version.Version, dir)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "message": res.Message, "error": err.Error()})
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleAPIUpdateProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, update.GetProgress())
}

