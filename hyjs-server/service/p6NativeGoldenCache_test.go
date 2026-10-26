package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"hyjs-server/model"
)

// Cache access is exclusive to strict original numerical fixtures. Production,
// supplied operation replays, mechanism probes and mutable-table tests keep their
// original calculation path. Each caller receives independent outcome/tick copies.
var p6NativeGoldenCache struct {
	sync.Mutex
	rows     map[string]*p6NativeGoldenCachedRun
	bindings map[string]string
	tables   map[string]map[string][]map[string]string
	clocks   map[string]map[string]bool
	source   string
	bypasses map[string]int
	rowPool  map[string]map[string]string
}

type p6NativeGoldenCachedRun struct {
	outcome      *model.CombatOutcome
	observed     bool
	calculations int
	hits         int
	ticks        []*combatRoundEngine
}

func p6NativeGoldenCacheSource(t *testing.T) string {
	t.Helper()
	files := []string{"../constUtil/p6Combat.go", "../model/p6Combat.go"}
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, "_test.go") && (strings.HasPrefix(p, "combat") || strings.HasPrefix(p, "p6")) {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	body := []byte{}
	for _, p := range files {
		raw, err := readTestAnalysis75(p)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, []byte(p+"/"+p6Digest(raw)+"\n")...)
	}
	return p6Digest(body)
}
func p6NativeGoldenCacheClocks(t *testing.T) map[string]map[string]bool {
	t.Helper()
	fixtures := p6NativeGoldenClockFixtures()
	clocks := map[string]map[string]bool{}
	for _, fixture := range fixtures {
		raw, err := readTestAnalysis75("../../analysis/p6/" + fixture[0])
		if err != nil || p6Digest(raw) != fixture[1] {
			t.Fatal("cache original clock SHA", fixture[0], err)
		}
		var data struct {
			Cases []struct {
				SHA   string `json:"wireSHA256"`
				Ticks []struct {
					Round int32   `json:"round"`
					Time  float64 `json:"time"`
				} `json:"ticks"`
			} `json:"cases"`
		}
		if err = json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		for _, c := range data.Cases {
			if clocks[c.SHA] == nil {
				clocks[c.SHA] = map[string]bool{}
			}
			for _, tick := range c.Ticks {
				clocks[c.SHA][fmt.Sprintf("%d/%d", tick.Round, int64(math.Round(tick.Time*10)))] = true
			}
		}
	}
	return clocks
}
func p6NativeGoldenCompute(t *testing.T, s *GameService, input *model.FrozenBattleInput, wireSHA string, observer func(*combatRoundEngine)) (*model.CombatOutcome, error) {
	t.Helper()
	if path := os.Getenv("HYJS_P6_NATIVE_GOLDEN_CACHE_REPORT"); path != "" {
		defer p6WriteNativeGoldenCacheReport(path)
	}
	if err := t.Context().Err(); err != nil {
		return nil, err
	}
	if input == nil || os.Getenv("HYJS_P6_NATIVE_GOLDEN_UNCACHED") == "1" || input.Monster != nil || input.KFReplay != nil || len(input.WinConditions) > 0 || input.PVECompatibilityRule != "" {
		return s.computeFrozenBattleObserved(t.Context(), input, nil, observer)
	}
	p6NativeGoldenCache.Lock()
	locked := true
	defer func() {
		if locked {
			p6NativeGoldenCache.Unlock()
		}
	}()
	if p6NativeGoldenCache.rows == nil {
		p6NativeGoldenCache.rows = map[string]*p6NativeGoldenCachedRun{}
		p6NativeGoldenCache.bypasses = map[string]int{}
		p6NativeGoldenCache.bindings = map[string]string{}
		p6NativeGoldenCache.tables = map[string]map[string][]map[string]string{}
		p6NativeGoldenCache.rowPool = map[string]map[string]string{}
		p6NativeGoldenCache.source = p6NativeGoldenCacheSource(t)
	}
	// A SHA-addressed immutable table copy avoids encoding the same large table
	// collection on every projection. Equality still reads every actual row/value.
	originalTables := p6NativeGoldenCache.tables[input.ConfigurationHash]
	if originalTables == nil || !reflect.DeepEqual(originalTables, input.Tables) {
		tableJSON, err := json.Marshal(input.Tables)
		if err != nil || p6Digest(tableJSON) != input.ConfigurationHash {
			p6NativeGoldenCache.bypasses["table hash changed"]++
			p6NativeGoldenCache.Unlock()
			locked = false
			return s.computeFrozenBattleObserved(t.Context(), input, nil, observer)
		}
		var copied map[string][]map[string]string
		if err = json.Unmarshal(tableJSON, &copied); err != nil {
			return nil, err
		}
		p6NativeGoldenCache.tables[input.ConfigurationHash] = copied
	}
	metadata := *input
	metadata.Tables = nil
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	metadataSHA := p6Digest(encoded)
	if previous, known := p6NativeGoldenCache.bindings[wireSHA]; known && previous != metadataSHA {
		p6NativeGoldenCache.bypasses["wire metadata variant"]++
		p6NativeGoldenCache.Unlock()
		locked = false
		return s.computeFrozenBattleObserved(t.Context(), input, nil, observer)
	}
	p6NativeGoldenCache.bindings[wireSHA] = metadataSHA
	var clocks map[string]bool
	if observer != nil {
		if p6NativeGoldenCache.clocks == nil {
			p6NativeGoldenCache.clocks = p6NativeGoldenCacheClocks(t)
		}
		var known bool
		clocks, known = p6NativeGoldenCache.clocks[wireSHA]
		if !known {
			p6NativeGoldenCache.bypasses["uncatalogued observation"]++
			p6NativeGoldenCache.Unlock()
			locked = false
			return s.computeFrozenBattleObserved(t.Context(), input, nil, observer)
		}
	}
	key := wireSHA + "/" + metadataSHA + "/" + p6NativeGoldenCache.source
	run := p6NativeGoldenCache.rows[key]
	if run == nil || observer != nil && !run.observed {
		previous := run
		run = &p6NativeGoldenCachedRun{observed: observer != nil, calculations: 1}
		if previous != nil {
			run.calculations = previous.calculations + 1
			run.hits = previous.hits
		}
		var capture func(*combatRoundEngine)
		if observer != nil {
			capture = func(e *combatRoundEngine) {
				clock := fmt.Sprintf("%d/%d", e.round, int64(math.Round(e.time*10)))
				if clocks[clock] || len(e.diagnosticCasts) > 0 {
					run.ticks = append(run.ticks, p6CloneNativeGoldenTick(e, true))
				}
			}
		}
		outcome, computeErr := s.computeFrozenBattleObserved(t.Context(), input, nil, capture)
		if computeErr != nil {
			return nil, computeErr
		}
		if outcome.Authoritative {
			t.Fatal("native fixture cache cannot certify authority")
		}
		if previous != nil {
			before, _ := json.Marshal(previous.outcome)
			after, _ := json.Marshal(outcome)
			if !bytes.Equal(before, after) {
				t.Fatal("tick observation changed actual immutable outcome")
			}
		}
		run.outcome = p6CloneNativeGoldenOutcome(outcome)
		p6NativeGoldenCache.rows[key] = run
	} else {
		run.hits++
	}
	p6NativeGoldenCache.Unlock()
	locked = false
	if observer != nil {
		for _, tick := range run.ticks {
			observer(p6CloneNativeGoldenTick(tick))
		}
	}
	return p6CloneNativeGoldenOutcome(run.outcome), nil
}
func p6CloneNativeGoldenOutcome(out *model.CombatOutcome) *model.CombatOutcome {
	result := *out
	if out.Operates != nil {
		result.Operates = proto.Clone(out.Operates).(*pb.BattleOperate)
	}
	if out.Attacker != nil {
		result.Attacker = proto.Clone(out.Attacker).(*pb.BattlePlayer)
	}
	if out.Defender != nil {
		result.Defender = proto.Clone(out.Defender).(*pb.BattlePlayer)
	}
	if out.Monster != nil {
		result.Monster = proto.Clone(out.Monster).(*pb.BattleMonster)
	}
	result.Events = append([]model.CombatEvent(nil), out.Events...)
	for i, event := range out.Events {
		if event.OriginalBuffParentSnapshot != nil {
			value := *event.OriginalBuffParentSnapshot
			result.Events[i].OriginalBuffParentSnapshot = &value
		}
		if event.OriginalBuffSnapshot != nil {
			value := *event.OriginalBuffSnapshot
			result.Events[i].OriginalBuffSnapshot = &value
		}
		if event.OriginalHPValue != nil {
			value := *event.OriginalHPValue
			result.Events[i].OriginalHPValue = &value
		}
		if event.OriginalHPType != nil {
			value := *event.OriginalHPType
			result.Events[i].OriginalHPType = &value
		}
	}
	return &result
}

