package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p12WorldBossPartyNativeBinding struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p12WorldBossPartyNativeBundle struct {
	Schema     string                                    `json:"schema"`
	Source     string                                    `json:"sourceManifest"`
	SourceSHA  string                                    `json:"sourceManifestSHA256"`
	Capture    string                                    `json:"captureManifest"`
	CaptureSHA string                                    `json:"captureManifestSHA256"`
	Count      int                                       `json:"caseCount"`
	Scope      string                                    `json:"nativeScope"`
	Authority  bool                                      `json:"businessAuthority"`
	Business   bool                                      `json:"actualBusinessStart"`
	Kind       string                                    `json:"sourceKind"`
	Fixtures   map[string]p12WorldBossPartyNativeBinding `json:"fixtures"`
}

type p12WorldBossPartyNativeFamily struct {
	Source, SHA, Kind, Schema, Env string
	Count                          int
	Boost                          bool
}

func p12WorldBossPartyNativeFamilyDefinition(boost bool) p12WorldBossPartyNativeFamily {
	if boost {
		return p12WorldBossPartyNativeFamily{Source: "analysis/p6/worldboss18-original-boost-effect-pending-20261016/manifest.json", SHA: "21359fb1f66d479b630f63b2f1c13e8e75f9d57720c181059eec0b754a1439ba", Kind: "finite-original-boost-effect-helper-diagnostic", Schema: "p12-worldboss-boost001003-native-matrix-v1", Env: "HYJS_P12_WORLDBOSS_BOOST_NATIVE_BUNDLE", Count: 30, Boost: true}
	}
	return p12WorldBossPartyNativeFamily{Source: "analysis/p6/worldboss18-actual-party001003-pending-20261016/manifest.json", SHA: "78a7d0d3d0bd7bc4585bf101627f927050237f37643fb01b847a0fa196811d47", Kind: "finite-historical-actual-p5-party001003-original-table-diagnostic", Schema: "p12-worldboss-party001003-native-matrix-v1", Env: "HYJS_P12_WORLDBOSS_PARTY_NATIVE_BUNDLE", Count: 2}
}

func p12WorldBossPartyNativeSelectedFamily() (p12WorldBossPartyNativeFamily, bool) {
	base, boost := p12WorldBossPartyNativeFamilyDefinition(false), p12WorldBossPartyNativeFamilyDefinition(true)
	b, x := os.Getenv(base.Env) != "", os.Getenv(boost.Env) != ""
	if b == x {
		return base, false
	}
	if x {
		return boost, true
	}
	return base, true
}

type p12WorldBossPartyNativeCase struct {
	family     p12WorldBossPartyNativeFamily
	BoostMask  int      `json:"boostMask"`
	BoostIDs   []string `json:"diagnosticBoostIDs"`
	Frozen     string   `json:"frozenInput"`
	FrozenSHA  string   `json:"frozenInputSHA256"`
	Name       string   `json:"case"`
	Wire       string   `json:"wire"`
	WireSHA    string   `json:"wireSHA256"`
	Typed      string   `json:"typedCallback"`
	TypedSHA   string   `json:"typedCallbackSHA256"`
	FightID    string   `json:"fightId"`
	Seed       int64    `json:"seed"`
	Type       int32    `json:"battleType"`
	Members    int      `json:"fullBossMembers"`
	Level      int64    `json:"bossLevel"`
	ActorSHAs  []string `json:"sourceActorSHA256"`
	MemberSHAs []string `json:"sourceBossMemberSHA256"`
}

func p12WorldBossPartyNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") {
		t.Fatal("P12 diagnostic evidence must remain under analysis/p6", relative)
	}
	return filepath.Join("../..", clean)
}

func p12WorldBossPartyNativeRead(t *testing.T, name, expected string) []byte {
	t.Helper()
	raw, err := readTestAnalysis75(p12WorldBossPartyNativePath(t, name))
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("P12 diagnostic immutable source SHA", name, err)
	}
	return raw
}

func p12WorldBossPartyNativeCases(t *testing.T) []p12WorldBossPartyNativeCase {
	return p12WorldBossPartyNativeFamilyCases(t, p12WorldBossPartyNativeFamilyDefinition(false))
}

