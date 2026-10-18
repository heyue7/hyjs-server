package service

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 沿现有原D任务/实际闲置伙伴域前提，不更改原时长、事件结果或奖励。
func expeditionCalendarSetup(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7ExpeditionSetup(t)
	// SQLite首角色可能为保护ID1，工具用另建真实demo身份；有限32级/100耐力/1000钻沿原远征域前提。
	account, err := LoginAccount(f.ctx, "demo-expedition-calendar-"+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "远征个人日期回归", "hero_047"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(32)
		p.Energy = proto.Int32(100)
		p.Diamond = proto.Int64(1000)
		_, e := f.s.GrantFixedReward(p, g, "hero_001,1;hero_025,1")
		// 新demo账号须沿用p7ExpeditionSetup的有限成熟伙伴前提。
		// 本组隔离日历/占用/防重，真实成功率及失败分支由专属用例覆盖。
		for _, hero := range g.Warriors {
			if hero.GetGuid() != p.GetHeroWarId() {
				hero.Level = proto.Int32(100)
				hero.Talent = proto.Int32(2)
			}
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	// 共用MySQL夹具可能保留上一例发布开关；本例先还原默认无事件投放。
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	return f
}

func expeditionCalendarEnable(t *testing.T, f *p1Fixture) *db.Profile {
	t.Helper()
	for _, fn := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities} {
		if err := fn(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("uid", fmt.Sprintf("demo-expedition-calendar-%d", f.id)).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = true
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletAPIContextKey, true)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), owner.Session...))
	localClockEnable(t)
	f.ctx = localClockBind(t, f, &owner)
	return &owner
}

func expeditionCalendarChange(t *testing.T, f *p1Fixture, owner *db.Profile, action, key string) {
	t.Helper()
	if _, err := f.s.LocalPlayTimeChange(f.ctx, f.id, action, key); err != nil {
		t.Fatal(err)
	}
	f.ctx = localClockBind(t, f, owner)
}

func expeditionCalendarAccept(t *testing.T, f *p1Fixture, at time.Time, stamp int64) db.P7ExpeditionMission {
	t.Helper()
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	_, game := f.profile(t)
	req := &pb.CSAcceptMission{MissionSerial: list.RefreshedMissions[0].MissionSerial, HeroGuids: []string{game.Warriors[1].GetGuid()}}
	p7ExpeditionCall(t, f, req, stamp, at)
	var mission db.P7ExpeditionMission
	if err := db.GetIns().First(&mission, "serial=?", req.GetMissionSerial()).Error; err != nil {
		t.Fatal(err)
	}
	return mission
}

func TestLocalPlayExpeditionCalendarPaidRefreshRevisitAndCold(t *testing.T) {
	f := expeditionCalendarSetup(t)
	owner := expeditionCalendarEnable(t, f)
	at := time.Now().Add(-time.Minute)
	request := &pb.CSRefreshMissionList{}
	for day := 0; day < 2; day++ {
		if day > 0 {
			expeditionCalendarChange(t, f, owner, "nextDay", "exp-refresh-next")
		}
		for n := int64(1); n <= 3; n++ {
			out := p7ExpeditionCall(t, f, request, int64(day)*100+n, at).(*pb.SCRefreshMissionList)
			if n < 3 && out.GetCost().GetNum() != 0 || n == 3 && (out.GetCost().GetId() != "diamond" || out.GetCost().GetNum() != 20) {
				t.Fatal("original daily free/paid refresh", out)
			}
		}
	}
	expeditionCalendarChange(t, f, owner, "reset", "exp-refresh-reset")
	out := p7ExpeditionCall(t, f, request, 201, at).(*pb.SCRefreshMissionList)
	if out.GetCost().GetNum() != 20 || out.GetFreeRefreshTime() != 0 {
		t.Fatal("rewind washed paid day", out)
	}
	f.restart(t)
	again := p7ExpeditionCall(t, f, request, 201, at)
	if !proto.Equal(out, again) {
		t.Fatal("cold receipt rerolled offers")
	}
	expeditionCalendarChange(t, f, owner, "nextDay", "exp-refresh-revisit")
	inter := p7ExpeditionCall(t, f, &pb.CSInterExpedition{}, 0, at).(*pb.SCInterExpedition)
	player, _ := f.profile(t)
	if inter.GetFreeRefreshTime() != 0 || player.GetDiamond() != 940 {
		t.Fatal("revisit budget or duplicate spend", inter, player.GetDiamond())
	}
	var facts []db.EventFact
	if err := db.GetIns().Where("player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent).Find(&facts).Error; err != nil {
		t.Fatal(err)
	}
	days := map[string]int64{}
	for _, fact := range facts {
		days[fact.Day] += fact.Amount
	}
	if len(facts) != 3 || days[p1Day(at)] != 40 || days[p1Day(at.AddDate(0, 0, 1))] != 20 {
		t.Fatal("refresh P1 current personal day", days)
	}
}

