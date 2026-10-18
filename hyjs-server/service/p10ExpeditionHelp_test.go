package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP10ExpeditionHelpOriginalVIPQuote 先核13档真实权益与不可改写报价；实际事件消费另由P7联动验证。
func TestP10ExpeditionHelpOriginalVIPQuoteAll13SameDayUpgradeColdSHA(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeP10ExpeditionHelp(f.ctx); err != nil {
		t.Fatal(err)
	}
	var policy db.P10ExpeditionHelpPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil || policy.Enabled {
		t.Fatal("independent helper policy default", err)
	}
	day := p1Day(time.Now())
	quote, err := f.s.p10ExpeditionHelpQuote(db.GetIns(), f.id, day, false)
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.GetIns().Model(&db.P10ExpeditionHelpDaily{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("readonly query froze day", err, count)
	}
	limits := []int64{10, 15, 15, 15, 20, 20, 25, 30, 30, 30, 30, 30, 30}
	for vip, wanted := range limits {
		actual, err := p10ExpeditionHelpLimit(quote, int32(vip))
		if err != nil || actual != wanted {
			t.Fatal("original all VIP helper rights", vip, actual, wanted, err)
		}
	}
	quote, err = f.s.p10ExpeditionHelpQuote(db.GetIns(), f.id, day, true)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Reward != "diamond,5" {
		t.Fatal("original five diamonds changed")
	}
	originalHash := quote.SourceHash
	f.open(t)
	if err = f.s.InitializeP10ExpeditionHelp(f.ctx); err != nil {
		t.Fatal(err)
	}
	quote, err = f.s.p10ExpeditionHelpQuote(db.GetIns(), f.id, day, false)
	if err != nil || quote.SourceHash != originalHash {
		t.Fatal("cold original source", err)
	}
	// 当天升级VIP只读取冻结目录中的新档；已经使用的次数仍由实际救援事件事实决定。
	lo, err := p10ExpeditionHelpLimit(quote, 0)
	if err != nil {
		t.Fatal(err)
	}
	hi, err := p10ExpeditionHelpLimit(quote, 12)
	if err != nil || lo != 10 || hi != 30 {
		t.Fatal("same day actual VIP upgrade locked to ten", lo, hi, err)
	}
	if err = db.GetIns().Model(&db.P10ExpeditionHelpDaily{}).Where("player_id=? AND day=?", f.id, day).Update("vip_limits", []byte(`{"0":30}`)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p10ExpeditionHelpQuote(db.GetIns(), f.id, day, false); err == nil {
		t.Fatal("VIP blob tamper ignored by source hash")
	}
}

// TestP10ExpeditionHelpNativePartnerPreparation 只为专库原HTTP已建角色准备32级与一名派遣伙伴，不准备帮助钻石或风险事件。
func TestP10ExpeditionHelpNativePartnerPreparation(t *testing.T) {
	raw := os.Getenv("HYJS_P10_HELP_NATIVE_IDS")
	if raw == "" {
		t.Skip("显式原帧有限准备")
	}
	name := os.Getenv("HYJS_P10_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p10_") {
		t.Fatal("helper native preparation isolation guard")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_P10_MYSQL_HOST"), User: os.Getenv("HYJS_P10_MYSQL_USER"), Password: os.Getenv("HYJS_P10_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	defer sql.Close()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeP1(ctx); err != nil {
		t.Fatal(err)
	}
	for _, value := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			t.Fatal("invalid actual created preparation id")
		}
		if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(32)
			for _, hero := range g.Warriors {
				if hero.GetId() == "hero_001" {
					return nil
				}
			}
			_, err := s.GrantFixedReward(p, g, "hero_001,1")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestP10ExpeditionHelpActual 全部风险由真实AcceptMission在冻结本地发布源下生成，帮助者未准备任何钻石。
func TestP10ExpeditionHelpActualNaturalHourlyVIPTenRaceOnceDiamondSpentColdDay(t *testing.T) {
	f, b, c := p10Setup(t)
	if err := f.s.LoadP7ExpeditionTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10ExpeditionHelpAction{}, &db.P10ExpeditionHelpDaily{}, &db.P7ExpeditionEvent{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P7ExpeditionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "risk_percent": 100, "help_reward": "energy,2", "source": "隔离实际小时厄运100%；原6风险剧情/当地energy2冻结；不伪造已生产事件"}).Error; err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600))
	// 所有写入均在真实过去，前3小时也保持同一北京日；不放宽P1未来事实护栏。
	at := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).Add(-24 * time.Hour)
	owners := []int64{b, c}
	for i := 0; i < 3; i++ {
		a, err := LoginAccount(f.ctx, fmt.Sprintf("p10-help-owner-%d-%d", i, time.Now().UnixNano()), "isolated-help-test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, a.Id, fmt.Sprintf("帮助真实%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		owners = append(owners, a.Id)
	}
	for _, id := range append([]int64{f.id}, owners...) {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(32)
			if id != f.id {
				_, err := f.s.GrantFixedReward(p, g, "hero_001,1")
				return err
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := f.profile(t)
	if before.GetDiamond() != 0 {
		t.Fatal("helper diamonds were prepared", before.GetDiamond())
	}
	for _, owner := range owners {
		p10Call(t, f, f.id, &pb.CSApplyFriend{FriendId: proto.Int64(owner), Word: proto.String("真实风险联动")}, owner+100, at)
		p10Call(t, f, owner, &pb.CSAgreeApply{FriendId: proto.Int64(f.id)}, owner+200, at)
		messages, err := f.s.p7Expedition(f.ctx, owner, &pb.CSGetRefreshedMissions{}, 0, at)
		if err != nil {
			t.Fatal(err)
		}
		offer := messages[0].(*pb.SCGetRefreshedMissions).RefreshedMissions[0]
		idle, err := f.s.p7Expedition(f.ctx, owner, &pb.CSGetExpeditionIdleHeros{MissionSerial: offer.MissionSerial}, 0, at)
		if err != nil {
			t.Fatal(err)
		}
		guids := idle[0].(*pb.SCGetExpeditionIdleHeros).CanSendGuids
		if len(guids) == 0 {
			t.Fatal("real partner unavailable")
		}
		if _, err = f.s.p7Expedition(f.ctx, owner, &pb.CSAcceptMission{MissionSerial: offer.MissionSerial, HeroGuids: []string{guids[0]}}, owner+300, at); err != nil {
			t.Fatal(err)
		}
	}
	current := at.Add(time.Hour)
	clock := func() time.Time { return current }
	f.ctx = context.WithValue(f.ctx, constUtil.P10ExpeditionHelpClockContextKey{}, clock)
	call := func(request proto.Message, stamp int64) ([]proto.Message, error) {
		messages, handled, err := f.s.DispatchP10ExpeditionHelp(f.ctx, f.id, request, stamp)
		if !handled {
			t.Fatal("actual helper Dispatcher missing")
		}
		return messages, err
	}
	rows, err := call(&pb.CSFriendBadEvents{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	list := rows[0].(*pb.SCFriendBadEvents)
	if list.GetTodayHelpTime() != 0 || len(list.EventList) != len(owners) {
		t.Fatal("actual hourly risk list", list)
	}
	first := list.EventList[0]
	request := &pb.CSHelpFriendEvent{FriendId: first.FriendsId, MissionSerial: first.MissionSerial, EventSerial: first.EventSerial}
	if _, err = call(request, 9000); err == nil {
		t.Fatal("closed consumer changed risk")
	}
	if err = db.GetIns().Model(&db.P10ExpeditionHelpPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = call(request, 9000); err != nil {
		t.Fatal(err)
	}
	if f.mysql.Database != "" {
		var workers sync.WaitGroup
		failures := make(chan error, 8)
		for i := 0; i < 8; i++ {
			workers.Add(1)
			go func() { defer workers.Done(); _, err := call(request, 9000); failures <- err }()
		}
		workers.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	player, _ := f.profile(t)
	if player.GetDiamond() != 5 {
		t.Fatal("natural original first5diamonds", player.GetDiamond())
	}
	var count int64
	if err = db.GetIns().Model(&db.P10ExpeditionHelpAction{}).Where("helper_id=?", f.id).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("eight receipts duplicated help", count, err)
	}
	if _, err = call(request, 9001); err == nil {
		t.Fatal("new stamp reused consumed risk")
	}
	if err = db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "risk_percent": 0, "help_reward": "energy,99"}).Error; err != nil {
		t.Fatal(err)
	}
	// 已接受的实际事件仍用原冻结源，后续停投/热改不得变为新99耐力。
	used := 1
	for hour := 1; hour <= 3; hour++ {
		current = at.Add(time.Duration(hour) * time.Hour)
		rows, err = call(&pb.CSFriendBadEvents{}, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range rows[0].(*pb.SCFriendBadEvents).EventList {
			if used == 10 {
				break
			}
			body := &pb.CSHelpFriendEvent{FriendId: event.FriendsId, MissionSerial: event.MissionSerial, EventSerial: event.EventSerial}
			if _, err = call(body, 9100+int64(used)); err != nil {
				t.Fatal(err)
			}
			used++
		}
	}
	if used != 10 {
		t.Fatal("natural source did not reach original VIP0 ten", used)
	}
	rows, err = call(&pb.CSFriendBadEvents{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	remaining := rows[0].(*pb.SCFriendBadEvents)
	if remaining.GetTodayHelpTime() != 10 || len(remaining.EventList) == 0 {
		t.Fatal("actual daily ten or further pending", remaining)
	}
	event := remaining.EventList[0]
	if _, err = call(&pb.CSHelpFriendEvent{FriendId: event.FriendsId, MissionSerial: event.MissionSerial, EventSerial: event.EventSerial}, 9500); err == nil {
		t.Fatal("VIP0 ten boundary bypassed")
	}
	player, _ = f.profile(t)
	if player.GetDiamond() != 50 {
		t.Fatal("natural ten real rewards", player.GetDiamond())
	}
	for i := 0; i < 3; i++ {
		if _, err = f.s.p7Expedition(f.ctx, f.id, &pb.CSRefreshMissionList{}, 9600+int64(i), current); err != nil {
			t.Fatal("actual natural20diamond refresh", err)
		}
	}
	player, _ = f.profile(t)
	if player.GetDiamond() != 30 {
		t.Fatal("help50 consumed original20 refresh", player.GetDiamond())
	}
	f.open(t)
	if err = f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P10ExpeditionHelpPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	rows, err = call(request, 9000)
	if err != nil || rows[1].(*pb.SCPlayer).Player.GetDiamond() != 30 {
		t.Fatal("cold old empty receipt/current wallet", err)
	}
	// 尚未领奖的owner原stroy104奖励必须保持真实冻结的energy2。
	var helps []db.P7ExpeditionEvent
	if err = db.GetIns().Where("kind=?", constUtil.P7ExpeditionHelpEvent).Find(&helps).Error; err != nil || len(helps) != 10 {
		t.Fatal("actual helper effect count", len(helps), err)
	}
	for _, help := range helps {
		if help.Reward != "energy,2" {
			t.Fatal("published source replaced frozen owner reward", help.Reward)
		}
	}
}
