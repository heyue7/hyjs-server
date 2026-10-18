package service

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WantedCatalogDefinitionFixture(t *testing.T) (*GameService, p12WantedDefinition) {
	t.Helper()
	s := p6OriginalTables(t)
	if err := s.LoadP12WantedTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	def := p12WantedDefinition{Tables: map[string][]map[string]string{}, SelectionRule: constUtil.P12WantedCatalogSelectionRule}
	for _, name := range []string{"WantedLevel", "WantedEnemy", "WantedTimeLimit"} {
		def.Tables[name] = s.Tables[name]
	}
	ids, err := p12WantedCatalogIDs(def)
	if err != nil {
		t.Fatal(err)
	}
	def.SupportedWantedIDs = ids
	return s, def
}

func TestP12WantedCatalogOriginalRowsSelectionAndMechanismScope(t *testing.T) {
	s, def := p12WantedCatalogDefinitionFixture(t)
	if len(def.SupportedWantedIDs) != 347 || len(def.Tables["WantedEnemy"]) != 1388 || !p12WantedDefinitionSelectionValid(def) {
		t.Fatal("original complete catalog")
	}
	counts := map[string]int{}
	for _, row := range def.Tables["WantedEnemy"] {
		if row["GroupIndex"] != "1" {
			t.Fatal("unknown original wave", row)
		}
		counts[row["WantedID"]]++
	}
	for _, id := range def.SupportedWantedIDs {
		if counts[id] != 4 {
			t.Fatal("original complete four members", id, counts[id])
		}
	}
	for _, tc := range []struct {
		level         int32
		draw          int64
		difficulty    int
		expectedLevel int
	}{{35, 0, 1, 35}, {35, 49, 1, 35}, {35, 50, 2, 35}, {35, 71, 2, 35}, {35, 72, 3, 35}, {35, 93, 3, 35}, {121, 0, 1, 120}, {1, 49, 1, 1}, {2, 50, 2, 2}} {
		id, err := p12WantedChooseCatalog(def, tc.level, func(total int64) (int64, error) {
			if tc.draw >= total {
				t.Fatal("weights not normalized among existing rows", tc, total)
			}
			return tc.draw, nil
		})
		if err != nil {
			t.Fatal(tc, err)
		}
		row, _, err := p12WantedDefinitionRow(def, id)
		if err != nil || row["Level"] != strconv.Itoa(tc.expectedLevel) || row["Difficulty"] != strconv.Itoa(tc.difficulty) {
			t.Fatal("user-approved choice", tc, id, row, err)
		}
	}
	lower := def
	lower.SupportedWantedIDs = []string{"1"}
	if _, err := p12WantedChooseCatalog(lower, 1, func(int64) (int64, error) { return 0, nil }); err != nil {
		t.Fatal(err)
	}
	lower.SupportedWantedIDs = []string{}
	if _, err := p12WantedChooseCatalog(lower, 35, func(int64) (int64, error) { t.Fatal("no lower task must not draw"); return 0, nil }); err == nil {
		t.Fatal("missing lower invented a task")
	}
	_, original := p12WantedOriginalFixture(t)
	oldProof, err := P12WantedProofForInput(original)
	if err != nil || oldProof == constUtil.P12WantedCatalogMechanismProof {
		t.Fatal("legacy proof changed", err)
	}
	for _, id := range []string{"1", "258"} {
		waves, _, _, err := s.p12WantedMonster(def, id, nil)
		if err != nil {
			t.Fatal(id, err)
		}
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: proto.Clone(original.Attacker).(*pb.BattlePlayer), Monster: waves[0], Seed: 42, MaxRound: 20, BattleType: 8, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{constUtil.P12WantedCatalogScope: {constUtil.P12WantedCatalogSelectionRule, id}}})
		if err != nil {
			t.Fatal(id, err)
		}
		proof, err := P12WantedProofForInput(input)
		if err != nil || proof != constUtil.P12WantedCatalogMechanismProof {
			t.Fatal("catalog exact original NPC qualification", id, proof, err)
		}
		input.Monster.Formation[0].Inst.CurHp = proto.Float64(input.Monster.Formation[0].Prop.GetHp() * .5)
		if err := p12MechanismScope(input, 8); err != nil {
			t.Fatal("original partial HP", id, err)
		}
		input.Monster.Formation[0].Prop.Att = proto.Float64(input.Monster.Formation[0].Prop.GetAtt() + 1)
		if err := p12MechanismScope(input, 8); err == nil {
			t.Fatal("forged original NPC panel admitted", id)
		}
	}
	t.Log("347 tasks / 1388 original NPC rows; first ID1 and later ID258 exact typed8 static admission, partialHP/tamper; same35/nearest120, difficulty boundaries50/72/94, missing low-level difficulties; legacy proof retained; zero static battles")
}