func TestLocalPlayExpeditionCalendarAcceptedFreezeRealCompatibilityAndOnce(t *testing.T) {
	f := expeditionCalendarSetup(t)
	// 未初始化新表时已接旧任务，之后工具开启也不能补写旧Quote或冒认新时钟。
	at := time.Now().Add(-13 * time.Hour).Truncate(time.Millisecond)
	legacy := expeditionCalendarAccept(t, f, at, 301)
	owner := expeditionCalendarEnable(t, f)
	expeditionCalendarChange(t, f, owner, "nextDay", "exp-legacy-next")
	view, err := f.s.p7ProjectExpeditionCalendar(db.GetIns().WithContext(f.ctx), legacy, at)
	if err != nil || view.GetState() != constUtil.P7ExpeditionRunning || view.GetEndTime() != legacy.EndAt+24*time.Hour.Milliseconds() {
		t.Fatal("legacy real remaining/UI axis", view, err)
	}
	if _, err = f.s.p7Expedition(f.ctx, f.id, &pb.CSGetFinalReward{MissionSerial: proto.String(legacy.Serial)}, 302, at); err == nil {
		t.Fatal("calendar expired legacy task")
	}
	if p1Count(t, &db.LocalPlayActivityOperation{}, "player_id=? AND request_id=?", f.id, constUtil.LocalPlayExpeditionClockPrefix+legacy.Serial) != 0 {
		t.Fatal("legacy clock backfilled")
	}
	// 领取旧任务只推进有界真实测试时钟，旧任务仍走原真实到期。
	p7ExpeditionCall(t, f, &pb.CSGetFinalReward{MissionSerial: proto.String(legacy.Serial)}, 302, time.UnixMilli(legacy.EndAt))
	// 新任务在+1接受；再次+1快进，到期/账本不重写原报价或真实时间。
	mission := expeditionCalendarAccept(t, f, at, 303)
	quote := append([]byte(nil), mission.Quote...)
	clock, err := f.s.expeditionClock(db.GetIns().WithContext(f.ctx), mission)
	if err != nil || clock == nil || clock.AcceptedOffset != 1 || clock.StartAt != at.UnixMilli() {
		t.Fatal("accepted immutable clock", clock, err)
	}
	early := &pb.CSGetFinalReward{MissionSerial: proto.String(mission.Serial)}
	if _, err = f.s.p7Expedition(f.ctx, f.id, early, 304, at); err == nil {
		t.Fatal("fresh +1 accepted task immediately ready")
	}
	expeditionCalendarChange(t, f, owner, "nextDay", "exp-new-fast-forward")
	var wg sync.WaitGroup
	replies := make(chan proto.Message, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.s.p7Expedition(f.ctx, f.id, early, 304, at)
			if e == nil {
				replies <- r[0]
			}
			failures <- e
		}()
	}
	wg.Wait()
	close(replies)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first proto.Message
	for r := range replies {
		if first == nil {
			first = r
		} else if !proto.Equal(first, r) {
			t.Fatal("parallel claim differs")
		}
	}
	player, game := f.profile(t)
	if simpleOwned(player, game, "item_001") != 2 || p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 0 {
		t.Fatal("reward/held hero multiplicity")
	}
	expeditionCalendarChange(t, f, owner, "reset", "exp-new-rewind")
	f.restart(t)
	if !proto.Equal(first, p7ExpeditionCall(t, f, early, 305, at)) {
		t.Fatal("cold rewind replay changed result")
	}
	var stored db.P7ExpeditionMission
	if err = db.GetIns().First(&stored, "serial=?", mission.Serial).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored.Quote, quote) || stored.QuoteHash != mission.QuoteHash || stored.StartAt != mission.StartAt || stored.EndAt != mission.EndAt {
		t.Fatal("calendar rewrote original task bytes")
	}
	var fact db.EventFact
	if err = db.GetIns().First(&fact, "player_id=? AND source_key=? AND kind=?", f.id, "exp-finish:"+mission.Serial, "expedition_complete").Error; err != nil || fact.Day != p1Day(at.AddDate(0, 0, 2)) {
		t.Fatal("completion fact is not completion personal day", fact, err)
	}
	// 冻结来源不能跨serial/quote/real轴搬用。
	altered := mission
	altered.EndAt++
	if _, err = f.s.p7ExpeditionMissionNow(db.GetIns(), altered, at); err == nil {
		t.Fatal("changed real axis accepted")
	}
}

