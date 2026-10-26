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

func p6Arena047ScopeInput(t *testing.T, index int) *model.FrozenBattleInput {
	t.Helper()
	raw, err := readTestAnalysis75(fmt.Sprintf("../../analysis/p6/native-actual-arena047-typed-inputs-v121/%03d.pb", index))
	if err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	input, err := p6OriginalTables(t).FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, BattleType: 2, Seed: wire.GetRandSeed(), MaxRound: 40, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP6Arena047RequiresIndependentOriginalTypedProof(t *testing.T) {
	s := p6OriginalTables(t)
	for index := 0; index < 3; index++ {
		input := p6Arena047ScopeInput(t, index)
		if err := validateP6Arena047Scope(input); err != nil {
			t.Fatal(index, err)
		}
		if validateP6Starter047Scope(input) == nil || p6KnownStarterSettlementProof(input, constUtil.P6StarterNativeProof) {
			t.Fatal("mode11 proof silently extended to arena2")
		}
		out, err := s.ComputeP6ArenaSettlement(t.Context(), input)
		if constUtil.P6ArenaNativeProof == "" {
			if err == nil || out != nil {
				t.Fatal("uncertified arena returned winner")
			}
		} else if err != nil || out == nil || !out.Outcome.Authoritative || !p6KnownArenaSettlementProof(input, out.Outcome.CapabilityHash) {
			t.Fatal("independent arena scope refused", err)
		}
	}
	input := p6Arena047ScopeInput(t, 0)
	input.Seed = -987654321
	input.Attacker.Formation[0].Guid = proto.String("actual-new-arena-source")
	if err := validateP6Arena047Scope(input); err != nil {
		t.Fatal("source input whitelist", err)
	}
	input.BattleType = 11
	if err := validateP6Arena047Scope(input); err == nil {
		t.Fatal("arena scope accepted Friend mode")
	}
}

func TestP6Arena047CertificatePinsActualProofAndPolicy(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("original arena comparison pending; computation remains rejected")
	}
	raw, err := readTestAnalysis75("../../analysis/p6/" + constUtil.P6ArenaCertificateFile)
	if err != nil || p6Digest(raw) != constUtil.P6ArenaNativeProof {
		t.Fatal("compiled arena proof changed", err)
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
	if err = json.Unmarshal(raw, &proof); err != nil || proof.Scope != constUtil.P6ArenaScope || proof.Engine != constUtil.P6ArenaSettlementEngine || !proof.NativeCompared || len(proof.RuntimeSources) == 0 || len(proof.Evidence) == 0 {
		t.Fatal("arena certificate scope or evidence incomplete", err)
	}
	source, err := readTestAnalysis75(filepath.Base(proof.ScopePolicy.Source))
	if err != nil || p6Digest(source) != proof.ScopePolicy.SHA {
		t.Fatal("compiled arena mechanism policy changed", err)
	}
	constants, err := readTestAnalysis75("../constUtil/p6ArenaSettlement.go")
	if err != nil || p6Digest([]byte(strings.ReplaceAll(string(constants), constUtil.P6ArenaNativeProof, ""))) != proof.ScopePolicy.ConstantSHA {
		t.Fatal("arena constants changed beyond pinned proof", err)
	}
	for _, source := range append(proof.RuntimeSources, proof.SupportingPolicies...) {
		path := filepath.Join("..", strings.TrimPrefix(source.Path, "hyjs-server/"))
		raw, err := readTestAnalysis75(path)
		if err != nil || p6Digest(raw) != source.SHA {
			t.Fatal("certified arena runtime source changed", source.Path, err)
		}
	}
	for _, evidence := range proof.Evidence {
		raw, err := readTestAnalysis75("../../" + evidence.Path)
		if err != nil || p6Digest(raw) != evidence.SHA {
			t.Fatal("original arena proof asset changed", evidence.Path, err)
		}
	}
}
