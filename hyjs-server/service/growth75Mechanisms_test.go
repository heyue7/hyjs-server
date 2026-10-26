package service

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestCalendar75DurationRoundTrip(t *testing.T) {
	f, id, _ := localClockFixture(t)
	if e := f.s.InitializeServerGameClock(f.ctx); e != nil {
		t.Fatal(e)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	now := time.Now().UnixMilli()
	if e := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.WantedAttTime = proto.Int64(now - 123456)
		p.WantedAttTimes = proto.Int32(4)
		if g.Recruit == nil {
			g.Recruit = map[int32]*model.RecruitState{}
		}
		g.Recruit[3] = &model.RecruitState{NextFree: now + 123456, Timestamp: 991, SinceGuarantee: 6}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	var source db.P4CommerceRule
	if e := db.GetIns().Where("kind=?", "memory").First(&source).Error; e != nil {
		t.Fatal(e)
	}
	raw := []byte(`{"day":"2026-10-27","freeAt":` + fmtInt75(now-1500) + `,"freeLeft":2,"resourceUsed":4,"bought":{"keep":3},"unknownFutureField":{"preserve":true}}`)
	state := db.P4CommerceState{PlayerID: id, RuleID: source.ID, Payload: raw}
	if e := db.GetIns().Save(&state).Error; e != nil {
		t.Fatal(e)
	}
	before, game := f.profile(t)
	beforeRecruit := *game.Recruit[3]
	c, e := f.s.AdminServerGameClock(f.ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	request := model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: c.Revision, RequestID: uuid.NewString()}
	moved, e := f.s.AdminChangeServerGameClock(f.ctx, actor, request)
	if e != nil {
		t.Fatal(e)
	}
	p, g := f.profile(t)
	if p.GetWantedAttTime() != before.GetWantedAttTime()+86400000 || p.GetWantedAttTimes() != 4 || g.Recruit[3].NextFree != beforeRecruit.NextFree+86400000 || g.Recruit[3].Timestamp != 991 {
		t.Fatal("cooldown moved without preserving state", p, g.Recruit[3])
	}
	if _, e = f.s.AdminChangeServerGameClock(f.ctx, actor, request); e != nil {
		t.Fatal(e)
	}
	p, _ = f.profile(t)
	if p.GetWantedAttTime() != before.GetWantedAttTime()+86400000 {
		t.Fatal("retry shifted cooldown twice")
	}
	_, e = f.s.AdminChangeServerGameClock(f.ctx, actor, model.ServerGameClockChangeRequest{DeltaDays: -1, Revision: moved.Revision, RequestID: uuid.NewString()})
	if e != nil {
		t.Fatal(e)
	}
	p, g = f.profile(t)
	if p.GetWantedAttTime() != before.GetWantedAttTime() || p.GetWantedAttTimes() != 4 || g.Recruit[3].NextFree != beforeRecruit.NextFree || p.GetDiamond() != before.GetDiamond() || p.GetCoin() != before.GetCoin() {
		t.Fatal("round trip changed resources or timers")
	}
	if e = db.GetIns().First(&state, "player_id=? AND rule_id=?", id, source.ID).Error; e != nil {
		t.Fatal(e)
	}
	var old, next map[string]any
	json.Unmarshal(raw, &old)
	json.Unmarshal(state.Payload, &next)
	if p6Digest(adminJSON(old)) != p6Digest(adminJSON(next)) {
		t.Fatal("memory cooldown or unknown payload fields changed", string(state.Payload))
	}
}
func fmtInt75(v int64) string { b, _ := json.Marshal(v); return string(b) }

func TestFieldRuntime75SharedDeathRuleBinding(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.InitializeFieldBossDeath73(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeSharedFieldBoss55(f.ctx); e != nil {
		t.Fatal(e)
	}
	clone := f.s.withRuntimeTables(f.s.Tables)
	boss := &pb.SideBoss{Uuid: proto.String("runtime75"), BossPosId: proto.String("runtime-pos75"), State: proto.Int32(1)}
	raw, _ := json.Marshal(boss)
	if e := db.GetIns().Create(&db.SharedFieldBoss55{ServerID: f.s.OriginalServerID(), Scene: "field_001", PosID: boss.GetBossPosId(), UUID: boss.GetUuid(), Payload: raw, HP: []byte(`{}`)}).Error; e != nil {
		t.Fatal(e)
	}
	rule := model.AdminFieldBossGroup{DeathSettlement: true}
	ruleRaw, _ := json.Marshal(rule)
	if e := db.GetIns().Transaction(func(tx *gorm.DB) error {
		bound, e := clone.freezeFieldBossDeath73(tx, "field_001", boss, map[string]string{"adminDeathRules73": string(ruleRaw)})
		if e == nil && (bound == nil || !bound.DeathSettlement) {
			t.Fatal("published death rule not bound")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
}

func TestTurntable75NoFreeCountdownLoop(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "wheel75-no-free", 80)
	key := s.p2TurntablePoolID(db.GetIns())
	now := time.Now().UnixMilli()
	if e := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", key).Updates(map[string]any{"enabled": true, "start_at": now - 1000, "end_at": now + 86400000, "reward_end_at": now + 86400000, "free_per_day": 0}).Error; e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		out, e := s.turntableRead(ctx, id, "info")
		if e != nil {
			t.Fatal(e)
		}
		info := out[0].(*pb.SCBigTurnTableInfo)
		if info.GetIsFree() != 0 || info.GetNextFreeTime() <= info.GetServerTime() {
			t.Fatal("original client would immediately requery", info)
		}
	}
}

func TestPointShop75FragmentAndSelectorTabs(t *testing.T) {
	s, ctx := newP2Test(t)
	_ = ctx
	for _, name := range []string{"Frag", "Equip", "Treasure", "ItemUse"} {
		if len(s.Tables[name]) == 0 {
			t.Fatal("missing original table", name)
		}
	}
	cases := map[string]int32{"frag_equip_025": 3, "frag_equip_041": 4, "item_119": 4, "item_339": 4, "item_008": 1}
	for id, want := range cases {
		got := s.pointShop48Priority(9876, []*pb.BaseResource{{Id: proto.String(id), Num: proto.Int32(1)}})
		if got != want*1000+876 {
			t.Fatalf("%s tab %d expected %d", id, got, want)
		}
	}
}

func TestKingRecruit75RotationExposesResolvedWorld(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "king75-rotation", 80)
	if e := s.InitializeKingRecruitWorlds(ctx); e != nil {
		t.Fatal(e)
	}
	session := []byte("world-75-session")
	if e := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("session", session).Error; e != nil {
		t.Fatal(e)
	}
	out, e := s.KingRecruitWorlds(ctx, id, session, "", "")
	if e != nil {
		t.Fatal(e)
	}
	_, space, _, e := s.kingConfiguration(db.GetIns(), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Worlds) != 1 || out.Worlds[0].World != int64(space) || space < 1 || space > 3 {
		t.Fatal("client filters world zero", out, space)
	}
}

func TestGuildBox75RealPublicationDuringCalendarRewind(t *testing.T) {
	f, ids, now := p13GuildDonateBoxSetup(t)
	p13GuildDonateBoxEnable(t, now)
	ctx := context.WithValue(f.ctx, constUtil.ServerGameClockBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockRevisionKey, int64(9))
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, int64(-9))
	d := db.GetIns().WithContext(ctx)
	virtual := now.AddDate(0, 0, -9)
	policy, e := p13GuildDonateBoxPolicy(d, virtual)
	if e != nil || policy == nil || policy.PublishedAt > virtual.UnixMilli() {
		t.Fatal("valid published rule rejected in target calendar", policy, e)
	}
	var m db.P13GuildMember
	if e = d.First(&m, "player_id=?", ids[0]).Error; e != nil {
		t.Fatal(e)
	}
	if e = d.Transaction(func(tx *gorm.DB) error {
		return f.s.RecordP13GuildDonateBox(tx, &m, &pb.CSUnionDonate{DonateType: proto.String("coin")}, time.Now().UnixMilli(), 100000, 1, virtual)
	}); e != nil {
		t.Fatal(e)
	}
	boxes, score, open, e := f.s.ProjectP13GuildDonateBoxes(d, &m, virtual.Add(-time.Minute))
	if e != nil || !open || len(boxes) != 4 || score != 1 {
		t.Fatal("old-day source failed after revisit", boxes, score, e)
	}
}
