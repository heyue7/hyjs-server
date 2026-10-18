package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// 原50级完整六档覆盖，旧定义缺具名来源继续闭门；只展示已存在的可能ID，不选择概率结果。
func TestP12WorldBossPossibleIDOriginalAllLevelsHiddenQuantityAndSource(t *testing.T) {
	f, _ := p12WorldBossSetup(t)
	def := p12WorldBossDefinition{Tables: f.s.Tables, WeightedRewardPreviewSource: constUtil.P12WorldBossIDPreviewSource}
	before, _ := json.Marshal(def)
	for level := int64(1); level <= 50; level++ {
		preview, err := p12WorldBossRewardPreview(def, level)
		if err != nil || len(preview.GetRewardInfos()) != 6 {
			t.Fatal(level, err)
		}
		for kind, row := range preview.GetRewardInfos() {
			if row.GetType() != int32(kind) || len(row.Rewards) == 0 {
				t.Fatal("incomplete original tier", level, kind)
			}
			weighted := level >= 30 && kind >= 4
			for _, reward := range row.Rewards {
				if weighted && reward.GetNum() != -1 || !weighted && reward.GetNum() <= 0 {
					t.Fatal("fabricated weighted quantity or changed fixed tier", level, kind, reward)
				}
			}
		}
	}
	after, _ := json.Marshal(def)
	if string(before) != string(after) {
		t.Fatal("preview mutated original definition")
	}
	for _, source := range []string{"", "unknown-preview", "local-guessed-probability"} {
		legacy := def
		legacy.WeightedRewardPreviewSource = source
		if _, err := p12WorldBossRewardPreview(legacy, 30); err == nil {
			t.Fatal("missing/unknown source opened weighted preview")
		}
		if _, err := p12WorldBossRewardPreview(legacy, 29); err != nil {
			t.Fatal("legacy fixed preview changed", err)
		}
	}
	preview, err := p12WorldBossRewardPreview(def, 44)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, reward := range preview.RewardInfos[4].Rewards {
		if reward.GetId() == "frag_equip_039" && reward.GetNum() == -1 {
			found = true
		}
	}
	if !found {
		t.Fatal("original44 partial weight total was normalized/dropped")
	}
}
func TestP12WorldBossPossibleIDGrammarNoDrawNoPromiseAndNegative(t *testing.T) {
	for _, raw := range []string{"coin:", "coin:1", ":1,1", "coin:-1,1000", "coin:1,-1", "coin:2147483648,1", "coin:1,2147483648", "coin:1,1|coin:2,2", "coin:1,1|", "coin:0,1000", "bad id:1,1000", "coin:1,1;bad", "coin:1,1|item_139,1"} {
		if _, err := p12WorldBossPossibleIDs(raw); err == nil {
			t.Fatal("invalid possible-ID definition accepted", raw)
		}
	}
	raw := "coin:200000,1000|MineItem_083:1,200;0,800|item_139:2,100;3,700|item_150:0,900;1,0"
	first, err := p12WorldBossPossibleIDs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 {
		t.Fatal("zero-quantity or zero-weight option invented resource", first)
	}
	expected := []string{"coin", "MineItem_083", "item_139"}
	for index, row := range first {
		if row.GetId() != expected[index] || row.GetNum() != -1 {
			t.Fatal("ID-only projection guessed quantity/order", row)
		}
	}
	for i := 0; i < 20; i++ {
		got, err := p12WorldBossPossibleIDs(raw)
		if err != nil {
			t.Fatal(err)
		}
		for k := range got {
			if !proto.Equal(got[k], first[k]) {
				t.Fatal("preview drew random candidate")
			}
		}
	}
}
func TestP12WorldBossPossibleIDCycleFreezeColdGateAndResources(t *testing.T) {
	f, b := p12WorldBossSetup(t)
	now := p12WorldBossDay().Add(12 * time.Hour)
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "level": 30}).Error; err != nil {
		t.Fatal(err)
	}
	original := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossReward{}, now).(*pb.SCWorldBossReward)
	if original.RewardInfos[4].Rewards[0].GetNum() != -1 {
		t.Fatal("original quantity not hidden")
	}
	var cfg db.P12WorldBossPolicy
	if err := db.GetIns().First(&cfg, 1).Error; err != nil {
		t.Fatal(err)
	}
	var edited p12WorldBossDefinition
	if err := json.Unmarshal([]byte(cfg.DefinitionJSON), &edited); err != nil {
		t.Fatal(err)
	}
	edited.WeightedRewardPreviewSource = ""
	for _, row := range edited.Tables["WorldBossReward"] {
		if row["level"] == "30" && row["type"] == "4" {
			row["reward"] = "coin:9999999,1000"
		}
	}
	raw, _ := json.Marshal(edited)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"definition_json": string(raw), "definition_hash": p6Digest(raw), "level": 50}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := f.s.InitializeP12WorldBoss(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(original, p12WorldBossCall(t, f, b, &pb.CSWorldBossReward{}, now.Add(time.Minute))) {
		t.Fatal("same frozen cycle followed live source/edit/level")
	}
	if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossReward{}, now.Add(10*time.Hour)); err == nil {
		t.Fatal("next cycle without source opened high-level preview")
	}
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossReward{}, now); err == nil {
		t.Fatal("closed public gate reused preview")
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("possible-ID query changed wallet/inventory/profile")
	}
	var count int64
	db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&count)
	if count != 1 {
		t.Fatal("missing-source failed query committed next instance", count)
	}
}
