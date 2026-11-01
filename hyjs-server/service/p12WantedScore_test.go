package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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

func p12WantedScoreTestInput() (*model.FrozenBattleInput, *db.P12WantedFight, *db.P12WantedInstance, map[string]float64) {
	input := &model.FrozenBattleInput{
		Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(123)},
		Monster:  &pb.BattleMonster{},
		AppliedProperties: map[string][]string{
			"localWantedRule":         {constUtil.P12WantedLocalRule, "score-test", "1"},
			"wantedSettlementIndices": {"1000", "1000", constUtil.P12WantedScoreRuleV2, "1"},
		},
	}
	hp := map[string]float64{}
	for i := 1; i <= 4; i++ {
		input.Monster.Formation = append(input.Monster.Formation, &pb.BattleHero{Guid: proto.String(fmt.Sprintf("wanted-%d", i)), Prop: &pb.BaseProp{Hp: proto.Float64(100)}})
		hp[fmt.Sprintf("wanted_%d", i)] = 100
	}
	fight := &db.P12WantedFight{PlayerID: 123, InstanceID: "score-test", AttackType: 1}
	instance := &db.P12WantedInstance{ID: "score-test", WantedID: "1", Difficulty: 1, MaxHP: 400}
	return input, fight, instance, hp
}

func p12WantedScoreCopyHP(source map[string]float64) map[string]float64 {
	copyHP := make(map[string]float64, len(source))
	for key, value := range source {
		copyHP[key] = value
	}
	return copyHP
}

func TestP12WantedScoreV2DifficultyAndContributions(t *testing.T) {
	input, _, instance, full := p12WantedScoreTestInput()
	for _, tc := range []struct {
		difficulty int64
		pool       int64
	}{{1, 1000}, {2, 5000}, {3, 10000}} {
		pool, err := p12WantedScorePool(tc.difficulty)
		if err != nil || pool != tc.pool {
			t.Fatal("difficulty pool", tc, pool, err)
		}
		for _, pct := range []int64{0, 10, 50, 100} {
			after := p12WantedScoreCopyHP(full)
			for key := range after {
				after[key] -= float64(pct)
			}
			got, err := p12WantedShareScore(input, full, after, instance.MaxHP, pool)
			if err != nil || got != pool*pct/100 {
				t.Fatal("effective share", tc, pct, got, err)
			}
		}
	}
	for _, invalid := range []int64{-1, 0, 4, math.MaxInt64} {
		if _, err := p12WantedScorePool(invalid); err == nil {
			t.Fatal("invalid difficulty accepted", invalid)
		}
	}
}

func TestP12WantedScoreV2QuantizationAndSplitCannotFarm(t *testing.T) {
	for _, tc := range []struct {
		hp   float64
		want string
	}{
		{0, "0"}, {math.SmallestNonzeroFloat64, "1"}, {1.0 / 64, "15625"},
		{math.Nextafter(1.0/64, 0), "15625"}, {math.Nextafter(1.0/64, math.Inf(1)), "15626"},
	} {
		got, err := p12WantedHPUnits(tc.hp)
		if err != nil || got.String() != tc.want {
			t.Fatal("exact float quantization", tc, got, err)
		}
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := p12WantedHPUnits(bad); err == nil {
			t.Fatal("invalid HP accepted", bad)
		}
	}
	input, _, instance, full := p12WantedScoreTestInput()
	previous := p12WantedScoreCopyHP(full)
	var split int64
	for i := 1; i <= 1000; i++ {
		next := p12WantedScoreCopyHP(previous)
		next["wanted_1"] = 100 - float64(i)/10
		score, err := p12WantedShareScore(input, previous, next, instance.MaxHP, 1000)
		if err != nil {
			t.Fatal("split settlement", i, err)
		}
		split += score
		previous = next
	}
	combined, err := p12WantedShareScore(input, full, previous, instance.MaxHP, 1000)
	if err != nil || combined != 250 || split > combined {
		t.Fatal("splitting increased score", split, combined, err)
	}
	end := p12WantedScoreCopyHP(previous)
	for key := range end {
		end[key] = 0
	}
	remainder, err := p12WantedShareScore(input, previous, end, instance.MaxHP, 1000)
	if err != nil || remainder != 750 || split+remainder > 1000 {
		t.Fatal("shared total exceeds pool", split, remainder, err)
	}
}

