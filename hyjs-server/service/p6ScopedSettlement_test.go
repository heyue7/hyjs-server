package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func p6LinearPhysicalScopeInput(t *testing.T) *model.FrozenBattleInput {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6/native-linear-physical-candidate-inputs-v110b", "000.pb"))
	if err != nil {
		t.Fatal(err)
	}
	p := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, p); err != nil {
		t.Fatal(err)
	}
	input, err := p6OriginalTables(t).FreezeBattleInput(model.FrozenBattleInput{Attacker: p.Attacker, Defender: p.Defender1, BattleType: 2, MaxRound: 40, Seed: p.GetRandSeed(), AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP6LinearPhysicalScopeStructuralAndSeedIndependent(t *testing.T) {
	input := p6LinearPhysicalScopeInput(t)
	if err := validateP6LinearPhysicalScope(input); err != nil {
		t.Fatal(err)
	}
	// A genuinely new combination passes the same closed mechanism conditions;
	// no exact source PB, GUID, seed or panel-value membership controls eligibility.
	input.Seed = -9223372036854775807
	input.Attacker.Formation[0].Guid = proto.String("new-current-player-instance")
	input.Attacker.Formation[0].Prop.Att = proto.Float64(100001)
	input.Attacker.Formation[0].Prop.Hp = proto.Float64(12345)
	input.Attacker.Formation[0].Inst.CurHp = proto.Float64(12345)
	if err := validateP6LinearPhysicalScope(input); err != nil {
		t.Fatal("structural family became a golden-input whitelist", err)
	}
	out, err := p6OriginalTables(t).ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil || out.Outcome.Authoritative {
		t.Fatal("structural candidate promoted authority before original composition certificate", err)
	}
}

func TestP6LinearPhysicalScopeRejectsReachableUnknowns(t *testing.T) {
	base := p6LinearPhysicalScopeInput(t)
	mutations := map[string]func(*model.FrozenBattleInput){
		"mode":            func(x *model.FrozenBattleInput) { x.BattleType = 14 },
		"roundCriterion":  func(x *model.FrozenBattleInput) { x.MaxRound = 41 },
		"nonstandardView": func(x *model.FrozenBattleInput) { x.IsAttToDef = true },
		"winnerCondition": func(x *model.FrozenBattleInput) { x.WinConditions = []*pb.BattleWinCondition{{}} },
		"compat":          func(x *model.FrozenBattleInput) { x.PVECompatibilityRule = "local-pve-no-target-finish-v1" },
		"zeroFactor":      func(x *model.FrozenBattleInput) { x.DefenderDamageFactor = proto.Float64(0) },
		"anger":           func(x *model.FrozenBattleInput) { x.Attacker.Inst.Anger = proto.Float64(1) },
		"firstOrder":      func(x *model.FrozenBattleInput) { x.Attacker.Inst.FirstOrder = proto.Int32(0) },
		"orientation":     func(x *model.FrozenBattleInput) { x.DefenderFirst = true },
		"twoActors": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation = append(x.Attacker.Formation, proto.Clone(x.Attacker.Formation[0]).(*pb.BattleHero))
		},
		"otherHero": func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Id = proto.String("hero_001") },
		"skillReorder": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation[0].SkillOrder = proto.String("069_2,069_1,069_3")
		},
		"talent":          func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].TalentList = []string{"talent_1"} },
		"awake":           func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].AwakeStage = proto.Int32(1) },
		"pose":            func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Pose = proto.Int32(2) },
		"percentHP":       func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Prop.Perhp = proto.Float64(1) },
		"fractionalPanel": func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Prop.Pdef = proto.Float64(255.5) },
		"criticalBranch":  func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Prop.CriFactor = proto.Float64(51) },
		"injured":         func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Inst.CurHp = proto.Float64(5) },
		"zeroHP": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation[0].Prop.Hp = proto.Float64(0)
			x.Attacker.Formation[0].Inst.CurHp = proto.Float64(0)
		},
		"panelLimit": func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Prop.Att = proto.Float64(1000001) },
		"unknownProto": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation[0].ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
		},
		"engine":    func(x *model.FrozenBattleInput) { x.EngineVersion = "old-engine" },
		"tableHash": func(x *model.FrozenBattleInput) { x.ConfigurationHash = "unknown" },
	}
	for name, change := range mutations {
		t.Run(name, func(t *testing.T) {
			x := *base
			x.Attacker = proto.Clone(base.Attacker).(*pb.BattlePlayer)
			x.Defender = proto.Clone(base.Defender).(*pb.BattlePlayer)
			change(&x)
			if err := validateP6LinearPhysicalScope(&x); err == nil {
				t.Fatal("unsafe reachable source admitted")
			}
		})
	}
	x, err := cloneP6CalculationInput(base)
	if err != nil {
		t.Fatal(err)
	}
	x.Tables["Skill"][0]["Duration"] = "999"
	if err = validateP6LinearPhysicalScope(x); err == nil {
		t.Fatal("mutated rows accepted under copied hash label")
	}
}