func p12WorldBossPartyNativeFamilyCases(t *testing.T, family p12WorldBossPartyNativeFamily) []p12WorldBossPartyNativeCase {
	t.Helper()
	raw := p12WorldBossPartyNativeRead(t, family.Source, family.SHA)
	var source struct {
		Kind      string                        `json:"sourceKind"`
		Business  bool                          `json:"actualBusinessStart"`
		Paid      bool                          `json:"actualPaid"`
		Ledger    bool                          `json:"purchaseLedgerCreated"`
		Compared  bool                          `json:"nativeCompared"`
		Authority bool                          `json:"authoritative"`
		Executed  bool                          `json:"engineExecuted"`
		Helper    string                        `json:"effectHelper"`
		HelperSHA string                        `json:"effectHelperFileSHA256"`
		Cases     []p12WorldBossPartyNativeCase `json:"cases"`
	}
	if json.Unmarshal(raw, &source) != nil || source.Kind != family.Kind || source.Business || source.Paid || source.Ledger || source.Compared || source.Authority || source.Executed || len(source.Cases) != family.Count {
		t.Fatal("P12 independent diagnostic family identity/count/authority")
	}
	if family.Boost && (source.Helper != "GameService.applyP12WorldBossBoostRows" || source.HelperSHA != "5db3c79a01191969f955206e2c52a7accee3cb695db9113e8c1e0d54f14fa46e") {
		t.Fatal("P12 actual production effect-helper export identity")
	}
	for index := range source.Cases {
		row := &source.Cases[index]
		row.family = family
		if row.Seed != []int64{7, 314159}[index%2] || family.Boost && row.BoostMask != index/2+1 || !family.Boost && row.BoostMask != 0 {
			t.Fatal("P12 exact independent mask/seed matrix denominator")
		}
	}
	return source.Cases
}

// p12WorldBossPartyNativeTypedInput binds the complete original typed18 callback to
// its playback projection. No diagnostic AI override or result hint is added.
func p12WorldBossPartyNativeTypedInput(t *testing.T, row p12WorldBossPartyNativeCase) *pb.SCFightPlayback {
	t.Helper()
	base := filepath.ToSlash(filepath.Dir(row.family.Source)) + "/"
	var (
		wire  pb.SCFightPlayback
		start pb.SCWorldBossFight
	)
	if proto.Unmarshal(p12WorldBossPartyNativeRead(t, base+row.Wire, row.WireSHA), &wire) != nil || proto.Unmarshal(p12WorldBossPartyNativeRead(t, base+row.Typed, row.TypedSHA), &start) != nil {
		t.Fatal("P12 original protocol bytes")
	}
	if row.Type != 18 || row.Members != 4 || row.Level != 1 || row.Name != row.FightID || wire.GetFightid() != row.FightID || start.GetFightId() != row.FightID || start.RandSeed == nil || wire.GetRandSeed() != row.Seed || start.GetRandSeed() != row.Seed || wire.GetBattleType() != 18 || wire.GetMaxRound() != 40 || start.GetMaxround() != 40 || wire.Defender1 != nil || wire.Defender2 == nil || wire.Attacker == nil || len(start.BattleMonsterList) != 1 || len(wire.Defender2.Formation) != 4 || len(wire.Attacker.Formation) != 2 || wire.OptRecord != nil || start.GetFightScene() == "" || wire.AttackerFactor == nil || wire.GetAttackerFactor() != 1 || wire.DefenderFactor == nil || wire.GetDefenderFactor() != 1 || !proto.Equal(wire.Attacker, start.BattlePlayer) || !proto.Equal(wire.Defender2, start.BattleMonsterList[0]) || start.Code != nil || start.Damange != nil {
		t.Fatal("P12 exact typed18, four-body, factors1/1 original automatic diagnostic input")
	}
	if len(row.ActorSHAs) != 2 || len(row.MemberSHAs) != 4 {
		t.Fatal("P12 complete actor source denominator")
	}
	poses := []int32{13, 7, 9, 3}
	blood := []int32{2, 1, 1, 1}
	for camp, formation := range [][]*pb.BattleHero{wire.Attacker.Formation, wire.Defender2.Formation} {
		for index, actor := range formation {
			encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
			if err != nil || actor.Prop == nil || actor.Inst == nil {
				t.Fatal("P12 complete source actor", err)
			}
			if camp == 0 {
				expected := []string{"bf72e8364dd340aeea87a59f3001f046679c2a51833c526e17b9bc4b10368ef7", "01325a107d0e67d2bb97261af19c3e697ac13dacc1dd4edfabb4f1f7b28413be"}
				if p6Digest(encoded) != row.ActorSHAs[index] || !row.family.Boost && p6Digest(encoded) != expected[index] || actor.GetId() != []string{"hero_001", "hero_003"}[index] || actor.GetPose() != []int32{3, 7}[index] || actor.Inst.GetCurHp() != actor.Prop.GetHp() {
					t.Fatal("P12 unchanged complete historical actual001003 BattleHero")
				}
			} else if p6Digest(encoded) != row.MemberSHAs[index] || actor.GetPose() != poses[index] || actor.GetBloodNumber() != blood[index] || !strings.HasPrefix(actor.GetGuid(), "worldboss-") {
				t.Fatal("P12 full original body/talent/skill/blood source")
			}
		}
	}
	if row.family.Boost {
		p12WorldBossPartyNativeValidateBoostSource(t, row, &wire)
	}
	return &wire
}

