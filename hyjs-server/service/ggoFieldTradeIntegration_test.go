package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoTradeTestFixture struct {
	s        *GameService
	ctx      context.Context
	ids      [2]int64
	sessions [2][]byte
	config   *model.AdminFieldGGOConfiguration
}

func ggoTradeSetup(t *testing.T) *ggoTradeTestFixture {
	t.Helper()
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.AdminFieldDeployment{}, &db.GGOFieldReceipt{}, &db.P8GatherAttempt{}, &db.P10ScenePresence{}, &db.ClientUpdateSession{}); err != nil {
		t.Fatal(err)
	}
	other, err := LoginAccount(f.ctx, fmt.Sprintf("trade-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "交易另一方", "hero_043"); err != nil {
		t.Fatal(err)
	}
	fixture := &ggoTradeTestFixture{s: f.s, ctx: f.ctx, ids: [2]int64{f.id, other.Id}, config: ggoLineTestConfiguration(t)}
	values, err := ggoChipValues(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range fixture.ids {
		err = updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
			game.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(constUtil.GGOFieldScene), Current: &pb.PointNew{X: proto.Float32(-7), Y: proto.Float32(0), Z: proto.Float32(-6)}}
			game.PositionLoading = false
			game.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(constUtil.P8FieldIdle), CurHp: proto.Float64(1)}}
			chips := [6]int64{10}
			if i == 1 {
				chips = [6]int64{0, 0, 6}
			}
			game.GGOField = &model.GGOFieldState{Chips: chips, EconomyVersion: fixture.config.Runtime.EconomyVersion, ExchangeValues: values}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		fixture.sessions[i] = append([]byte(nil), profile.Session...)
		binding := db.ClientUpdateSession{PlayerID: id, SessionHash: clientUpdateSessionHash(profile.Session), BuildCode: fixture.config.Runtime.MinClientBuild}
		if err = db.GetIns().Save(&binding).Error; err != nil {
			t.Fatal(err)
		}
		sink := &p10ClientSink{session: fixture.sessions[i], send: func(proto.Message) error { return nil }}
		sink.tradeReady.Store(true)
		key := p10ClientKey{fixture.s.p10ClientOwner(), id}
		p10ClientSinks.Store(key, sink)
		t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
		presence := db.P10ScenePresence{PlayerID: id, Scene: constUtil.GGOFieldScene, Line: 1, SessionHash: p6Digest(profile.Session)}
		if err = db.GetIns().Save(&presence).Error; err != nil {
			t.Fatal(err)
		}
	}
	database := db.GetIns()
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	fixture.s.adminFieldRuntimeCache.Store(&adminFieldRuntimeSnapshot{serverID: fixture.s.OriginalServerID(), database: database, sqlDatabase: sqlDB, signature: adminFieldDeploymentSignature(nil), tables: fixture.s.Tables, ggo: fixture.config, ggoEconomy: fixture.config, ggoRevision: "isolated-trade-config"})
	return fixture
}

func (f *ggoTradeTestFixture) auth(i int) model.GGOFieldAuth {
	return model.GGOFieldAuth{PlayerID: f.ids[i], SessionHex: hex.EncodeToString(f.sessions[i])}
}
func (f *ggoTradeTestFixture) action(i int, action, requestID, tradeID string, version int64, offer []int64) (*model.GGOFieldTradeView, error) {
	return f.s.GGOFieldTradeAction(f.ctx, action, model.GGOFieldTradeActionRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(i), RequestID: requestID}, TradeID: tradeID, InitiatorID: f.ids[0], ReceiverID: f.ids[1], Version: version, Offer: offer}, f.sessions[i])
}

func TestGGOTradeRealTransactionAndReplay(t *testing.T) {
	f := ggoTradeSetup(t)
	trade := "00000000-0000-4000-8000-000000000001"
	invite, err := f.s.GGOFieldTradeInvite(f.ctx, model.GGOFieldTradeInviteRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: trade}, TargetPlayerID: f.ids[1]}, f.sessions[0])
	if err != nil || invite.Status != "invited" || invite.Version != 1 {
		t.Fatal(invite, err)
	}
	if v, e := f.action(1, "accept", "00000000-0000-4000-8000-000000000002", trade, 1, nil); e != nil || v.Status != "open" {
		t.Fatal(v, e)
	}
	if v, e := f.action(0, "offer", "00000000-0000-4000-8000-000000000003", trade, 1, []int64{3, 0, 0, 0, 0, 0}); e != nil || v.Version != 2 {
		t.Fatal(v, e)
	}
	if v, e := f.action(1, "offer", "00000000-0000-4000-8000-000000000004", trade, 2, []int64{0, 0, 2, 0, 0, 0}); e != nil || v.Version != 3 {
		t.Fatal(v, e)
	}
	if v, e := f.action(0, "confirm", "00000000-0000-4000-8000-000000000005", trade, 3, nil); e != nil || !v.Confirmed[0] {
		t.Fatal(v, e)
	}
	if v, e := f.action(1, "offer", "00000000-0000-4000-8000-000000000006", trade, 3, []int64{0, 0, 3, 0, 0, 0}); e != nil || v.Version != 4 || v.Confirmed[0] || v.Confirmed[1] {
		t.Fatal("offer must invalidate both confirmations", v, e)
	}
	if v, e := f.action(0, "confirm", "00000000-0000-4000-8000-000000000007", trade, 4, nil); e != nil || !v.Confirmed[0] {
		t.Fatal(v, e)
	}
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := f.action(1, "confirm", "00000000-0000-4000-8000-000000000008", trade, 4, nil)
			if e == nil && (v == nil || v.Status != "settled") {
				e = fmt.Errorf("bad settlement response")
			}
			out <- e
		}()
	}
	wg.Wait()
	close(out)
	for e := range out {
		if e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.action(1, "confirm", "00000000-0000-4000-8000-000000000008", trade, 3, nil); e == nil {
		t.Fatal("same request id with different payload accepted")
	}
	for i, id := range f.ids {
		var row db.Profile
		if e := db.GetIns().First(&row, id).Error; e != nil {
			t.Fatal(e)
		}
		_, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		want := [6]int64{7, 0, 3}
		if i == 1 {
			want = [6]int64{3, 0, 3}
		}
		if g.GGOField.Chips != want || g.GGOField.Revision != 1 || g.GGOField.Trade != nil {
			t.Fatal("bad committed chips", i, g.GGOField)
		}
		var count int64
		if e = db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND operation=? AND business_key=?", id, constUtil.GGOFieldOpTradeSettlement, trade).Count(&count).Error; e != nil || count != 1 {
			t.Fatal("missing settlement fact", i, count, e)
		}
	}
}
