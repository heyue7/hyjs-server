package service

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WantedOriginalDefinition(t *testing.T) (*GameService, p12WantedDefinition) {
	t.Helper()
	s := p6OriginalTables(t)
	if err := s.LoadP12WantedTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	return s, p12WantedDefinition{Tables: s.Tables}
}

func TestP12WantedPeriodOriginalCostsAndLocalDay(t *testing.T) {
	_, def := p12WantedOriginalDefinition(t)
	zone := time.FixedZone("UTC+8", 8*3600)
	for hour := 0; hour < 24; hour++ {
		now := time.Date(2026, 10, 16, hour, 59, 59, 0, zone)
		period, err := p12WantedPeriod(def, now.UTC())
		if err != nil {
			t.Fatal(err)
		}
		odds, rage, index := "10", "wantedAttTimes,2", "1000"
		if hour == 12 || hour == 13 {
			odds, rage = "25", "wantedAttTimes,1"
		}
		if hour >= 18 && hour <= 21 {
			odds, index = "25", "500"
		}
		if period["Odd"] != odds || period["RageAttackCost"] != rage || period["NormalAttackCost"] != "wantedAttTimes,1" || period["PerformanceIndex"] != index || period["CoreIndex"] != "50" {
			t.Fatal("original hour contract", hour, period)
		}
		day, start, end := p12WantedDay(now.UTC())
		if day != "2026-10-16" || start.Hour() != 0 || end.Sub(start) != 24*time.Hour {
			t.Fatal("local daily boundary", day, start, end)
		}
	}
}

func TestP12WantedMonsterOriginalFourBodiesAndPersistentView(t *testing.T) {
	s, def := p12WantedOriginalDefinition(t)
	wire := p12WantedNativeTypedInput(t, p12WantedNativeCases(t)[0])
	waves, monsters, maximum, err := s.p12WantedMonster(def, "1", nil)
	if err != nil || len(waves) != 1 || len(monsters) != 4 || maximum != 405371 || !proto.Equal(waves[0], wire.Defender2) {
		t.Fatal("complete original four-body Wanted1 projection", maximum, err)
	}
	current := map[string]float64{}
	for i, actor := range monsters {
		current[actor.GetWantedEnemyId()] = actor.GetHp() - float64(i+1)
	}
	rawHP, _ := json.Marshal(current)
	rawDef, _ := json.Marshal(def)
	instance := &db.P12WantedInstance{ID: "instance", FinderID: 7, WantedID: "1", Level: 1, Difficulty: 1, CurrentHP: maximum - 10, MaxHP: maximum, MonsterHPJSON: string(rawHP), DefinitionJSON: string(rawDef), DefinitionHash: p6Digest(rawDef), ExpiresAt: 1800000}
	view, err := s.p12WantedView(instance)
	if err != nil || view.GetHp() != maximum-10 || view.GetMaxHP() != maximum || view.GetRunTime() != instance.ExpiresAt || view.GetState() != 0 || len(view.Monsters) != 4 {
		t.Fatal("persistent original view", err)
	}
	instance.SharedAt = 1
	view, err = s.p12WantedView(instance)
	if err != nil || view.GetState() != 1 {
		t.Fatal("shared view", err)
	}
	for _, mode := range []string{"missing-body", "foreign-body", "negative-hp", "nonfinite-hp"} {
		copyHP := map[string]float64{}
		for key, value := range current {
			copyHP[key] = value
		}
		switch mode {
		case "missing-body":
			delete(copyHP, "wanted_4")
		case "foreign-body":
			copyHP["wanted_foreign"] = 1
		case "negative-hp":
			copyHP["wanted_4"] = -1
		case "nonfinite-hp":
			copyHP["wanted_4"] = math.NaN()
		}
		if _, _, _, err = s.p12WantedMonster(def, "1", copyHP); err == nil {
			t.Fatal("invalid stored actor HP admitted", mode)
		}
	}
}

func TestP12WantedSharedDamageClipsPerBodyWithoutOverdraw(t *testing.T) {
	_, input := p12WantedOriginalFixture(t)
	current := map[string]float64{"wanted_1": 10, "wanted_2": 20, "wanted_3": 30, "wanted_4": 40}
	terminal := &model.CombatFinalHPOutcome{}
	// Every frozen encounter actually removes 15 HP from each original body.
	for _, actor := range input.Monster.Formation {
		terminal.FinalActors = append(terminal.FinalActors, model.CombatFinalActorHP{Camp: 1, GUID: actor.GetGuid(), HeroID: actor.GetId(), MaxHP: actor.Prop.GetHp(), CurrentHP: actor.Inst.GetCurHp() - 15})
	}
	next, applied, err := p12WantedApplyTerminalHP(input, terminal, current)
	if err != nil || applied != 55 || next["wanted_1"] != 0 || next["wanted_2"] != 5 || next["wanted_3"] != 15 || next["wanted_4"] != 25 || current["wanted_1"] != 10 {
		t.Fatal("per-body actual damage clipping", applied, next, err)
	}
	last, applied, err := p12WantedApplyTerminalHP(input, terminal, next)
	if err != nil || applied != 35 || last["wanted_4"] != 10 {
		t.Fatal("concurrent finish cannot overdraw prior commit", applied, last, err)
	}
	for _, actor := range input.Monster.Formation {
		for i := range terminal.FinalActors {
			if terminal.FinalActors[i].GUID == actor.GetGuid() {
				terminal.FinalActors[i].CurrentHP = actor.Inst.GetCurHp()
			}
		}
	}
	_, applied, err = p12WantedApplyTerminalHP(input, terminal, current)
	if err != nil || applied != 0 {
		t.Fatal("zero actual damage is legal", applied, err)
	}
	terminal.FinalActors[1].GUID = terminal.FinalActors[0].GUID
	if _, _, err = p12WantedApplyTerminalHP(input, terminal, current); err == nil {
		t.Fatal("duplicate body accepted")
	}
}

func TestP12WantedPeriodAttemptRecoveryPreservesPurchasedOverflow(t *testing.T) {
	now := time.UnixMilli(1800000000000)
	player := &pb.Player{WantedAttTimes: proto.Int32(13), WantedAttTime: proto.Int64(now.Add(-3 * time.Hour).UnixMilli())}
	if err := p12WantedRecover(player, now); err != nil || player.GetWantedAttTimes() != 13 {
		t.Fatal("paid overflow was truncated", err)
	}
	player.WantedAttTimes = proto.Int32(2)
	player.WantedAttTime = proto.Int64(now.Add(-150 * time.Minute).UnixMilli())
	if err := p12WantedRecover(player, now); err != nil || player.GetWantedAttTimes() != 4 || player.GetWantedAttTime() != now.Add(-30*time.Minute).UnixMilli() {
		t.Fatal("natural whole-hour recovery", err)
	}
	player.WantedAttTime = proto.Int64(now.Add(time.Minute).UnixMilli())
	if p12WantedRecover(player, now) == nil {
		t.Fatal("future recovery clock accepted")
	}
}
