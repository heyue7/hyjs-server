package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WorldBossPartyScopeInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	t.Helper()
	raw, err := readTestAnalysis75("../../analysis/p6/worldboss18-actual-party001003-pending-20261016/000.pb")
	if err != nil || p6Digest(raw) != "b5da71b7c23d5b39f9031794f8af5846bc97226527dd9319eac942c66dab258c" {
		t.Fatal("actual historical party typed18 source", err)
	}
	var wire pb.SCFightPlayback
	if err = proto.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: proto.Float64(wire.GetAttackerFactor()), DefenderDamageFactor: proto.Float64(wire.GetDefenderFactor())})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP12WorldBossPartyBoostScopeUsesPurchasedSubset(t *testing.T) {
	s := p6OriginalTables(t)
	base := p12WorldBossPartyScopeInput(t, s)
	definitions, _ := p12WorldBossBoostOriginalTable(t, "WorldBossPropadd")
	sort.Slice(definitions, func(i, j int) bool { return definitions[i]["id"] < definitions[j]["id"] })
	if len(definitions) != 4 {
		t.Fatal("four original boost definitions required")
	}
	for mask := 1; mask < 16; mask++ {
		input, err := cloneP6CalculationInput(base)
		if err != nil {
			t.Fatal(err)
		}
		rows := []db.P12WorldBossBoost{}
		for index, definition := range definitions {
			if mask&(1<<index) != 0 {
				kind, err := strconv.ParseInt(definition["type"], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, db.P12WorldBossBoost{PropertyID: definition["id"], Kind: kind, Property: definition["prop"]})
			}
		}
		ids, err := s.applyP12WorldBossBoostRows(input.Attacker, rows)
		if err != nil {
			t.Fatal(err)
		}
		input.AppliedProperties = map[string][]string{"worldBossPurchasedBoosts": ids}
		if err = validateP12WorldBossPartyScope(input); err != nil {
			t.Fatal("exact original helper output and declared subset", mask, err)
		}
		input.AppliedProperties["worldBossPurchasedBoosts"] = append(ids, ids[0])
		if err = validateP12WorldBossPartyScope(input); err == nil {
			t.Fatal("duplicate purchased property admitted", mask)
		}
		input.AppliedProperties["worldBossPurchasedBoosts"] = nil
		if err = validateP12WorldBossPartyScope(input); err == nil {
			t.Fatal("boosted actors admitted without purchase subset", mask)
		}
	}
}

func TestP12WorldBossPartyScopeRejectsUnprovenMechanics(t *testing.T) {
	s := p6OriginalTables(t)
	input := p12WorldBossPartyScopeInput(t, s)
	if err := validateP12WorldBossPartyScope(input); err != nil {
		t.Fatal("unchanged actual normal party must fit structural boundary", err)
	}
	for name, change := range map[string]func(*model.FrozenBattleInput){
		"otherHero":    func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Id = proto.String("hero_047") },
		"talent":       func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].TalentList = []string{"tal_3163"} },
		"fraction":     func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Prop.Att = proto.Float64(822.1) },
		"residualHP":   func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Inst.CurHp = proto.Float64(100) },
		"residualAP":   func(v *model.FrozenBattleInput) { v.Attacker.Inst.Anger = proto.Float64(20) },
		"manualDriver": func(v *model.FrozenBattleInput) { v.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart },
		"bossBlood":    func(v *model.FrozenBattleInput) { v.Monster.Formation[0].BloodNumber = proto.Int32(1) },
		"bossSkill":    func(v *model.FrozenBattleInput) { v.Monster.Formation[0].SkillOrder = proto.String("worldboss1041_1") },
		"bossPanel":    func(v *model.FrozenBattleInput) { v.Monster.Formation[0].Prop.Hp = proto.Float64(1) },
		"tableBytes":   func(v *model.FrozenBattleInput) { v.Tables["Skill"][0]["Id"] = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			view, err := cloneP6CalculationInput(input)
			if err != nil {
				t.Fatal(err)
			}
			change(view)
			if err = validateP12WorldBossPartyScope(view); err == nil {
				t.Fatal("unproven source admitted", name)
			}
		})
	}
	if constUtil.P12WorldBossPartyNativeProof == "" {
		if _, err := s.computeP12WorldBossCertifiedFinalHP(t.Context(), input); err == nil {
			t.Fatal("empty native proof must not authorize structural match")
		}
	} else {
		out, err := s.computeP12WorldBossCertifiedFinalHP(t.Context(), input)
		if err != nil || validateP12WorldBossCertifiedReceipt(input, out) != nil {
			t.Fatal("same certified source must bind the complete original terminal and frozen proof", err)
		}
		input.AppliedProperties[constUtil.P12WorldBossPartyNativeScope] = []string{"foreign-proof"}
		if validateP12WorldBossCertifiedReceipt(input, out) == nil {
			t.Fatal("cold receipt accepted a substituted capability proof")
		}
	}
}

