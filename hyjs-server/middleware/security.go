package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ConfigureTrustedProxies rejects malformed proxy declarations. Empty preserves
// direct-connection trust; forwarded headers from arbitrary clients are ignored.
func ConfigureTrustedProxies(r *gin.Engine, configured string) error {
	var proxies []string
	for _, value := range strings.Split(configured, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if net.ParseIP(value) == nil {
			if _, _, err := net.ParseCIDR(value); err != nil {
				return err
			}
		}
		proxies = append(proxies, value)
	}
	return r.SetTrustedProxies(proxies)
}

// SecurityHeaders protects the browser application without changing original
// native game framing, media content types or client request fields.
func SecurityHeaders(secureAdminCookies bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("hyjs-secure-admin-cookies", secureAdminCookies)
		c.Header("X-Request-ID", uuid.NewString())
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		if c.Request.URL.Path == "/admin" || strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			c.Header("X-Frame-Options", "DENY")
			c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
			c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if SecureAdminCookies(c) {
				c.Header("Strict-Transport-Security", "max-age=31536000")
			}
		}
		c.Next()
	}
}

// SecureAdminCookies comes only from TLS or server configuration, never an
// untrusted X-Forwarded-Proto supplied by a browser.
func SecureAdminCookies(c *gin.Context) bool {
	secure, _ := c.Get("hyjs-secure-admin-cookies")
	return c.Request.TLS != nil || secure == true
}

// ValidAdminOrigin requires the exact authority and transport when Origin is
// present. Native/non-browser requests still require their session/CSRF checks.
func ValidAdminOrigin(r *http.Request, secure bool) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, ok := corsParseOrigin(origin)
	if !ok {
		return false
	}
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}
