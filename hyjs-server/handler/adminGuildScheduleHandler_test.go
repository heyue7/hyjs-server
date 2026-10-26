package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminGuildScheduleSingleBoundedJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"day":"2026-11-01","reason":"publication"}`, true},
		{"unknown", `{"epochMillis":123}`, false},
		{"second-object", `{} {}`, false},
		{"malformed", `{"day":`, false},
		{"oversized", `{"reason":"` + strings.Repeat("x", 5000) + `"}`, false},
		{"oversized-tail", `{}` + strings.Repeat(" ", 5000), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest("POST", "/guild-schedules", strings.NewReader(tc.body))
			var request model.AdminGuildScheduleSaveRequest
			if got := adminGuildScheduleDecode(ctx, &request); got != tc.valid {
				t.Fatal("bounded single JSON contract", got, tc.valid)
			}
		})
	}
}
