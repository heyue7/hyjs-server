package service

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 长期历史采用已存在报价的有限夹具；当前派遣、到时与领奖走真实原请求。
func TestP7ExpeditionActiveMissionSurvivesLongClaimedHistory(t *testing.T) {
	f := p7ExpeditionSetup(t)
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	offer := list.GetRefreshedMissions()[0]
	_, game := f.profile(t)
	p7ExpeditionCall(t, f, &pb.CSAcceptMission{MissionSerial: offer.MissionSerial, HeroGuids: []string{game.Warriors[1].GetGuid()}}, 31101, at)
	var active db.P7ExpeditionMission
	if err := db.GetIns().Where("serial=?", offer.GetMissionSerial()).First(&active).Error; err != nil {
		t.Fatal(err)
	}
	// 旧实现按随机serial取前50项，以下历史会遮住p7e-开头的活动任务。
	var history []db.P7ExpeditionMission
	for index := 0; index < 55; index++ {
		old := active
		old.Serial = fmt.Sprintf("history-%02d", index)
		old.State = constUtil.P7ExpeditionClaimed
		old.StartAt = at.Add(-time.Duration(index+1) * 24 * time.Hour).UnixMilli()
		old.EndAt = old.StartAt + (active.EndAt - active.StartAt)
		history = append(history, old)
	}
	if err := db.GetIns().Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		at    time.Time
		state int32
	}{{at, constUtil.P7ExpeditionRunning}, {time.UnixMilli(active.EndAt), constUtil.P7ExpeditionReady}} {
		got := p7ExpeditionCall(t, f, &pb.CSGetAcceptedMissions{}, 0, step.at).(*pb.SCGetAcceptedMissions).GetAcceptedMissions()
		if len(got) != 50 || got[0].GetMissionSerial() != active.Serial || got[0].GetState() != step.state || got[1].GetMissionSerial() != "history-00" {
			t.Fatal("活动任务或近期历史在受限列表中丢失", got)
		}
	}
	request := &pb.CSGetFinalReward{MissionSerial: offer.MissionSerial}
	claimed := p7ExpeditionCall(t, f, request, 31102, time.UnixMilli(active.EndAt))
	f.restart(t)
	if !proto.Equal(claimed, p7ExpeditionCall(t, f, request, 31102, time.UnixMilli(active.EndAt))) {
		t.Fatal("近期排序改变了冷恢复领奖回执")
	}
	if count := p1Count(t, &db.P7ExpeditionMission{}, "player_id=? AND state=?", f.id, constUtil.P7ExpeditionClaimed); count != 56 {
		t.Fatal("展示限制删除了历史任务", count)
	}
}