// Tick snapshots contain the read-only logic fields consumed by all original
// golden projections; maps, Buff identities, source links and skills are copied.
func p6CloneNativeGoldenTick(e *combatRoundEngine, storage ...bool) *combatRoundEngine {
	random := *e.rng
	result := &combatRoundEngine{round: e.round, time: e.time, camp: e.camp, rng: &random}
	actors := map[*combatRoundActor]*combatRoundActor{}
	buffs := map[*combatRoundBuff]*combatRoundBuff{}
	copyRow := func(row map[string]string) map[string]string {
		if len(storage) > 0 && storage[0] {
			key := e.input.ConfigurationHash + "/" + row["Id"]
			if existing := p6NativeGoldenCache.rowPool[key]; existing != nil {
				if reflect.DeepEqual(existing, row) {
					return existing
				}
				body, _ := json.Marshal(row)
				key += "/" + p6Digest(body)
				if existing = p6NativeGoldenCache.rowPool[key]; existing != nil {
					return existing
				}
			}
			cloned := map[string]string{}
			for k, v := range row {
				cloned[k] = v
			}
			p6NativeGoldenCache.rowPool[key] = cloned
			return cloned
		}

		out := map[string]string{}
		for k, v := range row {
			out[k] = v
		}
		return out
	}
	for camp, p := range e.players {
		result.players[camp] = &combatRoundPlayer{anger: p.anger, combo: p.combo, lastCombo: p.lastCombo}
		for _, a := range p.actors {
			b := *a
			b.hero = nil
			b.current = nil
			b.waiting = nil
			b.shareActors = nil
			b.talents = nil
			b.deathSource = nil
			b.immuneBuff = nil
			b.immuneTalent = nil
			b.base = map[string]float64{}
			for k, v := range a.base {
				b.base[k] = v
			}
			b.originalPercent = map[string]float64{}
			for k, v := range a.originalPercent {
				b.originalPercent[k] = v
			}
			b.originalPercentBase = map[string]float64{}
			for k, v := range a.originalPercentBase {
				b.originalPercentBase[k] = v
			}
			b.flags = map[string]bool{}
			for k, v := range a.flags {
				b.flags[k] = v
			}
			b.unableStates = map[int32]bool{}
			for k, v := range a.unableStates {
				b.unableStates[k] = v
			}
			b.skills = nil
			b.buffs = nil
			b.outgoingReduction = nil
			actors[a] = &b
			result.players[camp].actors = append(result.players[camp].actors, &b)
		}
	}
	cloneBuff := func(b *combatRoundBuff) *combatRoundBuff {
		if cached := buffs[b]; cached != nil {
			return cached
		}
		c := *b
		c.row = copyRow(b.row)
		c.source = actors[b.source]
		c.target = actors[b.target]
		buffs[b] = &c
		return &c
	}
	for original, a := range actors {
		for _, b := range original.buffs {
			a.buffs = append(a.buffs, cloneBuff(b))
		}
		for _, b := range original.outgoingReduction {
			a.outgoingReduction = append(a.outgoingReduction, cloneBuff(b))
		}
		for _, skill := range original.skills {
			a.skills = append(a.skills, &combatRoundSkill{id: skill.id, state: skill.state, cooldown: skill.cooldown})
		}
	}
	for _, cast := range e.diagnosticCasts {
		copy := map[string]any{}
		for k, v := range cast {
			copy[k] = v
		}
		result.diagnosticCasts = append(result.diagnosticCasts, copy)
	}
	return result
}

