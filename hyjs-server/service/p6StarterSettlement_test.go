package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6Starter047ScopeInput(t *testing.T, index int) *model.FrozenBattleInput {
	t.Helper()
	raw, err := readTestAnalysis75(fmt.Sprintf("../../analysis/p6/native-legal-starter-friend11-inputs-v115b/%03d.pb", index))
	if err != nil {
		t.Fatal(err)
	}
	p := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, p); err != nil {
		t.Fatal(err)
	}
	input, err := p6OriginalTables(t).FreezeBattleInput(model.FrozenBattleInput{Attacker: p.Attacker, Defender: p.Defender1, BattleType: 11, MaxRound: 40, Seed: p.GetRandSeed(), DefenderFirst: p.Attacker.Inst.GetFirstOrder() == 0, AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP6Starter047StructuralScopeAndPendingProof(t *testing.T) {
	s := p6OriginalTables(t)
	for index := 0; index < 8; index++ {
		input := p6Starter047ScopeInput(t, index)
		if err := validateP6Starter047Scope(input); err != nil {
			t.Fatal(index, err)
		}
		result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
		if err != nil || result.Outcome.Authoritative {
			t.Fatal(err)
		}
		if err = validateP6Starter047Terminal(input, result); err != nil {
			t.Fatal(index, err)
		}
		accepted, err := s.ComputeP6StarterSettlement(t.Context(), input)
		if constUtil.P6StarterNativeProof == "" {
			if err == nil || accepted != nil {
				t.Fatal("pending native family promoted")
			}
		} else if err != nil || accepted == nil || !accepted.Outcome.Authoritative || !p6KnownStarterSettlementProof(input, accepted.Outcome.CapabilityHash) {
			t.Fatal("verified starter family refused", err)
		}
	}
	input := p6Starter047ScopeInput(t, 0)
	input.Seed = -7654321
	input.Attacker.Formation[0].Guid = proto.String("new-owned-starter-instance")
	input.Attacker.Formation[0].Prop.Att = proto.Float64(12345)
	if err := validateP6Starter047Scope(input); err != nil {
		t.Fatal("scope became input whitelist", err)
	}
	if constUtil.P6StarterNativeProof != "" {
		result, err := s.ComputeP6StarterSettlement(t.Context(), input)
		if err != nil || result == nil || !result.Outcome.Authoritative || !p6KnownStarterSettlementProof(input, result.Outcome.CapabilityHash) {
			t.Fatal("new structural combination became a source-input whitelist", err)
		}
	}
}

func TestP6Starter047RejectsOtherMechanismFamilies(t *testing.T) {
	base := p6Starter047ScopeInput(t, 0)
	mutations := map[string]func(*model.FrozenBattleInput){
		"unobservedBreach": func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].BreachedLevel = proto.Int32(2) },
		"otherMain":        func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Id = proto.String("hero_044") },
		"skillRemoved":     func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].SkillOrder = proto.String("047_1,047_2") },
		"skillReordered": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation[0].SkillOrder = proto.String("047_2,047_1,047_3")
		},
		"partnerAdded": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation = append(x.Attacker.Formation, proto.Clone(x.Attacker.Formation[0]).(*pb.BattleHero))
		},
		"talent":          func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].TalentList = []string{"unknown-talent"} },
		"initialAnger":    func(x *model.FrozenBattleInput) { x.Attacker.Inst.Anger = proto.Float64(1) },
		"injury":          func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Inst.CurHp = proto.Float64(1) },
		"percentHP":       func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Prop.Perhp = proto.Float64(1) },
		"fractionalPanel": func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Prop.Att = proto.Float64(1.5) },
		"mode":            func(x *model.FrozenBattleInput) { x.BattleType = 2 },
		"factor":          func(x *model.FrozenBattleInput) { x.AttackerDamageFactor = proto.Float64(0) },
		"pose":            func(x *model.FrozenBattleInput) { x.Attacker.Formation[0].Pose = proto.Int32(1) },
		"overrideAI":      func(x *model.FrozenBattleInput) { x.DiagnosticAIDriverPolicy = "unknown" },
		"unknownField": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation[0].ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			input, err := cloneP6CalculationInput(base)
			if err != nil {
				t.Fatal(err)
			}
			mutate(input)
			if err = validateP6Starter047Scope(input); err == nil {
				t.Fatal("unknown mechanisms admitted")
			}
			if result, err := p6OriginalTables(t).ComputeP6StarterSettlement(t.Context(), input); err == nil || result != nil {
				t.Fatal("unknown returned settlement")
			}
		})
	}
}

// Keep the compiled starter capability bound to the actual compared native
// assets and recovered implementation; a new policy/runtime requires a new proof.
func TestP6Starter047CertificatePinsActualProofAndPolicy(t *testing.T) {
	if constUtil.P6StarterNativeProof == "" {
		t.Skip("original starter comparison pending; computation remains rejected")
	}
	raw, err := readTestAnalysis75("../../analysis/p6/" + constUtil.P6StarterCertificateFile)
	if err != nil || p6Digest(raw) != constUtil.P6StarterNativeProof {
		t.Fatal("compiled starter proof changed", err)
	}
	var proof struct {
		Scope          string `json:"scopeKey"`
		Engine         string `json:"engine"`
		NativeCompared bool   `json:"nativeCompared"`
		ScopePolicy    struct {
			Source      string `json:"source"`
			SHA         string `json:"sourceSHA256"`
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
	if err = json.Unmarshal(raw, &proof); err != nil || proof.Scope != constUtil.P6StarterScope || proof.Engine != constUtil.P6LinearPhysicalEngine || !proof.NativeCompared || len(proof.RuntimeSources) == 0 || len(proof.Evidence) == 0 {
		t.Fatal("starter certificate scope or evidence incomplete", err)
	}
	source, err := readTestAnalysis75(filepath.Base(proof.ScopePolicy.Source))
	if err != nil || p6Digest(source) != proof.ScopePolicy.SHA {
		t.Fatal("compiled starter mechanism policy changed", err)
	}
	constants, err := readTestAnalysis75("../constUtil/p6StarterSettlement.go")
	if err != nil || p6Digest([]byte(strings.ReplaceAll(string(constants), constUtil.P6StarterNativeProof, ""))) != proof.ScopePolicy.ConstantSHA {
		t.Fatal("starter constants changed beyond pinned proof", err)
	}
	for _, source := range proof.RuntimeSources {
		path := filepath.Join("..", strings.TrimPrefix(source.Path, "hyjs-server/"))
		raw, err := readTestAnalysis75(path)
		if err != nil || p6Digest(raw) != source.SHA {
			t.Fatal("certified starter runtime source changed", source.Path, err)
		}
	}
	for _, evidence := range proof.Evidence {
		raw, err := readTestAnalysis75("../../" + evidence.Path)
		if err != nil || p6Digest(raw) != evidence.SHA {
			t.Fatal("original starter proof asset changed", evidence.Path, err)
		}
	}
}
