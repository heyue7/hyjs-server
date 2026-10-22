package handler

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/service"
	"hyjs-server/wrapper"
)

func TestGGOShopMySQLHTTP(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("explicit managed MySQL allocation required")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") {
		t.Fatal("owned test database required")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	t.Cleanup(func() { sqlDB.Close() })
	old := gameService
	t.Cleanup(func() { gameService = old })
	s := &service.GameService{}
	gameService = s
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeP1(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := service.LoginAccount(ctx, "ggo-http-"+uuid.NewString(), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CreatePlayer(ctx, owner.Id, "兑换所HTTP验收", "hero_043"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{s.InitializeLocalWallet, s.InitializeAdminGameServers, s.InitializeGlobalGameAccounts, s.InitializeAccountWallets} {
		if err = initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().AutoMigrate(&db.P4CommerceRule{}, &db.ShopState{}, &db.P4CommerceStock{}); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeGGO(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeAdminShopAvailability(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(owner, owner.Id).Error; err != nil {
		t.Fatal(err)
	}
	var game model.GameState
	if err = json.Unmarshal([]byte(*owner.Game), &game); err != nil {
		t.Fatal(err)
	}
	game.Items = append(game.Items, &pb.SimpleItem{Id: proto.String(constUtil.GGOCurrencyID), Num: proto.Int32(5)})
	blob, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", owner.Id).Update("game", string(blob)).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/ggo-shop/catalogue", wrapper.Wrapper(GGOShopCatalogueHandler))
	router.POST("/ggo-shop/buy", wrapper.Wrapper(GGOShopBuyHandler))
	call := func(path string, body any) (int, json.RawMessage) {
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var envelope struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil {
			t.Fatal("invalid HTTP envelope")
		}
		return envelope.Code, envelope.Data
	}
	req := model.GGOShopCatalogueRequest{PlayerID: owner.Id, SessionHex: hex.EncodeToString(owner.Session)}
	code, data := call("/ggo-shop/catalogue", req)
	var catalogue model.GGOShopCatalogueResponse
	if code != 0 || json.Unmarshal(data, &catalogue) != nil || catalogue.Balance != 5 || len(catalogue.Items) != 1 {
		t.Fatal("catalogue authority", code)
	}
	invalid := req
	invalid.SessionHex = strings.Repeat("0", 32)
	if code, _ = call("/ggo-shop/catalogue", invalid); code == 0 {
		t.Fatal("invalid session accepted")
	}
	buy := model.GGOShopBuyRequest{PlayerID: owner.Id, SessionHex: req.SessionHex, ProductID: catalogue.Items[0].ID, Version: catalogue.Version, RequestID: uuid.NewString()}
	badUUID := buy
	badUUID.RequestID = "invalid"
	if code, _ = call("/ggo-shop/buy", badUUID); code == 0 {
		t.Fatal("invalid UUID accepted")
	}
	code, data = call("/ggo-shop/buy", buy)
	var result model.GGOShopBuyResponse
	if code != 0 || json.Unmarshal(data, &result) != nil || result.Catalogue.Balance != 4 || result.TerminalPointsAdded != 1 || result.RewardID != constUtil.GGOOverclockID {
		t.Fatal("buy HTTP authority", code)
	}
	code, replay := call("/ggo-shop/buy", buy)
	if code != 0 || !bytes.Equal(data, replay) {
		t.Fatal("HTTP replay changed")
	}
	invalidBuy := buy
	invalidBuy.RequestID = uuid.NewString()
	if code, _ = call("/ggo-shop/buy", invalidBuy); code == 0 {
		t.Fatal("daily cap bypassed")
	}
	var balance db.AccountWalletBalance
	if err = db.GetIns().First(&balance, "account_id=?", owner.AccountID).Error; err != nil || balance.BalanceCents != 100 {
		t.Fatal("HTTP terminal amount", err)
	}
	if err = db.GetIns().First(owner, owner.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(*owner.Game), &game); err != nil {
		t.Fatal(err)
	}
	for _, item := range game.Items {
		if item.GetId() == constUtil.GGOOverclockID {
			t.Fatal("HTTP reward remained in inventory")
		}
	}
	t.Log("Real GGO handlers+wrapper via httptest: authorized catalogue/buy/replay, invalid session/UUID, limit refusal; MySQL wallet+100 minimal units and no overclock inventory")
}