// Validate the producer's complete output by reusing its production effect
// helper on the bound base. This does not duplicate the property/talent math,
// create a purchase, or run a battle. Every frozen table row stays SHA-closed.
func p12WorldBossPartyNativeValidateBoostSource(t *testing.T, row p12WorldBossPartyNativeCase, wire *pb.SCFightPlayback) {
	t.Helper()
	base := p12WorldBossPartyNativeTypedInput(t, p12WorldBossPartyNativeCases(t)[0])
	party := proto.Clone(base.Attacker).(*pb.BattlePlayer)
	definitions, tableSHA := p12WorldBossBoostOriginalTable(t, "WorldBossPropadd")
	if tableSHA != "0d63b06098d671fa4d1875bb8517fdddb86d85833c83f81e6a5b7bc9a2129268" || len(definitions) != 4 || row.BoostMask < 1 || row.BoostMask > 15 {
		t.Fatal("original boost definition and subset identity")
	}
	var rows []db.P12WorldBossBoost
	for index, definition := range definitions {
		if definition["id"] != "wordboss_"+strconv.Itoa(index+1) || definition["cost"] != "diamond,150" {
			t.Fatal("canonical original property order/price")
		}
		if row.BoostMask&(1<<index) == 0 {
			continue
		}
		kind, err := strconv.ParseInt(definition["type"], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, db.P12WorldBossBoost{PropertyID: definition["id"], Kind: kind, Property: definition["prop"]})
	}
	s := p6OriginalTables(t)
	ids, err := s.applyP12WorldBossBoostRows(party, rows)
	if err != nil || strings.Join(ids, ",") != strings.Join(row.BoostIDs, ",") || !proto.Equal(party, wire.Attacker) || !proto.Equal(base.Defender2, wire.Defender2) {
		t.Fatal("typed18 actor output differs from actual production boost helper", err)
	}
	raw := p12WorldBossPartyNativeRead(t, filepath.ToSlash(filepath.Join(filepath.Dir(row.family.Source), row.Frozen)), row.FrozenSHA)
	var frozen model.FrozenBattleInput
	if json.Unmarshal(raw, &frozen) != nil || !proto.Equal(frozen.Attacker, wire.Attacker) || !proto.Equal(frozen.Monster, wire.Defender2) || frozen.Defender != nil || frozen.KFReplay != nil || frozen.KFReplayViewer != nil || frozen.BattleType != 18 || frozen.MaxRound != 40 || frozen.Seed != row.Seed || frozen.DefenderFirst || frozen.IsAttToDef || frozen.PVECompatibilityRule != "" || frozen.DiagnosticAIDriverPolicy != "" || frozen.DiagnosticManualAutoStartRoundTime != nil || len(frozen.WinConditions) != 0 || frozen.AttackerDamageFactor == nil || *frozen.AttackerDamageFactor != 1 || frozen.DefenderDamageFactor == nil || *frozen.DefenderDamageFactor != 1 || frozen.EngineVersion != constUtil.P6LinearPhysicalEngine || frozen.ClientVersion != constUtil.P6OriginalClientHash || frozen.ConfigurationHash != constUtil.P6LinearPhysicalTables {
		t.Fatal("complete original helper frozen input/source identity")
	}
	tables, err := json.Marshal(frozen.Tables)
	if err != nil || p6Digest(tables) != frozen.ConfigurationHash || len(frozen.AppliedProperties) != 1 || strings.Join(frozen.AppliedProperties["diagnosticUnpaidOriginalBoostEffects"], ",") != strings.Join(ids, ",") || frozen.Attacker.Inst.GetAnger() != 0 || frozen.Monster.Inst.GetAnger() != 0 {
		t.Fatal("closed original tables and unpaid AP0 effect provenance", err)
	}
}

