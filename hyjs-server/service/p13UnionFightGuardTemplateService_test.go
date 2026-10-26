package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFGuardTemplatePerGuardHPAndPublicationBoundary(t *testing.T) {
	f, _, _, clock, match := p13UFFieldSetup(t)
	now := time.UnixMilli(clock.Load())
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	policy := &db.P13UnionFightFieldPolicy{GuardHPSource: constUtil.P13UnionFightGuardHPSourceMonster}
	var existing []db.P13UnionFightGuardAuditSource
	if err = db.GetIns().Where("day=?", match.Day).Find(&existing).Error; err != nil {
		t.Fatal(err)
	}
	if len(existing) != 0 {
		t.Fatal("fixture must have no implicit guard template")
	}
	if _, err = p13UFGuardTemplateHP(db.GetIns(), match, policy, original, now); err == nil {
		t.Fatal("unpublished template accepted")
	}
	want := map[string]float64{}
	for index, row := range original.Tables["UnionFightGuard"] {
		id := row["UFGuardId"]
		hp := float64(1200 + index*101)
		monster := p13UFGuardAuditTemplate(original, id, hp)
		raw, _ := proto.Marshal(monster)
		source := db.P13UnionFightGuardAuditSource{Day: match.Day, GuardID: id, Enabled: true, Monster: raw, PublishedAt: min(now.UnixMilli(), match.StartAt-1), Source: "finite distinct template qualification"}
		if err = db.GetIns().Create(&source).Error; err != nil {
			t.Fatal(err)
		}
		want[id] = hp
	}
	if len(want) != 10 {
		t.Fatal("original guard denominator", len(want))
	}
	got, err := p13UFGuardTemplateHP(db.GetIns(), match, policy, original, now)
	if err != nil || len(got) != 10 {
		t.Fatal(got, err)
	}
	for id, hp := range want {
		if got[id] != hp {
			t.Fatal("per-guard HP lost", id, hp, got[id])
		}
	}
	policy.InitialGuardHP = 1000
	if _, err = p13UFGuardTemplateHP(db.GetIns(), match, policy, original, now); err == nil {
		t.Fatal("ambiguous uniform and per-guard values accepted")
	}
	policy.InitialGuardHP = 0
	if err = db.GetIns().Model(&db.P13UnionFightGuardAuditSource{}).Where("day=? AND guard_id=?", match.Day, "tower_guard_001").Update("published_at", match.StartAt).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13UFGuardTemplateHP(db.GetIns(), match, policy, original, now); err == nil {
		t.Fatal("start-time or late template accepted")
	}
	policy.GuardHPSource = ""
	policy.InitialGuardHP = 1000
	got, err = p13UFGuardTemplateHP(db.GetIns(), match, policy, original, now)
	if err != nil || len(got) != 0 {
		t.Fatal("legacy uniform mode consulted new templates", got, err)
	}
}

func TestP13UFGuardMirrorFullTemplateAndSource(t *testing.T) {
	f, _, _, clock, match := p13UFFieldSetup(t)
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range original.Tables["UnionFightGuard"] {
		id := definition["UFGuardId"]
		monster, err := original.P13UnionFightMirrorGuardTemplate(id)
		if err != nil {
			t.Fatal(id, err)
		}
		heroID, err := p13UFGuardTemplateHeroID(id, definition["HeroId"], constUtil.P13UFGuardMirrorRule)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := original.actor(original.table("Hero", "id", heroID), monster.Formation[0].GetGuid(), 3, false)
		if err != nil {
			t.Fatal(err)
		}
		expected.Level, expected.BloodNumber = proto.Int32(1), proto.Int32(1)
		if !proto.Equal(expected, monster.Formation[0]) {
			t.Fatal("complete original props/skills lost", id)
		}
		raw, _ := proto.Marshal(monster)
		source := &db.P13UnionFightGuardAuditSource{Day: match.Day, GuardID: id, GuardRule: constUtil.P13UFGuardMirrorRule, Enabled: true, Monster: raw, PublishedAt: min(clock.Load()-1, match.StartAt-1), Source: "named local full-template rule test"}
		if _, err = p13UFGuardMirrorSource(source, original, match, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(id, err)
		}
		damaged := proto.Clone(monster).(*pb.BattleMonster)
		damaged.Formation[0].Prop.Att = proto.Float64(damaged.Formation[0].Prop.GetAtt() + 1)
		source.Monster, _ = proto.Marshal(damaged)
		if _, err = p13UFGuardMirrorSource(source, original, match, time.UnixMilli(clock.Load())); err == nil {
			t.Fatal("HP-only qualification accepted changed ATT", id)
		}
		got, err := p13UFGuardTemplateHeroID(id, definition["HeroId"], "")
		if err != nil || got != definition["HeroId"] {
			t.Fatal("legacy source mapping changed", id)
		}
	}
	if _, err := p13UFGuardTemplateHeroID("unknown", "hero_002", constUtil.P13UFGuardMirrorRule); err == nil {
		t.Fatal("unknown identity admitted")
	}
}
