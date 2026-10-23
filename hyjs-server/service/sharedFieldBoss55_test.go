package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestSharedFieldBoss55TwoPlayersDeadlineColdGenerationAndSingleKill(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	scene := "field_001"
	makeGame := func() *model.GameState {
		return &model.GameState{Field: &model.FieldState{Scene: scene, RespawnAt: map[string]int64{}, BossHP: map[string]map[string]float64{}}}
	}
	a := makeGame()
	b := makeGame()
	run := func(fn func(*gorm.DB) error) {
		t.Helper()
		if err := db.GetIns().Transaction(fn); err != nil {
			t.Fatal(err)
		}
	}
	run(func(tx *gorm.DB) error { return f.s.refreshSharedFieldBosses55(tx, a) })
	boss := a.Field.Bosses[0]
	monster, err := f.s.fieldBossMonster(boss.GetUuid(), a, boss)
	if err != nil {
		t.Fatal(err)
	}
	leader := f.s.fieldBossLeaderInScene(boss.GetBossId(), scene)
	a.Field.Fight = &model.FieldFight{BossLeaderGUID: "field-boss-" + boss.GetUuid() + "-" + leader["Id"]}
	input := &model.FrozenBattleInput{Monster: monster}
	final := map[string]float64{}
	for _, actor := range monster.GetFormation() {
		final[actor.GetGuid()] = 0
	}
	cloneRaw, _ := json.Marshal(a)
	var competing model.GameState
	json.Unmarshal(cloneRaw, &competing)
	type settlement struct {
		killed bool
		err    error
	}
	done := make(chan settlement, 2)
	for i, g := range []*model.GameState{a, &competing} {
		go func(i int, g *model.GameState) {
			killed := false
			err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				_, k, e := f.s.applySharedFieldBoss55(tx, f.id+int64(i), g.Field, g.Field.Bosses[0], input, final, 3600000)
				killed = k
				return e
			})
			done <- settlement{killed, err}
		}(i, g)
	}
	kills := 0
	for i := 0; i < 2; i++ {
		result := <-done
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.killed {
			kills++
		}
	}
	if kills != 1 {
		t.Fatal("concurrent terminal kill count", kills)
	}
	due := a.Field.RespawnAt[boss.GetUuid()]
	if due <= time.Now().UnixMilli() {
		t.Fatal("missing deadline")
	}
	run(func(tx *gorm.DB) error { return f.s.refreshSharedFieldBosses55(tx, b) })
	if b.Field.Bosses[0].GetUuid() != boss.GetUuid() || b.Field.RespawnAt[boss.GetUuid()] != due || b.Field.Bosses[0].GetState() != 3 {
		t.Fatal("second player did not share death")
	}
	run(func(tx *gorm.DB) error {
		_, killed, e := f.s.applySharedFieldBoss55(tx, f.id+1, a.Field, boss, input, final, 3600000)
		if killed {
			t.Fatal("second killing reward")
		}
		return e
	})
	countdown := fieldBossCountdownProjection(b.Field, time.Now())
	if countdown[boss.GetNature()].(*pb.SCSideBossCountdown).GetTime() < 3598 {
		t.Fatal("countdown missing")
	}
	f.open(t)
	cold := &GameService{Tables: f.s.Tables, ServerID: f.s.ServerID}
	if err := cold.InitializeSharedFieldBoss55(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := makeGame()
	run(func(tx *gorm.DB) error { return cold.refreshSharedFieldBosses55(tx, c) })
	if c.Field.RespawnAt[boss.GetUuid()] != due {
		t.Fatal("cold lost deadline")
	}
	if err := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), scene, boss.GetBossPosId()).Update("respawn_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	c.Field.RespawnAt["monster-preserved"] = due
	c.Field.BossHP["obsolete-boss"] = map[string]float64{}
	c.Field.RespawnAt["obsolete-boss"] = due
	run(func(tx *gorm.DB) error { return cold.refreshSharedFieldBosses55(tx, c) })
	if _, ok := c.Field.BossHP[boss.GetUuid()]; ok {
		t.Fatal("old generation hp retained")
	}
	if _, ok := c.Field.RespawnAt["obsolete-boss"]; ok {
		t.Fatal("obsolete boss deadline retained")
	}
	if c.Field.RespawnAt["monster-preserved"] != due {
		t.Fatal("monster timer removed")
	}
	if c.Field.Bosses[0].GetUuid() == boss.GetUuid() || c.Field.Bosses[0].GetState() != 1 {
		t.Fatal("no new generation")
	}
	run(func(tx *gorm.DB) error {
		applied, _, e := cold.applySharedFieldBoss55(tx, f.id, a.Field, boss, input, final, 3600000)
		if applied {
			t.Fatal("stale generation changed spawn")
		}
		return e
	})
	raw, _ := json.Marshal(a.Field)
	var saved model.FieldState
	json.Unmarshal(raw, &saved)
	if saved.RespawnAt[boss.GetUuid()] != due {
		t.Fatal("personal receipt deadline lost")
	}
}

