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
	"devin-byok/internal/extinstall"
	"devin-byok/internal/ideinject"
	"devin-byok/internal/logx"
	"devin-byok/internal/lsinstall"
	"devin-byok/internal/platform"
	"devin-byok/internal/promptstore"
	"devin-byok/internal/update"
	"devin-byok/internal/upstream/openai"
	"devin-byok/internal/version"
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
	mux.HandleFunc("/api/local-account", s.handleAPILocalAccount)
	mux.HandleFunc("/api/devin", s.handleAPIDevin)
	mux.HandleFunc("/api/devin/restart", s.handleAPIDevinRestart)
	mux.HandleFunc("/api/chats/export", s.handleAPIChatExport)
	mux.HandleFunc("/api/test-upstream", s.handleAPITestUpstream)
	mux.HandleFunc("/api/control/", s.handleAPIControl)
	mux.HandleFunc("/api/desktop", s.handleAPIDesktop)
	mux.HandleFunc("/api/version", s.handleAPIVersion)
	mux.HandleFunc("/api/update/check", s.handleAPIUpdateCheck)
	mux.HandleFunc("/api/update/apply", s.handleAPIUpdateApply)
	mux.HandleFunc("/api/update/progress", s.handleAPIUpdateProgress)
	s.registerExtraAPI(mux)
	mux.HandleFunc("/api/prompts", s.handleAPIPrompts)
	mux.HandleFunc("/api/prompts/preview", s.handleAPIPromptPreview)
	mux.HandleFunc("/api/extension", s.handleAPIExtension)
}

// handleAPIPromptPreview returns metadata and short system previews from the
// same Composer used by real requests. It never returns credentials or the
// full user/project context.
func (s *Server) handleAPIPromptPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := s.GetConfig()
	route := r.URL.Query().Get("route")
	model := r.URL.Query().Get("model")
	task := r.URL.Query().Get("task")
	quality := r.URL.Query().Get("quality_mode")
	if model == "" {
		model = cfg.DefaultModelID()
	}
	if m, ok := cfg.ResolveModelUID(model); ok {
		model = m.ID
	}
	prov, _ := cfg.ResolveProvider(model)
	if task == "" {
		task = "general"
	}
	base := []openai.ChatMessage{{Role: "system", Content: "Upstream system prompt preview"}, {Role: "user", Content: "preview"}}
	composed := promptstore.ComposeMessages(base, promptstore.ComposeContext{
		Route: route, ModelID: model, ModelName: modelDisplayName(cfg, model), Family: prov.FamilyUID, Task: task,
		QualityMode: quality, HasTools: route == "chat", HasWorkspace: false, QualityEnabled: boolPtr(cfg.Quality.Enabled),
	})
	type messagePreview struct {
		Role    string `json:"role"`
		Preview string `json:"preview"`
	}
	previews := make([]messagePreview, 0, len(composed.Messages))
	for _, msg := range composed.Messages {
		if msg.Role != "system" {
			continue
		}
		text := openai.TextContent(msg.Content)
		if len([]rune(text)) > 2000 {
			text = string([]rune(text)[:2000]) + "…"
		}
		previews = append(previews, messagePreview{Role: msg.Role, Preview: text})
	}
	writeJSON(w, map[string]any{
		"ok": true, "route": composed.Route, "model": model, "task": composed.Task,
		"quality_mode": composed.QualityMode, "profile_ids": composed.ProfileIDs,
		"warnings": composed.Warnings, "prompt_hash": composed.Hash,
		"estimated_tokens": estimateTokens(strings.Join(func() []string {
			out := make([]string, 0, len(composed.Messages))
			for _, m := range composed.Messages {
				out = append(out, openai.TextContent(m.Content))
			}
			return out
		}(), "\n")),
		"messages": previews,
	})
}

