package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP14EngineUpgradeLegacyProducer 用真实旧引擎闭包产生已计算和未计算两类历史，禁止仅改版本字段准备升级数据。
func TestP14EngineUpgradeLegacyProducer(t *testing.T) {
	if os.Getenv("HYJS_P14_ENGINE_UPGRADE_ROLE") != "legacy" {
		t.Skip("explicit isolated old-engine producer")
	}
	if constUtil.P6EngineVersion != "apk-10056262-p6-v81" {
		t.Fatal("requires immutable actual v81 engine")
	}
	f, now := p14AuditionMatchSetup(t, 11)
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	round, err := f.s.PrepareP14AuditionRound(f.ctx, now)
	if err != nil || round == nil {
		t.Fatal(err)
	}
	var matches []db.P14AuditionMatch
	if err := db.GetIns().Order("pair_index").Find(&matches).Error; err != nil || len(matches) != 5 {
		t.Fatal(err)
	}
	outcome, err := f.s.ComputeP14AuditionMatch(f.ctx, matches[0].ID)
	if err != nil || outcome == nil || outcome.Authoritative {
		t.Fatal("actual old computed diagnostic", err)
	}
}

// TestP14EngineUpgradeCurrentConsumer 冷连接同一InnoDB后读旧回执并拒绝旧待计算来源，不把旧冻结阵容标为新引擎。
func TestP14EngineUpgradeCurrentConsumer(t *testing.T) {
	if os.Getenv("HYJS_P14_ENGINE_UPGRADE_ROLE") != "current" {
		t.Skip("explicit isolated current-engine consumer")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_upgrade_") || constUtil.P6EngineVersion == "apk-10056262-p6-v81" {
		t.Fatal("requires isolated actual engine upgrade")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sql, _ := db.GetIns().DB(); sql.Close() })
	s := &GameService{ServerID: 1}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeP14Cross(ctx); err != nil {
		t.Fatal(err)
	}
	var matches []db.P14AuditionMatch
	if err := db.GetIns().Order("pair_index").Find(&matches).Error; err != nil || len(matches) != 5 {
		t.Fatal(err)
	}
	var previous []db.P6BattleRecord
	if err := db.GetIns().Where("source=?", constUtil.P14AuditionBattleSource).Order("id").Find(&previous).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range previous {
		if record.EngineVersion != "apk-10056262-p6-v81" {
			t.Fatal("not a genuine old record")
		}
		if record.Status == constUtil.P6BattleComputed {
			outcome, err := s.ComputeP14AuditionMatch(ctx, record.SourceFight)
			if err != nil || outcome == nil || outcome.Authoritative {
				t.Fatal("old computed retry", err)
			}
			replay, err := s.fightPlayback(ctx, record.PlayerID, &pb.CSFightPlayback{FightId: proto.String(record.ID)})
			if err != nil || replay[0].(*pb.SCFightPlayback).GetBattleType() != 17 {
				t.Fatal("old immutable playback", err)
			}
		} else {
			if _, err := s.ComputeP14AuditionMatch(ctx, record.SourceFight); err == nil {
				t.Fatal("old captured source recomputed by new engine")
			}
		}
	}
	var after []db.P6BattleRecord
	if err := db.GetIns().Where("source=?", constUtil.P14AuditionBattleSource).Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	for i := range previous {
		a, b := previous[i], after[i]
		if a.EngineVersion != b.EngineVersion || a.InputHash != b.InputHash || a.OutcomeHash != b.OutcomeHash || a.ReplayHash != b.ReplayHash || a.Status != b.Status {
			t.Fatal("upgrade changed historical source or outcome")
		}
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionQualification{}).Count(&count)
	if count != 0 {
		t.Fatal("non-authoritative old results promoted")
	}
}