func p12WantedCatalogMigrateLegacyFixture(t *testing.T, f *p1Fixture) {
	t.Helper()
	var policy db.P12WantedPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	var def p12WantedDefinition
	if json.Unmarshal([]byte(policy.DefinitionJSON), &def) != nil {
		t.Fatal("definition")
	}
	def.SelectionRule = ""
	def.SupportedWantedIDs = []string{"1"}
	raw, _ := json.Marshal(def)
	// Only this isolated test database receives the old default and historical immutable instance.
	prior := db.P12WantedInstance{ID: "old-catalog-definition-retained", DefinitionJSON: string(raw), DefinitionHash: p6Digest(raw), WantedID: "1", State: constUtil.P12WantedKilled}
	if err := db.GetIns().Create(&prior).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=1").Updates(map[string]any{"definition_json": string(raw), "definition_hash": p6Digest(raw), "enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP12Wanted(f.ctx); err != nil {
		t.Fatal("known default catalog migration", err)
	}
	var next db.P12WantedPolicy
	var retained db.P12WantedInstance
	if err := db.GetIns().First(&next, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&retained, "id=?", prior.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !next.Enabled || next.Source != policy.Source || next.DefinitionHash == p6Digest(raw) || json.Unmarshal([]byte(next.DefinitionJSON), &def) != nil || len(def.SupportedWantedIDs) != 347 || def.SelectionRule != constUtil.P12WantedCatalogSelectionRule || retained.DefinitionJSON != prior.DefinitionJSON || retained.DefinitionHash != prior.DefinitionHash {
		t.Fatal("migration changed switch/source/history or missed catalog")
	}
	view := &dbWantedCatalogPolicyView{Source: policy.Source, Hash: p6Digest(raw), JSON: string(raw)}
	def.Rule += "custom"
	if ok, err := p12WantedDefaultCatalogUpgrade(view, def); err != nil || ok {
		t.Fatal("custom policy overwritten", err)
	}
}

func TestP12WantedCatalogMySQLFirstUnlockedDiscoveryBattleAndCold(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("isolated MySQL lease required")
	}
	f := p12WantedPersistenceFixture(t)
	p12WantedCatalogMigrateLegacyFixture(t, f)
	if err := f.s.InitializeCopyDrops(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP4CopyBenefits(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP9CopyIntimacy(f.ctx); err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 0, -1)
	now := time.Date(local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, local.Location())
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(35)
		p.Stamina = proto.Int32(1000)
		f.s.refreshCopyProgress(g)
		for _, info := range g.Copy.PassedCopyInfo {
			if info.GetChapterId() == "1" && info.GetMissionId() == "1" {
				info.Star = proto.Int32(5)
			}
		}
		for _, hero := range g.Warriors {
			hero.Level = proto.Int32(100)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var (
		found *pb.SCCopySweep
		stamp int64
	)
	request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	for attempt := int64(1); attempt <= 64; attempt++ {
		stamp = 914000 + attempt
		out, err := f.s.copySweepAt(ctx, f.id, request, stamp, now)
		if err != nil {
			t.Fatal("原扫荡自然发现入口", attempt, err)
		}
		found = out[0].(*pb.SCCopySweep)
		if found.GetWantedId() != "" {
			break
		}
	}
	if found.GetWantedId() == "" {
		t.Fatal("64次原25%抽取均未发现；保留随机结果，不改概率")
	}
	var discovered db.P12WantedDiscovery
	if err := db.GetIns().Where("player_id=? AND instance_id<>?", f.id, "").First(&discovered).Error; err != nil {
		t.Fatal(err)
	}
	if discovered.Odds != 25 || discovered.Draw >= discovered.Odds {
		t.Fatal("发现没有沿原时段/任务", discovered)
	}
	var selected db.P12WantedInstance
	if err := db.GetIns().First(&selected, "id=?", discovered.InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	if selected.Level != 35 || selected.Difficulty < 1 || selected.Difficulty > 3 || selected.WantedID != found.GetWantedId() {
		t.Fatal("first unlocked level selection", selected)
	}
	before, _ := f.profile(t)
	retry, err := f.s.copySweepAt(ctx, f.id, request, stamp, now)
	after, _ := f.profile(t)
	if err != nil || !proto.Equal(found, retry[0]) || !proto.Equal(before, after) {
		t.Fatal("发现重试重复发钻或重新抽取", err)
	}
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(discovered.InstanceID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	out, err := f.s.p12WantedStart(startCtx, f.id, startRequest, 915001, now)
	if err != nil {
		t.Fatal("自然发现实例原Start", err)
	}
	start := out[0].(*pb.SCRebelFightStart)
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.Outcome.CapabilityHash != constUtil.P12WantedCatalogMechanismProof {
		t.Fatal("缺真实预计算终末")
	}
	won := int32(0)
	if terminal.Outcome.Winner == 0 {
		won = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(won), OptRecord: terminal.OriginalOperates}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	settled, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil {
		t.Fatal("自然发现实例原End", err)
	}
	var saved db.P12WantedInstance
	if err = db.GetIns().First(&saved, "id=?", discovered.InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	result := settled[0].(*pb.SCRebelFightEnd)
	if difference := math.Abs((saved.MaxHP - saved.CurrentHP) - result.GetDamage()); difference > math.Max(1, saved.MaxHP)*1e-12 {
		t.Fatalf("共享扣血与真实伤害不一致: maxHP=%g currentHP=%g damage=%g difference=%g", saved.MaxHP, saved.CurrentHP, result.GetDamage(), difference)
	}
	before, game := f.profile(t)
	gameBytes, _ := json.Marshal(game)
	f.restart(t)
	again, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	after, current := f.profile(t)
	currentBytes, _ := json.Marshal(current)
	if err != nil || !proto.Equal(again[0], result) || !proto.Equal(before, after) || !bytes.Equal(gameBytes, currentBytes) {
		t.Fatal("自然链冷End重复扣发", err)
	}
	t.Logf("catalog347 level35 task=%s difficulty=%d actual sweep draws=%d damage=%v score=%d; original25%% unchanged, first-copy clear/mature roster/stamina finite prerequisites", selected.WantedID, selected.Difficulty, stamp-914000, result.GetDamage(), result.GetScoreAdd())
}
