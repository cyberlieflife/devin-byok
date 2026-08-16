package localapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"devin-byok/internal/config"
)

// TestOriginGuardRejectsCrossSiteRequests 验证管理 API 对浏览器跨站
// 来源的防护：恶意网页 Origin 必须被拒，本地同源 Origin 放行，
// 无浏览器头（curl / Devin 语言服务器）的本地客户端放行。
func TestOriginGuardRejectsCrossSiteRequests(t *testing.T) {
	cfg := &config.File{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 8787
	cfg.Upstream.Model = "model"
	s := &Server{cfg: cfg}

	// 恶意网页（跨站请求携带攻击者 Origin）
	req := httptest.NewRequest(http.MethodGet, "/api/prompts/preview", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-site Origin: status = %d, want 403", rr.Code)
	}

	// 同源本地管理页
	req = httptest.NewRequest(http.MethodGet, "/api/prompts/preview", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8787")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Fatal("same-origin request was rejected")
	}

	// 本地非浏览器客户端（无 Origin/Referer），如 curl、Devin LS
	req = httptest.NewRequest(http.MethodGet, "/api/prompts/preview", nil)
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Fatal("local client without browser headers was rejected")
	}

	// 恶意 Referer 同样拒绝
	req = httptest.NewRequest(http.MethodGet, "/api/prompts/preview", nil)
	req.Header.Set("Referer", "http://evil.example.com/")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-site Referer: status = %d, want 403", rr.Code)
	}

	// 同源 Referer 是完整 URL（浏览器同源请求带路径）：必须放行
	// （此前按精确字符串比较，路径导致所有带 Referer 的同源请求被误拒）
	for _, ref := range []string{
		"http://127.0.0.1:8787/ui/",
		"http://127.0.0.1:8787/ui/index.html",
		"http://localhost:8787/ui/",
		"http://127.0.0.1:8787/ui/index.html?x=1",
	} {
		req = httptest.NewRequest(http.MethodGet, "/api/prompts/preview", nil)
		req.Header.Set("Referer", ref)
		rr = httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusForbidden {
			t.Fatalf("same-origin Referer %q was rejected (forbidden origin)", ref)
		}
	}
	// 同源 Origin + 带路径 Referer 组合（浏览器实际形态）同样放行
	req = httptest.NewRequest(http.MethodPost, "/api/prompts/preview", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8787")
	req.Header.Set("Referer", "http://127.0.0.1:8787/ui/")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Fatal("same-origin Origin+Referer combination was rejected")
	}

	// 非 /api/ 路径（如 /healthz、/v1/chat/completions）不拦截，
	// Devin 客户端可正常访问
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Fatal("/healthz must not be blocked for Devin clients")
	}
}