func (s *Server) getConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfgPath
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()
	wrapper := false
	real := filepath.Join(platform.ExtensionsBinDir(cfg.Devin.InstallDir), platform.RealLanguageServerName())
	if _, err := os.Stat(real); err == nil {
		wrapper = true
	}
	accountExists := false
	accountConfigured := false
	serviceActive := false
	if account, err := devin.LoadLocalAccount(); err == nil {
		accountExists = true
		accountConfigured = cfg.Features.PureLocal && cfg.Auth.FakeUserID == account.ID && cfg.Auth.FakeAPIKey == account.APIKey
		wrapperPath := filepath.Join(platform.DataDir(), "bin", platform.WrapperExeName())
		imported, _ := devin.LocalAccountImported(cfg, wrapperPath)
		serviceActive = accountConfigured && imported
	}
	process := devin.ProcessStatus(cfg.Devin.InstallDir)
	writeJSON(w, map[string]any{
		"ok":                 true,
		"management_online":  true,
		"service_active":     serviceActive,
		"applied":            serviceActive,
		"account_exists":     accountExists,
		"account_imported":   accountConfigured && serviceActive,
		"devin_running":      process.Running,
		"restart_required":   s.restartRequired.Load(),
		"time":               time.Now().Format(time.RFC3339),
		"portal":             cfg.Server.PublicBase,
		"api":                cfg.Server.PublicBase + cfg.APIBasePath(),
		"default_model":      cfg.DefaultModelID(),
		"default_model_name": modelDisplayName(cfg, cfg.DefaultModelID()),
		"models":             cfg.ModelList(),
		"pure_local":         cfg.Features.PureLocal,
		"stream":             cfg.Features.EnableStream,
		"cascade_tools":      cfg.Features.EnableCascadeTools,
		"tools_mode":         cfg.ToolsMode(),
		"tools_timeout":      cfg.ResolveChatTimeoutSec(true),
		"wrapper":            wrapper,
		"upstream":           config.NormalizeChatCompletionsURL(cfg.Upstream.BaseURL),
		"config_path":        s.getConfigPath(),
		"cache_enabled":      cfg.Cache.Enabled,
		"cache_ttl_sec":      cfg.Cache.TTLSec,
	})
}

