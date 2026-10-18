package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestP13UnionFightBattleFrozenOriginalRegistrationTablesHotRuntimeMutationExactVersionAndReceipt(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := time.UnixMilli(clock.Load())
	id := ids[0]
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	// 此处验证真实报名冻结记录与P6 Capture。原场当前战斗身份已经由117/118独立域验证。
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("roster invalid")
	}
	var attacker, defender *pb.BattlePlayer
	guild := p13GuildID(t, f, id, now)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, row := range side {
			b := &pb.BattlePlayer{}
			if proto.Unmarshal(row.BattlePayload, b) != nil {
				t.Fatal("actual frozen roster body invalid")
			}
			if row.PlayerID == id {
				attacker = b
			}
			if row.GuildID != guild && defender == nil {
				defender = b
			}
		}
	}
	if attacker == nil || defender == nil {
		t.Fatal("actual opposing registration missing")
	}
	originalHero, _ := json.Marshal(original.Tables["Hero"])
	for _, row := range f.s.Tables["Hero"] {
		if row["id"] == attacker.GetFormation()[0].GetId() {
			row["hpBase"] = "999999"
		}
	}
	mutated, _ := json.Marshal(f.s.Tables["Hero"])
	if bytes.Equal(originalHero, mutated) {
		t.Fatal("test did not actually mutate runtime Hero table")
	}
	applied := map[string][]string{attacker.GetFormation()[0].GetGuid(): {"actual roster input"}}
	frozenEngine, err := f.s.p13UFBattleAuditFrozenEngine(db.GetIns(), match, applied)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := json.Marshal(frozenEngine.Tables["Hero"]); !bytes.Equal(got, originalHero) {
		t.Fatal("runtime hot table replaced entry")
	}
	input := model.FrozenBattleInput{Attacker: attacker, Defender: defender, BattleType: 14, MaxRound: 40, Seed: 42, ClientVersion: match.ClientVersion, AppliedProperties: applied, AttackerDamageFactor: proto.Float64(0), DefenderDamageFactor: proto.Float64(1)}
	var recordID string
	err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		recordID, err = frozenEngine.CaptureBattleReplay(f.ctx, tx, id, constUtil.P13UnionFightBattleAuditP6Source, "frozen-tables-test", input)
		if err != nil {
			return err
		}
		_, err = frozenEngine.ComputeBattleReplay(f.ctx, tx, id, constUtil.P13UnionFightBattleAuditP6Source, "frozen-tables-test", nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", recordID).Error; err != nil {
		t.Fatal(err)
	}
	saved, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Tables["Hero"], original.Tables["Hero"]) || saved.ClientVersion != match.ClientVersion || saved.EngineVersion != match.EngineVersion || saved.Attacker.GetFormation()[0].GetProp().GetHp() == 999999 || record.Authoritative {
		t.Fatal("actual Capture leaked runtime or version", saved.ConfigurationHash)
	}
	if len(saved.AppliedProperties[attacker.GetFormation()[0].GetGuid()]) < 2 || !strings.Contains(saved.AppliedProperties[attacker.GetFormation()[0].GetGuid()][1], match.ConfigurationHash) {
		t.Fatal("match table SHA missing perhero provenance")
	}
	changed := *match
	changed.EngineVersion = "previous-engine"
	if _, err = f.s.p13UFBattleAuditFrozenEngine(db.GetIns(), &changed, map[string][]string{}); err == nil {
		t.Fatal("old match mislabeled as currentengine")
	}
	changed = *match
	changed.ClientVersion = "different-client"
	if _, err = f.s.p13UFBattleAuditFrozenEngine(db.GetIns(), &changed, map[string][]string{}); err == nil {
		t.Fatal("changed client supplied originaldefault")
	}
	clock.Store(first + 100000)
	var previous db.P6BattleRecord
	db.GetIns().First(&previous, "id=?", recordID)
	err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		_, err := frozenEngine.ComputeBattleReplay(f.ctx, tx, id, constUtil.P13UnionFightBattleAuditP6Source, "frozen-tables-test", nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&record, "id=?", recordID)
	if !bytes.Equal(previous.Input, record.Input) || previous.OutcomeHash != record.OutcomeHash || previous.ReplayHash != record.ReplayHash {
		t.Fatal("retry changed actual computed frozen tables")
	}
}

// 实际原HTTP的GuardAttempt只读核对：计算核心表SHA严格属于原报名快照。
func TestP13UnionFightBattleFrozenNativeRead(t *testing.T) {
	raw := os.Getenv("HYJS_P13_FROZEN_NATIVE_PLAYER")
	if raw == "" {
		t.Skip("显式专库冻结表读取")
	}
	database := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(database, "hyjs_test_p13_") {
		t.Fatal("frozen native schema guard")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		t.Fatal("actual HTTP owner required")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: database}, s: &GameService{}}
	f.open(t)
	defer func() { sql, _ := db.GetIns().DB(); sql.Close() }()
	var attempt db.P13UnionFightGuardAuditAttempt
	if err = db.GetIns().First(&attempt, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if err = p13UFGuardAuditVerifyRecord(db.GetIns(), &attempt); err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	var match db.P13UnionFightMatch
	if err = db.GetIns().First(&match, "id=?", attempt.MatchID).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(&match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	expected, err := original.FreezeBattleInput(*input)
	if err != nil {
		t.Fatal(err)
	}
	if expected.ConfigurationHash != input.ConfigurationHash || input.EngineVersion != match.EngineVersion || input.ClientVersion != match.ClientVersion || !reflect.DeepEqual(expected.Tables, input.Tables) || record.Authoritative {
		t.Fatal("actual request P6 core tables differ from registration")
	}
	for _, sources := range input.AppliedProperties {
		found := false
		for _, source := range sources {
			if strings.Contains(source, match.ConfigurationHash) && strings.Contains(source, constUtil.P13UnionFightBattleFrozenSource) {
				found = true
			}
		}
		if !found {
			t.Fatal("actual input missing matchtables peractor source")
		}
	}
	fmt.Println("GUILD_FROZEN_NATIVE_VERIFIED " + match.ConfigurationHash + "/" + input.ConfigurationHash)
}
