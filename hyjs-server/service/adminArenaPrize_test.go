package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestAdminArenaPrizePersistentRangeActualNextWinAndFrozenReceipts(t *testing.T) {
	f, peer, other := p11ArenaSetup(t)
	p11FeedbackStartupTables(t, f.s)
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "isolated-prize-operator", Role: constUtil.AdminRoleOperator, Enabled: true, PasswordHash: "isolated-no-http-login"}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	initial, err := f.s.AdminArenaPrizes(f.ctx, actor)
	if err != nil || initial.RegularCoin.Min != 3000 || initial.RegularCoin.Max != 6000 || initial.OperatorPool.Enabled {
		t.Fatal("persistent defaults or rare-pool state", initial, err)
	}
	var stored db.P11ArenaPrizePolicy
	db.GetIns().First(&stored, 1)
	if stored.RegularCoinMin != 3000 || stored.RegularCoinMax != 6000 {
		t.Fatal("defaults were not stored", stored.RegularCoinMin, stored.RegularCoinMax)
	}
	for _, id := range []int64{peer, other} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(9); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		g.Warriors[0].Level = proto.Int32(20)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	fight := func(stamp int64) (*pb.SCPvpFightResult, *pb.CSPvpFightResult) {
		t.Helper()
		list, _, err := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFindMatch{}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		selected := list[0].(*pb.SCPvpFindMatch).GetRankList()[0].GetInfo()
		request := &pb.CSPvpFightResult{PlayerId: selected.PlayerId, ServerId: selected.ServerId}
		before, _ := p10LuckyProfile(t, f.id)
		out, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, stamp+1)
		if err != nil {
			t.Fatal(err)
		}
		response := out[0].(*pb.SCPvpFightResult)
		after, _ := p10LuckyProfile(t, f.id)
		if response.GetWinPlayerId() != f.id || after.GetCoin()-before.GetCoin() != int64(response.BoxList[0].GetNum()) {
			t.Fatal("actual first-card grant")
		}
		return response, request
	}
	first, oldRequest := fight(996001)
	firstRaw, _ := proto.Marshal(first)
	saved, err := f.s.AdminSaveArenaPrizes(f.ctx, actor, model.AdminArenaPrizeSaveRequest{Version: initial.Version, RegularCoin: model.AdminArenaCoinRange{Min: 7777, Max: 7777}})
	if err != nil || saved.Version == initial.Version || saved.OperatorPool.Enabled || saved.RegularCoin.Min != 7777 {
		t.Fatal("typed config save", saved, err)
	}
	if _, err = f.s.AdminSaveArenaPrizes(f.ctx, actor, model.AdminArenaPrizeSaveRequest{Version: initial.Version, RegularCoin: model.AdminArenaCoinRange{Min: 1, Max: 1}}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version", err)
	}
	second, _ := fight(996011)
	for _, card := range second.BoxList {
		if card.GetNum() != 7777 {
			t.Fatal("next victory ignored persistent range", second.BoxList)
		}
	}
	var award db.P11ArenaPrizeAward
	db.GetIns().First(&award, "fight_id=?", second.GetFightId())
	var draw p11ArenaRegularCoinDraw
	if err = json.Unmarshal([]byte(award.DrawJSON), &draw); err != nil || draw.Min != 7777 || draw.Max != 7777 || draw.Source != constUtil.P11ArenaPrizeConfiguredCoinSource {
		t.Fatal("per-fight range snapshot", draw, err)
	}
	// Represent the prior v1 source under its original 3000..6000 semantics.
	// Response, balance and P6 terminal remain those of the actual first fight.
	var oldAward db.P11ArenaPrizeAward
	db.GetIns().First(&oldAward, "fight_id=?", first.GetFightId())
	var oldDraw p11ArenaRegularCoinDraw
	json.Unmarshal([]byte(oldAward.DrawJSON), &oldDraw)
	oldDraw.Source = constUtil.P11ArenaPrizeRegularCoinSource
	oldDraw.Min, oldDraw.Max = 0, 0
	encoded, _ := json.Marshal(oldDraw)
	oldAward.DrawJSON = string(encoded)
	oldAward.SourceHash = p11ArenaPrizeAwardHash(oldAward)
	if err = db.GetIns().Save(&oldAward).Error; err != nil {
		t.Fatal(err)
	}
	beforeCold, _ := p10LuckyProfile(t, f.id)
	connection, _ := db.GetIns().DB()
	connection.Close()
	f.open(t)
	f.s = &GameService{}
	p11FeedbackStartupTables(t, f.s)
	replay, _, err := f.s.DispatchP11Arena(f.ctx, f.id, oldRequest, 996002)
	if err != nil {
		t.Fatal("v1 old random receipt", err)
	}
	replayRaw, _ := proto.Marshal(replay[0])
	afterCold, _ := p10LuckyProfile(t, f.id)
	if !bytes.Equal(firstRaw, replayRaw) || !proto.Equal(beforeCold, afterCold) {
		t.Fatal("config change rerolled/regranted v1 receipt")
	}
	cold, err := f.s.AdminArenaPrizes(f.ctx, actor)
	if err != nil || cold.RegularCoin.Min != 7777 {
		t.Fatal("range not persistent after new service", err)
	}
	var count int64
	db.GetIns().Model(&db.AdminAudit{}).Where("action=?", constUtil.AdminArenaPrizeAuditAction).Count(&count)
	if count != 1 {
		t.Fatal("failed version save audited as success", count)
	}
	if err = db.GetIns().Model(&user).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminSaveArenaPrizes(f.ctx, actor, model.AdminArenaPrizeSaveRequest{Version: cold.Version, RegularCoin: cold.RegularCoin}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("DB viewer role allowed write", err)
	}
	t.Logf("typed admin persists3000..6000->7777..7777; original next-win delta7777; v1 random receipt unchanged after cold; stale version denied; audit once; viewer denied")
}