// Compare a real SHA-bound native input directly and through the cache, then
// mutate every returned container family and verify the next caller stays intact.
func TestP6NativeGoldenCacheIsolationAndRealResult(t *testing.T) {
	wire, err := readTestAnalysis75("../../analysis/p6/native-next-real-inputs-v59/000.pb")
	if err != nil {
		t.Fatal(err)
	}
	playback := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(wire, playback); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := s.computeFrozenBattleObserved(t.Context(), input, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	var firstBefore []byte
	got, err := p6NativeGoldenCompute(t, s, input, p6Digest(wire), func(e *combatRoundEngine) {
		if firstBefore != nil {
			return
		}
		a := e.players[0].actors[0]
		firstBefore, _ = json.Marshal([]any{a.uid, a.hp, a.base, a.originalPercent, a.originalPercentBase, a.flags, a.unableStates, e.rng.Draws()})
		a.uid = "modified"
		a.hp = -99
		a.base["att"] = -100
		a.originalPercent["damageAdd"] = -999
		a.originalPercentBase["damageAdd"] = -999
		a.flags["modified"] = true
		a.unableStates[99] = true
		e.rng.Value()
		for _, b := range a.buffs {
			b.row["Id"] = "modified"
			b.value = -100
			if b.source != nil {
				b.source.uid = "modified"
			}
		}
		for _, skill := range a.skills {
			skill.id = "modified"
			skill.state = -99
		}
		for _, cast := range e.diagnosticCasts {
			cast["skill"] = "modified"
		}
	})
	if err != nil || firstBefore == nil {
		t.Fatal("cache first real result", err)
	}
	raw, _ := json.Marshal(got)
	if !bytes.Equal(raw, expectedJSON) {
		t.Fatal("cached outcome differs from actual engine")
	}
	got.Events[0].Kind = "modified"
	mutatedBuffSnapshots := 0
	for i := range got.Events {
		if got.Events[i].OriginalHPValue != nil {
			*got.Events[i].OriginalHPValue = -99
		}
		if got.Events[i].OriginalHPType != nil {
			*got.Events[i].OriginalHPType = -99
		}
		if got.Events[i].OriginalBuffSnapshot != nil {
			mutatedBuffSnapshots++
			got.Events[i].OriginalBuffSnapshot.ID = "modified"
			got.Events[i].OriginalBuffSnapshot.Source = "modified"
			got.Events[i].OriginalBuffSnapshot.Args1 = -99
			got.Events[i].OriginalBuffSnapshot.DefCount = -99
			got.Events[i].OriginalBuffSnapshot.Val = -99
		}
	}
	if mutatedBuffSnapshots == 0 {
		t.Fatal("original load Buff snapshot mutation denominator")
	}
	got.Operates.SkillOpts[0].ActorUid = proto.String("modified")
	got.Attacker.Formation[0].Guid = proto.String("modified")
	hitsBefore := 0
	p6NativeGoldenCache.Lock()
	for key, row := range p6NativeGoldenCache.rows {
		if strings.HasPrefix(key, p6Digest(wire)+"/") {
			hitsBefore += row.hits
		}
	}
	p6NativeGoldenCache.Unlock()
	var firstAfter []byte
	again, err := p6NativeGoldenCompute(t, s, input, p6Digest(wire), func(e *combatRoundEngine) {
		if firstAfter == nil {
			a := e.players[0].actors[0]
			firstAfter, _ = json.Marshal([]any{a.uid, a.hp, a.base, a.originalPercent, a.originalPercentBase, a.flags, a.unableStates, e.rng.Draws()})
		}
	})
	if err != nil || !bytes.Equal(firstBefore, firstAfter) {
		t.Fatal("returned snapshot mutated cached original", err)
	}
	raw, _ = json.Marshal(again)
	if !bytes.Equal(raw, expectedJSON) {
		t.Fatal("returned outcome mutated cached original")
	}
	hitsAfter := 0
	p6NativeGoldenCache.Lock()
	for key, row := range p6NativeGoldenCache.rows {
		if strings.HasPrefix(key, p6Digest(wire)+"/") {
			hitsAfter += row.hits
		}
	}
	p6NativeGoldenCache.Unlock()
	if hitsAfter <= hitsBefore {
		t.Fatal("native cache isolation did not exercise a real warm hit")
	}
	// Validation still rejects modified table bytes; a warm cache cannot conceal them.
	variant, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	variant.Tables["GameConfigure"][0]["NameValue"] = "cache-table-variant"
	if _, err = p6NativeGoldenCompute(t, s, variant, p6Digest(wire), nil); err == nil {
		t.Fatal("warm cache accepted changed table hash")
	}
	variant, err = cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	variant.EngineVersion = "cache-source-variant"
	if _, err = p6NativeGoldenCompute(t, s, variant, p6Digest(wire), nil); err == nil {
		t.Fatal("warm cache accepted changed engine source/version")
	}
	// A seed variant must execute separately and leave the original cached row intact.
	p6NativeGoldenCache.Lock()
	beforeRows := len(p6NativeGoldenCache.rows)
	p6NativeGoldenCache.Unlock()
	variant, err = cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	variant.Seed++
	directVariant, err := s.computeFrozenBattleObserved(t.Context(), variant, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cachedVariant, err := p6NativeGoldenCompute(t, s, variant, p6Digest(wire), nil)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(directVariant)
	after, _ := json.Marshal(cachedVariant)
	p6NativeGoldenCache.Lock()
	afterRows := len(p6NativeGoldenCache.rows)
	p6NativeGoldenCache.Unlock()
	if !bytes.Equal(before, after) || beforeRows != afterRows {
		t.Fatal("seed variant reused or populated original cache")
	}

	p6NativeGoldenCache.Lock()
	runs := len(p6NativeGoldenCache.rows)
	p6NativeGoldenCache.Unlock()
	t.Logf("strict original cached calculation identities=%d", runs)
}

func p6WriteNativeGoldenCacheReport(path string) {
	p6NativeGoldenCache.Lock()
	defer p6NativeGoldenCache.Unlock()
	rows := []map[string]any{}
	calculations, hits := 0, 0
	for key, run := range p6NativeGoldenCache.rows {
		parts := strings.Split(key, "/")
		calculations += run.calculations
		hits += run.hits
		rows = append(rows, map[string]any{"originalWireSHA256": parts[0], "frozenMetadataSHA256": parts[1], "engineSourceSHA256": parts[2], "calculations": run.calculations, "hits": run.hits, "originalClockSnapshots": len(run.ticks), "events": len(run.outcome.Events)})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["originalWireSHA256"].(string) < rows[j]["originalWireSHA256"].(string)
	})
	report := map[string]any{"scope": "strict original golden tests only; every original value is still compared", "nativeCompared": false, "authoritative": false, "distinctOriginalIdentities": len(rows), "calculations": calculations, "hits": hits, "bypasses": p6NativeGoldenCache.bypasses, "tablesSHA256": []string{}, "cases": rows}
	hashes := []string{}
	for hash := range p6NativeGoldenCache.tables {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	report["tablesSHA256"] = hashes
	raw, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		temporary := path + ".tmp"
		if os.WriteFile(temporary, append(raw, '\n'), 0600) == nil {
			_ = os.Rename(temporary, path)
		}
	}
}

// Opt-in benchmark runs the same fourteen real inputs through all eight original
// numerical classes. Uncached mode changes reuse only, preserving every assertion.
func TestP6NativeGoldenAllEightClassesBenchmark(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_GOLDEN_BENCHMARK") != "1" {
		t.Skip("explicit same-scope cache benchmark disabled")
	}
	const (
		file = "native-hit-exports-golden-v59-v59batch.json"
		sha  = "c76967629ee4abb5326bd4720a803e3f195b3ba42c7a22dbe91a79840d280b52"
	)
	for _, class := range []int32{1, 2, 3, 4, 5, 6} {
		t.Run(fmt.Sprintf("cast-class%d", class), func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, class) })
	}
	for _, class := range []int32{1, 2} {
		t.Run(fmt.Sprintf("Buff-class%d", class), func(t *testing.T) { p6OriginalBuffHPFixture(t, file, sha, class) })
	}
}

