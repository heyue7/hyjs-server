package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

// TestAdminPlayerInventoryQuery37OptionalQuality covers the real UI query with cleared optional filters.
func TestAdminPlayerInventoryQuery37OptionalQuality(t *testing.T) {
	for _, tc := range []struct {
		query   string
		valid   bool
		quality bool
	}{
		{"category=&q=&quality=&page=1&pageSize=25", true, false},
		{"category=warrior&quality=0&page=1&pageSize=100", true, true},
		{"category=pet&quality=6&page=2&pageSize=25", true, true},
		{"quality=7", false, false}, {"quality=x", false, false}, {"category=unknown", false, false}, {"pageSize=101", false, false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Params = gin.Params{{Key: "id", Value: "16"}}
			c.Request = httptest.NewRequest("GET", "/admin/api/game-players/16/inventory?"+tc.query, nil)
			_, _, _, quality, _, _, valid := adminInventoryQuery(c)
			if valid != tc.valid || valid && (quality != nil) != tc.quality {
				t.Fatal("unexpected query result", valid, quality)
			}
		})
	}
}
