package service

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestLocalPlayRushCalendarRealResetCostsRevisitPendingAndCold(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	// 原fixture首号是受保护账号1；工具使用独立正常创建的demo角色。
	account, err := LoginAccount(f.ctx, "demo-rush-calendar-"+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "个人试炼日期", "hero_047"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Diamond = proto.Int32(22), proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.formationList(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("uid", fmt.Sprintf("demo-rush-calendar-%d", f.id)).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = true
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletAPIContextKey, true)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), owner.Session...))
	base := f.ctx
	localClockEnable(t)
	f.ctx = localClockBind(t, f, &owner)
	change := func(action, key string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(f.ctx, f.id, action, key); err != nil {
			t.Fatal(err)
		}
		f.ctx = base
		f.ctx = localClockBind(t, f, &owner)
	}
	read := func() db.P7RushState {
		t.Helper()
		if _, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, time.Now()); err != nil {
			t.Fatal(err)
		}
		var state db.P7RushState
		if err := db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil {
			t.Fatal(err)
		}
		return state
	}
	before, _ := f.profile(t)
	var firstReset proto.Message
	for run := 0; run < 3; run++ {
		for floor := int32(1); floor <= 3; floor++ {
			p7SectionPlayFloor(t, f, floor, 1, 705000+int64(run)*100+int64(floor))
		}
		if run == 2 {
			change("nextDay", "rush-calendar-next")
		}
		var group sync.WaitGroup
		var results [8][]proto.Message
		var failures [8]error
		for i := range results {
			group.Add(1)
			go func(index int) {
				defer group.Done()
				results[index], failures[index] = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 705050+int64(run)*100)
			}(i)
		}
		group.Wait()
		for i := range results {
			if failures[i] != nil || len(results[i]) != 1 || !proto.Equal(results[0][0], results[i][0]) {
				t.Fatal("same-stamp reset once", run, i, failures[i])
			}
		}
		if run == 0 {
			firstReset = results[0][0]
		}
		cost := results[0][0].(*pb.SCRushResetChallengeCount).GetCost()
		if run == 1 {
			if len(cost) != 1 || cost[0].GetId() != "diamond" || cost[0].GetNum() != 50 {
				t.Fatal("original paid reset quote", cost)
			}
		} else if len(cost) != 0 {
			t.Fatal("first free reset of personal date charged", run, cost)
		}
	}
	tomorrow := read()
	if tomorrow.RunID != 4 || tomorrow.Floor != 1 || tomorrow.FreeResetUsed != 1 || tomorrow.PaidResetUsed != 0 {
		t.Fatal("new personal day quota or run", tomorrow)
	}
	change("previousDay", "rush-calendar-back")
	yesterday := read()
	if yesterday.FreeResetUsed != 1 || yesterday.PaidResetUsed != 1 || yesterday.RunID != tomorrow.RunID || yesterday.Floor != tomorrow.Floor {
		t.Fatal("rewind restored quota or rewound earned run", yesterday)
	}
	// 切日不改变真实pending；回原日后必须仍续同一个冻结战斗。
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(1)}, 705400, false)
	if err != nil {
		t.Fatal(err)
	}
	change("nextDay", "rush-calendar-pending-next")
	resumed, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(1)}, 705401, false)
	if err != nil || !proto.Equal(start, resumed) {
		t.Fatal("personal day changed frozen pending", err)
	}
	f.restart(t)
	f.ctx = base
	f.ctx = localClockBind(t, f, &owner)
	cold := read()
	if cold.FreeResetUsed != 1 || cold.PaidResetUsed != 0 || cold.ActiveFight != start.GetFightId() || cold.RunID != 4 {
		t.Fatal("cold lost current day or pending", cold)
	}
	current, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil || len(current[0].(*pb.SCRushCurrentInfo).GetTaskInfoList()) != 1 || current[0].(*pb.SCRushCurrentInfo).GetTaskInfoList()[0].GetStar() != 1 {
		t.Fatal("cold original Ready must expose only the frozen star", err)
	}
	if _, err = f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(2)}, 705402, false); err == nil {
		t.Fatal("different star replaced paid frozen first-section fight")
	}
	resumed, err = f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(1)}, 705403, false)
	if err != nil || !proto.Equal(start, resumed) {
		t.Fatal("cold first-section Start cannot resume same frozen fight", err)
	}
	after, game := f.profile(t)
	replay, err := f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 705050)
	if err != nil || !proto.Equal(replay[0], firstReset) {
		t.Fatal("old accepted reset receipt cannot recover during new pending", err)
	}
	still, stillGame := f.profile(t)
	if !proto.Equal(after, still) || !reflect.DeepEqual(game, stillGame) || still.GetDiamond() != before.GetDiamond()-50 {
		t.Fatal("cold receipt changed inventory or original paid cost")
	}
	change("previousDay", "rush-calendar-pending-back")
	returned := read()
	if returned.FreeResetUsed != 1 || returned.PaidResetUsed != 1 || returned.ActiveFight != start.GetFightId() {
		t.Fatal("old day quota disappeared during pending", returned)
	}
}
