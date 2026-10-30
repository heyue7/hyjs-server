package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The migration case owns a newly allocated p1 database. It never removes an
// existing table to manufacture the old schema, including on MySQL.
func TestFieldBossLines85LegacyPrimaryKeyMigration(t *testing.T) {
	f := p1Setup(t)
	if db.GetIns().Migrator().HasTable(&db.SharedFieldBoss55{}) {
		t.Skip("legacy-key test needs a fresh isolated p1 database")
	}
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	legacyDDL := `CREATE TABLE shared_field_boss55 (
		server_id BIGINT NOT NULL, scene VARBINARY(64) NOT NULL, pos_id VARBINARY(64) NOT NULL,
		uuid VARBINARY(128) NOT NULL, payload LONGBLOB NOT NULL, hp LONGBLOB NOT NULL,
		death_rules73 LONGBLOB NULL, respawn_at BIGINT NOT NULL, killer_id BIGINT NOT NULL,
		PRIMARY KEY (server_id, scene, pos_id))`
	if err := db.GetIns().Exec(legacyDDL).Error; err != nil {
		t.Fatal(err)
	}
	pos := splitNonEmpty(f.s.fieldSceneRow("field_001")["bossPosIds"])[0]
	boss, err := f.s.fieldBossSpawn(pos)
	if err != nil {
		t.Fatal(err)
	}
	boss.State, boss.CurHp = proto.Int32(3), proto.Float64(0)
	payload, err := json.Marshal(boss)
	if err != nil {
		t.Fatal(err)
	}
	hp := []byte(fmt.Sprintf(`{"field-boss-%s-1":0}`, boss.GetUuid()))
	rules := []byte(`{"deathSettlement":true,"id":"legacy"}`)
	due := time.Now().Add(time.Hour).UnixMilli()
	insert := `INSERT INTO shared_field_boss55
		(server_id,scene,pos_id,uuid,payload,hp,death_rules73,respawn_at,killer_id)
		VALUES (?,?,?,?,?,?,?,?,?)`
	if err := db.GetIns().Exec(insert, f.s.OriginalServerID(), "", "", "migration55", []byte(`{}`), []byte(`{}`), nil, 0, 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Exec(insert, f.s.OriginalServerID(), "field_001", pos, boss.GetUuid(), payload, hp, rules, due, f.id).Error; err != nil {
		t.Fatal(err)
	}
	assertOld := func() {
		t.Helper()
		row := fieldBossLines85Row(t, f, "field_001", pos, 1)
		if row.UUID != boss.GetUuid() || row.RespawnAt != due || row.KillerID != f.id || !bytes.Equal(row.Payload, payload) || !bytes.Equal(row.HP, hp) || !bytes.Equal(row.DeathRules73, rules) {
			t.Fatalf("legacy generation changed during migration: %+v", row)
		}
	}
	for i := 0; i < 2; i++ {
		if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
			t.Fatal("repeatable legacy migration", i, err)
		}
		assertOld()
	}
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	// A persisted old selection models a player returning on line two.
	if err := db.GetIns().Create(&db.P10ScenePresence{PlayerID: f.id, Scene: "field_001", Line: 2, Frame: []byte{}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	_, state := fieldBoss33State(t, f.id)
	if state.Field.BossLine85 != 2 || len(state.Field.Bosses) != 1 || state.Field.Bosses[0].GetUuid() == boss.GetUuid() {
		t.Fatal("line two did not independently spawn after migration", state.Field)
	}
	newRow := fieldBossLines85Row(t, f, "field_001", pos, 2)
	if newRow.UUID != state.Field.Bosses[0].GetUuid() || newRow.RespawnAt != 0 {
		t.Fatal("new line did not get a live row", newRow)
	}
	if err := db.GetIns().Create(&newRow).Error; err == nil {
		t.Fatal("duplicate realm, scene, line and position was accepted")
	}
	assertOld()
}

func TestFieldBossLines85PresenceFallback(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !db.GetIns().Migrator().HasTable(&db.P10ScenePresence{}) {
		line, err := f.s.fieldBossCurrentLine85(db.GetIns(), f.id)
		if err != nil || line != 1 {
			t.Fatal("missing presence table must select line one", line, err)
		}
		if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		_, game := fieldBoss33State(t, f.id)
		if game.Field.BossLine85 != 1 || len(game.Field.Bosses) != 1 {
			t.Fatal("missing table did not project the original line-one spawn")
		}
	} else {
		t.Log("presence table already exists; missing-table branch requires a fresh database")
	}
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	line, err := f.s.fieldBossCurrentLine85(db.GetIns(), f.id)
	if err != nil || line != 1 {
		t.Fatal("missing presence row must select line one", line, err)
	}
	if err := db.GetIns().Create(&db.P10ScenePresence{PlayerID: f.id, Scene: "field_001", Line: 0, Frame: []byte{}}).Error; err != nil {
		t.Fatal(err)
	}
	line, err = f.s.fieldBossCurrentLine85(db.GetIns(), f.id)
	if err != nil || line != 1 {
		t.Fatal("legacy zero line must select line one", line, err)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	line, err = f.s.fieldBossCurrentLine85(db.GetIns(), f.id)
	if err != nil || line != 2 {
		t.Fatal("persisted line was not authoritative", line, err)
	}
}

func fieldBossLines85Setup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f := p1Setup(t)
	// Separate realm rows let a dedicated MySQL p1 database run several cases.
	f.s.ServerID = 10000000 + f.id
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	fieldBossLines85Initialize(t, f, false)
	newPlayer := func(name string) int64 {
		t.Helper()
		account, err := LoginAccount(f.ctx, fmt.Sprintf("field85-%s-%d", name, time.Now().UnixNano()), "isolated-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, account.Id, name, "hero_043"); err != nil {
			t.Fatal(err)
		}
		return account.Id
	}
	f.id = newPlayer("线路甲")
	b, c := newPlayer("线路乙"), newPlayer("线路丙")
	for _, id := range []int64{f.id, b, c} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
			// Level is the finite access fixture; battle attributes use original tables.
			p.Level = proto.Int32(100)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, b, c} {
		p10SceneConnect(t, f, id, &p10SceneRecorder{})
	}
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, c, &pb.CSTransferLine{Line: proto.Int32(2)}, 0); err != nil || !handled {
		t.Fatal("server line transfer", handled, err)
	}
	return f, b, c
}