func TestP6LinearPhysicalTerminalAndProofGate(t *testing.T) {
	s := p6OriginalTables(t)
	for index := 0; index < 8; index++ {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/native-linear-physical-candidate-inputs-v110b/%03d.pb", index))
		if err != nil {
			t.Fatal(err)
		}
		p := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(raw, p); err != nil {
			t.Fatal(err)
		}
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: p.Attacker, Defender: p.Defender1, BattleType: 2, MaxRound: 40, Seed: p.GetRandSeed(), DefenderFirst: p.Attacker.Inst.GetFirstOrder() == 0, AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor})
		if err != nil {
			t.Fatal(err)
		}
		if err = validateP6LinearPhysicalScope(input); err != nil {
			t.Fatal(index, err)
		}
		result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateP6LinearPhysicalTerminal(input, result); err != nil {
			t.Fatal(index, "closed-family terminal invariant", err)
		}
		if constUtil.P6LinearPhysicalNativeProof == "" {
			admitted, err := s.ComputeP6ScopedSettlement(t.Context(), input)
			if err == nil || admitted != nil {
				t.Fatal("missing native certificate admitted settlement")
			}
		}
		result.Outcome.Winner = 1 - result.Outcome.Winner
		if err = validateP6LinearPhysicalTerminal(input, result); err == nil {
			t.Fatal("contradictory terminal winner admitted")
		}
	}
}

func TestP6LinearPhysicalScopedSettlementCertificateAndOwnership(t *testing.T) {
	input := p6LinearPhysicalScopeInput(t)
	input.Seed = 7654321
	input.Attacker.Formation[0].Guid = proto.String("new-real-instance-outside-native-cases")
	input.Attacker.Formation[0].Prop.Att = proto.Float64(100001)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p6OriginalTables(t).ComputeP6ScopedSettlement(t.Context(), input)
	if err != nil || result == nil || !result.Outcome.Authoritative || result.Outcome.CapabilityHash != constUtil.P6LinearPhysicalNativeProof {
		t.Fatal("certified structural family refused", err)
	}
	if err = validateP6LinearPhysicalTerminal(input, result); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(input)
	if err != nil || string(before) != string(after) {
		t.Fatal("scoped compute mutated frozen source", err)
	}
	generic, err := p6OriginalTables(t).ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil || generic.Outcome.Authoritative {
		t.Fatal("generic audit inherited scoped authority", err)
	}
	for _, capability := range P6Capabilities() {
		if capability.NativeCompared {
			t.Fatal("global capability was promoted", capability)
		}
	}
	input.Attacker.Formation[0].TalentList = []string{"unknown-effect"}
	rejected, err := p6OriginalTables(t).ComputeP6ScopedSettlement(t.Context(), input)
	if err == nil || rejected != nil {
		t.Fatal("out-of-family input returned a settlement")
	}
}

