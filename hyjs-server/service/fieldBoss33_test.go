package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func fieldBoss33State(t *testing.T, id int64) (*pb.Player, *model.GameState) {
	t.Helper()
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	return player, game
}

// TestFieldBoss33OriginalDataSpawnCheckColdAndOriginalReplay covers the two
// original maps without substituting rewards, respawn or a fake winner.
func TestFieldBoss33OriginalDataSpawnCheckColdAndOriginalReplay(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, scene := range []string{"field_001", "field_002"} {
		row := f.s.fieldSceneRow(scene)
		if row == nil || row["bossPosIds"] == "" {
			t.Fatal("missing original boss position", scene)
		}
		for _, posID := range splitNonEmpty(row["bossPosIds"]) {
			pos := f.s.table("FieldBossPos", "BoxxPosId", posID)
			if pos == nil || pos["BornPos"] == "" || pos["bossIdProbs"] == "" {
				t.Fatal("missing original boss position row", posID)
			}
			bossID := strings.SplitN(pos["bossIdProbs"], ",", 2)[0]
			rows := f.s.fieldBossRows(bossID)
			if len(rows) != 4 || f.s.fieldBossLeader(bossID) == nil || f.s.fieldBossLeader(bossID)["RebornTime"] != "1" {
				t.Fatal("original first boss group/reborn hour incomplete", posID)
			}
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	_, game := fieldBoss33State(t, f.id)
	if len(game.Field.Bosses) != 1 || game.Field.Bosses[0].GetBossPosId() != "boss_pos_001" || game.Field.Bosses[0].GetState() != 1 || game.Field.Bosses[0].GetCurHp() != 1 {
		t.Fatal("forest boss not spawned at original position")
	}
	bossUUID := game.Field.Bosses[0].GetUuid()
	if out := f.s.fieldBossProjection(game); len(out) != 1 || out[0].(*pb.SCSideBoss).GetMonsters()[0].GetUuid() != bossUUID {
		t.Fatal("field boss projection missing")
	}
	check, err := f.s.fieldBossCheck(f.ctx, f.id, &pb.CSSideBossCheck{Uuid: proto.String(bossUUID)})
	if err != nil || check[0].(*pb.SCSideBossCheck).GetCode() != 1 {
		t.Fatal("live original boss check failed", err)
	}

	started, err := f.s.fieldBossFightStart(f.ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(bossUUID)})
	if err != nil {
		t.Fatal("original field boss start failed", err)
	}
	start := started[0].(*pb.SCSidePveBossFightStart)
	if start.GetFightScene() != "" || start.GetCode() != 1 || start.GetMaxround() != 40 || len(start.GetBattleMonsterList()) != 1 || len(start.GetBattleMonsterList()[0].GetFormation()) != 4 {
		t.Fatal("field boss start contract differs")
	}
	_, pending := fieldBoss33State(t, f.id)
	if pending.Field.Fight == nil || pending.Field.Fight.BossStart == nil || pending.Field.Fight.BossCooldownMS != 3600000 || pending.Field.Bosses[0].GetState() != 2 {
		t.Fatal("field boss did not reuse FieldFight and one-hour original cooldown")
	}
	if _, err = f.s.fieldBossFightStart(f.ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(bossUUID)}); err != nil || pending.Field.Fight.BossStart.GetFightId() != start.GetFightId() {
		t.Fatal("same boss retry did not return frozen start", err)
	}
	if _, err = f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(pending.Field.Monsters[0].GetUuid())}); err == nil {
		t.Fatal("pending boss did not block ordinary field battle")
	}

	// 登录恢复必须释放占用与state2，且不能结算奖励。
	if _, err = f.s.fieldScene(f.ctx, f.id, true); err != nil {
		t.Fatal(err)
	}
	_, cold := fieldBoss33State(t, f.id)
	if cold.Field.Fight != nil || cold.Field.Bosses[0].GetState() != 1 {
		t.Fatal("cold login did not release pending boss without reward")
	}
	if _, err = f.s.fieldBossFightEnd(f.ctx, f.id, &pb.CSSidePveBossFightEnd{FightId: proto.String(start.GetFightId()), Uuid: proto.String(bossUUID), Result: proto.Int32(1)}); err == nil {
		t.Fatal("cold stale fight was accepted")
	}

	started, err = f.s.fieldBossFightStart(f.ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(bossUUID)})
	if err != nil {
		t.Fatal(err)
	}
	start = started[0].(*pb.SCSidePveBossFightStart)
	_, replayState := fieldBoss33State(t, f.id)
	frozen := replayState.Field.Fight.BossInput
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(f.ctx, frozen, nil)
	if err != nil || terminal.OriginalOperates == nil {
		t.Fatal("original type12 replay failed", err)
	}
	replayed, replayErr := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, frozen, terminal.OriginalOperates)
	if replayErr != nil {
		t.Fatal("original recorded driver", replayErr)
	}
	if !proto.Equal(terminal.OriginalOperates, replayed.OriginalOperates) {
		for i, op := range terminal.OriginalOperates.GetSkillOpts() {
			if i < len(replayed.OriginalOperates.GetSkillOpts()) && !proto.Equal(op, replayed.OriginalOperates.SkillOpts[i]) {
				t.Logf("recorded driver operation %d: source=%v replay=%v", i, op, replayed.OriginalOperates.SkillOpts[i])
				break
			}
		}
	}
	forged := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	forged.DefenderTotalLeftHp = proto.Float64(forged.GetDefenderTotalLeftHp() + 1)
	if _, err = f.s.fieldBossFightEnd(f.ctx, f.id, &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(bossUUID), OptRecord: forged}); err == nil {
		t.Fatal("forged recorded terminal HP was accepted")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	// A forged client result or absent operation record is rejected before replay.
	if _, err = f.s.fieldBossFightEnd(f.ctx, f.id, &pb.CSSidePveBossFightEnd{FightId: proto.String(start.GetFightId()), Uuid: proto.String(bossUUID), Result: proto.Int32(1 - result), OptRecord: terminal.OriginalOperates}); err == nil {
		t.Fatal("forged boss winner was accepted")
	}
	ended, err := f.s.fieldBossFightEnd(f.ctx, f.id, &pb.CSSidePveBossFightEnd{FightId: proto.String(start.GetFightId()), Uuid: proto.String(bossUUID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates})
	if err != nil {
		t.Fatal("original field boss settlement failed", err)
	}
	if ended[0].(*pb.SCSidePveBossFightEnd).GetMonster() == nil {
		t.Fatal("field boss result has no state")
	}
	_, settled := fieldBoss33State(t, f.id)
	if settled.Field.Fight == nil || !settled.Field.Fight.Settled || settled.Field.BossHP[bossUUID] == nil {
		t.Fatal("field boss settlement did not persist slot HP")
	}
	if settled.Field.Bosses[0].GetState() == 3 {
		due := settled.Field.RespawnAt[bossUUID]
		delta := due - time.Now().UnixMilli()
		if delta < 3600*1000-5000 || delta > 3600*1000+5000 {
			t.Fatal("dead boss did not keep original one-hour respawn", delta)
		}
	}
	again, err := f.s.fieldBossFightEnd(f.ctx, f.id, &pb.CSSidePveBossFightEnd{FightId: proto.String(start.GetFightId()), Uuid: proto.String(bossUUID)})
	if err != nil || !proto.Equal(again[0], ended[0]) {
		t.Fatal("settled boss repeated settlement or reward", err)
	}
}

func splitNonEmpty(value string) []string {
	out := []string{}
	for _, part := range strings.Split(value, ";") {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// A finite saved-health fixture represents a previous attempt that killed the
// three adds. The real original engine must kill the remaining leader; no test
// winner, reward or battle result is substituted.
func TestFieldBoss33RemnantVictoryRewardsOnceColdAndRespawn(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitGems, f.s.InitCooking} {
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
	before, game := fieldBoss33State(t, f.id)
	uuid := game.Field.Bosses[0].GetUuid()
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
	started, err := f.s.fieldBossFightStart(f.ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(uuid)})
	if err != nil {
		t.Fatal(err)
	}
	start := started[0].(*pb.SCSidePveBossFightStart)
	_, game = fieldBoss33State(t, f.id)
	terminal, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, game.Field.Fight.BossInput, nil)
	if err != nil || terminal.Outcome.Winner != 0 {
		t.Fatal("actual engine must kill remnant", err)
	}
	req := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(uuid), Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}
	ended, err := f.s.fieldBossFightEnd(f.ctx, f.id, req)
	if err != nil {
		t.Fatal(err)
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
	again, err := f.s.fieldBossFightEnd(f.ctx, f.id, req)
	if err != nil || !proto.Equal(ended[0], again[0]) {
		t.Fatal("cold retry changed frozen reward", err)
	}
	cold, game := fieldBoss33State(t, f.id)
	if cold.GetCoin() != after.GetCoin() || cold.GetDiamond() != after.GetDiamond() {
		t.Fatal("cold retry granted again")
	}
	f.s.table("Field", "FieldId", "field_001")["adminRevision"] = "1234567890abcde0"
	if err = f.s.refreshField(f.id, game); err != nil || game.Field.RespawnAt[uuid] != deadline {
		t.Fatal("monster publication erased boss deadline", err)
	}
	game.Field.RespawnAt[uuid] = time.Now().UnixMilli() - 1
	if err = f.s.refreshField(f.id, game); err != nil || game.Field.Bosses[0].GetUuid() == uuid || game.Field.Bosses[0].GetCurHp() != 1 || game.Field.BossHP[uuid] != nil {
		t.Fatal("elapsed original respawn did not replace only dead instance", err)
	}
}
