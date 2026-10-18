package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"testing"
)

// Actual local guard uses original strict automatic PVE; no compatibility caller.
func TestP6NativeLocalUF14GuardActualOriginalCoreAndDynamic(t *testing.T) {
	const file = "native-local-uf14-guard-golden-v85.json"
	const sha = "03b850f2f0b2026b1b4726b441371804445cdcb85aff714d5c2b9b54dcff4d78"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-uf14-guard-manager-golden-v85.json", "6e6a6c050588f956ad41ddb1c7f1bd24c14ab83a46781390e00d965705af6e16", 1, 1)
	})
}

func TestP6NativeLocalUF14GuardActualOriginalExports(t *testing.T) {
	const file = "native-local-uf14-guard-hit-golden-v85.json"
	const sha = "4423c5cd9cd1b2f8ed15c091681bf563bc7c78cd3a074334053f8fcbd3c41e82"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
		p6OriginalBuffHPFixture(t, file, sha, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

func TestP6NativeLocalUF14GuardActualFrozenSourceIdentity(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "analysis/p13/actual-battle14-20261015/guard-actual-4165694016210520132.frozen-input.json"))
	if err != nil || p6Digest(raw) != "18d3d4834703036ac4f1ba0bcf6b04a7f39e1e8423a10f6ef006b02e594d0bd0" {
		t.Fatal("actual local UF14 frozen source SHA", err)
	}
	var stored model.FrozenBattleInput
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(root, "analysis/p6/native-local-uf14-guard-input-v83/000.pb"))
	if err != nil || p6Digest(raw) != "3d69ec109c3d37c6817c871c8e83d14b5924ad96eb843163838b56fd75b5aea8" {
		t.Fatal("actual local UF14 wire SHA", err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(stored.Attacker, wire.Attacker) || !proto.Equal(stored.Monster, wire.Defender2) || stored.Defender != nil || stored.Seed != wire.GetRandSeed() || stored.BattleType != 14 || stored.IsAttToDef || stored.DefenderFirst || stored.AttackerDamageFactor == nil || *stored.AttackerDamageFactor != 0 || stored.DefenderDamageFactor == nil || *stored.DefenderDamageFactor != 1 || stored.Attacker.GetInst().GetFirstOrder() != 0 || wire.Defender1 != nil {
		t.Fatal("actual typed UF14 actors/factors/seed/defaultFirstOrder/view differ")
	}
	svc := p6OriginalTables(t)
	current, err := svc.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor, ClientVersion: stored.ClientVersion, AppliedProperties: stored.AppliedProperties})
	if err != nil || current.ConfigurationHash != stored.ConfigurationHash {
		t.Fatal("actual local UF14 frozen original table configuration differs", err)
	}
	if !proto.Equal(current.Attacker, stored.Attacker) || !proto.Equal(current.Monster, stored.Monster) {
		t.Fatal("actual frozen frame/pose currentHP/explicitWard props changed")
	}
	// The historical committed v59 outcome is preserved as its own record.
	// This new test uses current verified engine while keeping exact source input.
	if current.EngineVersion == stored.EngineVersion {
		t.Fatal("test must distinguish historical committed source from current engine validation")
	}
}
