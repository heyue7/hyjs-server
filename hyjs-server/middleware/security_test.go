package middleware

import (
	"crypto/tls"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestSecurityProxyTrustAndBrowserHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := ConfigureTrustedProxies(r, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	r.Use(SecurityHeaders(false))
	r.GET("/admin/", func(c *gin.Context) { c.String(200, c.ClientIP()) })
	for _, test := range []struct{ remote, want string }{{"127.0.0.1:5000", "198.51.100.9"}, {"203.0.113.8:5000", "203.0.113.8"}} {
		req := httptest.NewRequest("GET", "http://admin.example/admin/", nil)
		req.RemoteAddr = test.remote
		req.Header.Set("X-Forwarded-For", "198.51.100.9")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		if out.Body.String() != test.want {
			t.Fatal("proxy trust", out.Body.String())
		}
		if out.Header().Get("X-Frame-Options") != "DENY" || out.Header().Get("Content-Security-Policy") == "" || out.Header().Get("X-Request-ID") == "" {
			t.Fatal("browser headers missing")
		}
		if out.Header().Get("Strict-Transport-Security") != "" {
			t.Fatal("HSTS enabled on HTTP")
		}
	}
	if ConfigureTrustedProxies(gin.New(), "garbage") == nil {
		t.Fatal("malformed proxy accepted")
	}
}

func TestSecurityAdminOriginAndCookieTransport(t *testing.T) {
	for _, origin := range []string{"http://other.example", "null", "http://user@admin.example", "http://admin.example/path", "https://admin.example", "http://admin.example?secret=x"} {
		req := httptest.NewRequest("POST", "http://admin.example/admin/api/auth/login", nil)
		req.Header.Set("Origin", origin)
		if ValidAdminOrigin(req, false) {
			t.Fatal("invalid origin accepted", origin)
		}
	}
	req := httptest.NewRequest("POST", "http://admin.example/admin/api/auth/login", nil)
	req.Header.Set("Origin", "http://admin.example")
	if !ValidAdminOrigin(req, false) {
		t.Fatal("same origin rejected")
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	if SecureAdminCookies(c) {
		t.Fatal("untrusted forwarded protocol accepted")
	}
	c.Request.TLS = &tls.ConnectionState{}
	if !SecureAdminCookies(c) {
		t.Fatal("direct TLS ignored")
	}
}