func TestSharedFieldBoss55OriginalEngineKillAndColdReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitGems, f.s.InitCooking, f.s.InitializeSharedFieldBoss55} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(40); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		return f.s.refreshSharedFieldBosses55(tx, g)
	}); err != nil {
		t.Fatal(err)
	}
	before, game := fieldBoss33State(t, f.id)
	uuid := game.Field.Bosses[0].GetUuid()
	var authenticated db.Profile
	if err := db.GetIns().First(&authenticated, f.id).Error; err != nil {
		t.Fatal(err)
	}
	notices := 0
	p12NoticeSink(t, f.s, f.id, authenticated.Session, func(message proto.Message) error {
		notice, ok := message.(*pb.SCPublicMessage)
		if !ok || notice.GetPubmsg().GetType() != 1 || !strings.Contains(notice.GetPubmsg().GetMsg(), "击败") {
			t.Fatal("missing native boss kill marquee")
		}
		_, committed := fieldBoss33State(t, f.id)
		if !committed.Field.Fight.Settled || committed.Field.Bosses[0].GetState() != 3 || committed.Field.RespawnAt[uuid] <= time.Now().UnixMilli() {
			t.Fatal("kill was published before committed result and respawn deadline")
		}
		notices++
		return nil
	})
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Field.BossHP = map[string]map[string]float64{}
		g.Field.BossHP[uuid] = map[string]float64{}
		for _, row := range f.s.fieldBossRows("boss_001") {
			hp := float64(0)
			if row["TeamLeader"] == "1" {
				hp = 1
			}
			g.Field.BossHP[uuid]["field-boss-"+uuid+"-"+row["Id"]] = hp
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Explicit remnant fixture uses the shared source, then the real engine and real settlement.
	hpRaw, _ := json.Marshal(game.Field.BossHP[uuid])
	_ = hpRaw
	_, remnant := fieldBoss33State(t, f.id)
	hpRaw, _ = json.Marshal(remnant.Field.BossHP[uuid])
	if err := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND uuid=?", f.s.OriginalServerID(), "field_001", uuid).Update("hp", hpRaw).Error; err != nil {
		t.Fatal(err)
	}
	started, err := f.s.fieldBossFightStart(f.ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(uuid)})
	if err != nil {
		t.Fatal(err)
	}
	start := started[0].(*pb.SCSidePveBossFightStart)
	_, game = fieldBoss33State(t, f.id)
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil || terminal.Outcome.Winner != 0 {
		t.Fatal("actual engine must kill remnant", err)
	}
	req := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(uuid), Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}
	ended, err := f.s.fieldBossFightEnd(f.ctx, f.id, req)
	if err != nil {
		t.Fatal(err)
	}
	if notices != 1 || ended[2].(*pb.SCSideBossCountdown).GetTime() < 3598 {
		t.Fatal("committed kill did not publish once with native refresh countdown")
	}
	after, game := fieldBoss33State(t, f.id)
	if after.GetCoin()-before.GetCoin() != 51000 || after.GetDiamond()-before.GetDiamond() != 50 || game.Field.Bosses[0].GetState() != 3 {
		t.Fatal("original participation and kill rewards were not granted")
	}
	deadline := game.Field.RespawnAt[uuid]
	if delta := deadline - time.Now().UnixMilli(); delta < 3595000 || delta > 3605000 {
		t.Fatal("original hour deadline changed", delta)
	}
	f.restart(t)
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.fieldBossFightEnd(f.ctx, f.id, req)
	if err != nil || !proto.Equal(ended[0], again[0]) {
		t.Fatal("cold retry changed frozen reward", err)
	}
	cold, game := fieldBoss33State(t, f.id)
	if cold.GetCoin() != after.GetCoin() || cold.GetDiamond() != after.GetDiamond() || notices != 1 {
		t.Fatal("cold retry granted again")
	}

	account, err := LoginAccount(f.ctx, "shared-boss-b-"+uuid, "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, account.Id, "共享首领后来者", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(40); return nil }); err != nil {
		t.Fatal(err)
	}
	checkEntry := func() {
		t.Helper()
		if _, err := f.s.fieldTransfer(f.ctx, account.Id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		messages, err := f.s.fieldScene(f.ctx, account.Id, false)
		if err != nil {
			t.Fatal(err)
		}
		entitySeen, countdownSeen := false, false
		_, state := fieldBoss33State(t, account.Id)
		if state.Field.RespawnAt[uuid] != deadline {
			t.Fatal("later real profile deadline differs")
		}
		for _, message := range messages {
			switch m := message.(type) {
			case *pb.SCSideBoss:
				for _, b := range m.GetMonsters() {
					if b.GetUuid() == uuid && b.GetState() == 3 {
						entitySeen = true
					}
				}
			case *pb.SCSideBossCountdown:
				if m.GetNature() == game.Field.Bosses[0].GetNature() {
					expected := (deadline - time.Now().UnixMilli() + 999) / 1000
					if delta := m.GetTime() - expected; delta >= -1 && delta <= 1 && m.GetTime() > 0 {
						countdownSeen = true
					}
				}
			}
		}
		if !entitySeen || !countdownSeen {
			t.Fatal("later real entrant missing dead entity/countdown", entitySeen, countdownSeen)
		}
	}
	checkEntry()
	f.restart(t)
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	checkEntry()

}