func TestP12WorldBossPartyTerminalBindsEveryOriginalActor(t *testing.T) {
	s := p6OriginalTables(t)
	input := p12WorldBossPartyScopeInput(t, s)
	out, err := s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateP12WorldBossCertifiedTerminal(input, out); err != nil {
		t.Fatal("computed actual six-actor terminal", err)
	}
	for name, change := range map[string]func(*model.CombatFinalHPOutcome){
		"source":    func(v *model.CombatFinalHPOutcome) { v.FinalActors[0].InitialActorSHA256 = "forged" },
		"blood":     func(v *model.CombatFinalHPOutcome) { v.FinalActors[2].BloodNumber = 1 },
		"maxHP":     func(v *model.CombatFinalHPOutcome) { v.FinalActors[2].MaxHP *= 2 },
		"duplicate": func(v *model.CombatFinalHPOutcome) { v.FinalActors[1] = v.FinalActors[0] },
		"recordHP":  func(v *model.CombatFinalHPOutcome) { v.OriginalOperates.DefenderTotalLeftHp = proto.Float64(0) },
		"closure":   func(v *model.CombatFinalHPOutcome) { v.CalculationSourceSHA256 = "forged" },
	} {
		t.Run(name, func(t *testing.T) {
			raw, marshalErr := json.Marshal(out)
			var view model.CombatFinalHPOutcome
			if marshalErr != nil || json.Unmarshal(raw, &view) != nil {
				t.Fatal("terminal clone", marshalErr)
			}
			change(&view)
			if err := validateP12WorldBossCertifiedTerminal(input, &view); err == nil {
				t.Fatal("altered six-actor terminal admitted", name)
			}
		})
	}
}

func TestP12WorldBossCertifiedReceiptRejectsMismatchedTypedStart(t *testing.T) {
	s := p6OriginalTables(t)
	input := p12WorldBossPartyScopeInput(t, s)
	out, err := s.computeP12WorldBossCertifiedFinalHP(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	damage, err := p12WorldBossBattleDamage(input, out)
	if err != nil {
		t.Fatal(err)
	}
	start := &pb.SCWorldBossFight{FightId: proto.String("bound-original-start"), RandSeed: proto.Int64(input.Seed), BattlePlayer: input.Attacker, BattleMonsterList: []*pb.BattleMonster{input.Monster}, Maxround: proto.Int32(input.MaxRound), Code: proto.Int32(out.Outcome.Winner + 1), Damange: proto.Int32(int32(damage))}
	inputRaw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	terminalRaw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	startRaw, err := proto.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	row := db.P12WorldBossBattleReceipt{FightID: start.GetFightId(), PlayerID: input.Attacker.GetPlayerId(), PolicyName: constUtil.P12WorldBossBattlePolicy, InputJSON: string(inputRaw), InputHash: p6Digest(inputRaw), TerminalJSON: string(terminalRaw), TerminalHash: p6Digest(terminalRaw), StartProto: startRaw, StartHash: p6Digest(startRaw), PredictedDamage: damage}
	if _, err = p12WorldBossBattleStartProto(&row); err != nil {
		t.Fatal("closed same-source original Start", err)
	}
	for name, change := range map[string]func(*pb.SCWorldBossFight){
		"seed":  func(v *pb.SCWorldBossFight) { v.RandSeed = proto.Int64(input.Seed + 1) },
		"actor": func(v *pb.SCWorldBossFight) { v.BattlePlayer.Formation[0].Prop.Hp = proto.Float64(1) },
		"code":  func(v *pb.SCWorldBossFight) { v.Code = proto.Int32(1 - out.Outcome.Winner + 1) },
	} {
		t.Run(name, func(t *testing.T) {
			altered := proto.Clone(start).(*pb.SCWorldBossFight)
			change(altered)
			raw, err := proto.Marshal(altered)
			if err != nil {
				t.Fatal(err)
			}
			view := row
			view.StartProto, view.StartHash = raw, p6Digest(raw)
			if _, err = p12WorldBossBattleStartProto(&view); err == nil {
				t.Fatal("rehashed typed Start diverged from certified input", name)
			}
		})
	}
	row.PredictedDamage++
	if _, err = p12WorldBossBattleStartProto(&row); err == nil {
		t.Fatal("stored damage diverged from certified terminal")
	}
}