func p12WorldBossPartyNativeLoadBundle(t *testing.T) *p12WorldBossPartyNativeBundle {
	t.Helper()
	family, ok := p12WorldBossPartyNativeSelectedFamily()
	if !ok {
		if os.Getenv("HYJS_P12_WORLDBOSS_PARTY_NATIVE_BUNDLE") != "" || os.Getenv("HYJS_P12_WORLDBOSS_BOOST_NATIVE_BUNDLE") != "" {
			t.Fatal("select exactly one independent P12 diagnostic bundle")
		}
		return nil
	}
	return p12WorldBossPartyNativeLoadFamilyBundle(t, family)
}

func p12WorldBossPartyNativeLoadFamilyBundle(t *testing.T, family p12WorldBossPartyNativeFamily) *p12WorldBossPartyNativeBundle {
	t.Helper()
	name := os.Getenv(family.Env)
	if name == "" {
		return nil
	}
	raw, err := readTestAnalysis75(name)
	expected := os.Getenv(family.Env + "_SHA256")
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("explicit P12 native bundle SHA", err)
	}
	var bundle p12WorldBossPartyNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != family.Schema || bundle.Source != family.Source || bundle.SourceSHA != family.SHA || bundle.Count != family.Count || bundle.Scope != "original-worldboss-automatic-ai" || bundle.Authority || bundle.Business || bundle.Kind != family.Kind || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded P12 original typed18 diagnostic bundle identity/denominator/authority")
	}
	p12WorldBossPartyNativeRead(t, bundle.Source, bundle.SourceSHA)
	captureRaw := p12WorldBossPartyNativeRead(t, bundle.Capture, bundle.CaptureSHA)
	var capture struct {
		Source    string                        `json:"sourceManifest"`
		SourceSHA string                        `json:"sourceManifestSHA256"`
		Selector  string                        `json:"selector"`
		Cases     []p12WorldBossPartyNativeCase `json:"cases"`
	}
	if json.Unmarshal(captureRaw, &capture) != nil || capture.SourceSHA != bundle.SourceSHA || capture.Selector != "highest" || len(capture.Cases) != family.Count {
		t.Fatal("P12 original capture source/selector/denominator")
	}
	actualSource, err := filepath.Abs(capture.Source)
	expectedSource, pathErr := filepath.Abs(p12WorldBossPartyNativePath(t, bundle.Source))
	if err != nil || pathErr != nil || actualSource != expectedSource {
		t.Fatal("P12 capture must bind the exact diagnostic source manifest")
	}
	seen := map[string]bool{}
	for _, row := range capture.Cases {
		for _, original := range p12WorldBossPartyNativeFamilyCases(t, family) {
			if row.WireSHA == original.WireSHA && row.TypedSHA == original.TypedSHA && row.FightID == original.FightID && row.Seed == original.Seed && !seen[row.WireSHA] {
				seen[row.WireSHA] = true
			}
		}
	}
	if len(seen) != family.Count {
		t.Fatal("P12 capture cannot borrow another typed18 source")
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all P12 native comparison layers required", kind)
		}
		p12WorldBossPartyNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// The count hook admits this exact independent bundle; earlier P7/other fixture
