package service

import (
	"bytes"
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func TestRecruitBalance35WorldProbability(t *testing.T) {
	f := p1Setup(t)
	for _, world := range []int64{0, 1, 2, 3} {
		rows, err := f.s.balancedRecruit35Rewards("recruit_3", world, true)
		if err != nil {
			t.Fatal(world, err)
		}
		spaces := []int32{int32(world)}
		if world == 0 {
			spaces = []int32{1, 2, 3}
		}
		for _, space := range spaces {
			filtered, _ := f.s.kingRewards(rows, space)
			weights := map[string]int64{}
			var total int64
			for _, r := range filtered {
				if r.Weight < 1 || r.Weight > 1000000000000 {
					t.Fatal("invalid backend weight", r.Weight)
				}
				q := f.s.table("Hero", "id", r.ResourceID)["quality"]
				weights[q] += r.Weight
				total += r.Weight
			}
			if weights["23"]*1000000 != total*constUtil.RecruitBalance35RedPPM || weights["20"]*1000000 != total*6000 {
				t.Fatal("world probabilities differ", world, space, weights, total)
			}
			for _, q := range []string{"12", "13", "18", "20", "23"} {
				if weights[q] <= 0 {
					t.Fatal("missing quality", q)
				}
			}
		}
	}
	rows, err := f.s.balancedRecruit35Rewards("recruit_2", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var total, orange int64
	greenCommon := false
	for _, r := range rows {
		hero := f.s.table("Hero", "id", r.ResourceID)
		total += r.Weight
		if hero["quality"] == "20" {
			orange += r.Weight
		}
		if hero["quality"] == "12" && hero["space"] == "0" {
			greenCommon = true
		}
	}
	if !greenCommon || orange*1000000 != total*6000 {
		t.Fatal("strong pool probability/common green")
	}
}

func TestRecruitBalance35GlobalOnceAndVirtualExpiredHistory(t *testing.T) {
	f := p1Setup(t)
	for _, initialize := range []func() error{func() error { return f.s.InitializeGameplayPools(f.ctx) }, func() error { return f.s.InitializeP2(f.ctx) }, func() error { return InitializeAdminOperations(f.ctx) }, func() error { return f.s.InitializeRecruitRuntime(f.ctx) }} {
		if err := initialize(); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.ServerGameClock{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.ServerGameClock{ServerID: 1, OffsetDays: 2, HighDays: 2}).Error; err != nil {
		t.Fatal(err)
	}
	var base db.GameplayPool
	var rules db.KingRecruitRules
	if err := db.GetIns().First(&base, "id=?", "recruit_3").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&rules, 1).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	form := model.AdminActivityForm{Name: "old form", Source: "old", Options: model.AdminActivityOptions{KingWorld: 1}, Prizes: []model.AdminActivityPrize{{ID: 1, ResourceID: "hero_043", Amount: 1, Weight: 7}}}
	raw := adminJSON(form)
	template := db.AdminActivityDefinition{ID: "balance35-template", Mode: constUtil.AdminKingRecruitMode, Name: "old", FormJSON: raw, Revision: 1}
	if err := db.GetIns().Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	type saved struct {
		occ  db.AdminActivityOccurrence
		pool db.GameplayPool
		snap db.KingRecruitActivitySnapshot
	}
	var fixtures []saved
	for _, spec := range []struct {
		id      string
		server  int64
		end     time.Time
		enabled bool
	}{{"expired-wall", 2, now.Add(-time.Hour), true}, {"expired-virtual", 1, now.Add(24 * time.Hour), true}, {"current-other", 2, now.Add(time.Hour), true}, {"future", 1, now.Add(4 * 24 * time.Hour), true}, {"disabled", 2, now.Add(time.Hour), false}} {
		pool := base
		pool.ID = constUtil.AdminKingRecruitPrefix + spec.id
		pool.Name = "old pool"
		pool.Enabled = spec.enabled
		pool.EndAt = spec.end.UnixMilli()
		source := adminKingRecruitSource{World: 1, Pool: pool, Rules: rules}
		snapshot := db.KingRecruitActivitySnapshot{PoolID: pool.ID, Payload: adminJSON(source), Frozen: true}
		occurrence := db.AdminActivityOccurrence{ID: spec.id, ServerID: spec.server, TemplateID: template.ID, Mode: constUtil.AdminKingRecruitMode, ConsumerKey: pool.ID, ConsumerID: "pool:" + pool.ID, EndAt: pool.EndAt, Enabled: spec.enabled, FormJSON: raw, Revision: 1}
		reward := db.GameplayPoolReward{PoolID: pool.ID, ID: 1, ResourceID: "hero_043", Amount: 1, Weight: 7}
		for _, row := range []any{&pool, &snapshot, &occurrence, &reward} {
			if err := db.GetIns().Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		fixtures = append(fixtures, saved{occurrence, pool, snapshot})
	}
	if err := f.s.InitializeRecruitBalance35(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, before := range fixtures {
		var occurrence db.AdminActivityOccurrence
		var pool db.GameplayPool
		var snapshot db.KingRecruitActivitySnapshot
		var rewards []db.GameplayPoolReward
		db.GetIns().First(&occurrence, "id=?", before.occ.ID)
		db.GetIns().First(&pool, "id=?", before.pool.ID)
		db.GetIns().First(&snapshot, "pool_id=?", before.pool.ID)
		db.GetIns().Where("pool_id=?", pool.ID).Find(&rewards)
		changed := before.occ.ID == "current-other" || before.occ.ID == "future"
		if !changed {
			if !bytes.Equal(occurrence.FormJSON, before.occ.FormJSON) || pool.Name != before.pool.Name || !bytes.Equal(snapshot.Payload, before.snap.Payload) || len(rewards) != 1 || rewards[0].Weight != 7 {
				t.Fatal("history/disabled changed", before.occ.ID)
			}
		} else {
			if bytes.Equal(occurrence.FormJSON, before.occ.FormJSON) || len(rewards) < 10 || !snapshot.Frozen {
				t.Fatal("live/future migration missing", before.occ.ID)
			}
		}
	}
	var audits []db.AdminAudit
	if err := db.GetIns().Where("action=?", constUtil.RecruitBalance35BeforeAction).Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	foundSnapshot := false
	for _, audit := range audits {
		if audit.Target == "pool:"+fixtures[2].pool.ID {
			var backup map[string]json.RawMessage
			if json.Unmarshal([]byte(audit.Detail), &backup) != nil {
				t.Fatal("bad audit")
			}
			var source struct {
				Payload json.RawMessage `json:"payload"`
			}
			if json.Unmarshal(backup["snapshot"], &source) != nil || !bytes.Equal(source.Payload, fixtures[2].snap.Payload) {
				t.Fatal("source snapshot not backed up")
			}
			foundSnapshot = true
		}
	}
	if !foundSnapshot {
		t.Fatal("missing before audit")
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=?", "recruit_2").Update("weight", 17).Error; err != nil {
		t.Fatal(err)
	}
	f.s.ServerID = 2
	if err := f.s.InitializeRecruitBalance35(f.ctx); err != nil {
		t.Fatal(err)
	}
	var changed int64
	db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND weight<>?", "recruit_2", 17).Count(&changed)
	if changed != 0 {
		t.Fatal("second realm overwrote admin edits")
	}
	var applied int64
	db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.RecruitBalance35ApplyAction, constUtil.RecruitBalance35Version).Count(&applied)
	if applied != 1 {
		t.Fatal("global marker", applied)
	}
}
