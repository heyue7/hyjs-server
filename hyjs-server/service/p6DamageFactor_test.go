package service

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-sql-driver/mysql"
	"math"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalDamageFactorEnemyCampBeforeShieldAndPositiveHeal(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	e.input.AttackerDamageFactor = proto.Float64(2)
	e.input.DefenderDamageFactor = proto.Float64(.5)
	for _, row := range []map[string]string{{"Id": "factor-generic", "Factor": "shield", "Args1": "100", "TotalRound": "2"}, {"Id": "factor-physical", "Factor": "pshield", "Args1": "50", "TotalRound": "2"}} {
		if err := e.loadBuff(d, d, row, 1); err != nil {
			t.Fatal(err)
		}
	}
	e.damageHP(a, d, 100, 1)
	closeCombat(t, d.hp, 79950)
	closeCombat(t, d.shield("shield"), 0)
	closeCombat(t, d.shield("pshield"), 0)
	// Recipient.enemyCamp governs self-origin/periodic negative HP as well.
	e.changeHP(d, d, -10, "periodic_damage")
	closeCombat(t, d.hp, 79930)
	e.changeHP(a, d, 20, "skill_heal")
	closeCombat(t, d.hp, 79950)
	e.damageHP(d, a, 100, 2)
	closeCombat(t, a.hp, 79950)
	e.input.AttackerDamageFactor = proto.Float64(0)
	e.damageHP(a, d, 1e6, 1)
	closeCombat(t, d.hp, 79949)
	e.changeHP(d, d, -100, "periodic_damage")
	closeCombat(t, d.hp, 79948)
	// Original matching shields take a zero-HP branch before MinusHP's minimum.
	if err := e.loadBuff(d, d, map[string]string{"Id": "factor-zero-shield", "Factor": "shield", "Args1": "50", "TotalRound": "2"}, 1); err != nil {
		t.Fatal(err)
	}
	e.damageHP(a, d, 100, 1)
	e.changeHP(d, d, -100, "periodic_damage")
	e.damageHP(a, d, 0, 1)
	closeCombat(t, d.hp, 79948)
	closeCombat(t, d.shield("shield"), 50)
	if e.rng.Draws() != 0 {
		t.Fatal("AddHp multiplier introduced RNG")
	}
}

func TestP6DamageFactorFreezeCloneInvalidAndLegacyDefault(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "factor-owner", "047_1", 1, 1000), p6Actor("hero_047", "factor-enemy", "047_1", 1, 1000), 2)
	if FrozenP6DamageFactor(f, 0) != 1 || FrozenP6DamageFactor(f, 1) != 1 {
		t.Fatal("legacy omitted multipliers changed")
	}
	f.AttackerDamageFactor = proto.Float64(0)
	f.DefenderDamageFactor = proto.Float64(2)
	cloned, err := s.FreezeBattleInput(*f)
	if err != nil {
		t.Fatal(err)
	}
	*f.AttackerDamageFactor = 5
	*f.DefenderDamageFactor = 6
	if FrozenP6DamageFactor(cloned, 0) != 0 || FrozenP6DamageFactor(cloned, 1) != 2 {
		t.Fatal("freeze multiplier pointer aliases producer")
	}
	calculation, err := cloneP6CalculationInput(cloned)
	if err != nil {
		t.Fatal(err)
	}
	*cloned.AttackerDamageFactor = 3
	if FrozenP6DamageFactor(calculation, 0) != 0 {
		t.Fatal("calculation multiplier pointer aliases frozen input")
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 1e16} {
		f.AttackerDamageFactor = proto.Float64(value)
		if _, err = s.FreezeBattleInput(*f); err == nil {
			t.Fatal("current invalid multiplier accepted", value)
		}
		if _, err = s.ComputeFrozenBattle(t.Context(), f, nil); err == nil {
			t.Fatal("invalid calculation multiplier accepted", value)
		}
	}
	f.AttackerDamageFactor = proto.Float64(-1)
	if err = ValidateP6DamageFactors(f); err == nil || !strings.Contains(err.Error(), constUtil.CombatUnsupported) {
		t.Fatal("unrestored signed inversion silently normalized", err)
	}
}