func TestP12WantedScoreV2RejectsInvalidInputsAndHandlesHugeHP(t *testing.T) {
	for _, mode := range []string{"nil-input", "missing-body", "foreign-body", "negative", "increased", "exceeds-maximum", "nan", "infinite", "duplicate-guid", "invalid-guid", "missing-prop", "zero-max", "different-instance-max", "invalid-pool"} {
		t.Run(mode, func(t *testing.T) {
			input, _, instance, before := p12WantedScoreTestInput()
			after := p12WantedScoreCopyHP(before)
			pool := int64(1000)
			switch mode {
			case "nil-input":
				input = nil
			case "missing-body":
				delete(after, "wanted_1")
			case "foreign-body":
				delete(after, "wanted_1")
				after["wanted_other"] = 100
			case "negative":
				after["wanted_1"] = -1
			case "increased":
				after["wanted_1"] = 101
			case "exceeds-maximum":
				before["wanted_1"] = 101
			case "nan":
				before["wanted_1"] = math.NaN()
			case "infinite":
				after["wanted_1"] = math.Inf(1)
			case "duplicate-guid":
				input.Monster.Formation[1].Guid = proto.String("wanted-1")
			case "invalid-guid":
				input.Monster.Formation[0].Guid = proto.String("other")
			case "missing-prop":
				input.Monster.Formation[0].Prop = nil
			case "zero-max":
				input.Monster.Formation[0].Prop.Hp = proto.Float64(0)
			case "different-instance-max":
				instance.MaxHP++
			case "invalid-pool":
				pool = int64(math.MaxInt32) + 1
			}
			if _, err := p12WantedShareScore(input, before, after, instance.MaxHP, pool); err == nil {
				t.Fatal("invalid score input accepted", mode)
			}
		})
	}
	input, _, instance, before := p12WantedScoreTestInput()
	large := math.MaxFloat64 / 8
	for _, actor := range input.Monster.Formation {
		actor.Prop.Hp = proto.Float64(large)
	}
	instance.MaxHP = large * 4
	after := p12WantedScoreCopyHP(before)
	for key := range before {
		before[key], after[key] = large, large/2
	}
	got, err := p12WantedShareScore(input, before, after, instance.MaxHP, math.MaxInt32)
	if err != nil || got != int64(math.MaxInt32)/2 {
		t.Fatal("overflow-safe large input", got, err)
	}
}

func TestP12WantedScoreSnapshotsPreserveV1AndFrozenV2(t *testing.T) {
	input, fight, instance, before := p12WantedScoreTestInput()
	after := p12WantedScoreCopyHP(before)
	for key := range after {
		after[key] = 90
	}
	input.AppliedProperties["wantedSettlementIndices"] = []string{"1000", "50", constUtil.P12WantedRewardRule}
	performance, score, err := p12WantedSettlementValues(input, fight, instance, nil, nil, 52111.41)
	if err != nil || performance != 52 || score != 1042 {
		t.Fatal("legacy frozen divisor changed", performance, score, err)
	}
	// An existing V2 fight keeps its own frozen pool even if defaults later change.
	input.AppliedProperties["wantedSettlementIndices"] = []string{"1000", "700", constUtil.P12WantedScoreRuleV2, "1"}
	performance, score, err = p12WantedSettlementValues(input, fight, instance, before, after, 40)
	if err != nil || performance != 0 || score != 70 {
		t.Fatal("frozen score pool", performance, score, err)
	}
	for _, mode := range []string{"unknown-version", "wrong-difficulty", "wrong-instance", "wrong-player", "wrong-attack", "wrong-catalog", "invalid-divisor", "applied-nan", "performance-overflow"} {
		t.Run(mode, func(t *testing.T) {
			input, fight, instance, before := p12WantedScoreTestInput()
			after := p12WantedScoreCopyHP(before)
			applied := float64(40)
			switch mode {
			case "unknown-version":
				input.AppliedProperties["wantedSettlementIndices"][2] = "unknown"
			case "wrong-difficulty":
				input.AppliedProperties["wantedSettlementIndices"][3] = "2"
			case "wrong-instance":
				fight.InstanceID = "other"
			case "wrong-player":
				fight.PlayerID++
			case "wrong-attack":
				fight.AttackType++
			case "wrong-catalog":
				input.AppliedProperties[constUtil.P12WantedCatalogScope] = []string{constUtil.P12WantedCatalogSelectionRule, "2"}
			case "invalid-divisor":
				input.AppliedProperties["wantedSettlementIndices"][0] = "0"
			case "applied-nan":
				applied = math.NaN()
			case "performance-overflow":
				applied = float64(math.MaxInt32+1) * 1000
			}
			if _, _, err := p12WantedSettlementValues(input, fight, instance, before, after, applied); err == nil {
				t.Fatal("invalid snapshot accepted", mode)
			}
		})
	}
}

