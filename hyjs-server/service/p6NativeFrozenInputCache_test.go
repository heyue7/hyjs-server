package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Test-only native comparators share immutable frozen tables. Every returned
// participant/metadata and every actual simulation remain independently cloned.
var p6NativeFrozenInputCache struct {
	sync.Mutex
	source        string
	tables        map[string][]map[string]string
	configuration string
	rows          map[string]*model.FrozenBattleInput
	stats         map[string]*p6NativeFrozenInputCacheStats
	bypasses      int
}

type p6NativeFrozenInputCacheStats struct {
	Freezes          int    `json:"freezes"`
	Hits             int    `json:"hits"`
	WireSHA          string `json:"wireSHA256"`
	MetadataSHA      string `json:"metadataSHA256"`
	ConfigurationSHA string `json:"configurationSHA256"`
	SourceSHA        string `json:"sourceSHA256"`
}

func p6NativeGoldenFreeze(t *testing.T, s *GameService, input model.FrozenBattleInput, wireSHA string) (*model.FrozenBattleInput, error) {
	t.Helper()
	if path := os.Getenv("HYJS_P6_NATIVE_FREEZE_CACHE_REPORT"); path != "" {
		defer p6NativeFrozenInputCacheReport(path)
	}
	if os.Getenv("HYJS_P6_NATIVE_FREEZE_UNCACHED") == "1" || len(wireSHA) != 64 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || len(input.WinConditions) > 0 || input.PVECompatibilityRule != "" || input.Tables != nil {
		p6NativeFrozenInputCache.Lock()
		p6NativeFrozenInputCache.bypasses++
		p6NativeFrozenInputCache.Unlock()
		return s.FreezeBattleInput(input)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return s.FreezeBattleInput(input)
	}
	attacker, err := proto.Marshal(input.Attacker)
	if err != nil {
		return s.FreezeBattleInput(input)
	}
	defender, err := proto.Marshal(input.Defender)
	if err != nil {
		return s.FreezeBattleInput(input)
	}
	metadataSHA := p6Digest(append(encoded, []byte("/"+p6Digest(attacker)+"/"+p6Digest(defender))...))
	actualTables := map[string][]map[string]string{}
	for _, name := range []string{"Hero", "Skill", "SkillCast", "Buff", "Talent", "GameConfigure", "CopyEnemy", "BuffInfExtend", "HeroAwakenAdd", "BootHero", "RecruitShowHero"} {
		if len(s.Tables[name]) > 0 {
			actualTables[name] = s.Tables[name]
		}
	}
	p6NativeFrozenInputCache.Lock()
	defer p6NativeFrozenInputCache.Unlock()
	if p6NativeFrozenInputCache.source == "" {
		p6NativeFrozenInputCache.source = p6NativeGoldenCacheSource(t)
		p6NativeFrozenInputCache.rows = map[string]*model.FrozenBattleInput{}
		p6NativeFrozenInputCache.stats = map[string]*p6NativeFrozenInputCacheStats{}
	}
	// Read every actual table row/value on each lookup. A live table change or
	// accidental returned-table mutation restores a freshly validated pool.
	tablesEqual := reflect.DeepEqual(actualTables, p6NativeFrozenInputCache.tables)
	key := wireSHA + "/" + metadataSHA + "/" + p6NativeFrozenInputCache.configuration + "/" + p6NativeFrozenInputCache.source + "/" + constUtil.P6EngineVersion
	cached := p6NativeFrozenInputCache.rows[key]
	if !tablesEqual || cached == nil {
		frozen, err := s.FreezeBattleInput(input)
		if err != nil {
			return nil, err
		}
		if !tablesEqual {
			p6NativeFrozenInputCache.tables = frozen.Tables
			p6NativeFrozenInputCache.configuration = frozen.ConfigurationHash
			p6NativeFrozenInputCache.rows = map[string]*model.FrozenBattleInput{}
		}
		key = wireSHA + "/" + metadataSHA + "/" + p6NativeFrozenInputCache.configuration + "/" + p6NativeFrozenInputCache.source + "/" + constUtil.P6EngineVersion
		frozen.Tables = nil
		p6NativeFrozenInputCache.rows[key] = frozen
		cached = frozen
		if p6NativeFrozenInputCache.stats[key] == nil {
			p6NativeFrozenInputCache.stats[key] = &p6NativeFrozenInputCacheStats{WireSHA: wireSHA, MetadataSHA: metadataSHA, ConfigurationSHA: p6NativeFrozenInputCache.configuration, SourceSHA: p6NativeFrozenInputCache.source}
		}
		p6NativeFrozenInputCache.stats[key].Freezes++
	} else {
		p6NativeFrozenInputCache.stats[key].Hits++
	}
	result, err := cloneP6CalculationInput(cached)
	if err != nil {
		return nil, err
	}
	result.Tables = p6NativeFrozenInputCache.tables
	return result, nil
}
func p6NativeFrozenInputCacheReport(path string) {
	p6NativeFrozenInputCache.Lock()
	defer p6NativeFrozenInputCache.Unlock()
	rows := make([]p6NativeFrozenInputCacheStats, 0, len(p6NativeFrozenInputCache.stats))
	freezes, hits := 0, 0
	for _, row := range p6NativeFrozenInputCache.stats {
		rows = append(rows, *row)
		freezes += row.Freezes
		hits += row.Hits
	}
	report := map[string]any{"scope": "test-only native comparator input; immutable table pool with full actual row equality, independent protobuf/metadata and actual simulation clones", "identities": len(rows), "freezes": freezes, "hits": hits, "bypasses": p6NativeFrozenInputCache.bypasses, "rows": rows}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}
	if path == "" || !strings.HasSuffix(path, ".json") {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, append(raw, '\n'), 0644) == nil {
		os.Rename(tmp, path)
	}
}

