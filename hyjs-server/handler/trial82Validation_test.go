package handler

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/service"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

func TestTrial82RefreshRequestValidation(t *testing.T) {
	for _, shop := range []int32{3, 5} {
		for _, body := range []proto.Message{&pb.CSRefreshShopQuery{ShopType: proto.Int32(shop)}, &pb.CSRefreshShopRefresh{ShopType: proto.Int32(shop)}, &pb.CSRefreshShopBuy{ShopType: proto.Int32(shop), ShopItemId: proto.String("p4q:1234567890123456:tr82:slot")}} {
			if err := ValidateGameRequest(&model.GamePacket{Body: body, Timestamp: 1}); err != nil {
				t.Fatal("valid refresh request rejected", shop, body, err)
			}
		}
	}
	for _, packet := range []*model.GamePacket{
		{Body: &pb.CSRefreshShopQuery{ShopType: proto.Int32(4)}},
		{Body: &pb.CSRefreshShopRefresh{ShopType: proto.Int32(4)}, Timestamp: 1},
		{Body: &pb.CSRefreshShopRefresh{ShopType: proto.Int32(5)}},
		{Body: &pb.CSRefreshShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("sku")}},
		{Body: &pb.CSRefreshShopBuy{ShopType: proto.Int32(5)}, Timestamp: 1},
		{Body: &pb.CSRefreshShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(strings.Repeat("x", 257))}, Timestamp: 1},
	} {
		if ValidateGameRequest(packet) == nil {
			t.Fatal("invalid refresh request accepted", packet.Body)
		}
	}
}

func trial82HTTPFrame(t *testing.T, id, stamp int64, session []byte, msg proto.Message) []byte {
	t.Helper()
	body, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var opcode int32
	for code, name := range model.OpcodeNames {
		if name == string(msg.ProtoReflect().Descriptor().Name()) {
			opcode = code
			break
		}
	}
	if opcode == 0 || len(session) != 16 {
		t.Fatal("missing opcode or session")
	}
	raw := make([]byte, 68, 68+len(body))
	binary.BigEndian.PutUint32(raw, uint32(68+len(body)))
	binary.BigEndian.PutUint32(raw[4:], 10000)
	binary.BigEndian.PutUint32(raw[8:], uint32(opcode))
	binary.BigEndian.PutUint64(raw[12:], uint64(id))
	binary.BigEndian.PutUint32(raw[20:], 1)
	binary.BigEndian.PutUint32(raw[24:], uint32(len(body)))
	copy(raw[28:44], session)
	binary.BigEndian.PutUint64(raw[60:], uint64(stamp))
	sig := md5.New()
	fmt.Fprintf(sig, "%d%d%d%s", id, 1, stamp, constUtil.SigningSuffix)
	sig.Write(body)
	copy(raw[44:60], sig.Sum(nil))
	return append(raw, body...)
}