func (s *Server) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.GetConfig()
		writeJSON(w, map[string]any{
			"base_url":                     cfg.Upstream.BaseURL,
			"api_key_masked":               config.MaskAPIKey(cfg.Upstream.APIKey),
			"api_key_set":                  strings.TrimSpace(cfg.Upstream.APIKey) != "",
			"model":                        cfg.Upstream.Model,
			"timeout_sec":                  cfg.Upstream.TimeoutSec,
			"tools_mode":                   cfg.ToolsMode(),
			"tools_timeout_sec":            cfg.Tools.TimeoutSec,
			"enable_stream":                cfg.Features.EnableStream,
			"enable_cascade_tools":         cfg.Features.EnableCascadeTools,
			"pure_local":                   cfg.Features.PureLocal,
			"enable_deepwiki":              cfg.Features.EnableDeepWiki,
			"enable_codemap":               cfg.Features.EnableCodeMap,
			"deepwiki_model":               cfg.Features.DeepWikiModel,
			"codemap_model":                cfg.Features.CodeMapModel,
			"codemap_fast_model":           cfg.Features.CodeMapFastModel,
			"codemap_smart_model":          cfg.Features.CodeMapSmartModel,
			"command_model":                cfg.Features.CommandModel,
			"title_model":                  cfg.Features.TitleModel,
			"fast_context_model":           cfg.Features.FastContextModel,
			"enable_fast_context":          cfg.Features.EnableFastContext,
			"deepwiki_model_resolved":      cfg.FeatureModelID("deepwiki"),
			"codemap_model_resolved":       cfg.FeatureModelID("codemap"),
			"codemap_fast_model_resolved":  cfg.FeatureModelID("codemap_fast"),
			"codemap_smart_model_resolved": cfg.FeatureModelID("codemap_smart"),
			"command_model_resolved":       cfg.FeatureModelID("command"),
			"title_model_resolved":         cfg.FeatureModelID("title"),
			"fast_context_model_resolved":  cfg.FeatureModelID("fast_context"),
			"default_model":                cfg.DefaultModelID(),
			"models":                       cfg.ModelList(),
			"config_path":                  s.getConfigPath(),
			"update_enabled":               cfg.Update.Enabled,
			"update_auto_apply":            cfg.Update.AutoApply,
			"update_repo":                  cfg.Update.Repo,
			"quality_enabled":              cfg.Quality.Enabled,
			"quality_mode":                 cfg.QualityMode(),
			"max_verification_rounds":      cfg.Quality.MaxVerificationRounds,
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
		writeJSON(w, map[string]any{"ok": true, "message": uiMsg(langFromRequest(r), "msg.savedAndReloaded")})
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

func localAccountResponse(lang string, cfg *config.File, account *devin.LocalAccount, created bool) map[string]any {
	response := map[string]any{
		"ok":             true,
		"created":        created,
		"account_exists": account != nil,
		"imported":       false,
		"need_restart":   false,
		"account_path":   devin.LocalAccountPath(),
	}
	if account == nil {
		response["message"] = uiMsg(lang, "msg.accountNotCreated")
		return response
	}
	wrapperPath := filepath.Join(platform.DataDir(), "bin", platform.WrapperExeName())
	imported, message := devin.LocalAccountImported(cfg, wrapperPath)
	configured := cfg.Features.PureLocal &&
		cfg.Auth.FakeUserID == account.ID &&
		cfg.Auth.FakeName == account.Name &&
		cfg.Auth.FakeEmail == account.Email &&
		cfg.Auth.FakeAPIKey == account.APIKey
	response["id"] = account.ID
	response["name"] = account.Name
	response["email"] = account.Email
	response["api_key_masked"] = config.MaskAPIKey(account.APIKey)
	response["created_at"] = account.CreatedAt
	response["configured"] = configured
	response["imported"] = configured && imported
	response["need_restart"] = configured && imported
	response["message"] = message
	return response
}

func applyLocalRuntime(cfg *config.File) error {
	wrapperPath, err := lsinstall.MaterializeWrapper()
	if err != nil {
		return err
	}
	if _, err := devin.ApplyLocalRuntime(cfg.Server.PublicBase, cfg.Devin.PortalURLKeys, wrapperPath); err != nil {
		return err
	}
	if _, err := extinstall.InstallFromFS(extinstall.ExtFS, extinstall.ExtRoot); err != nil {
		return err
	}
	return extinstall.Enable()
}

func (s *Server) handleAPILocalAccount(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		account, err := devin.LoadLocalAccount()
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, localAccountResponse(langFromRequest(r), s.GetConfig(), nil, false))
				return
			}
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, localAccountResponse(langFromRequest(r), s.GetConfig(), account, false))
	case http.MethodPost:
		account, created, err := devin.EnsureLocalAccount()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		lang := langFromRequest(r)
		cfg, err := devin.ApplyLocalAccountToConfig(s.getConfigPath(), account)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": uiMsg(lang, "msg.saveAccountFailed", err.Error())})
			return
		}
		if err := applyLocalRuntime(cfg); err != nil {
			writeJSON(w, map[string]any{
				"ok": false, "created": created,
				"message": uiMsg(lang, "msg.importFailed", err.Error()),
			})
			return
		}
		if err := s.ReloadConfig(); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": uiMsg(lang, "msg.importedReloadFailed", err.Error())})
			return
		}
		s.restartRequired.Store(true)
		response := localAccountResponse(lang, cfg, account, created)
		response["message"] = uiMsg(lang, "msg.accountImportedRestart")
		writeJSON(w, response)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIDevin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, devin.ProcessStatus(s.GetConfig().Devin.InstallDir))
}

func (s *Server) handleAPIDevinRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := devin.Restart(s.GetConfig().Devin.InstallDir)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	s.restartRequired.Store(false)
	writeJSON(w, map[string]any{"ok": true, "running": result.Running, "message": result.Message})
}

func (s *Server) handleAPIChatExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := devin.ExportChats("")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "message": result.Message, "path": result.Path,
		"file_count": result.FileCount, "size": result.Size,
	})
}