func fieldBossLines85Initialize(t *testing.T, f *p1Fixture, freshService bool) {
	t.Helper()
	if freshService {
		serverID := f.s.ServerID
		f.s = &GameService{ServerID: serverID}
		f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
		if err := f.s.LoadTables("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{
		f.s.InitializeP1, f.s.InitializeP5, f.s.InitializeDailyCopy,
		f.s.InitGems, f.s.InitCooking, f.s.InitializeP10Scene,
		f.s.InitializeSharedFieldBoss55, f.s.InitializeFieldBossDeath73,
	} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func fieldBossLines85Row(t *testing.T, f *p1Fixture, scene, pos string, line int64) db.SharedFieldBoss55 {
	t.Helper()
	var row db.SharedFieldBoss55
	if err := db.GetIns().Where("server_id=? AND scene=? AND line=? AND pos_id=?", f.s.OriginalServerID(), scene, line, pos).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func fieldBossLines85View(t *testing.T, f *p1Fixture, id int64) *model.GameState {
	t.Helper()
	if _, err := f.s.fieldScene(f.ctx, id, false); err != nil {
		t.Fatal(err)
	}
	_, game := fieldBoss33State(t, id)
	if game.Field == nil || len(game.Field.Bosses) != 1 {
		t.Fatal("missing original field boss projection")
	}
	return game
}

func fieldBossLines85FixtureHP(t *testing.T, f *p1Fixture, id int64, leaderHP float64) (string, float64) {
	t.Helper()
	_, game := fieldBoss33State(t, id)
	boss := game.Field.Bosses[0]
	monster, err := f.s.fieldBossMonster(boss.GetUuid(), game, boss)
	if err != nil {
		t.Fatal(err)
	}
	leader := f.s.fieldBossLeaderInScene(boss.GetBossId(), game.Field.Scene)
	leaderGUID := "field-boss-" + boss.GetUuid() + "-" + leader["Id"]
	hp := map[string]float64{}
	var full float64
	for _, actor := range monster.GetFormation() {
		hp[actor.GetGuid()] = 0
		if actor.GetGuid() == leaderGUID {
			full = actor.GetProp().GetHp()
			if leaderHP < 0 {
				hp[leaderGUID] = full
			} else {
				hp[leaderGUID] = leaderHP
			}
		}
	}
	if full <= 1 || len(hp) != 4 {
		t.Fatal("original four-member boss fixture invalid")
	}
	raw, err := json.Marshal(hp)
	if err != nil {
		t.Fatal(err)
	}
	updated := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND line=? AND pos_id=? AND uuid=?", f.s.OriginalServerID(), game.Field.Scene, 1, boss.GetBossPosId(), boss.GetUuid()).Update("hp", raw)
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatal("failed to set finite line-one HP fixture", updated.Error, updated.RowsAffected)
	}
	return leaderGUID, full
}

func fieldBossLines85Start(t *testing.T, f *p1Fixture, id int64, uuid string) (*pb.SCSidePveBossFightStart, *model.FrozenBattleInput) {
	t.Helper()
	started, err := f.s.fieldBossFightStart(f.ctx, id, &pb.CSSidePveBossFightStart{Uuid: proto.String(uuid)})
	if err != nil {
		t.Fatal("original boss start", err)
	}
	_, game := fieldBoss33State(t, id)
	if game.Field.Fight == nil || game.Field.Fight.BossLine85 != 1 {
		t.Fatal("accepted fight did not freeze line one", game.Field.Fight)
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	return started[0].(*pb.SCSidePveBossFightStart), input
}

func fieldBossLines85Finish(t *testing.T, f *p1Fixture, id int64, uuid string, start *pb.SCSidePveBossFightStart, input *model.FrozenBattleInput) {
	t.Helper()
	terminal, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("original battle computation", err)
	}
	if terminal.Outcome.Winner != 0 && terminal.Outcome.Winner != 1 {
		t.Fatal("original battle produced no terminal winner", terminal.Outcome.Winner)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(uuid), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	if _, err := f.s.fieldBossFightEnd(f.ctx, id, request); err != nil {
		t.Fatal("original replay and shared apply", err)
	}
}

func TestFieldBossLines85RealFightColdAndIndependentRespawn(t *testing.T) {
	for _, scene := range []string{"field_001", "field_002"} {
		t.Run(scene, func(t *testing.T) {
			f, peer, otherLine := fieldBossLines85Setup(t)
			for _, id := range []int64{f.id, peer, otherLine} {
				if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
					t.Fatal(err)
				}
				fieldBossLines85View(t, f, id)
				if err := f.s.SyncP10Scene(f.ctx, id, false); err != nil {
					t.Fatal(err)
				}
			}
			a := fieldBossLines85View(t, f, f.id)
			b := fieldBossLines85View(t, f, peer)
			c := fieldBossLines85View(t, f, otherLine)
			boss1, boss2 := a.Field.Bosses[0], c.Field.Bosses[0]
			if a.Field.BossLine85 != 1 || b.Field.BossLine85 != 1 || c.Field.BossLine85 != 2 || boss1.GetUuid() == "" || boss1.GetUuid() != b.Field.Bosses[0].GetUuid() || boss1.GetUuid() == boss2.GetUuid() || boss2.GetCurHp() != 1 {
				t.Fatal("same-line sharing or cross-line independent spawn failed")
			}
			pos := boss1.GetBossPosId()
			if boss2.GetBossPosId() != pos {
				t.Fatal("original positions differ across lines")
			}
			for _, step := range []struct {
				line int32
				uuid string
			}{{2, boss2.GetUuid()}, {1, boss1.GetUuid()}} {
				if _, handled, err := f.s.DispatchP10Scene(f.ctx, peer, &pb.CSTransferLine{Line: proto.Int32(step.line)}, 0); err != nil || !handled {
					t.Fatal("actual field line transfer", step.line, handled, err)
				}
				changed := fieldBossLines85View(t, f, peer)
				if changed.Field.BossLine85 != int64(step.line) || changed.Field.Bosses[0].GetUuid() != step.uuid {
					t.Fatal("line transfer did not change boss projection", step.line)
				}
			}
			// Distinct harmless rule IDs prove each generation freezes its own row.
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				one, err := f.s.freezeFieldBossDeath73(tx, scene, 1, boss1, map[string]string{"adminDeathRules73": `{"id":"line-one"}`})
				if err != nil {
					return err
				}
				two, err := f.s.freezeFieldBossDeath73(tx, scene, 2, boss2, map[string]string{"adminDeathRules73": `{"id":"line-two"}`})
				if err != nil {
					return err
				}
				retry, err := f.s.freezeFieldBossDeath73(tx, scene, 1, boss1, map[string]string{"adminDeathRules73": `{"id":"changed"}`})
				if err != nil {
					return err
				}
				if one.ID != "line-one" || two.ID != "line-two" || retry.ID != "line-one" {
					return fmt.Errorf("rules crossed lines or changed after freeze")
				}
				return nil
			}); err != nil {
				t.Fatal("line-bound death rules", err)
			}
			line2Before := fieldBossLines85Row(t, f, scene, pos, 2)

			// Three defeated adds and one full-health leader shorten the real
			// first fight while keeping a live boss for the partial-damage check.
			leaderGUID, full := fieldBossLines85FixtureHP(t, f, f.id, -1)
			start, input := fieldBossLines85Start(t, f, f.id, boss1.GetUuid())
			fieldBossLines85Finish(t, f, f.id, boss1.GetUuid(), start, input)
			a = fieldBossLines85View(t, f, f.id)
			b = fieldBossLines85View(t, f, peer)
			c = fieldBossLines85View(t, f, otherLine)
			damaged := b.Field.BossHP[boss1.GetUuid()][leaderGUID]
			if b.Field.Bosses[0].GetUuid() != boss1.GetUuid() || b.Field.Bosses[0].GetState() != 1 || damaged <= 0 || damaged >= full || b.Field.Bosses[0].GetCurHp() != damaged/full || !reflect.DeepEqual(a.Field.BossHP[boss1.GetUuid()], b.Field.BossHP[boss1.GetUuid()]) {
				t.Fatal("same-line player did not see real partial damage", damaged, full, b.Field.Bosses[0])
			}
			if c.Field.Bosses[0].GetUuid() != boss2.GetUuid() || c.Field.Bosses[0].GetCurHp() != 1 || !reflect.DeepEqual(line2Before, fieldBossLines85Row(t, f, scene, pos, 2)) {
				t.Fatal("partial damage reached the other line")
			}

			// The second finite fixture leaves only one HP on the original leader;
			// the real engine and normal end path still decide the result.
			fieldBossLines85FixtureHP(t, f, peer, 1)
			start, input = fieldBossLines85Start(t, f, peer, boss1.GetUuid())
			if _, handled, err := f.s.DispatchP10Scene(f.ctx, peer, &pb.CSTransferLine{Line: proto.Int32(2)}, 0); err == nil || !handled {
				t.Fatal("pending fight allowed a line transfer", handled, err)
			}
			var presence db.P10ScenePresence
			if err := db.GetIns().First(&presence, "player_id=?", peer).Error; err != nil || presence.Line != 1 {
				t.Fatal("rejected transfer changed saved line", presence.Line, err)
			}
			// Simulate a pre-existing inconsistent context that normal transfer
			// rejects: the accepted fight remains owned by its frozen line.
			if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", peer).Update("line", 2).Error; err != nil {
				t.Fatal(err)
			}
			if scene == "field_002" {
				if err := updateProfile(f.ctx, peer, func(_ *pb.Player, game *model.GameState) error {
					game.Field.Fight.BossLine85 = 0 // Legacy accepted fights belong to line one.
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			fieldBossLines85Finish(t, f, peer, boss1.GetUuid(), start, input)
			_, settled := fieldBoss33State(t, peer)
			if settled.Field.BossLine85 != 2 || settled.Field.Fight == nil || !settled.Field.Fight.Settled {
				t.Fatal("test did not exercise a different current projection line")
			}
			dead := fieldBossLines85Row(t, f, scene, pos, 1)
			if dead.UUID != boss1.GetUuid() || dead.RespawnAt <= time.Now().UnixMilli() || dead.KillerID != peer {
				t.Fatal("frozen line did not receive the actual kill", dead)
			}
			a = fieldBossLines85View(t, f, f.id)
			c = fieldBossLines85View(t, f, otherLine)
			if a.Field.Bosses[0].GetUuid() != boss1.GetUuid() || a.Field.Bosses[0].GetState() != 3 || a.Field.RespawnAt[boss1.GetUuid()] != dead.RespawnAt {
				t.Fatal("same-line player missed death and deadline")
			}
			if c.Field.Bosses[0].GetUuid() != boss2.GetUuid() || c.Field.Bosses[0].GetCurHp() != 1 || !reflect.DeepEqual(line2Before, fieldBossLines85Row(t, f, scene, pos, 2)) {
				t.Fatal("frozen fight changed the other line")
			}
			b = fieldBossLines85View(t, f, peer)
			if b.Field.BossLine85 != 2 || b.Field.Bosses[0].GetUuid() != boss2.GetUuid() {
				t.Fatal("current projection did not switch to the selected line")
			}

			f.restart(t)
			fieldBossLines85Initialize(t, f, true)
			a = fieldBossLines85View(t, f, f.id)
			c = fieldBossLines85View(t, f, otherLine)
			if a.Field.Bosses[0].GetUuid() != boss1.GetUuid() || a.Field.Bosses[0].GetState() != 3 || a.Field.RespawnAt[boss1.GetUuid()] != dead.RespawnAt || c.Field.Bosses[0].GetUuid() != boss2.GetUuid() || !reflect.DeepEqual(line2Before, fieldBossLines85Row(t, f, scene, pos, 2)) {
				t.Fatal("cold reopen lost line ownership or deadline")
			}
			// Expire only the owned line-one deadline; line two's refresh must
			// leave that expired row untouched until line one is accessed.
			past := time.Now().Add(-time.Second).UnixMilli()
			result := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND line=? AND pos_id=?", f.s.OriginalServerID(), scene, 1, pos).Update("respawn_at", past)
			if result.Error != nil || result.RowsAffected != 1 {
				t.Fatal("deadline fixture", result.Error, result.RowsAffected)
			}
			fieldBossLines85View(t, f, otherLine)
			stillDead := fieldBossLines85Row(t, f, scene, pos, 1)
			if stillDead.UUID != boss1.GetUuid() || stillDead.RespawnAt != past {
				t.Fatal("line two refreshed a different line", stillDead)
			}
			fresh := fieldBossLines85View(t, f, f.id)
			row1 := fieldBossLines85Row(t, f, scene, pos, 1)
			if row1.UUID == boss1.GetUuid() || row1.RespawnAt != 0 || fresh.Field.Bosses[0].GetUuid() != row1.UUID || fresh.Field.Bosses[0].GetCurHp() != 1 || !reflect.DeepEqual(line2Before, fieldBossLines85Row(t, f, scene, pos, 2)) {
				t.Fatal("independent line-one respawn failed")
			}
		})
	}
}
