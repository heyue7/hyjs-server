package service

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Use the real native frame parser/dispatcher and the committed server combat
// result. Shrinking the isolated shared HP after two Starts simulates earlier
// contributors; no client-reported damage is used as the kill decision.
func TestGGOBossNoticeNativeCommitRollbackRetryAndScope(t *testing.T) {
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	ggoBossLines86UseLegacyEvent(t, f, event)
	peer, _ := ggoBossLines86Peer(t, f, ctx, event, 1)
	stale, _ := ggoBossLines86Peer(t, f, ctx, event, 1)
	var peerProfile db.Profile
	if err := db.GetIns().First(&peerProfile, peer).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Name = proto.String("[ff0000]勇者[-]\n")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hero := f.s.table("Hero", "id", event.Boss.HeroID)
	if hero == nil || hero["name"] == "" {
		t.Fatal("fixture missing localized boss name key")
	}
	f.s.AdminCatalogueData = &model.AdminCatalogueData{DisplayNames: map[string]string{hero["name"]: "[red]沙漠魔王[-]"}}
	cache := func() {
		t.Helper()
		database := db.GetIns()
		sqlDB, err := database.DB()
		if err != nil {
			t.Fatal(err)
		}
		f.s.adminFieldRuntimeCache.Store(&adminFieldRuntimeSnapshot{serverID: f.s.OriginalServerID(), database: database, sqlDatabase: sqlDB,
			signature: adminFieldDeploymentSignature(nil), tables: f.s.Tables, ggo: f.s.GGOFieldConfig, ggoEconomy: f.s.GGOFieldEconomy,
			ggoRevision: "notice-test-revision"})
	}
	cache()
	counts := map[int64]int{}
	record := func(id int64) func(proto.Message) error {
		return func(message proto.Message) error {
			frame, ok := message.(*pb.SCPublicMessage)
			if !ok {
				return nil
			}
			raw, err := model.EncodeGameResponses([]proto.Message{frame})
			if err != nil || len(raw) < 8 || binary.BigEndian.Uint32(raw[4:8]) != 40001 || int(binary.BigEndian.Uint32(raw[:4])) != len(raw)-8 {
				t.Errorf("invalid original 40001 response frame: %v", err)
				return err
			}
			var decoded pb.SCPublicMessage
			if err = proto.Unmarshal(raw[8:], &decoded); err != nil {
				t.Error(err)
				return err
			}
			msg := decoded.GetPubmsg().GetMsg()
			if decoded.GetPubmsg().GetType() != 1 || decoded.GetPubmsg().GetScrollType() != 0 ||
				strings.ContainsAny(msg, "[]\n") || !strings.Contains(msg, "勇者") || !strings.Contains(msg, "沙漠魔王") {
				t.Errorf("incorrect or unsanitized marquee: %q", msg)
			}
			var row db.SharedFieldBoss55
			var receipts int64
			err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&row).Error
			if err == nil {
				err = db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND operation=?", f.id, constUtil.GGOFieldBossFightOperation).Count(&receipts).Error
			}
			if err != nil || row.KillerID != f.id || row.RespawnAt == 0 || receipts != 2 {
				t.Errorf("notice preceded committed death and receipt: killer=%d receipts=%d err=%v", row.KillerID, receipts, err)
			}
			counts[id]++
			return nil
		}
	}
	p12NoticeSink(t, f.s, f.id, session, record(f.id))
	p12NoticeSink(t, f.s, peer, peerProfile.Session, record(peer))
	p12NoticeSink(t, &GameService{ServerID: 2}, f.id, session, func(proto.Message) error {
		t.Error("different server service received marquee")
		return nil
	})
	makeEnd := func(id int64) *pb.CSSidePveBossFightEnd {
		t.Helper()
		messages := bossHandshake86Call(t, f, id, 30083, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
		start, ok := messages[0].(*pb.SCSidePveBossFightStart)
		if !ok || start.GetFightId() == "" {
			t.Fatalf("original 30083 Start response: %v", messages)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		input, err := fieldBossFrozenInput(game.Field.Fight)
		if err != nil {
			t.Fatal(err)
		}
		terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
		if err != nil || terminal == nil || terminal.OriginalOperates == nil {
			t.Fatal("real server battle result", err)
		}
		result := int32(0)
		if terminal.Outcome.Winner == 0 {
			result = 1
		}
		return &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	}
	partial := makeEnd(f.id)
	bossHandshake86Call(t, f, f.id, 30085, partial)
	if len(counts) != 0 {
		t.Fatal("partial damage sent a kill notice", counts)
	}
	lethal := makeEnd(f.id)
	competing := makeEnd(peer)
	staleKey := p10ClientKey{f.s.p10ClientOwner(), stale}
	staleSink := &p10ClientSink{session: []byte("expired-session"), send: func(proto.Message) error {
		t.Error("expired session received marquee")
		return nil
	}}
	p10ClientSinks.Store(staleKey, staleSink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(staleKey, staleSink) })
	if err := sharedFieldBoss55Query(db.GetIns().Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).
		Update("hp", adminJSON(map[string]float64{ggoBossGUID(event.ID): 1})).Error; err != nil {
		t.Fatal(err)
	}
	broken := errors.New("isolated rollback after boss death")
	const callback = "ggo-boss-notice-rollback"
	if err := db.GetIns().Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "ggo_field_receipts" {
			tx.AddError(broken)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Create().Remove(callback) })
	rollback := bossHandshake86Call(t, f, f.id, 30085, lethal, true)
	db.GetIns().Callback().Create().Remove(callback)
	if len(rollback) != 1 {
		t.Fatal("rollback did not reject original End", rollback)
	}
	if _, ok := rollback[0].(*pb.ErrorMessage); !ok || len(counts) != 0 {
		t.Fatal("rollback emitted notice or succeeded", rollback, counts)
	}
	var row db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	_, _, hp, err := ggoBossDecode(&row)
	if err != nil || row.KillerID != 0 || hp[ggoBossGUID(event.ID)] != 1 {
		t.Fatal("rollback persisted death", row.KillerID, hp, err)
	}
	first := bossHandshake86Call(t, f, f.id, 30085, lethal)
	if counts[f.id] != 1 || counts[peer] != 1 {
		t.Fatal("committed kill did not publish once to live realm sessions", counts)
	}
	for i := 0; i < 3; i++ {
		repeat := bossHandshake86Call(t, f, f.id, 30085, lethal)
		if !proto.Equal(first[0], repeat[0]) {
			t.Fatal("native receipt changed on retry")
		}
	}
	bossHandshake86Call(t, f, peer, 30085, competing)
	if counts[f.id] != 1 || counts[peer] != 1 {
		t.Fatal("retry or a later finish repeated the notice", counts)
	}
	view, err := f.s.GGOFieldSnapshot(ctx, model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, session)
	if err != nil || view == nil || view.Boss == nil || view.Boss.State != "waiting" ||
		view.Boss.EventID != event.ID || view.Boss.StartsAtMS <= view.ServerTimeMS {
		t.Fatal("authenticated snapshot missing respawn countdown", view, err)
	}
	if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&row).Error; err != nil || view.Boss.StartsAtMS != row.RespawnAt {
		t.Fatal("snapshot deadline differs from committed death", view.Boss, row.RespawnAt, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, exists := p10ClientSinks.Load(staleKey)
		if !exists || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, exists := p10ClientSinks.Load(staleKey); exists {
		t.Fatal("stale session was not retired")
	}
	f.restart(t)
	cache()
	for _, item := range []struct {
		id      int64
		request *pb.CSSidePveBossFightEnd
	}{{f.id, lethal}, {peer, competing}} {
		bossHandshake86Call(t, f, item.id, 30085, item.request)
	}
	if counts[f.id] != 1 || counts[peer] != 1 {
		t.Fatal("cold replay sent a historical notice", counts)
	}
}
