package service

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP7ExpeditionStoryPoolAll92OriginalRowsMatchingIdentityAndSpecialGate(t *testing.T) {
	f := p7ExpeditionSetup(t)
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "risk_percent": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7ExpeditionStoryPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, row := range f.s.Tables["ExpeditionStory"] {
		if row["storyId"] >= "stroy_001" && row["storyId"] <= "stroy_073" {
			ids[row["rewardId"]] = true
		}
	}
	seen := map[string]bool{}
	for id := range ids {
		var rules *p7ExpeditionEventRules
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var err error
			rules, err = f.s.p7FreezeExpeditionEventRules(tx, id+",3")
			return err
		}); err != nil {
			t.Fatal(id, err)
		}
		if rules.Stories == nil || rules.Stories.LuckyPercent != 20 || !strings.Contains(rules.Source, "local-matching-expedition-stories-v1") {
			t.Fatal("unfrozen named pool")
		}
		for _, choice := range rules.Stories.Ordinary {
			if choice.Reward != id+",3" {
				t.Fatal(choice)
			}
			seen[choice.StoryID] = true
		}
		for _, choice := range rules.Stories.Lucky {
			if choice.Reward != id+",6" {
				t.Fatal(choice)
			}
			seen[choice.StoryID] = true
		}
	}
	if len(ids) != 19 || len(seen) != 92 {
		t.Fatal("incomplete original regular/lucky catalogue", len(ids), len(seen))
	}
	for _, reward := range []string{"item_371,1", "item_372,1", "item_373,1", "item_001,1073741824"} {
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error { _, err := f.s.p7FreezeExpeditionEventRules(tx, reward); return err }); err == nil {
			t.Fatal("unknown special or overflow published", reward)
		}
	}
	if err := db.GetIns().Model(&db.P7ExpeditionStoryPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		rules, err := f.s.p7FreezeExpeditionEventRules(tx, "item_001,1")
		if err == nil && rules.Stories != nil {
			t.Fatal("disabled story policy changed legacy")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP7ExpeditionStoryActualAcceptLuckyNormalFrozenColdClaimAndTamper(t *testing.T) {
	for _, lucky := range []int32{0, 100} {
		t.Run(map[int32]string{0: "ordinary", 100: "lucky"}[lucky], func(t *testing.T) {
			f := p7ExpeditionSetup(t)
			for _, step := range []struct {
				model  any
				fields map[string]any
			}{
				{&db.P7ExpeditionPolicy{}, map[string]any{"reward": "item_005,3"}},
				{&db.P7ExpeditionEventPolicy{}, map[string]any{"enabled": true, "risk_percent": 0}},
				{&db.P7ExpeditionStoryPolicy{}, map[string]any{"enabled": true, "lucky_percent": lucky}},
			} {
				if err := db.GetIns().Model(step.model).Where("id=1").Updates(step.fields).Error; err != nil {
					t.Fatal(err)
				}
			}
			at := time.Now().Add(-13 * time.Hour).Truncate(time.Millisecond)
			offers := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
			chosen := offers.RefreshedMissions[0]
			if chosen.GetRewards() != "item_005" || chosen.HeroReward == nil || chosen.BigReward == nil || chosen.GetHeroReward() != "" || chosen.GetBigReward() != "" {
				t.Fatal("original ID-only preview/null Split contract violated", chosen)
			}
			_, game := f.profile(t)
			idlePreview := p7ExpeditionCall(t, f, &pb.CSGetExpeditionIdleHeros{MissionSerial: chosen.MissionSerial}, 0, at).(*pb.SCGetExpeditionIdleHeros)
			if idlePreview.GetPossibleReward() != "item_005" || idlePreview.BigReward == nil || idlePreview.GetBigReward() != "" {
				t.Fatal("original picker ID-only preview/null Split contract violated", idlePreview)
			}
			p7ExpeditionCall(t, f, &pb.CSAcceptMission{MissionSerial: chosen.MissionSerial, HeroGuids: []string{game.Warriors[1].GetGuid()}}, 61, at)
			var mission db.P7ExpeditionMission
			if err := db.GetIns().Where("serial=?", chosen.GetMissionSerial()).First(&mission).Error; err != nil {
				t.Fatal(err)
			}
			q, err := p7ReadExpeditionQuote(mission)
			if err != nil {
				t.Fatal(err)
			}
			if got := p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: chosen.MissionSerial}, 0, at.Add(time.Hour-time.Millisecond)).(*pb.SCExpeditionMissionDetail); len(got.Events) != 0 {
				t.Fatal("future story leaked")
			}
			kind, quantity := constUtil.P7ExpeditionNormalEvent, int32(3)
			if lucky == 100 {
				kind, quantity = constUtil.P7ExpeditionLuckyEvent, 6
			}
			first := p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: chosen.MissionSerial}, 0, at.Add(time.Hour)).(*pb.SCExpeditionMissionDetail)
			if len(first.Events) != 1 || first.Events[0].GetEventType() != kind || first.Events[0].GetReward() != map[int32]string{0: "item_005,3", 100: "item_005,6"}[lucky] || f.s.table("ExpeditionStory", "storyId", first.Events[0].GetStoryId())["rewardId"] != "item_005" {
				t.Fatal("original kind/story/reward mismatch", first)
			}
			var event db.P7ExpeditionEvent
			if err := db.GetIns().Where("mission_serial=?", mission.Serial).First(&event).Error; err != nil {
				t.Fatal(err)
			}
			original := event
			event.StoryID = "stroy_001"
			if err := p7SaveExpeditionEvent(db.GetIns(), &event, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSGetFinalReward{MissionSerial: chosen.MissionSerial}, 62, time.UnixMilli(mission.EndAt)); err == nil {
				t.Fatal("rehashed different resource story accepted")
			}
			if err := p7SaveExpeditionEvent(db.GetIns(), &original, false); err != nil {
				t.Fatal(err)
			}
			// Even a recomputed row hash cannot enable an outcome excluded by
			// the task's frozen 0% or 100% policy boundary.
			wrong := original
			choices := q.Events.Stories.Lucky
			wrong.Kind = constUtil.P7ExpeditionLuckyEvent
			if lucky == 100 {
				choices = q.Events.Stories.Ordinary
				wrong.Kind = constUtil.P7ExpeditionNormalEvent
			}
			wrong.StoryID, wrong.Reward = choices[0].StoryID, choices[0].Reward
			if err := p7SaveExpeditionEvent(db.GetIns(), &wrong, false); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSGetFinalReward{MissionSerial: chosen.MissionSerial}, 62, time.UnixMilli(mission.EndAt)); err == nil {
				t.Fatal("frozen excluded probability boundary accepted")
			}
			if err := p7SaveExpeditionEvent(db.GetIns(), &original, false); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P7ExpeditionStoryPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "lucky_percent": 100 - lucky, "source": "closed future"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "risk_percent": 100}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P7ExpeditionPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "reward": "diamond,999"}).Error; err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			if err := f.s.InitializeP7Expedition(f.ctx); err != nil {
				t.Fatal(err)
			}
			var offersBefore int64
			if err := db.GetIns().Model(&db.P7ExpeditionMission{}).Where("player_id=? AND state=?", f.id, constUtil.P7ExpeditionOffered).Count(&offersBefore).Error; err != nil {
				t.Fatal(err)
			}
			end := time.UnixMilli(mission.EndAt)
			p7ExpeditionCall(t, f, &pb.CSInterExpedition{}, 0, end)
			accepted := p7ExpeditionCall(t, f, &pb.CSGetAcceptedMissions{}, 0, end).(*pb.SCGetAcceptedMissions)
			if len(accepted.AcceptedMissions) != 1 {
				t.Fatal("closed task lost accepted UI recovery")
			}
			p7ExpeditionCall(t, f, &pb.CSGetExpeditionIdleHeros{MissionSerial: chosen.MissionSerial}, 0, end)
			detail := p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: chosen.MissionSerial}, 0, end).(*pb.SCExpeditionMissionDetail)
			if len(detail.Events) != int(q.Hours)-1 {
				t.Fatal("closed task lost frozen detail")
			}
			var offersAfter int64
			if err := db.GetIns().Model(&db.P7ExpeditionMission{}).Where("player_id=? AND state=?", f.id, constUtil.P7ExpeditionOffered).Count(&offersAfter).Error; err != nil || offersAfter != offersBefore {
				t.Fatal("recovery read published offers", err)
			}
			if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSGetRefreshedMissions{}, 0, end); err == nil {
				t.Fatal("closed policy allowed new offer UI")
			}
			if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSExpeditionMissionDetail{MissionSerial: proto.String("unowned")}, 0, end); err == nil {
				t.Fatal("closed recovery allowed unowned task")
			}
			request := &pb.CSGetFinalReward{MissionSerial: chosen.MissionSerial}
			claim := p7ExpeditionCall(t, f, request, 62, time.UnixMilli(mission.EndAt)).(*pb.SCGetFinalReward)
			player, game := f.profile(t)
			if simpleOwned(player, game, "item_005") != int64(3+quantity*(q.Hours-1)) || claim.GetExp() != q.Exp {
				t.Fatal("actual deferred frozen story resources missing", claim)
			}
			for i := 0; i < 8; i++ {
				if !proto.Equal(claim, p7ExpeditionCall(t, f, request, 62, time.UnixMilli(mission.EndAt+1000))) {
					t.Fatal("replayed story reward differs")
				}
			}
			p2, g2 := f.profile(t)
			if !proto.Equal(player, p2) || simpleOwned(p2, g2, "item_005") != simpleOwned(player, game, "item_005") {
				t.Fatal("retry duplicated item")
			}
			if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "expedition_complete") != 1 {
				t.Fatal("successful story final fact duplicate")
			}
		})
	}
}
