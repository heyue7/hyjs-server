package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Explicitly enabled diagnostic producer export. The fixture performs real
// enrollment/field/movement/Ask auditing in its isolated test store. No war HP,
// winner, result, skill or seed is replaced and no authority is granted.
func TestP13UnionFightGuardSourceExport(t *testing.T) {
	requested := os.Getenv("HYJS_GUILD_GUARD_SOURCE_EXPORT_ROOT")
	if requested == "" {
		t.Skip("explicit isolated guard source export only")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out, err := filepath.Abs(requested)
	if err != nil || filepath.Dir(out) != filepath.Join(repo, "analysis", "p6") || !strings.HasPrefix(filepath.Base(out), "guild-guard-inputs-") {
		t.Fatal("new analysis/p6/guild-guard-inputs-* export directory required")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("preserve existing evidence directory", err)
	}
	f, _, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if err = f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P13UnionFightGuardAuditSource{}, &db.P13UnionFightBattleAuditSource{}, &db.P13UnionFightGuardAuditOffer{}, &db.P13UnionFightGuardAuditAttempt{}, &db.P13UnionFightBattleAuditOffer{}} {
		if err = db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "map_projection": constUtil.P13UnionFightMapProjectionReFlat, "source": "diagnostic isolated prepublished local HP1000/flat-v1; no original guard stat conversion claim"}).Error; err != nil {
		t.Fatal(err)
	}
	one := float64(1)
	// Both factors are published before field creation through the existing
	// source->offer path. They are never rewritten in an exported input.
	if err = db.GetIns().Create(&db.P13UnionFightBattleAuditSource{Day: match.Day, Enabled: true, SeedSalt: 42, AttackerDamageFactor: &one, DefenderDamageFactor: &one, PublishedAt: now().UnixMilli(), Source: "diagnostic isolated prepublished factors1/1 and salt42; no private-server numeric claim"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	guardIDs := []string{}
	for _, guard := range original.Tables["UnionFightGuard"] {
		guardID := guard["UFGuardId"]
		hero := original.table("Hero", "id", guard["HeroId"])
		if hero == nil {
			t.Fatal("guard lacks actual TableHero source", guardID, guard["HeroId"])
		}
		monster := p13UFGuardAuditTemplate(original, guardID, 1000)
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&db.P13UnionFightGuardAuditSource{Day: match.Day, GuardID: guardID, Enabled: true, Monster: raw, PublishedAt: now().UnixMilli(), Source: "diagnostic existing local template: level1/blood1/pose3/TableHero base actor and original skills except HP1000; original guard scaling unknown"}).Error; err != nil {
			t.Fatal(err)
		}
		guardIDs = append(guardIDs, guardID)
	}
	sort.Strings(guardIDs)
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil || len(roster.Left) == 0 || len(roster.Right) == 0 {
		t.Fatal("actual frozen roster unavailable")
	}
	byCamp := map[int64]int64{}
	for _, id := range []int64{roster.Left[0].PlayerID, roster.Right[0].PlayerID} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
		actor, _, _, _, err := f.s.p13UFFieldCurrent(db.GetIns(), id, now())
		if err != nil {
			t.Fatal(err)
		}
		byCamp[actor.Camp] = id
	}
	var field db.P13UnionFightField
	if err = db.GetIns().First(&field, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot p13UFFieldSnapshot
	if p6Digest(field.Snapshot) != field.SnapshotHash || json.Unmarshal(field.Snapshot, &snapshot) != nil {
		t.Fatal("frozen field corrupt")
	}
	var buildings pb.SCUFBuildingRefresh
	if err = proto.Unmarshal(snapshot.Buildings, &buildings); err != nil {
		t.Fatal(err)
	}
	guards, err := p13UFGuardAuditUnits(&buildings)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, raw []byte) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
		return p6Digest(raw)
	}
	pins := map[string]string{}
	for _, relative := range []string{"hyjs-server/service/p13UnionFightGuardSourceExport_test.go", "hyjs-server/service/p13UnionFightGuardAudit_test.go", "hyjs-server/service/p13UnionFightGuardAuditService.go", "hyjs-server/service/p13UnionFightBattleAuditService.go", "hyjs-server/service/p13UnionFightBattleFrozenService.go", "hyjs-server/service/p13UnionFightFieldService.go", "hyjs-server/service/p6FinalHPService.go", "hyjs-server/service/combatRoundService.go", "analysis/tables/TableUnionFightGuard.tsv", "analysis/tables/TableHero.tsv", "analysis/tables/TableSkill.tsv", "analysis/tables/TableSkillCast.tsv", "analysis/tables/TableBuff.tsv"} {
		raw, err := readTestAnalysis75(filepath.Join(repo, relative))
		if err != nil {
			t.Fatal(err)
		}
		pins[relative] = p6Digest(raw)
	}
	cases := []map[string]any{}
	for index, guardID := range guardIDs {
		guid := p13UFFieldGUID(match.ID, "guard", guardID)
		guard := guards[guid]
		if guard == nil {
			t.Fatal("original guard not bound in real field", guardID)
		}
		id := byCamp[1-int64(guard.GetCamp())]
		if id == 0 {
			t.Fatal("actual opposing player missing", guardID)
		}
		clock.Store(first + 20000 + int64(index)*20000)
		point := proto.Clone(guard.GetInitPosition()).(*pb.PointNew)
		point.Y = proto.Float32(0)
		if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
			t.Fatal("actual movement", guardID, err)
		}
		actor, _, _, frame, err := f.s.p13UFFieldCurrent(db.GetIns(), id, now())
		if err != nil {
			t.Fatal(err)
		}
		beforeFrame := actor.FrameHash
		stamp := int64(91000 + index)
		_, askErr := f.s.p13UFAskFightAnyAudit(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(guid)}, stamp, now())
		entry := map[string]any{"guardID": guardID, "heroID": original.table("UnionFightGuard", "UFGuardId", guardID)["HeroId"], "guardGUID": guid, "playerID": id, "stamp": stamp, "authoritative": false, "warApplied": false}
		if askErr == nil || askErr.Error() != constUtil.P13UnionFightBattleAuditRefusal {
			entry["status"] = "producer-blocked"
			entry["error"] = fmt.Sprint(askErr)
			cases = append(cases, entry)
			continue
		}
		var attempt db.P13UnionFightGuardAuditAttempt
		if err = db.GetIns().First(&attempt, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error; err != nil {
			t.Fatal(err)
		}
		if attempt.SourceHash != p13UFGuardAttemptHash(&attempt) {
			t.Fatal("attempt corrupt")
		}
		if err = p13UFGuardAuditVerifyRecord(db.GetIns(), &attempt); err != nil {
			t.Fatal(err)
		}
		var record db.P6BattleRecord
		if err = db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
			t.Fatal(err)
		}
		input, err := decodeP6Record(&record)
		if err != nil || record.Authoritative || input.BattleType != 14 || input.Defender != nil || input.Monster == nil || input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 || input.DiagnosticAIDriverPolicy != "" {
			t.Fatal("actual current producer shape/factors changed", err)
		}
		canonical, err := json.Marshal(input)
		if err != nil || p6Digest(canonical) != record.InputHash {
			t.Fatal("input bytes changed", err)
		}
		var quote db.P13UnionFightGuardAuditOffer
		var common db.P13UnionFightBattleAuditOffer
		if err = db.GetIns().First(&quote, "match_id=? AND guard_guid=?", match.ID, guid).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().First(&common, "match_id=?", match.ID).Error; err != nil {
			t.Fatal(err)
		}
		if quote.SourceHash != p13UFGuardOfferHash(&quote) || quote.SourceHash != attempt.OfferHash || !p13UFBattleAuditQuoteValid(&common) || common.SourceHash != quote.BattleOfferHash {
			t.Fatal("frozen source links changed")
		}
		typed := &pb.SCUFPveFightRecordReplay{FightId: proto.String(record.SourceFight), RandomSeed: proto.Int64(input.Seed), Player: input.Attacker, Monster: input.Monster, MaxRound: proto.Int32(input.MaxRound), AttackerDamageFactor: input.AttackerDamageFactor, DefenderDamageFactor: input.DefenderDamageFactor}
		playback := &pb.SCFightPlayback{Fightid: proto.String(record.SourceFight), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(input.BattleType), Attacker: input.Attacker, Defender2: input.Monster, MaxRound: proto.Int32(input.MaxRound), AttackerFactor: input.AttackerDamageFactor, DefenderFactor: input.DefenderDamageFactor}
		prefix := fmt.Sprintf("%03d", index)
		files := map[string]string{}
		for suffix, value := range map[string]proto.Message{"SCUFPveFightRecordReplay.pb": typed, "SCFightPlayback.pb": playback, "guard-frame.pb": guard, "player-frame.pb": frame} {
			raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			files[prefix+"."+suffix] = write(prefix+"."+suffix, raw)
		}
		files[prefix+".frozen-input.json"] = write(prefix+".frozen-input.json", canonical)
		files[prefix+".field-snapshot.json"] = write(prefix+".field-snapshot.json", field.Snapshot)
		files[prefix+".go-audit-outcome.json"] = write(prefix+".go-audit-outcome.json", record.Outcome)
		entry["files"], entry["prefix"], entry["p6InputHash"], entry["attempt"], entry["quote"], entry["commonQuote"] = files, prefix, record.InputHash, attempt, quote, common
		entry["status"] = "awaiting-original-automatic-ai"
		entry["configurationHash"], entry["engineVersion"], entry["clientVersion"] = input.ConfigurationHash, input.EngineVersion, input.ClientVersion
		entry["playerFrameHash"], entry["playerBattleHash"], entry["fieldSnapshotHash"] = beforeFrame, actor.BattleHash, field.SnapshotHash
		// Exact terminal diagnostic is separate from audit/business state. Its
		// output may seed a later explicitly prospective study, never a war save.
		if terminal, diagnosticErr := original.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil); diagnosticErr == nil {
			raw, err := json.Marshal(terminal)
			if err != nil {
				t.Fatal(err)
			}
			name := prefix + ".go-prospective-terminal.json"
			files[name] = write(name, raw)
			entry["prospectiveTerminalOnly"] = true
		} else {
			entry["prospectiveTerminalError"] = diagnosticErr.Error()
		}
		var unchanged db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&unchanged, "match_id=? AND player_id=?", match.ID, id).Error; err != nil || unchanged.FrameHash != beforeFrame {
			t.Fatal("diagnostic changed war frame", err)
		}
		cases = append(cases, entry)
	}
	metadata := map[string]any{"schema": "guild-guard-producer-export-v1", "cases": cases, "sourcePins": pins, "allOriginalGuardIDs": guardIDs, "authoritative": false, "nativeCompared": false, "warApplied": false, "storeDialect": db.GetIns().Dialector.Name(), "fixture": "actual isolated signup/field/move/Ask; prepublished existing local template level1/blood1/pose3/HP1000, factors1/1; no artificial injury or winner", "typedOrigin": "lossless original callback shape projection from actual FrozenBattleInput; audit refuses and emits no successful network replay", "nativeTypedLoader": "SCFightPlayback pve-auto UF14 projection; originalCallbackFile retained independently because prepare.py typedCallback loader routes14 to Rush", "futureInjury": "prospective terminal only; no next producer input and no natural war aftermath claim"}
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write("source-export.json", append(raw, '\n'))
	fmt.Println("GUILD_GUARD_SOURCE_EXPORT " + filepath.Join(out, "source-export.json") + " guards=" + strconv.Itoa(len(cases)))
}
