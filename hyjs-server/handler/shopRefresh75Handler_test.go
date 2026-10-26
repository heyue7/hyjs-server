package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/service"
)

func TestShopRefresh75StatusRequestBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := gameService
	gameService = &service.GameService{}
	t.Cleanup(func() { gameService = previous })
	run := func(raw []byte) error {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/shop-refresh/status", bytes.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		return ShopRefresh75StatusHandler(c)
	}
	valid := model.ShopRefresh75StatusRequest{PlayerID: 1, SessionHex: strings.Repeat("a", 32), ShopType: 5}
	for _, mutate := range []func(*model.ShopRefresh75StatusRequest){
		func(r *model.ShopRefresh75StatusRequest) { r.PlayerID = 0 },
		func(r *model.ShopRefresh75StatusRequest) { r.PlayerID = -1 },
		func(r *model.ShopRefresh75StatusRequest) { r.ShopType = 4 },
		func(r *model.ShopRefresh75StatusRequest) { r.ShopType = 0 },
		func(r *model.ShopRefresh75StatusRequest) { r.SessionHex = strings.Repeat("g", 32) },
		func(r *model.ShopRefresh75StatusRequest) { r.SessionHex = strings.Repeat("a", 30) },
	} {
		r := valid
		mutate(&r)
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := run(raw); err == nil || err.Error() != model.NewErrParamsError().Error() {
			t.Fatal("invalid refresh request reached the database", string(raw), err)
		}
	}
	oversized := `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 32) + `","shopType":5,"padding":"` + strings.Repeat("x", 1024) + `"}`
	if err := run([]byte(oversized)); err == nil || err.Error() != model.NewErrParamsError().Error() {
		t.Fatal("oversized refresh request accepted", err)
	}
}
