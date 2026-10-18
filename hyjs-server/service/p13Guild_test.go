package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func p13GuildSetup(t *testing.T) (*p1Fixture, []int64) {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p13.db")}
	if name := os.Getenv("HYJS_P13_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p13_") {
			t.Fatal("P13 isolation guard")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	t.Cleanup(func() { sql, _ := db.GetIns().DB(); sql.Close() })
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P13GuildReceipt{}, &db.P13GuildApply{}, &db.P13GuildMember{}, &db.P13Guild{}, &db.P13GuildPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := 0; i < 5; i++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p13-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("公会真实%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(30)
			p.Diamond = proto.Int64(1000)
			p.Coin = proto.Int64(20000000)
			g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("item_089"), Num: proto.Int32(50)})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	f.id = ids[0]
	return f, ids
}
func p13GuildOpen(t *testing.T, economy bool) {
	t.Helper()
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Updates(map[string]any{"management_enabled": true, "economy_enabled": economy}).Error; err != nil {
		t.Fatal(err)
	}
}
func p13GuildCall(t *testing.T, f *p1Fixture, id int64, req proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	var messages []proto.Message
	var err error
	switch b := req.(type) {
	case *pb.CSUnionCreate:
		messages, err = f.s.p13GuildCreate(f.ctx, id, b, stamp, now)
	case *pb.CSUnionApply, *pb.CSUnionUnApply:
		messages, err = f.s.p13GuildApply(f.ctx, id, b, stamp, now)
	case *pb.CSUnionDonate, *pb.CSUnionUpgrade, *pb.CSGetUnionStamina:
		messages, err = f.s.p13GuildEconomy(f.ctx, id, b, stamp, now)
	case *pb.CSGetUnionInfo, *pb.CSUnionSearch, *pb.CSUnionMembers, *pb.CSUnionApplyList, *pb.CSUnionStore:
		messages, err = f.s.p13GuildQuery(f.ctx, id, b, now)
	default:
		messages, err = f.s.p13GuildManage(f.ctx, id, b, stamp, now)
	}
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	clone := proto.Clone(messages[0])
	proto.Reset(clone)
	if err = proto.Unmarshal(raw, clone); err != nil || !proto.Equal(clone, messages[0]) {
		t.Fatal("original binary roundtrip", err)
	}
	return messages[0]
}
func p13GuildID(t *testing.T, f *p1Fixture, id int64, now time.Time) string {
	t.Helper()
	return p13GuildCall(t, f, id, &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo).GetUnionInfo().GetUnionId()
}
func p13Join(t *testing.T, f *p1Fixture, leader, member int64, guild string, stamp int64, now time.Time) {
	t.Helper()
	p13GuildCall(t, f, member, &pb.CSUnionApply{UnionId: proto.String(guild)}, stamp, now)
	p13GuildCall(t, f, leader, &pb.CSUnionAgreeApply{PlayerId: proto.Int64(member)}, stamp, now)
}
func TestP13GuildOriginalManagementPermissionsCooldownAndRestart(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	if _, err := f.s.p13GuildCreate(f.ctx, ids[0], &pb.CSUnionCreate{Name: proto.String("真实公会")}, 1, now); err == nil {
		t.Fatal("closed management allowed create")
	}
	p13GuildOpen(t, false)
	created := p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("真实公会")}, 1, now).(*pb.SCUnionCreate)
	if len(created.Cost) != 1 || created.Cost[0].GetId() != "diamond" || created.Cost[0].GetNum() != 300 {
		t.Fatal("original create cost", created)
	}
	guild := p13GuildID(t, f, ids[0], now)
	if guild == "" {
		t.Fatal("missing actual guild")
	}
	searched := p13GuildCall(t, f, ids[1], &pb.CSUnionSearch{Name: proto.String("真实公会"), Page: proto.Int32(1)}, 0, now).(*pb.SCUnionSearch)
	if len(searched.Unions) != 1 || searched.Unions[0].GetChairmanName() == "" || searched.Unions[0].GetCurrentMemberNum() != 1 {
		t.Fatal("actual search", searched)
	}
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	p13Join(t, f, ids[0], ids[2], guild, 3, now)
	if _, err := f.s.p13GuildManage(f.ctx, ids[1], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[2])}, 4, now); err == nil {
		t.Fatal("member promoted vice")
	}
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 4, now)
	p13Join(t, f, ids[1], ids[3], guild, 5, now)
	if _, err := f.s.p13GuildManage(f.ctx, ids[1], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[2])}, 6, now); err == nil {
		t.Fatal("vice transferred chairman")
	}
	p13GuildCall(t, f, ids[1], &pb.CSUnionNoticeChange{Content: proto.String("实际公告")}, 6, now)
	if _, err := f.s.p13GuildManage(f.ctx, ids[1], &pb.CSUnionDeclarationChange{Content: proto.String("宣言")}, 7, now); err == nil {
		t.Fatal("vice changed declaration")
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionDeclarationChange{Content: proto.String("实际宣言")}, 7, now)
	p13GuildCall(t, f, ids[0], &pb.CSChangeUnionFlag{UnionFlag: proto.Int32(1)}, 8, now)
	if _, err := f.s.p13GuildManage(f.ctx, ids[0], &pb.CSQuitUnion{}, 9, now); err == nil {
		t.Fatal("chairman quit")
	}
	transfer := &pb.CSTransferChairman{PlayerId: proto.Int64(ids[2])}
	p13GuildCall(t, f, ids[0], transfer, 9, now)
	p13GuildCall(t, f, ids[0], transfer, 9, now)
	if _, err := f.s.p13GuildManage(f.ctx, ids[0], &pb.CSChangeUnionFlag{UnionFlag: proto.Int32(2)}, 10, now); err == nil {
		t.Fatal("previous chairman kept authority")
	}
	p13GuildCall(t, f, ids[1], &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(ids[3])}, 11, now)
	scope, _, err := P13GuildScope(db.GetIns(), ids[3])
	if err != nil || scope != "" {
		t.Fatal("removed player retained scope", scope, err)
	}
	if _, err = f.s.p13GuildApply(f.ctx, ids[3], &pb.CSUnionApply{UnionId: proto.String(guild)}, 12, now); err == nil {
		t.Fatal("quit cooldown bypass")
	}
	p13GuildCall(t, f, ids[3], &pb.CSUnionApply{UnionId: proto.String(guild)}, 12, now.Add(25*time.Hour))
	p13GuildCall(t, f, ids[3], &pb.CSUnionUnApply{UnionId: proto.String(guild)}, 13, now.Add(25*time.Hour))
	list := p13GuildCall(t, f, ids[2], &pb.CSUnionApplyList{}, 0, now).(*pb.SCUnionApplyList)
	if len(list.List) != 0 {
		t.Fatal("cancel retained apply")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	info := p13GuildCall(t, f, ids[2], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetMyRole() != 2 || info.GetNotice() != "实际公告" || info.GetUnionInfo().GetDeclaration() != "实际宣言" || info.GetUnionInfo().GetUnionFlag() != 1 {
		t.Fatal("restart state", info)
	}
	p13GuildCall(t, f, ids[2], &pb.CSDissolveUnion{}, 14, now)
	for _, id := range ids {
		scope, _, err = P13GuildScope(db.GetIns(), id)
		if err != nil || scope != "" {
			t.Fatal("dissolution retained scope", id, scope, err)
		}
	}
}
func TestP13GuildConcurrentApprovalTransferUniqueMembershipAndReceipts(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, false)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("并发甲")}, 1, now)
	a := p13GuildID(t, f, ids[0], now)
	p13GuildCall(t, f, ids[1], &pb.CSUnionCreate{Name: proto.String("并发乙")}, 1, now)
	b := p13GuildID(t, f, ids[1], now)
	for i, g := range []string{a, b} {
		p13GuildCall(t, f, ids[2], &pb.CSUnionApply{UnionId: proto.String(g)}, int64(10+i), now)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids[:2] {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, err := f.s.p13GuildManage(f.ctx, id, &pb.CSUnionAgreeApply{PlayerId: proto.Int64(ids[2])}, 20, now)
			errs <- err
		}(id)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("cross guild approvals", success)
	}
	m, err := p13GuildMember(db.GetIns(), ids[2])
	if err != nil || m.GuildID == "" {
		t.Fatal("member lost", err)
	}
	var applications int64
	db.GetIns().Model(&db.P13GuildApply{}).Where("player_id=?", ids[2]).Count(&applications)
	if applications != 0 {
		t.Fatal("accepted player retained applications")
	}
	leader := ids[0]
	if m.GuildID == b {
		leader = ids[1]
	}
	req := &pb.CSTransferChairman{PlayerId: proto.Int64(ids[2])}
	errs = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.p13GuildManage(f.ctx, leader, req, 30, now); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var chairmen int64
	db.GetIns().Model(&db.P13GuildMember{}).Where("guild_id=? AND role=2", m.GuildID).Count(&chairmen)
	if chairmen != 1 {
		t.Fatal("concurrent transfer chairmen", chairmen)
	}
	if _, err = f.s.p13GuildManage(f.ctx, leader, &pb.CSTransferChairman{PlayerId: proto.Int64(ids[3])}, 30, now); err == nil {
		t.Fatal("same stamp different target")
	}
}
func TestP13GuildOriginalEconomyConcurrentDonationDailyCapsAndAtomicFailure(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, false)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("真实经济")}, 1, now)
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[0], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 2, now); err == nil {
		t.Fatal("closed economy allowed donation")
	}
	p13GuildOpen(t, true)
	req := &pb.CSUnionDonate{DonateType: proto.String("coin")}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.p13GuildEconomy(f.ctx, ids[0], req, 2, now); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	store := p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if store.GetCoinNum() != 100000 || store.GetCurrentScore() != 1 || store.GetDonateCoin() != 100000 || store.GetMaxPerson() != 35 {
		t.Fatal("atomic donation", store)
	}
	reply := p13GuildCall(t, f, ids[0], req, 2, now).(*pb.SCUnionDonate)
	if reply.GetScore() != 5 || reply.GetContributionAdd() != 20 || reply.GetDonateScoreAdd() != 1 {
		t.Fatal("original donation delta", reply)
	}
	for i := 3; i <= 16; i++ {
		p13GuildCall(t, f, ids[0], req, int64(i), now)
	}
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[0], req, 17, now); err == nil {
		t.Fatal("coin cap bypass")
	}
	before, _ := p10LuckyProfile(t, ids[0])
	if before.GetCoin() != 18500000 || before.GetContribution() != 300 {
		t.Fatal("wallet donation total", before)
	}
	badge := &pb.CSUnionDonate{DonateType: proto.String("badge")}
	for i := 30; i < 60; i++ {
		p13GuildCall(t, f, ids[0], badge, int64(i), now)
	}
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[0], badge, 60, now); err == nil {
		t.Fatal("badge cap bypass")
	}
	store = p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if store.GetBadgeNum() != 30 || store.GetDonateBadge() != 30 || store.GetCurrentScore() != 255 {
		t.Fatal("badge fund", store)
	}
	p13GuildCall(t, f, ids[0], &pb.CSGetUnionStamina{}, 70, now)
	p13GuildCall(t, f, ids[0], &pb.CSGetUnionStamina{}, 70, now)
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[0], &pb.CSGetUnionStamina{}, 71, now); err == nil {
		t.Fatal("stamina repeated")
	}
	if _, err := f.s.p13GuildEconomy(f.ctx, ids[0], &pb.CSUnionUpgrade{}, 80, now); err == nil {
		t.Fatal("upgrade with insufficient exp/coin")
	}
	afterStore := p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if !proto.Equal(store, afterStore) {
		t.Fatal("failed upgrade moved resources")
	}
	p13GuildCall(t, f, ids[0], req, 81, now.Add(24*time.Hour))
	// 原升级成本从等级1行读取，同事务扣10徽章/1000万金币/2000积分。
	guild := p13GuildID(t, f, ids[0], now)
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": int64(10000000), "badge": int32(10), "exp": int32(2000)}).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionUpgrade{}, 90, now)
	info := p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetUnionInfo().GetLevel() != 2 || info.GetCoinNum() != 0 || info.GetBadgeNum() != 0 || info.GetCurrentExp() != 0 {
		t.Fatal("original upgrade cost", info)
	}
}