func TestSharedFieldBoss55MigrationMaximumDeadlineOnce(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(time.Hour).UnixMilli()
	for i := 0; i < 3; i++ {
		b, err := f.s.fieldBossSpawn("boss_pos_001")
		if err != nil {
			t.Fatal(err)
		}
		b.State = proto.Int32(3)
		deadline := due + int64(i)*1000
		g := model.GameState{Field: &model.FieldState{Scene: "field_001", Bosses: []*pb.SideBoss{b}, RespawnAt: map[string]int64{b.GetUuid(): deadline}}}
		// Current scene has an older death; cached visited scene carries the greatest deadline.
		cached := proto.Clone(b).(*pb.SideBoss)
		cached.Uuid = proto.String(b.GetUuid() + "-cached")
		g.Field.Areas = map[string]*model.FieldAreaState{"field_001": {Bosses: []*pb.SideBoss{cached}, RespawnAt: map[string]int64{cached.GetUuid(): deadline + 5000}, BossHP: map[string]map[string]float64{cached.GetUuid(): {}}}}
		raw, _ := json.Marshal(g)
		game := string(raw)
		sid := f.s.OriginalServerID()
		if i == 2 {
			sid++
		}
		row := db.Profile{Session: []byte{}, Uid: "migration-boss-" + b.GetUuid(), ServerID: sid, Game: &game}
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	var shared db.SharedFieldBoss55
	if err := db.GetIns().Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), "field_001", "boss_pos_001").First(&shared).Error; err != nil {
		t.Fatal(err)
	}
	if shared.RespawnAt != due+6000 {
		t.Fatal("wrong max/server deadline", shared.RespawnAt, due)
	}
	if err := db.GetIns().Model(&shared).Update("respawn_at", due+10000).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	var again db.SharedFieldBoss55
	if err := db.GetIns().Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), "field_001", "boss_pos_001").First(&again).Error; err != nil {
		t.Fatal(err)
	}
	if again.RespawnAt != due+10000 {
		t.Fatal("migration reran")
	}
}