func trial82HTTPCall(t *testing.T, id, stamp int64, session []byte, msg proto.Message) proto.Message {
	t.Helper()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/sao/", bytes.NewReader(trial82HTTPFrame(t, id, stamp, session, msg)))
	GameHandler(c)
	raw := response.Body.Bytes()
	if response.Code != http.StatusOK || len(raw) < 8 || int(binary.BigEndian.Uint32(raw)) > len(raw)-8 {
		t.Fatal("invalid protocol HTTP response", response.Code, len(raw))
	}
	length := int(binary.BigEndian.Uint32(raw))
	decoded, err := model.GameMessage(model.OpcodeNames[int32(binary.BigEndian.Uint32(raw[4:]))], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = proto.Unmarshal(raw[8:8+length], decoded); err != nil {
		t.Fatal(err)
	}
	if failure, ok := decoded.(*pb.ErrorMessage); ok {
		t.Fatal("HTTP game request failed", failure.GetCode(), failure.GetMsg())
	}
	return decoded
}

// Exercise the same binary HTTP and TCP consumers used by the original client.
func TestTrial82BinaryProtocolEntrypoints(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "trial82-http.sqlite3")); err != nil {
		t.Fatal(err)
	}
	connection, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	s := &service.GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{s.InitializeGameplayPools, s.InitializeShop, s.InitializeP1, s.InitializeP4} {
		if err = initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err = db.GetIns().Where("kind=?", "point").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	rules := `{"trialRefreshVersion":1,"liveEditable":true,"slots":6,"freeCap":10,"freeSeconds":7200,"resourceRefreshCap":15,"refreshItem":"item_013,1","refreshCost":"diamond,20","products":[{"id":"trial82wire","index":1,"rewards":"item_005,1","cost":"point,10","priority":1001,"dailyLimit":1,"stock":-1,"minLevel":1,"discountBps":10000,"weight":1}]}`
	if err = db.GetIns().Model(&cfg).Updates(map[string]any{"rules": []byte(rules), "enabled": true, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	account, err := service.LoginAccount(ctx, "trial82-wire-fixture", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CreatePlayer(ctx, account.Id, "试炼协议验证", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err = db.GetIns().First(&row, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	var player pb.Player
	if err = json.Unmarshal([]byte(*row.Player), &player); err != nil {
		t.Fatal(err)
	}
	player.Level, player.Point = proto.Int32(75), proto.Int32(1000)
	playerJSON, err := json.Marshal(&player)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&row).Update("player", string(playerJSON)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&db.P7RushState{PlayerID: account.Id, RecordStars: 150}).Error; err != nil {
		t.Fatal(err)
	}
	previous := gameService
	ConfigureGame(s)
	t.Cleanup(func() { gameService = previous })
	query, ok := trial82HTTPCall(t, account.Id, 0, account.Session, &pb.CSRefreshShopQuery{ShopType: proto.Int32(5)}).(*pb.SCRefreshShopQuery)
	if !ok || query.GetShopType() != 5 || len(query.GetShopItemList()) != 6 {
		t.Fatal("HTTP trial query did not reach commerce", query)
	}
	refresh, ok := trial82HTTPCall(t, account.Id, 1, account.Session, &pb.CSRefreshShopRefresh{ShopType: proto.Int32(5)}).(*pb.SCRefreshShopRefresh)
	if !ok || refresh.GetShopType() != 5 || len(refresh.GetShopItemList()) != 6 || refresh.GetFreeLeftTimes() != 0 {
		t.Fatal("HTTP trial refresh did not commit", refresh)
	}
	buy, ok := trial82HTTPCall(t, account.Id, 2, account.Session, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: refresh.GetShopItemList()[0].Id, BuyCount: proto.Int32(1)}).(*pb.SCShopBuy)
	if !ok || buy.GetShopType() != 5 || len(buy.GetCost()) != 1 || buy.GetCost()[0].GetId() != "point" || buy.GetCost()[0].GetNum() != 10 {
		t.Fatal("HTTP trial purchase did not commit", buy)
	}
	buyRefresh, ok := trial82HTTPCall(t, account.Id, 3, account.Session, &pb.CSRefreshShopBuy{ShopType: proto.Int32(5), ShopItemId: refresh.GetShopItemList()[1].Id}).(*pb.SCRefreshShopBuy)
	if !ok || buyRefresh.GetShopType() != 5 || len(buyRefresh.GetCost()) != 1 || buyRefresh.GetCost()[0].GetNum() != 10 {
		t.Fatal("HTTP refresh purchase did not commit", buyRefresh)
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { serveGameConnection(ctx, server, s); close(done) }()
	defer func() {
		client.Close()
		server.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("TCP consumer did not exit")
		}
	}()
	if err = client.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Write(tcpRequest(t, account.Id, "CSSocketLogin", map[string]any{"sessionKey": account.Session})); err != nil {
		t.Fatal(err)
	}
	loginMessage := tcpResponse(t, client)
	if login, ok := loginMessage.(*pb.SCSocketLogin); !ok || login.GetResult() != 100 {
		t.Fatal("TCP fixture login failed", loginMessage)
	}
	if _, err = client.Write(tcpRequest(t, account.Id, "CSRefreshShopQuery", map[string]any{"shopType": 5})); err != nil {
		t.Fatal(err)
	}
	for attempts := 0; attempts < 8; attempts++ {
		replyMessage := tcpResponse(t, client)
		if reply, ok := replyMessage.(*pb.SCRefreshShopQuery); ok {
			if reply.GetShopType() != 5 || len(reply.GetShopItemList()) != 6 {
				t.Fatal("TCP trial query returned the wrong catalogue", reply)
			}
			return
		}
		if failure, ok := replyMessage.(*pb.ErrorMessage); ok {
			t.Fatal("TCP trial query failed", failure)
		}
	}
	t.Fatal("TCP trial query returned no catalogue")
}