func TestP6LinearPhysicalCertificatePinsActualProofAndPolicy(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/p6/" + constUtil.P6LinearPhysicalCertificateFile)
	if err != nil || p6Digest(raw) != constUtil.P6LinearPhysicalNativeProof {
		t.Fatal("compiled scope proof changed", err)
	}
	var proof struct {
		ScopePolicy struct {
			Source      string `json:"source"`
			SHA         string `json:"sourceSHA256"`
			Constants   string `json:"constantSource"`
			ConstantSHA string `json:"constantSourceNormalizedSHA256"`
		} `json:"scopePolicy"`
		RuntimeSources []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"runtimeSources"`
		Evidence []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"originalNativeEvidence"`
	}
	if err = json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Base(proof.ScopePolicy.Source))
	if err != nil || p6Digest(source) != proof.ScopePolicy.SHA {
		t.Fatal("compiled structural mechanism policy changed", err)
	}
	constants, err := os.ReadFile("../constUtil/p6ScopedSettlement.go")
	if err != nil || p6Digest([]byte(strings.ReplaceAll(string(constants), constUtil.P6LinearPhysicalNativeProof, ""))) != proof.ScopePolicy.ConstantSHA {
		t.Fatal("scope constants changed beyond pinned proof", err)
	}
	for _, source := range proof.RuntimeSources {
		path := filepath.Join("..", strings.TrimPrefix(source.Path, "hyjs-server/"))
		raw, err := os.ReadFile(path)
		if err != nil || p6Digest(raw) != source.SHA {
			t.Fatal("certified runtime source changed", source.Path, err)
		}
	}
	for _, evidence := range proof.Evidence {
		body, readErr := os.ReadFile("../../" + evidence.Path)
		if readErr != nil || p6Digest(body) != evidence.SHA {
			t.Fatal("native proof asset changed", evidence.Path, readErr)
		}
	}
}

func TestP6LinearPhysicalFriend11ScopedOriginalSource(t *testing.T) {
	s := p6OriginalTables(t)
	for index := 0; index < 2; index++ {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/native-linear-physical-friend11-inputs-v112/%03d.pb", index))
		if err != nil {
			t.Fatal(err)
		}
		p := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(raw, p); err != nil {
			t.Fatal(err)
		}
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: p.Attacker, Defender: p.Defender1, BattleType: 11, MaxRound: 40, Seed: p.GetRandSeed(), AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor})
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.ComputeP6ScopedSettlement(t.Context(), input)
		if err != nil || !result.Outcome.Authoritative || !p6KnownScopedSettlementProof(input, result.Outcome.CapabilityHash) {
			t.Fatal("original Friend11 scope refused", err)
		}
		if p6KnownScopedSettlementProof(input, constUtil.P6LinearPhysicalOriginalProof) {
			t.Fatal("PVP2-only old certificate promoted Friend11")
		}
	}
}

func TestP6HistoricalScopedProofRetainsAcceptedOldEngine(t *testing.T) {
	input := p6LinearPhysicalScopeInput(t)
	input.EngineVersion = constUtil.P6LinearPhysicalOriginalEngine
	if !p6KnownScopedSettlementProof(input, constUtil.P6LinearPhysicalOriginalProof) || !p6KnownScopedSettlementProof(input, constUtil.P6LinearPhysicalPreviousProof) {
		t.Fatal("old accepted scopes lost their immutable proof")
	}
	input.BattleType = 11
	if p6KnownScopedSettlementProof(input, constUtil.P6LinearPhysicalOriginalProof) || !p6KnownScopedSettlementProof(input, constUtil.P6LinearPhysicalPreviousProof) {
		t.Fatal("historical mode boundary changed")
	}
	if p6KnownScopedSettlementProof(input, constUtil.P6LinearPhysicalNativeProof) {
		t.Fatal("new engine proof promoted old frozen data")
	}
}
