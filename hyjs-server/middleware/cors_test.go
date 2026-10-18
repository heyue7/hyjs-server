package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSExactOriginsAndNativeCompatibility(t *testing.T) {
	t.Setenv("HYJS_CORS_ORIGINS", " http://deploy.example:28860, http://deploy.example:28861,https://admin.example,*,null,https://bad.example/path ")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(CORS())
	engine.Any("/*path", func(c *gin.Context) { c.Status(http.StatusCreated) })
	for _, test := range []struct {
		name, method, target, origin string
		status                       int
		allowed                      bool
	}{
		{"native game", "POST", "http://game.example/sao/", "", 201, false},
		{"native options", "OPTIONS", "http://game.example/sao/", "", 204, false},
		{"same origin", "POST", "http://game.example/sao/", "http://game.example", 201, true},
		{"same origin default port", "POST", "http://game.example:80/sao/", "http://game.example", 201, true},
		{"same origin https", "GET", "https://game.example/admin/", "https://game.example", 201, true},
		{"deployment game", "OPTIONS", "http://internal:18088/sao/", "http://deploy.example:28860", 204, true},
		{"deployment admin", "OPTIONS", "http://internal:18088/admin/api/auth/login", "http://deploy.example:28861", 204, true},
		{"allowed tls offload", "POST", "http://internal:18088/admin/api/auth/login", "https://admin.example", 201, true},
		{"foreign preflight", "OPTIONS", "http://game.example/sao/", "http://other.example", 403, false},
		{"foreign actual", "POST", "http://game.example/sao/", "http://other.example", 403, false},
		{"wrong scheme", "OPTIONS", "http://game.example/sao/", "https://game.example", 403, false},
		{"wrong port", "OPTIONS", "http://internal:18088/sao/", "http://deploy.example:28862", 403, false},
		{"suffix bypass", "OPTIONS", "http://internal:18088/sao/", "http://deploy.example.evil:28860", 403, false},
		{"opaque origin", "OPTIONS", "http://game.example/sao/", "null", 403, false},
		{"wildcard config ignored", "OPTIONS", "http://game.example/sao/", "*", 403, false},
		{"path config ignored", "OPTIONS", "http://game.example/sao/", "https://bad.example/path", 403, false},
		{"userinfo rejected", "OPTIONS", "http://game.example/sao/", "http://user@game.example", 403, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			request.Header.Set("Access-Control-Request-Method", "POST")
			request.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token")
			request.Header.Set("X-Forwarded-Proto", "https")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d", response.Code, test.status)
			}
			if !strings.Contains(strings.Join(response.Header().Values("Vary"), ","), "Origin") {
				t.Fatal("missing Vary Origin")
			}
			if test.allowed {
				if response.Header().Get("Access-Control-Allow-Origin") != test.origin || response.Header().Get("Access-Control-Allow-Credentials") != "true" {
					t.Fatal("allowed origin missing exact credentialed response")
				}
				if !strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), "X-CSRF-Token") || response.Header().Get("Access-Control-Allow-Headers") == "*" {
					t.Fatal("credentialed request headers are not explicit")
				}
			} else if response.Header().Get("Access-Control-Allow-Origin") != "" || response.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("unapproved response grants browser credentials")
			}
		})
	}
}