func TestP6DamageFactorOriginalWireZeroAndRetryHashConflict(t *testing.T) {
	s, ctx, path := newDailyCopyTest(t)
	if err := s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "factor-cold", 10)
	f := p6Input(t, s, p6Actor("hero_047", "factor-player", "047_1", 1, 1000), p6Actor("hero_047", "factor-enemy", "047_1", 1, 1000), 2)
	f.Attacker.PlayerId = proto.Int64(id)
	f.AttackerDamageFactor = proto.Float64(0)
	f.DefenderDamageFactor = proto.Float64(.5)
	f.MaxRound = 2
	var recordID string
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		var err error
		recordID, err = s.CaptureBattleReplay(ctx, tx, id, "factor-original", "zero-half", *f)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.ComputeBattleReplay(ctx, tx, id, "factor-original", "zero-half", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", recordID).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := decodeP6StoredInput(record.Input)
	if err != nil || p6Digest(raw) != record.InputHash || record.Authoritative {
		t.Fatal("original factor canonical stored hash", err)
	}
	var stored model.FrozenBattleInput
	if err = json.Unmarshal(raw, &stored); err != nil || stored.AttackerDamageFactor == nil || FrozenP6DamageFactor(&stored, 0) != 0 || FrozenP6DamageFactor(&stored, 1) != .5 {
		t.Fatal("zero factor lost explicit presence", err)
	}
	frames, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
	if err != nil {
		t.Fatal(err)
	}
	playback := frames[0].(*pb.SCFightPlayback)
	if playback.AttackerFactor == nil || playback.GetAttackerFactor() != 0 || playback.GetDefenderFactor() != .5 || !proto.Equal(playback.Attacker, f.Attacker) {
		t.Fatal("original wire factors or paid actor changed")
	}
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.CaptureBattleReplay(ctx, tx, id, "factor-original", "zero-half", *f)
		return err
	}); err != nil {
		t.Fatal("unchanged factor retry rejected", err)
	}
	for _, change := range []func(){func() { f.AttackerDamageFactor = nil }, func() { f.AttackerDamageFactor = proto.Float64(1) }, func() { f.AttackerDamageFactor = proto.Float64(0); f.DefenderDamageFactor = proto.Float64(1) }} {
		change()
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			_, err := s.CaptureBattleReplay(ctx, tx, id, "factor-original", "zero-half", *f)
			return err
		}); err == nil {
			t.Fatal("factor value/presence replacement accepted")
		}
	}
	// Close the real storage connection before checking byte-identical current and legacy transport.
	wire, err := proto.Marshal(playback)
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	coldConnection, _ := db.GetIns().DB()
	t.Cleanup(func() { coldConnection.Close() })
	frames, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
	if err != nil {
		t.Fatal(err)
	}
	coldWire, err := proto.Marshal(frames[0])
	if err != nil || !bytes.Equal(wire, coldWire) {
		t.Fatal("factor cold wire changed", err)
	}

	// A valid hash cannot make a current unsupported signed multiplier executable/readable.
	stored.AttackerDamageFactor = proto.Float64(-1)
	malformed, err := json.Marshal(&stored)
	if err != nil {
		t.Fatal(err)
	}
	record.Input, err = encodeP6StoredInput(malformed)
	if err != nil {
		t.Fatal(err)
	}
	record.InputHash = p6Digest(malformed)
	if err = db.GetIns().Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)}); err == nil {
		t.Fatal("current invalid factor with correct hash accepted")
	}

	// Historical completed records keep their original omitted factor wire and never recompute.
	stored.EngineVersion = "apk-10056262-p6-v48"
	stored.AttackerDamageFactor = nil
	stored.DefenderDamageFactor = nil
	legacy, err := json.Marshal(&stored)
	if err != nil {
		t.Fatal(err)
	}
	record.Input = legacy
	record.InputHash = p6Digest(legacy)
	record.EngineVersion = stored.EngineVersion
	if err = db.GetIns().Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	before, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
	if err != nil || before[0].(*pb.SCFightPlayback).GetAttackerFactor() != 1 || before[0].(*pb.SCFightPlayback).GetDefenderFactor() != 1 {
		t.Fatal("historical omitted multiplier", err)
	}
	oldWire, err := proto.Marshal(before[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = coldConnection.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	reopened, _ := db.GetIns().DB()
	t.Cleanup(func() { reopened.Close() })
	after, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
	if err != nil {
		t.Fatal(err)
	}
	legacyWire, err := proto.Marshal(after[0])
	if err != nil || !bytes.Equal(oldWire, legacyWire) {
		t.Fatal("legacy omitted multiplier cold wire changed", err)
	}
	if _, err = s.ComputeFrozenBattle(ctx, &stored, nil); err == nil {
		t.Fatal("historical factor record recomputed")
	}

}

// This reads only a harness-owned schema; it cannot connect to the formal game database.
func TestP6MySQLDamageFactorFrozenWireColdIsolated(t *testing.T) {
	dsn := os.Getenv("HYJS_MYSQL_P6_DSN")
	if dsn == "" {
		t.Skip("isolated P6 MySQL disabled")
	}
	configuration, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(configuration.DBName, "hyjs_test_p6_") {
		t.Fatal("dedicated P6 schema required")
	}
	ctx := context.Background()
	open := func() {
		if err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: configuration.Addr, User: configuration.User, Password: configuration.Passwd, Database: configuration.DBName}); err != nil {
			t.Fatal(err)
		}
	}
	open()
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	s := p6OriginalTables(t)
	if err = s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "factor-mysql-cold", 10)
	input := p6Input(t, s, p6Actor("hero_047", "factor-mysql-player", "047_1", 1, 1000), p6Actor("hero_047", "factor-mysql-enemy", "047_1", 1, 1000), 2)
	input.Attacker.PlayerId = proto.Int64(id)
	type savedWire struct {
		id                 string
		bytes              []byte
		attacker, defender float64
	}
	var snapshots []savedWire
	for index, factors := range [][2]*float64{{nil, nil}, {proto.Float64(0), proto.Float64(.5)}, {proto.Float64(2), proto.Float64(.5)}} {
		input.AttackerDamageFactor, input.DefenderDamageFactor = factors[0], factors[1]
		source := []string{"omitted", "zero-half", "double-half"}[index]
		var recordID string
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var captureErr error
			recordID, captureErr = s.CaptureBattleReplay(ctx, tx, id, "factor-mysql", source, *input)
			if captureErr != nil {
				return captureErr
			}
			_, captureErr = s.ComputeBattleReplay(ctx, tx, id, "factor-mysql", source, nil)
			return captureErr
		}); err != nil {
			t.Fatal(err)
		}
		original, queryErr := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		wire, marshalErr := proto.Marshal(original[0])
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		snapshots = append(snapshots, savedWire{id: recordID, bytes: wire, attacker: FrozenP6DamageFactor(input, 0), defender: FrozenP6DamageFactor(input, 1)})
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			_, retryErr := s.CaptureBattleReplay(ctx, tx, id, "factor-mysql", source, *input)
			return retryErr
		}); err != nil {
			t.Fatal("factor same retry", err)
		}
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	open()
	cold, _ := db.GetIns().DB()
	t.Cleanup(func() { cold.Close() })
	for _, snapshot := range snapshots {
		frames, queryErr := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(snapshot.id), ServerId: proto.Int32(1)})
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		wire, marshalErr := proto.Marshal(frames[0])
		replay := frames[0].(*pb.SCFightPlayback)
		if marshalErr != nil || !bytes.Equal(snapshot.bytes, wire) || replay.GetAttackerFactor() != snapshot.attacker || replay.GetDefenderFactor() != snapshot.defender {
			t.Fatal("InnoDB cold factor original wire", snapshot.id, marshalErr)
		}
		var record db.P6BattleRecord
		if err = db.GetIns().First(&record, "id=?", snapshot.id).Error; err != nil {
			t.Fatal(err)
		}
		raw, decodeErr := decodeP6StoredInput(record.Input)
		if decodeErr != nil || p6Digest(raw) != record.InputHash || record.Authoritative {
			t.Fatal("factor canonical InnoDB hash", decodeErr)
		}
	}
}
