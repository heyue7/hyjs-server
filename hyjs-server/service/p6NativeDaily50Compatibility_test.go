package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"strings"
	"testing"
)

// Same actual paid Daily50 source under explicitly named local controller
// recovery. Old known clocks only; never original parity or terminal-record proof.
func TestP6NativeDaily50NamedCompatibilityLegacyCoreAndDynamic(t *testing.T) {
	const file = "native-local-daily50-compat-legacy-golden-v101.json"
	const sha = "a008992ebf9df8f6556289880e4ded460d6b32af4c103877085c8ce45f76cbd0"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-daily50-compat-legacy-manager-golden-v101.json", "931fd0de11a7c809fc4754d4af325e5baaffa324c4c4564d54c84bcfdbbaaaf4", 1, 1)
	})
}

func TestP6NativeDaily50NamedCompatibilityLegacyExports(t *testing.T) {
	const file = "native-local-daily50-compat-legacy-hit-golden-v101.json"
	const sha = "a7365bafa691146eed55a7c11de33cc70c59e06a038dd08ef49d2de8d338405d"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

// A successful named legacy comparison cannot make the same paid wire succeed
// under unbound or strict native evidence, or mutate the cached frozen source.
func TestP6NativeDaily50NamedCompatibilityScopeDoesNotAuthorizeStrict(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := os.ReadFile("../../analysis/p6/daily50-local-pve-no-target-finish-v1-input-20261014/001.pb")
	if err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(input)
	out, err := p6NativeGoldenComputeProjection(t, s, input, p6Digest(raw), "a008992ebf9df8f6556289880e4ded460d6b32af4c103877085c8ce45f76cbd0", nil, nil)
	if err != nil || out.Authoritative || out.PVECompatibilityRule != constUtil.P6LocalPVENoTargetFinish {
		t.Fatal("exact compatibility source projection", err)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) || input.PVECompatibilityRule != "" {
		t.Fatal("named comparison mutated original strict input")
	}
	for _, binding := range [][2]string{{p6Digest(raw), "unbound-strict-native-source"}, {"different-wire", "a008992ebf9df8f6556289880e4ded460d6b32af4c103877085c8ce45f76cbd0"}} {
		result, e := p6NativeGoldenComputeProjection(t, s, input, binding[0], binding[1], nil, nil)
		if result != nil || e == nil || !strings.Contains(e.Error(), constUtil.P6OriginalAINoTargetStall) {
			t.Fatal("compatibility evidence leaked into strict original path", binding, e)
		}
	}
}