func p6NativeGoldenClockFixtures() [][2]string {
	return [][2]string{
		{"native-next-real-golden-v64.json", "a767c3a3b3f9afe293131c079a6d0d7567d1ea9607504923abc154cefaae8996"},
		{"native-next-real-golden-v66a.json", "755f6ce57473743d484bf90c8de2873d2b0164f704ec1da0ed9fec4d9a9c63e7"},
		{"native-next-real-golden-v65a.json", "20efa010a55df78caed84d8dedadbf2ddac85f694195bef9ad18447db8f445c3"},
		{"native-pvp-priority-tick-golden-v48.json", "643db0054ff2e421fa34c209692a85c321a85f581ee0f81868df1c14c8333cf4"},
		{"native-damage-factor-tick-golden-v51.json", "7f081406545f3cf72b290445a6adcc33b5f49088de0c586d5035268e4d3f7735"},
		{"native-next-real-golden-v54.json", "9046936d6c5691e792305abd5554a02cc23f652da3a10db5dc1ae319c7f59fc1"},
		{"native-next-real-golden-v55.json", "bcc334e612ff9efee0c0082c5df8927b7ecf5591e8c93971e78225b0027bbc13"},
		{"native-next-real-golden-v56.json", "76b02ca8ac602664ddaf2ddb7d9d7e766e64aeeb7d5420969ecd0bf898eb586f"},
		{"native-next-real-golden-v57.json", "c0d2304f65d0891fd27cf267d42468207d9d43367f4b9dbb3bff78cfe8b9c181"},
		{"native-next-real-golden-v58.json", "8430083aa0bb0ac2aba8608d6e9a631f1515098bf88553cc83a6366aa2ec96c7"},
		{"native-next-real-golden-v59.json", "91b155a2bdb6fae3fbcfa447920f1b9851746aee76115048a55f00680884fc7f"},
		{"native-next-real-golden-v60.json", "aba778143e9b90a553bfa2c7c6d27b69ac8889a35ee213ff6510c432e1e2ae8a"},
		{"native-next-real-golden-v61.json", "dfdf85e1d825401b9b6ce843285a489bab210c1e1a3fa4f429a39d336e96b3dd"},
		{"native-next-real-golden-v62.json", "408ceb554dcaf42ac57d3afcec18f1ade52abe2c899fa9c9b7b84f69b168be56"},
		{"native-next-real-golden-v63.json", "5cd373890312aa230b5bc3deecbc5dd0ea32764f79a56e43ff8425692bf415ca"},
	}
}
