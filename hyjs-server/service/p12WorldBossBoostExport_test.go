package service

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WorldBossBoostOriginalTable(t *testing.T, name string) ([]map[string]string, string) {
	t.Helper()
	path := "../../analysis/tables/Table" + name + ".tsv"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 6 {
		t.Fatal("original world-boss table", name, err)
	}
	var output []map[string]string
	for _, row := range rows[5:] {
		if len(row) == 0 || row[0] == "" {
			continue
		}
		entry := map[string]string{}
		for index, value := range row {
			if index < len(rows[1]) {
				entry[rows[1][index]] = value
			}
		}
		output = append(output, entry)
	}
	return output, p6Digest(raw)
}

// TestP12WorldBossExportBoostInputs uses the unchanged real party and the actual
// production effect helper. Rows are diagnostic values, never purchases or DB
// ledger entries. No battle, SQL, wallet, native execution or business Start runs.
func TestP12WorldBossExportBoostInputs(t *testing.T) {
	directory := os.Getenv("HYJS_P12_WORLDBOSS_BOOST_EXPORT_DIR")
	if directory == "" {
		t.Skip("explicit unpaid original-effect diagnostic export disabled")
	}
	analysis, err := filepath.Abs("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	output, err := filepath.Abs(directory)
	if err != nil || !strings.HasPrefix(output, analysis+string(os.PathSeparator)) {
		t.Fatal("new owned analysis/p6 diagnostic directory required")
	}
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("preserve existing diagnostic evidence", err)
	}
	// The base has its own two-case source and native capture. Do not recapture
	// it or the old solo047 family when preparing incremental boost evidence.
	masks := []int{1, 2, 4, 8, 15}
	if value := os.Getenv("HYJS_P12_WORLDBOSS_BOOST_MASKS"); value != "" {
		masks = nil
		seen := map[int]bool{}
		for _, text := range strings.Split(value, ",") {
			mask, parseErr := strconv.Atoi(text)
			if parseErr != nil || mask < 1 || mask > 15 || seen[mask] {
				t.Fatal("unique nonempty original boost masks1..15 required")
			}
			seen[mask] = true
			masks = append(masks, mask)
		}
		sort.Ints(masks)
	}
	TestP12WorldBossPartyNativeSourceBinding(t)
	baseRow := p12WorldBossPartyNativeCases(t)[0]
	base := p12WorldBossPartyNativeTypedInput(t, baseRow)
	var baseStart pb.SCWorldBossFight
	if proto.Unmarshal(p12WorldBossPartyNativeRead(t, "analysis/p6/worldboss18-actual-party001003-pending-20261016/"+baseRow.Typed, baseRow.TypedSHA), &baseStart) != nil || baseStart.GetFightScene() == "" {
		t.Fatal("original typed18 scene source")
	}
	s := p6OriginalTables(t)
	def := &p12WorldBossDefinition{Tables: map[string][]map[string]string{}}
	tableSHAs := map[string]string{}
	for _, name := range []string{"WorldBoss", "WorldBossFactor", "WorldBossPropadd"} {
		rows, hash := p12WorldBossBoostOriginalTable(t, name)
		def.Tables[name] = rows
		tableSHAs[name] = hash
	}
	definitions := def.Tables["WorldBossPropadd"]
	sort.Slice(definitions, func(i, j int) bool { return definitions[i]["id"] < definitions[j]["id"] })
	if len(definitions) != 4 {
		t.Fatal("four original world-boss properties required")
	}
	monster, _, err := s.p12WorldBossBattleMonster(def, 1)
	if err != nil || !proto.Equal(monster, base.Defender2) {
		t.Fatal("production original Lv1 four-body builder differs from independent source", err)
	}
	write := func(name string, raw []byte) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(output, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
		return p6Digest(raw)
	}
	if err = os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	var cases []map[string]any
	for _, mask := range masks {
		var rows []db.P12WorldBossBoost
		for index, definition := range definitions {
			if mask&(1<<index) == 0 {
				continue
			}
			kind, parseErr := strconv.ParseInt(definition["type"], 10, 64)
			if parseErr != nil || definition["id"] != fmt.Sprintf("wordboss_%d", index+1) || definition["cost"] != "diamond,150" {
				t.Fatal("exact original property definition", parseErr)
			}
			// Paid remains0 and nothing is persisted. Only original property rows
			// reach the same production helper used after authentic purchases.
			rows = append(rows, db.P12WorldBossBoost{PropertyID: definition["id"], Kind: kind, Property: definition["prop"]})
		}
		for _, seed := range []int64{7, 314159} {
			party := proto.Clone(base.Attacker).(*pb.BattlePlayer)
			ids, applyErr := s.applyP12WorldBossBoostRows(party, rows)
			if applyErr != nil {
				t.Fatal(applyErr)
			}
			frozen, freezeErr := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: party, Monster: monster, Seed: seed, MaxRound: 40, BattleType: 18, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{"diagnosticUnpaidOriginalBoostEffects": ids}})
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			fightID := fmt.Sprintf("diagnostic-worldboss18-party-boost%02d-seed%d", mask, seed)
			typed := &pb.SCWorldBossFight{FightId: proto.String(fightID), RandSeed: proto.Int64(seed), BattlePlayer: party, BattleMonsterList: []*pb.BattleMonster{monster}, FightScene: proto.String(baseStart.GetFightScene()), Maxround: proto.Int32(40)}
			wire := &pb.SCFightPlayback{Fightid: proto.String(fightID), RandSeed: proto.Int64(seed), BattleType: proto.Int32(18), MaxRound: proto.Int32(40), Attacker: party, Defender2: monster, AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
			stem := fmt.Sprintf("%03d", len(cases))
			encoded := func(value proto.Message) []byte {
				t.Helper()
				raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return raw
			}
			input, marshalErr := json.MarshalIndent(frozen, "", "  ")
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			actorSHAs := []string{}
			for _, actor := range party.Formation {
				actorSHAs = append(actorSHAs, p6Digest(encoded(actor)))
			}
			cases = append(cases, map[string]any{"case": fightID, "fightId": fightID, "battleType": 18, "seed": seed, "nativeScope": "original-worldboss-automatic-ai", "wire": stem + ".pb", "wireSHA256": write(stem+".pb", encoded(wire)), "typedCallback": stem + ".SCWorldBossFight.pb", "typedCallbackSHA256": write(stem+".SCWorldBossFight.pb", encoded(typed)), "frozenInput": stem + ".frozen-input.json", "frozenInputSHA256": write(stem+".frozen-input.json", append(input, '\n')), "sourceActorSHA256": actorSHAs, "sourceBossMemberSHA256": baseRow.MemberSHAs, "fullBossMembers": 4, "bossLevel": 1, "boostMask": mask, "diagnosticBoostIDs": ids, "actualPaid": false, "actualBusinessStart": false, "status": "awaiting-original-automatic-ai"})
		}
	}
	helper, err := os.ReadFile("p12WorldBossBattleService.go")
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"schema": "p12-worldboss-original-boost-effect-input-v1", "sourceKind": "finite-original-boost-effect-helper-diagnostic", "nativeScope": "original-worldboss-automatic-ai", "actorSource": "analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.frozen-input.json", "actorSourceSHA256": "d0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94", "baseTyped18Manifest": "analysis/p6/worldboss18-actual-party001003-pending-20261016/manifest.json", "baseTyped18ManifestSHA256": "78a7d0d3d0bd7bc4585bf101627f927050237f37643fb01b847a0fa196811d47", "effectHelper": "GameService.applyP12WorldBossBoostRows", "effectHelperFileSHA256": p6Digest(helper), "originalTableSHA256": tableSHAs, "boostMasks": masks, "cases": cases, "actualPaid": false, "purchaseLedgerCreated": false, "actualBusinessStart": false, "nativeCompared": false, "authoritative": false, "engineExecuted": false, "finitePrerequisites": "Same complete historical actual001003 actors and original Lv1 four-body boss; original configured rows applied only by production helper, no duplicated mutation implementation. No wallet, SQL, purchase transaction, accepted Start, expected result or native execution. Seeds7/314159 are prepublished diagnostic context. Missing boost subsets are not automatically certified."}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write("manifest.json", append(raw, '\n'))
	t.Logf("unpaid original boost-effect source exported: cases=%d masks=%v path=%s", len(cases), masks, output)
}