// denominators remain unchanged. Missing or malformed opt-in evidence cannot pass.
func p12WorldBossPartyNativeHitCaseCount(name string) int {
	family, ok := p12WorldBossPartyNativeSelectedFamily()
	if !ok {
		return 0
	}
	raw, err := readTestAnalysis75(os.Getenv(family.Env))
	if err != nil || p6Digest(raw) != os.Getenv(family.Env+"_SHA256") {
		return 0
	}
	var bundle p12WorldBossPartyNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != family.Schema || bundle.Count != family.Count || bundle.Authority || bundle.Business || bundle.Kind != family.Kind || bundle.Scope != "original-worldboss-automatic-ai" || bundle.Source != family.Source || bundle.SourceSHA != family.SHA || len(bundle.Fixtures) != 4 {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	for _, binding := range []p12WorldBossPartyNativeBinding{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}, {filepath.ToSlash(filepath.Join("analysis/p6", name)), hits.SHA}} {
		if filepath.IsAbs(binding.Path) || !strings.HasPrefix(filepath.ToSlash(filepath.Clean(binding.Path)), "analysis/p6/") {
			return 0
		}
		data, err := readTestAnalysis75(filepath.Join("../..", binding.Path))
		if err != nil || len(binding.SHA) != 64 || p6Digest(data) != binding.SHA {
			return 0
		}
	}
	return family.Count
}

func p12WorldBossPartyNativeBoundInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) bool {
	t.Helper()
	bundle := p12WorldBossPartyNativeLoadBundle(t)
	if bundle == nil {
		return false
	}
	bound := false
	for _, fixture := range bundle.Fixtures {
		if fixture.SHA == fixtureSHA {
			p12WorldBossPartyNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			bound = true
		}
	}
	if !bound {
		return false
	}
	family, _ := p12WorldBossPartyNativeSelectedFamily()
	for _, row := range p12WorldBossPartyNativeFamilyCases(t, family) {
		if row.WireSHA != wireSHA {
			continue
		}
		wire := p12WorldBossPartyNativeTypedInput(t, row)
		if input == nil || input.BattleType != 18 || input.Monster == nil || input.Defender != nil || !proto.Equal(input.Attacker, wire.Attacker) || !proto.Equal(input.Monster, wire.Defender2) || input.Seed != wire.GetRandSeed() || input.MaxRound != 40 || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 1 || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
			t.Fatal("P12 original typed18 complete frozen projection differs")
		}
		return true
	}
	t.Fatal("P12 fixture attempted another input family")
	return false
}

func TestP12WorldBossPartyNativeSourceBinding(t *testing.T) {
	// Verify actual P5 provenance before admitting this finite diagnostic source.
	p12WorldBossPartyNativeRead(t, "analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.actual-profile-source.json", "89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b")
	p12WorldBossPartyNativeRead(t, "analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.frozen-input.json", "d0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94")
	rows := p12WorldBossPartyNativeCases(t)
	for index, row := range rows {
		if row.Seed != []int64{7, 314159}[index] {
			t.Fatal("prepublished finite original seeds changed")
		}
		p12WorldBossPartyNativeTypedInput(t, row)
	}
}

func TestP12WorldBossPartyNativeAllLayers(t *testing.T) {
	p12WorldBossPartyRunNativeAllLayers(t, p12WorldBossPartyNativeFamilyDefinition(false))
}

func TestP12WorldBossBoostNativeSourceBinding(t *testing.T) {
	for _, row := range p12WorldBossPartyNativeFamilyCases(t, p12WorldBossPartyNativeFamilyDefinition(true)) {
		p12WorldBossPartyNativeTypedInput(t, row)
	}
}

func TestP12WorldBossBoostNativeAllLayers(t *testing.T) {
	p12WorldBossPartyRunNativeAllLayers(t, p12WorldBossPartyNativeFamilyDefinition(true))
}