func (s *Server) handleAPIControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/control/"), "/")
	lang := langFromRequest(r)
	cfg := s.GetConfig()
	switch action {
	case "start":
		account, _, err := devin.EnsureLocalAccount()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": uiMsg(lang, "msg.createAccountFailed", err.Error())})
			return
		}
		cfg, err = devin.ApplyLocalAccountToConfig(s.getConfigPath(), account)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": uiMsg(lang, "msg.saveAccountFailed", err.Error())})
			return
		}
		if err := applyLocalRuntime(cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": uiMsg(lang, "msg.applyFailed", err.Error())})
			return
		}
		if err := s.ReloadConfig(); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": uiMsg(lang, "msg.enabledReloadFailed", err.Error())})
			return
		}
		s.restartRequired.Store(true)
		writeJSON(w, map[string]any{"ok": true, "message": uiMsg(lang, "msg.serviceEnabledRestart")})
	case "install-wrapper", "install_wrapper":
		meta, err := lsinstall.Install(cfg.Devin.InstallDir)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": uiMsg(lang, "msg.wrapperInstalled"), "target": meta.Target, "real": meta.Real})
	case "uninstall-wrapper", "uninstall_wrapper":
		if err := lsinstall.Uninstall(cfg.Devin.InstallDir); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": uiMsg(lang, "msg.wrapperUninstalled")})
	case "stop":
		// 保存计数、restore settings、禁用提示词扩展、清空日志
		MetricsSave()
		MetricsClearLogs()
		if err := extinstall.Disable(); err != nil {
			logx.Warnf("extension disable: %v", err)
		} else {
			logx.Infof("extension disabled")
		}
		_ = ideinject.RestoreContextUsageDonut()
		if _, err := devin.RestorePortal(); err != nil && !os.IsNotExist(err) {
			logx.Warnf("control stop restore: %v", err)
		} else {
			logx.Infof("control stop restore ok")
		}
		s.restartRequired.Store(false)
		// 用户显式「停止并恢复」（?reset=1）：清除启用标记，下次启动不再自动启用。
		// 缺省（如 GUI 退出时自动停止）保留标记，下次打开 GUI 自动恢复启用。
		if r.URL.Query().Get("reset") == "1" {
			_ = os.Remove(filepath.Join(platform.DataDir(), "last-apply.json"))
		}
		writeJSON(w, map[string]any{"ok": true, "message": uiMsg(lang, "msg.serviceStopped")})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
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
			"autostart":        p.Autostart,
			"start_minimized":  p.StartMinimized,
			"minimize_to_tray": p.MinimizeToTray,
			"autostart_path":   desktop.AutostartPath(),
		})
	case http.MethodPut, http.MethodPost:
		var body struct {
			Autostart      *bool `json:"autostart"`
			StartMinimized *bool `json:"start_minimized"`
			MinimizeToTray *bool `json:"minimize_to_tray"`
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
			exe := mustExe()
			cli := platform.BundledCLIPath(exe)
			if _, err := os.Stat(cli); err != nil {
				if strings.EqualFold(filepath.Base(exe), platform.CLIName()) {
					cli = exe
				} else {
					http.Error(w, "autostart: release executable is missing", 500)
					return
				}
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
		return platform.CLIName()
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
		dir = platform.ReleaseInstallDir(exe)
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

// handleAPIPrompts 系统提示词 CRUD（扩展与 GUI 共用）。
func (s *Server) handleAPIPrompts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := promptstore.Load()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "prompts": list, "path": promptstore.Path()})
	case http.MethodPost, http.MethodPut:
		var p promptstore.Prompt
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": "bad json: " + err.Error()})
			return
		}
		list, err := promptstore.Upsert(p)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "prompts": list})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, map[string]any{"ok": false, "message": "missing id"})
			return
		}
		list, err := promptstore.Delete(id)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "prompts": list})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

// handleAPIExtension 扩展安装状态。
func (s *Server) handleAPIExtension(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{
			"ok":        true,
			"installed": extinstall.IsInstalled(),
			"disabled":  extinstall.IsDisabled(),
			"id":        extinstall.ExtID,
			"dir":       extinstall.UserExtensionsDir(),
			"folder":    extinstall.FolderName(),
		})
	case http.MethodPost:
		action := r.URL.Query().Get("action")
		switch action {
		case "install":
			dst, err := extinstall.InstallFromFS(extinstall.ExtFS, extinstall.ExtRoot)
			if err != nil {
				writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
				return
			}
			_ = extinstall.Enable()
			writeJSON(w, map[string]any{"ok": true, "path": dst})
		case "disable":
			if err := extinstall.Disable(); err != nil {
				writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		case "uninstall":
			if err := extinstall.Uninstall(); err != nil {
				writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		default:
			writeJSON(w, map[string]any{"ok": false, "message": "action=install|disable|uninstall"})
		}
	default:
		http.Error(w, "method not allowed", 405)
	}
}
