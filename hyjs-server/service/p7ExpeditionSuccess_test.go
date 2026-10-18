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

func TestP7ExpeditionOriginalSuccessFormulaMissingSlotsTalentAndSpecialty(t *testing.T) {
	for _, test := range []struct {
		name                string
		kind                string
		difficulty, maximum int32
		heroes              []p7ExpeditionSuccessHero
		want                float32
	}{
		{"empty", "hunt", 0, 1, nil, 0},
		{"minimum", "hunt", 0, 1, []p7ExpeditionSuccessHero{{GUID: "a", HeroID: "hero_001", Level: 1}}, .1},
		{"matching", "hunt", 0, 1, []p7ExpeditionSuccessHero{{GUID: "a", HeroID: "hero_001", Level: 1, Expertise: "guard,hunt"}}, .51},
		{"talent", "guard", 10, 1, []p7ExpeditionSuccessHero{{GUID: "a", HeroID: "hero_001", Level: 20, Talent: 2}}, .2},
		{"missing-slot", "hunt", 30, 2, []p7ExpeditionSuccessHero{{GUID: "a", HeroID: "hero_001", Level: 100, Expertise: "hunt"}}, .6},
		{"maximum", "hunt", 0, 1, []p7ExpeditionSuccessHero{{GUID: "a", HeroID: "hero_001", Level: 100, Talent: 2}}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := p7ExpeditionSuccessChance(test.kind, test.difficulty, test.maximum, test.heroes)
			if err != nil || math.Abs(float64(got-test.want)) > 1e-6 {
				t.Fatal("原客户端成功率公式", got, test.want, err)
			}
		})
	}
}

// 此用例读取真正Accept抽到的一次结果，不注入预设成败；用有界时钟完成短代表。
func TestP7ExpeditionSuccessFreezesAtAcceptAndColdClaimOnce(t *testing.T) {
	f := p7ExpeditionSetup(t)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Warriors[1].Level = proto.Int32(1)
		g.Warriors[1].Talent = proto.Int32(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	offer := list.GetRefreshedMissions()[0]
	_, game := f.profile(t)
	request := &pb.CSAcceptMission{MissionSerial: offer.MissionSerial, HeroGuids: []string{game.Warriors[1].GetGuid()}}
	p7ExpeditionCall(t, f, request, 31301, at)
	var mission db.P7ExpeditionMission
	if err := db.GetIns().Where("serial=?", offer.GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	q, err := p7ReadExpeditionQuote(mission)
	if err != nil || q.Success == nil || q.Success.Heroes[0].Level != 1 {
		t.Fatal("接受时未冻结实际贡献", err)
	}
	frozen := append([]byte(nil), mission.Quote...)
	before, _ := f.profile(t)
	p7ExpeditionCall(t, f, request, 31301, at)
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("接受重试重复扣成本")
	}
	// 修改未来配置不改变已有任务；当前伙伴仍由原派遣占用保护。
	f.s.table("ExpeditionLevel", "level", "D")["difficulty"] = "99"
	f.restart(t)
	finish := time.UnixMilli(mission.EndAt)
	claim := &pb.CSGetFinalReward{MissionSerial: offer.MissionSerial}
	result := p7ExpeditionCall(t, f, claim, 31302, finish).(*pb.SCGetFinalReward)
	wantExp := q.FailExp
	if q.Success.Succeeded {
		wantExp = q.Exp
	}
	if result.GetExp() != wantExp || (len(result.GetGet().GetSimps()) > 0) != q.Success.Succeeded {
		t.Fatal("领奖未使用冻结的一次成败", q.Success, result)
	}
	if !proto.Equal(result, p7ExpeditionCall(t, f, claim, 31302, finish)) {
		t.Fatal("冷领奖回执漂移")
	}
	var stored db.P7ExpeditionMission
	if err = db.GetIns().Where("serial=?", mission.Serial).First(&stored).Error; err != nil || string(stored.Quote) != string(frozen) {
		t.Fatal("领奖改写冻结概率", err)
	}
	if !q.Success.Succeeded {
		detail := p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: offer.MissionSerial}, 0, finish).(*pb.SCExpeditionMissionDetail)
		if len(detail.GetEvents()) != 1 || detail.GetEvents()[0].GetStoryId() != "stroy_105" {
			t.Fatal("基础失败未返回原失败故事", detail)
		}
	}
}

func TestP7ExpeditionLegacyOfferKeepsNilSuccessContract(t *testing.T) {
	f := p7ExpeditionSetup(t)
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	offer := list.GetRefreshedMissions()[0]
	var mission db.P7ExpeditionMission
	if err := db.GetIns().Where("serial=?", offer.GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	q, err := p7ReadExpeditionQuote(mission)
	if err != nil {
		t.Fatal(err)
	}
	q.SuccessRule = ""
	mission.Quote, _ = json.Marshal(q)
	mission.QuoteHash = p6Digest(mission.Quote)
	if err = db.GetIns().Save(&mission).Error; err != nil {
		t.Fatal(err)
	}
	_, game := f.profile(t)
	p7ExpeditionCall(t, f, &pb.CSAcceptMission{MissionSerial: offer.MissionSerial, HeroGuids: []string{game.Warriors[1].GetGuid()}}, 31311, at)
	if err = db.GetIns().Where("serial=?", offer.GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	q, err = p7ReadExpeditionQuote(mission)
	if err != nil || q.Success != nil {
		t.Fatal("旧报价被改成新随机成败", err)
	}
	result := p7ExpeditionCall(t, f, &pb.CSGetFinalReward{MissionSerial: offer.MissionSerial}, 31312, time.UnixMilli(mission.EndAt)).(*pb.SCGetFinalReward)
	if result.GetExp() != q.Exp || len(result.GetGet().GetSimps()) != 1 {
		t.Fatal("旧任务基础成败合同漂移", result)
	}
	if p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 0 {
		t.Fatal("完成未释放伙伴")
	}
}