func TestP6NativeFrozenInputCacheFreshIdentityAndIsolation(t *testing.T) {
	t.Setenv("HYJS_P6_NATIVE_FREEZE_UNCACHED", "")
	wire, err := readTestAnalysis75("../../analysis/p6/native-next-real-inputs-v64/000.pb")
	if err != nil {
		t.Fatal(err)
	}
	wireSHA := p6Digest(wire)
	if wireSHA != "da735f93de5ab4aa152784a14a8135effc1f56b8bd1fa59ce1aebb9567d4f9ea" {
		t.Fatal("native frozen cache actual PB SHA")
	}
	playback := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(wire, playback); err != nil {
		t.Fatal(err)
	}
	input := model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}
	s := p6OriginalTables(t)
	verify := func(input model.FrozenBattleInput) *model.FrozenBattleInput {
		t.Helper()
		fresh, err := s.FreezeBattleInput(input)
		if err != nil {
			t.Fatal(err)
		}
		cached, err := p6NativeGoldenFreeze(t, s, input, wireSHA)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(fresh)
		b, _ := json.Marshal(cached)
		if !bytes.Equal(a, b) {
			t.Fatal("frozen input cache differs from actual FreezeBattleInput")
		}
		return cached
	}
	first := verify(input)
	first.Attacker.Formation[0].Prop.Att = proto.Float64(first.Attacker.Formation[0].GetProp().GetAtt() + 77)
	first.AppliedProperties["changed"] = []string{"changed"}
	second := verify(input)
	if first.Attacker.Formation[0].GetProp().GetAtt() == second.Attacker.Formation[0].GetProp().GetAtt() || len(second.AppliedProperties["changed"]) > 0 {
		t.Fatal("frozen participant/metadata alias")
	}
	variant := input
	variant.Seed++
	verify(variant)
	variant = input
	variant.DefenderFirst = !variant.DefenderFirst
	_, freshError := s.FreezeBattleInput(variant)
	_, cachedError := p6NativeGoldenFreeze(t, s, variant, wireSHA)
	if freshError == nil || cachedError == nil || freshError.Error() != cachedError.Error() {
		t.Fatal("invalid first order bypassed original guard", freshError, cachedError)
	}
	s.Tables["Buff"][0]["Args1"] = "123"
	changed := verify(input)
	if changed.ConfigurationHash == second.ConfigurationHash {
		t.Fatal("actual hot table edit silently reused original input")
	}
	// Read-only table reuse never aliases actual service rows. Corruption is
	// detected by the next full equality check and restores the validated source.
	changed.Tables["Buff"][0]["Args1"] = "corrupted"
	repaired := verify(input)
	if s.Tables["Buff"][0]["Args1"] != "123" || repaired.Tables["Buff"][0]["Args1"] != "123" {
		t.Fatal("table pool mutation reached actual source")
	}
	invalid := input
	invalid.MaxRound = 41
	if _, err := p6NativeGoldenFreeze(t, s, invalid, wireSHA); err == nil {
		t.Fatal("invalid native input bypassed original Freeze validation")
	}
}