func TestP12WantedScoreDifficultyRatesOriginalLevel35To120(t *testing.T) {
	_, def := p12WantedCatalogDefinitionFixture(t)
	hp := map[string]float64{}
	for _, row := range def.Tables["WantedEnemy"] {
		value, err := strconv.ParseFloat(row["Hp"], 64)
		if err != nil || value <= 0 {
			t.Fatal("invalid original HP", row, err)
		}
		hp[row["WantedID"]] += value
	}
	byLevel := map[int]map[int]float64{}
	for _, row := range def.Tables["WantedLevel"] {
		level, e1 := strconv.Atoi(row["Level"])
		difficulty, e2 := strconv.Atoi(row["Difficulty"])
		if e1 != nil || e2 != nil {
			t.Fatal("invalid original level", row)
		}
		if level < 35 || level > 120 || difficulty < 1 || difficulty > 3 {
			continue
		}
		if byLevel[level] == nil {
			byLevel[level] = map[int]float64{}
		}
		if byLevel[level][difficulty] != 0 || hp[row["WantedID"]] <= 0 {
			t.Fatal("duplicate or missing original wanted", row)
		}
		byLevel[level][difficulty] = hp[row["WantedID"]]
	}
	for level := 35; level <= 120; level++ {
		h := byLevel[level]
		if len(h) != 3 || 5000*h[1] <= 1000*h[2] || 10000*h[2] <= 5000*h[3] {
			t.Fatal("same-level same-damage score rate must increase with difficulty", level, h)
		}
	}
}