func TestP13GuildDailyLoginOriginalSourceNoDoubleCreditAndExit(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("登录原来源")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.p13GuildLoginAt(f.ctx, ids[1], now) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	info := p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetActiveScore() != 10 || info.GetCurrentExp() != 5 {
		t.Fatal("original login source duplicated", info)
	}
	if err := f.s.p13GuildLoginAt(f.ctx, ids[1], now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	info = p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetActiveScore() != 20 || info.GetCurrentExp() != 10 {
		t.Fatal("daily rollover", info)
	}
	p13GuildCall(t, f, ids[1], &pb.CSQuitUnion{}, 3, now)
	if err := f.s.p13GuildLoginAt(f.ctx, ids[1], now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	after := p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if after.GetActiveScore() != 20 || after.GetCurrentExp() != 10 {
		t.Fatal("former member credited guild", after)
	}
}

func TestP13GuildMissingTablesIsolatedFixtureCompatibility(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.RecordP13GuildLogin(f.ctx, f.id); err != nil {
		t.Fatal("older fixture missing P13 schema", err)
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("id=1").Delete(&db.P13GuildPolicy{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordP13GuildLogin(f.ctx, f.id); err == nil {
		t.Fatal("existing schema missing mandatory policy silently accepted")
	}
}

func TestP13GuildOriginalWeightedNameNoticeDeclarationBudgets(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		key, text string
		valid     bool
	}{
		{"UnionNameLengthLimit", strings.Repeat("剑", 6), true}, {"UnionNameLengthLimit", strings.Repeat("剑", 7), false},
		{"UnionNameLengthLimit", strings.Repeat("A", 12), true}, {"UnionNameLengthLimit", strings.Repeat("A", 13), false},
		{"UnionNameLengthLimit", strings.Repeat("Ā", 12), true}, {"UnionNameLengthLimit", strings.Repeat("Ā", 13), false},
		{"UnionNameLengthLimit", strings.Repeat("ā", 6), true}, {"UnionNameLengthLimit", strings.Repeat("ā", 7), false},
		{"UnionNameLengthLimit", strings.Repeat("😀", 3), true}, {"UnionNameLengthLimit", strings.Repeat("😀", 4), false},
		{"UnionDeclarationLengthLimit", strings.Repeat("宣", 40), true}, {"UnionDeclarationLengthLimit", strings.Repeat("宣", 41), false},
		{"UnionDeclarationLengthLimit", strings.Repeat("A", 80), true}, {"UnionDeclarationLengthLimit", strings.Repeat("A", 81), false},
		{"UnionNoticeLengthLimit", strings.Repeat("公", 70), true}, {"UnionNoticeLengthLimit", strings.Repeat("公", 71), false},
		{"UnionNoticeLengthLimit", strings.Repeat("A", 140), true}, {"UnionNoticeLengthLimit", strings.Repeat("A", 141), false},
	} {
		err := s.p13GuildText(check.text, check.key, false)
		if (err == nil) != check.valid {
			t.Fatal(check.key, len(check.text), check.valid, err)
		}
	}
}
