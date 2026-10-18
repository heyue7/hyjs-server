package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6ActualOwnedPartnerInput(t *testing.T, index int) *model.FrozenBattleInput {
	t.Helper()
	raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/native-owned-partner-friend11-inputs-v118/%03d.pb", index))
	if err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	frozen, err := p6OriginalTables(t).FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, BattleType: 11, MaxRound: 40, Seed: wire.GetRandSeed(), DefenderFirst: wire.Attacker.GetInst().GetFirstOrder() == 0, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestP6OwnedPartnerStructuralAndObservedExecutionClosure(t *testing.T) {
	s := p6OriginalTables(t)
	for index := 0; index < 4; index++ {
		input := p6ActualOwnedPartnerInput(t, index)
		a, b := proto.Clone(input.Attacker), proto.Clone(input.Defender)
		if err := validateP6OwnedPartnerScope(input); err != nil {
			t.Fatal(index, err)
		}
		if !proto.Equal(a, input.Attacker) || !proto.Equal(b, input.Defender) {
			t.Fatal("validation rewrote real poses/skills/source")
		}
		result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateP6OwnedPartnerTerminal(input, result); err != nil {
			t.Fatal(index, "actual original-observed team rejected", err)
		}
		if len(result.FinalActors) != 4 || result.Outcome.Authoritative {
			t.Fatal("generic authority drift")
		}
		accepted, err := s.ComputeP6OwnedPartnerSettlement(t.Context(), input)
		if constUtil.P6OwnedPartnerNativeProof == "" {
			if err == nil || accepted != nil {
				t.Fatal("pending proof admitted")
			}
		} else if err != nil || accepted == nil || !accepted.Outcome.Authoritative || !p6KnownOwnedPartnerSettlementProof(input, accepted.Outcome.CapabilityHash) {
			t.Fatal("independent team certificate refused", err)
		}
	}
	input := p6ActualOwnedPartnerInput(t, 0)
	input.Seed = -7654321
	input.Attacker.Formation[1].Guid = proto.String("new-owned-partner-guid")
	if err := validateP6OwnedPartnerScope(input); err != nil {
		t.Fatal("structural admission became exact-source whitelist", err)
	}
	if constUtil.P6OwnedPartnerNativeProof != "" {
		if result, err := s.ComputeP6OwnedPartnerSettlement(t.Context(), input); err != nil || result == nil || !result.Outcome.Authoritative {
			t.Fatal("new GUID/seed refused", err)
		}
	}
}

func TestP6OwnedPartnerUnknownMechanismsAndFormationRemainClosed(t *testing.T) {
	base := p6ActualOwnedPartnerInput(t, 0)
	edits := map[string]func(*model.FrozenBattleInput){
		"wrongMode":    func(x *model.FrozenBattleInput) { x.BattleType = 2 },
		"wrongPartner": func(x *model.FrozenBattleInput) { x.Attacker.Formation[1].Id = proto.String("hero_071") },
		"wrongPose":    func(x *model.FrozenBattleInput) { x.Attacker.Formation[1].Pose = proto.Int32(8) },
		"extraActor": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation = append(x.Attacker.Formation, proto.Clone(x.Attacker.Formation[1]).(*pb.BattleHero))
		},
		"removedMain": func(x *model.FrozenBattleInput) { x.Attacker.Formation = x.Attacker.Formation[1:] },
		"changedOrder": func(x *model.FrozenBattleInput) {
			x.Attacker.Formation[0], x.Attacker.Formation[1] = x.Attacker.Formation[1], x.Attacker.Formation[0]
		},
		"extraTalent":   func(x *model.FrozenBattleInput) { x.Attacker.Formation[1].TalentList = []string{"unknown"} },
		"partnerGrowth": func(x *model.FrozenBattleInput) { x.Attacker.Formation[1].BreachedLevel = proto.Int32(2) },
		"initialInjury": func(x *model.FrozenBattleInput) { x.Attacker.Formation[1].Inst.CurHp = proto.Float64(1) },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			input, err := cloneP6CalculationInput(base)
			if err != nil {
				t.Fatal(err)
			}
			edit(input)
			if err = validateP6OwnedPartnerScope(input); err == nil {
				t.Fatal("unknown team mechanism admitted")
			}
			if result, err := p6OriginalTables(t).ComputeP6OwnedPartnerSettlement(t.Context(), input); err == nil || result != nil {
				t.Fatal("unknown team produced settlement")
			}
		})
	}
}

func TestP6OwnedPartnerUnobservedUltimateNeverSettles(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6ActualOwnedPartnerInput(t, 0)
	result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateP6OwnedPartnerTerminal(input, result); err != nil {
		t.Fatal(err)
	}
	result.Outcome.Events = append(result.Outcome.Events, model.CombatEvent{Kind: "skill_press", Skill: "069_3"})
	if err = validateP6OwnedPartnerTerminal(input, result); err == nil {
		t.Fatal("unobserved partner ultimate execution accepted")
	}
	result.Outcome.Events = result.Outcome.Events[:len(result.Outcome.Events)-1]
	result.FinalActors[1].MaxHP++
	if err = validateP6OwnedPartnerTerminal(input, result); err == nil {
		t.Fatal("invented terminal maxHP accepted")
	}
}

func TestP6OwnedPartnerCertificatePinsActualProofAndPolicy(t *testing.T) {
	if constUtil.P6OwnedPartnerNativeProof == "" {
		t.Skip("original owned partner comparison pending; computation remains rejected")
	}
	raw, err := os.ReadFile("../../analysis/p6/" + constUtil.P6OwnedPartnerCertificateFile)
	if err != nil || p6Digest(raw) != constUtil.P6OwnedPartnerNativeProof {
		t.Fatal("compiled owned partner proof changed", err)
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
		SupportingPolicies []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"supportingPolicies"`
		RuntimeSources []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"runtimeSources"`
		Evidence []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"originalNativeEvidence"`
	}
	if err = json.Unmarshal(raw, &proof); err != nil || proof.Scope != constUtil.P6OwnedPartnerScope || proof.Engine != constUtil.P6OwnedPartnerSettlementEngine || !proof.NativeCompared || len(proof.RuntimeSources) == 0 || len(proof.Evidence) == 0 {
		t.Fatal("owned partner certificate scope or evidence incomplete", err)
	}
	source, err := os.ReadFile(filepath.Base(proof.ScopePolicy.Source))
	if err != nil || p6Digest(source) != proof.ScopePolicy.SHA {
		t.Fatal("compiled owned partner mechanism policy changed", err)
	}
	constants, err := os.ReadFile("../constUtil/p6OwnedPartnerSettlement.go")
	if err != nil || p6Digest([]byte(strings.ReplaceAll(string(constants), constUtil.P6OwnedPartnerNativeProof, ""))) != proof.ScopePolicy.ConstantSHA {
		t.Fatal("owned partner constants changed beyond pinned proof", err)
	}
	for _, source := range append(proof.RuntimeSources, proof.SupportingPolicies...) {
		path := filepath.Join("..", strings.TrimPrefix(source.Path, "hyjs-server/"))
		raw, err := os.ReadFile(path)
		if err != nil || p6Digest(raw) != source.SHA {
			t.Fatal("certified owned partner runtime source changed", source.Path, err)
		}
	}
	for _, evidence := range proof.Evidence {
		raw, err := os.ReadFile("../../" + evidence.Path)
		if err != nil || p6Digest(raw) != evidence.SHA {
			t.Fatal("original owned partner proof asset changed", evidence.Path, err)
		}
	}
}