// The legacy fixture rewrites only this isolated paid fight's frozen source and
// recomputes its certified terminal, reproducing an old unresolved v1 fight.
func TestP12WantedScoreVersionedPaidFightAndColdReceipt(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			f := p12WantedPersistenceFixture(t)
			now := time.Now()
			if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
				t.Fatal(err)
			}
			instance := p12WantedOwnershipInstance(t, f, "wanted-score-"+version, now)
			beforeHP := map[string]float64{}
			if err := json.Unmarshal([]byte(instance.MonsterHPJSON), &beforeHP); err != nil {
				t.Fatal(err)
			}
			startRequest := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
			startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
			started, err := f.s.p12WantedStart(startCtx, f.id, startRequest, 919000, now)
			if err != nil {
				t.Fatal(err)
			}
			start := started[0].(*pb.SCRebelFightStart)
			paid, _ := f.profile(t)
			var fight db.P12WantedFight
			if err := db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
				t.Fatal(err)
			}
			var input model.FrozenBattleInput
			var terminal model.CombatFinalHPOutcome
			if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil {
				t.Fatal("frozen fight or terminal missing")
			}
			indices := input.AppliedProperties["wantedSettlementIndices"]
			if len(indices) != 4 || indices[1] != "1000" || indices[2] != constUtil.P12WantedScoreRuleV2 || indices[3] != "1" {
				t.Fatal("new fight did not freeze difficulty and score pool", indices)
			}
			if version == "v1" {
				input.AppliedProperties["wantedSettlementIndices"] = []string{indices[0], "50", constUtil.P12WantedRewardRule}
				legacyTerminal, err := f.s.ComputeP12WantedProtocolFinalHP(f.ctx, &input)
				if err != nil {
					t.Fatal(err)
				}
				terminal = *legacyTerminal
				rawInput, err := json.Marshal(&input)
				if err != nil {
					t.Fatal(err)
				}
				rawTerminal, err := json.Marshal(&terminal)
				if err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().Model(&fight).Updates(map[string]interface{}{
					"input_json": string(rawInput), "input_hash": p6Digest(rawInput),
					"terminal_json": string(rawTerminal), "terminal_hash": p6Digest(rawTerminal),
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if terminal.Outcome == nil || terminal.OriginalOperates == nil {
				t.Fatal("certified terminal missing")
			}
			expectedHP, applied, err := p12WantedApplyTerminalHP(&input, &terminal, beforeHP)
			if err != nil {
				t.Fatal(err)
			}
			expectedScore := int64(math.Floor(applied / 50))
			if version == "v2" {
				expectedScore, err = p12WantedShareScore(&input, beforeHP, expectedHP, instance.MaxHP, 1000)
				if err != nil {
					t.Fatal(err)
				}
			}
			result := int32(0)
			if terminal.Outcome.Winner == 0 {
				result = 1
			}
			end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
			endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
			if expectedScore > 0 {
				if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
					p.Score = proto.Int32(math.MaxInt32)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.p12WantedEnd(endCtx, f.id, end, now); err == nil || !strings.Contains(err.Error(), "奖励溢出") {
					t.Fatal("score overflow did not reject settlement", err)
				}
				var rolledInstance db.P12WantedInstance
				var rolledFight db.P12WantedFight
				if err := db.GetIns().First(&rolledInstance, "id=?", instance.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().First(&rolledFight, "id=?", fight.ID).Error; err != nil {
					t.Fatal(err)
				}
				rolledPlayer, _ := f.profile(t)
				if rolledFight.Committed || rolledInstance.MonsterHPJSON != instance.MonsterHPJSON || rolledInstance.CurrentHP != instance.CurrentHP || rolledPlayer.GetScore() != math.MaxInt32 {
					t.Fatal("overflow must roll back HP, fight and wallet")
				}
				if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
					p.Score = proto.Int32(paid.GetScore())
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Log("zero-damage fixture cannot exercise score overflow")
			}
			finished, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
			if err != nil {
				t.Fatal(err)
			}
			receipt := finished[0].(*pb.SCRebelFightEnd)
			after, _ := f.profile(t)
			if receipt.GetDamage() != applied || int64(receipt.GetScoreAdd()) != expectedScore || int64(after.GetScore())-int64(paid.GetScore()) != expectedScore {
				t.Fatal("versioned paid settlement differs", version, applied, expectedScore, receipt.GetDamage(), receipt.GetScoreAdd())
			}
			var savedInstance db.P12WantedInstance
			var savedFight db.P12WantedFight
			if err := db.GetIns().First(&savedInstance, "id=?", instance.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().First(&savedFight, "id=?", fight.ID).Error; err != nil {
				t.Fatal(err)
			}
			persistedHP := map[string]float64{}
			if json.Unmarshal([]byte(savedInstance.MonsterHPJSON), &persistedHP) != nil || savedFight.Score != expectedScore || !savedFight.Committed {
				t.Fatal("settlement not committed atomically")
			}
			for key, expected := range expectedHP {
				if persistedHP[key] != expected {
					t.Fatal("authoritative HP differs", key, persistedHP[key], expected)
				}
			}
			f.restart(t)
			replayed, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
			if err != nil || !proto.Equal(receipt, replayed[0]) {
				t.Fatal("cold retry changed receipt", err)
			}
			conflict := proto.Clone(end).(*pb.CSRebelFightEnd)
			conflict.Result = proto.Int32(1 - result)
			conflictCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, conflict)
			if _, err := f.s.p12WantedEnd(conflictCtx, f.id, conflict, now); err == nil {
				t.Fatal("conflicting replay accepted")
			}
			last, _ := f.profile(t)
			if !proto.Equal(after, last) {
				t.Fatal("retry changed balance or profile")
			}
		})
	}
}
