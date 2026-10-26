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

// Preserve every existing fourteen-case denominator. Dedicated typed original
// protocol proofs explicitly bind their own source case count after full SHA check.
func p6NativeDiagnosticCaseCount(t *testing.T, expected ...int) int {
	t.Helper()
	if len(expected) == 0 {
		return 14
	}
	if len(expected) != 1 || expected[0] <= 0 {
		t.Fatal("invalid explicitly pinned native fixture count", expected)
	}
	return expected[0]
}

func TestP6NativeLocalUF14ActualOriginalCoreAndDynamic(t *testing.T) {
	const file = "native-local-uf14-golden-v82.json"
	const sha = "45af2777020516dcdc354447eb5a1a38a21115b22643762f043399aaffa55473"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-uf14-manager-golden-v82.json", "da770f410207e0be3d900a7a8459365b6f123e421d3904758c49cfc6e59798da", 1, 1)
	})
}

// The originalPVP fixtures remain exactly14. This separately pinned one-case
// typed UF14 callback evidence is scoped to the actual local frozen source.
func p6NativeOriginalPerHitCaseCount(name string) int {
	if count := p6UFPartyPVPPhone20261026HitCaseCount(name); count != 0 {
		return count
	}
	if count := p6UFGuardPartyNew2Phone20261026HitCaseCount(name); count != 0 {
		return count
	}
	if count := p6UFGuard14Phone20261026HitCaseCount(name); count != 0 {
		return count
	}
	if count := p6UFGuardPhone20261026HitCaseCount(name); count != 0 {
		return count
	}
	if count := p6GenericLocalNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p7NumericPhoneRushNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p7NaturalPhoneRushNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p7NumericLocalRushNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p12WantedPartialNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p12WantedNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p7AwakeNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p12FieldPVPNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p7PartyRushNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p12WorldBossNativeHitCaseCount(name); count != 0 {
		return count
	}
	if count := p12WorldBossPartyNativeHitCaseCount(name); count != 0 {
		return count
	}
	if name == "native-actual-war-wounded14-hit-golden-v196.json" {
		return 2
	}
	if name == "native-original-uf14-player-factor1-hit-golden-v192.json" {
		return 7
	}
	if name == "native-original-full-party-limit20-hit-golden-v188.json" || name == "native-original-full-party-limit20-mirror-hit-golden-v190.json" {
		return 2
	}
	if name == "native-original-grab19-hit-golden-v184.json" {
		return 7
	}
	if name == "native-original-limit20-strong-hit-golden-v180.json" {
		return 2
	}
	if name == "native-original-limit20-hit-golden-v176.json" {
		return 1
	}
	if name == "native-local-kf-joint-realm-hit-golden-v175.json" {
		return 2
	}
	if name == "native-elite-manual-hit-golden-v168.json" {
		return 2
	}
	if name == "native-actual-phone-rush-manual-auto-hit-golden-v163.json" {
		return 1
	}
	if name == "native-original-plunder3-hit-golden-v162.json" {
		return 11
	}
	if name == "native-full-party-arena2-hit-golden-v153.json" {
		return 8
	}
	if name == "native-actual-demo-ultimate-hit-golden-v150.json" {
		return 4
	}
	if name == "native-actual-demo-branches-hit-golden-v142.json" {
		return 8
	}
	if name == "native-qualified-prayer-type17-hit-golden-v139.json" {
		return 3
	}
	if name == "native-actual-demo4-vs-demo3-friend11-hit-golden-v138.json" {
		return 1
	}
	if name == "native-local-p7-rush-terminal-hit-golden-v136.json" {
		return 4
	}
	if name == "native-legal-main-choices-friend11-hit-golden-v125.json" {
		return 8
	}
	if name == "native-owned-partner-friend11-hit-golden-v124.json" {
		return 4
	}
	if name == "native-local-p14-type17-qualified-hit-golden-v123.json" {
		return 1
	}
	if name == "native-actual-arena047-typed-hit-golden-v122.json" {
		return 3
	}
	if name == "native-legal-starter-friend11-hit-golden-v118.json" {
		return 8
	}
	if name == "native-linear-physical-friend11-hit-golden-v113.json" {
		return 2
	}
	if name == "native-linear-physical-candidate-hit-golden-v111.json" {
		return 8
	}
	if name == "native-local-p7-rush-typed-hit-golden-v106.json" || name == "native-actual-paid-copy-auto-hit-golden-v88.json" || name == "native-actual-paid-copy-playback-hit-golden-v88.json" || name == "native-actual-paid-copy-playback-hit-golden-v89.json" || name == "native-original-both-camps-recorded-replay-hit-golden-v91.json" || name == "native-initial-zero-hp-hit-golden-v94.json" {
		return 2
	}
	if name == "native-local-daily50-compat-hit-golden-v108.json" || name == "native-local-daily50-compat-legacy-hit-golden-v101.json" || name == "native-local-uf14-hit-golden-v82.json" || name == "native-local-uf14-guard-hit-golden-v85.json" || name == "native-local-uf14-guard-mode-hit-golden-v94.json" {
		return 1
	}
	return 14
}
func TestP6NativeLocalUF14ActualOriginalExports(t *testing.T) {
	const file = "native-local-uf14-hit-golden-v82.json"
	const sha = "076ef888eba5dde6311897df691fbb4ae4a6420fb94dab02221c59e3be97d8d6"
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

func TestP6NativeLocalUF14ActualFrozenSourceIdentity(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "analysis/p13/actual-battle14-20261014/actual-2383639473834764406.frozen-input.json"))
	if err != nil || p6Digest(raw) != "1e003daa6e4058449bf0b5631d66d6fbb4aa1ef25a2c36d977f968c97e990d8c" {
		t.Fatal("actual local UF14 frozen source SHA", err)
	}
	var stored model.FrozenBattleInput
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(root, "analysis/p6/native-local-uf14-input-v82b/000.pb"))
	if err != nil || p6Digest(raw) != "523cb4140120d996788bd56476ff8452d8de10bcefeef4ee94ec41fe97ccd7dc" {
		t.Fatal("actual local UF14 wire SHA", err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(stored.Attacker, wire.Attacker) || !proto.Equal(stored.Defender, wire.Defender1) || stored.Monster != nil || stored.Seed != wire.GetRandSeed() || stored.BattleType != 14 || stored.IsAttToDef || stored.DefenderFirst || stored.AttackerDamageFactor == nil || *stored.AttackerDamageFactor != 0 || stored.DefenderDamageFactor == nil || *stored.DefenderDamageFactor != 1 || stored.Attacker.GetInst().GetFirstOrder() != 0 || stored.Defender.GetInst().GetFirstOrder() != 0 {
		t.Fatal("actual typed UF14 actors/factors/seed/defaultFirstOrder/view differ")
	}
	svc := p6OriginalTables(t)
	current, err := svc.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor, ClientVersion: stored.ClientVersion, AppliedProperties: stored.AppliedProperties})
	if err != nil || current.ConfigurationHash != stored.ConfigurationHash {
		t.Fatal("actual local UF14 frozen original table configuration differs", err)
	}
	if !proto.Equal(current.Attacker, stored.Attacker) || !proto.Equal(current.Defender, stored.Defender) {
		t.Fatal("actual frozen frame/pose currentHP/explicitWard props changed")
	}
	// The historical committed v59 outcome is preserved as its own record.
	// This new test uses current verified engine while keeping exact source input.
	if current.EngineVersion == stored.EngineVersion {
		t.Fatal("test must distinguish historical committed source from current engine validation")
	}
}