func p12WorldBossPartyRunNativeAllLayers(t *testing.T, family p12WorldBossPartyNativeFamily) {
	bundle := p12WorldBossPartyNativeLoadFamilyBundle(t, family)
	if bundle == nil {
		t.Skip("PENDING: independent two-case full-four-body typed18 native capture and SHA-bound diagnostic bundle")
	}
	TestP12WorldBossPartyNativeSourceBinding(t)
	for kind, fixture := range bundle.Fixtures {
		fixtureRaw := p12WorldBossPartyNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
		var tree any
		if json.Unmarshal(fixtureRaw, &tree) != nil {
			t.Fatal("P12 derived fixture JSON", kind)
		}
		allowed := map[string]bool{}
		for _, row := range p12WorldBossPartyNativeFamilyCases(t, family) {
			allowed[row.WireSHA] = true
		}
		seenWires := map[string]bool{}
		var walk func(any)
		walk = func(node any) {
			switch value := node.(type) {
			case map[string]any:
				if hash, ok := value["wireSHA256"].(string); ok {
					if !allowed[hash] {
						t.Fatal("P12 comparison layer attempted another wire family", kind)
					}
					seenWires[hash] = true
				}
				for _, child := range value {
					walk(child)
				}
			case []any:
				for _, child := range value {
					walk(child)
				}
			}
		}
		walk(tree)
		if len(seenWires) != family.Count {
			t.Fatal("P12 every comparison layer must retain both diagnostic sources", kind)
		}
		if kind == "records" {
			var records struct {
				Cases []struct {
					WireSHA string                     `json:"wireSHA256"`
					Actors  []model.CombatFinalActorHP `json:"originalFinalActorHP"`
				} `json:"cases"`
			}
			raw := p12WorldBossPartyNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			if json.Unmarshal(raw, &records) != nil || len(records.Cases) != family.Count {
				t.Fatal("P12 native complete terminal actor cases required")
			}
			seen := map[string]bool{}
			for _, record := range records.Cases {
				for _, row := range p12WorldBossPartyNativeFamilyCases(t, family) {
					if record.WireSHA != row.WireSHA {
						continue
					}
					wire := p12WorldBossPartyNativeTypedInput(t, row)
					if seen[record.WireSHA] || len(record.Actors) != 6 {
						t.Fatal("P12 native six-actor terminal denominator")
					}
					seen[record.WireSHA] = true
					for index, actor := range append(append([]*pb.BattleHero{}, wire.Attacker.Formation...), wire.Defender2.Formation...) {
						final := record.Actors[index]
						camp := int32(1)
						if index < 2 {
							camp = 0
						}
						if final.Camp != camp || final.HeroID != actor.GetId() || final.UID == "" {
							t.Fatal("P12 full native terminal actor identity/order")
						}
					}
				}
			}
			if len(seen) != family.Count {
				t.Fatal("P12 records cannot borrow another native wire")
			}
		}
	}
	core, hits, manager, records := bundle.Fixtures["core"], bundle.Fixtures["hits"], bundle.Fixtures["manager"], bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != family.Count {
		t.Fatal("P12 SHA-bound two-case denominator hook is missing")
	}
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, core.Path, core.SHA, family.Count) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, core.Path, core.SHA, family.Count) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, core.Path, core.SHA, family.Count) })
	t.Run("Buffmetadata", func(t *testing.T) {
		p6NativeBuffSourceAndValuesFromFixture(t, "", true, core.Path, core.SHA, family.Count)
	})
	t.Run("manager", func(t *testing.T) { p6NativeOriginalManagerFixture(t, manager.Path, manager.SHA, 1, family.Count) })
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, hits.Path, hits.SHA, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, hits.Path, hits.SHA) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, hits.Path, hits.SHA) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, hits.Path, hits.SHA) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, hits.Path, hits.SHA, 1)
		p6OriginalBuffHPFixture(t, hits.Path, hits.SHA, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, hits.Path, hits.SHA) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, hits.Path, hits.SHA) })
	// Require actual camp1 container entries and the complete six-actor terminal HP.
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) {
		p6NativeOriginalProtocolRecordFixture(t, records.Path, records.SHA, true, family.Count)
	})
}