func TestLocalPlayExpeditionCalendarHelpUsesOwnerAndOriginalEventAxis(t *testing.T) {
	f := expeditionCalendarSetup(t)
	owner := expeditionCalendarEnable(t, f)
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "risk_percent": 100}).Error; err != nil {
		t.Fatal(err)
	}
	helper, err := LoginAccount(f.ctx, "exp-calendar-helper-"+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, helper.Id, "远征帮助时钟隔离", "hero_047"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-25 * time.Hour).Truncate(time.Millisecond)
	mission := expeditionCalendarAccept(t, f, at, 401)
	// 模拟已认证帮助者自己的个人日，与任务主人没有共享偏移。
	helperCtx := context.WithValue(f.ctx, constUtil.LocalPlayOwnerKey, helper.Id)
	helperCtx = context.WithValue(helperCtx, constUtil.LocalPlayOffsetDaysKey, int64(1000))
	list := func(real time.Time) []P7ExpeditionPendingHelp {
		t.Helper()
		var out []P7ExpeditionPendingHelp
		if e := db.GetIns().WithContext(helperCtx).Transaction(func(tx *gorm.DB) error {
			var e error
			out, e = f.s.ListP7ExpeditionHelp(tx, []int64{f.id}, real)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		return out
	}
	if got := list(at.Add(time.Hour)); len(got) != 1 {
		t.Fatal("helper offset leaked into owner visibility", got)
	}
	expeditionCalendarChange(t, f, owner, "previousDay", "exp-owner-rewind")
	// 原真实End已过，但主人回拨后的有效任务仅到第1小时，不可被real SQL过滤。
	now := at.Add(25 * time.Hour)
	pending := list(now)
	if len(pending) != 1 {
		t.Fatal("owner clock task filtered by real deadline", pending)
	}
	if err = db.GetIns().WithContext(helperCtx).Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ApplyP7ExpeditionHelp(tx, helper.Id, f.id, mission.Serial, pending[0].EventSerial, now)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var resolved db.P7ExpeditionEvent
	if err = db.GetIns().First(&resolved, "serial=?", pending[0].EventSerial).Error; err != nil || resolved.ResolvedAt != at.Add(time.Hour).UnixMilli() || resolved.HelperID != helper.Id {
		t.Fatal("resolved event must remain on frozen real axis", resolved, err)
	}
	if _, err = f.s.p7ExpeditionEvents(db.GetIns().WithContext(helperCtx), mission, now); err != nil {
		t.Fatal("strict original event invariants rejected own help", err)
	}
	expeditionCalendarChange(t, f, owner, "reset", "exp-owner-due")
	if got := list(now); len(got) != 0 {
		t.Fatal("expired owner still allows help", got)
	}
	if err = db.GetIns().WithContext(helperCtx).Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ApplyP7ExpeditionHelp(tx, helper.Id, f.id, mission.Serial, pending[0].EventSerial, now)
		return e
	}); err == nil {
		t.Fatal("expired help mutation accepted")
	}
	// 同实例/原报价再读，无当前offset补写或方法级重算。
	f.restart(t)
	c, err := f.s.expeditionClock(db.GetIns(), mission)
	if err != nil || c == nil || c.AcceptedOffset != 0 {
		t.Fatal("cold immutable clock", c, err)
	}
}

func TestLocalPlayExpeditionCalendarFastForwardRiskFailureKeepsStrictLedger(t *testing.T) {
	f := expeditionCalendarSetup(t)
	owner := expeditionCalendarEnable(t, f)
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "risk_percent": 100}).Error; err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	mission := expeditionCalendarAccept(t, f, at, 501)
	expeditionCalendarChange(t, f, owner, "nextDay", "exp-risk-fast-forward")
	claim := &pb.CSGetFinalReward{MissionSerial: proto.String(mission.Serial)}
	out := p7ExpeditionCall(t, f, claim, 502, at).(*pb.SCGetFinalReward)
	quote, err := p7ReadExpeditionQuote(mission)
	if err != nil || out.GetExp() != quote.FailExp || len(out.GetGet().GetSimps()) != 0 {
		t.Fatal("unresolved original risk manufactured success", out, err)
	}
	var stored db.P7ExpeditionMission
	if err = db.GetIns().First(&stored, "serial=?", mission.Serial).Error; err != nil {
		t.Fatal(err)
	}
	events, err := f.s.p7ExpeditionEvents(db.GetIns().WithContext(f.ctx), stored, at)
	if err != nil || len(events) != int(quote.Hours) {
		t.Fatal("calendar failure broke strict event chain", events, err)
	}
	last := events[len(events)-1]
	if last.Kind != constUtil.P7ExpeditionFailEvent || last.ResolvedAt != at.AddDate(0, 0, 1).UnixMilli() {
		t.Fatal("failure resolution real-axis clock", last)
	}
	expeditionCalendarChange(t, f, owner, "reset", "exp-risk-rewind")
	f.restart(t)
	if !proto.Equal(out, p7ExpeditionCall(t, f, claim, 503, at)) {
		t.Fatal("risk failure cold replay")
	}
}
