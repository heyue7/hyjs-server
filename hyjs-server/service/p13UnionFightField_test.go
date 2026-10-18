package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestP13UnionFightFieldAdvanceIsolatedNativeClock 只推进专属隔离库声明的测试时刻，不改系统时钟或任何正式存档。
func TestP13UnionFightFieldAdvanceIsolatedNativeClock(t *testing.T) {
	text := os.Getenv("HYJS_P13_FIELD_NATIVE_ADVANCE")
	if text == "" {
		t.Skip("仅原帧隔离场景测试启用")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_") {
		t.Fatal("P13 field native isolation guard")
	}
	at, err := strconv.ParseInt(text, 10, 64)
	if err != nil || at <= 0 {
		t.Fatal("explicit test clock missing")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}, s: &GameService{}}
	f.open(t)
	t.Cleanup(func() { sql, _ := db.GetIns().DB(); sql.Close() })
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP13UnionFight(f.ctx, time.UnixMilli(at)); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightMatch{}).Count(&count).Error; err != nil || count == 0 {
		t.Fatal("genuine native matches missing", count, err)
	}
}

func p13UFFieldSetup(t *testing.T) (*p1Fixture, []int64, int64, *atomic.Int64, *db.P13UnionFightMatch) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	p13UFOpen(t)
	for _, table := range []any{&db.P13UnionFightFieldPlayer{}, &db.P13UnionFightField{}, &db.P13UnionFightMatch{}, &db.P13UnionFightGroup{}, &db.P13UnionFightSchedule{}, &db.P13UnionFightLoadout{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("match_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p13-field-%d-%d", i, time.Now().UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("战场真实%02d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(30)
			p.Diamond = proto.Int64(1000)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	now := p13UFMondays()
	var guilds []string
	for i := 0; i < 10; i++ {
		p13GuildCall(t, f, ids[i], &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("场景原%02d", i))}, 1, now)
		guild := p13GuildID(t, f, ids[i], now)
		guilds = append(guilds, guild)
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 10; i < len(ids); i++ {
		p13GuildCall(t, f, ids[i], &pb.CSUnionApply{UnionId: proto.String(guilds[0])}, 2, now)
		p13GuildCall(t, f, ids[0], &pb.CSUnionAgreeApply{PlayerId: proto.Int64(ids[i])}, 100+int64(i), now)
	}
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[11])}, 190, now)
	for i := 0; i < 10; i++ {
		p13UFCall(t, f, ids[i], &pb.CSUFUnionApply{}, 200, now)
		p13UFCall(t, f, ids[i], &pb.CSUFPlayerApply{}, 201, now)
	}
	for i := 10; i < len(ids); i++ {
		p13UFCall(t, f, ids[i], &pb.CSUFPlayerApply{}, 202, now)
	}
	next := now.Add(12 * time.Hour)
	first := next.Add(20 * time.Hour).UnixMilli()
	if err := db.GetIns().Create(&db.P13UnionFightSchedule{Day: "2026-10-20", FirstStartAt: first, SecondStartAt: first + 600000, DurationMillis: 300000, Source: "隔离完整场景日历，原运营日期未知"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP13UnionFight(f.ctx, next); err != nil {
		t.Fatal(err)
	}
	var match db.P13UnionFightMatch
	if err := db.GetIns().First(&match, "round=1 AND (left_guild_id=? OR right_guild_id=?)", guilds[0], guilds[0]).Error; err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Int64{}
	clock.Store(first - 60000)
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return time.UnixMilli(clock.Load()) })
	return f, ids, first, clock, &match
}

// TestP13UnionFightField 覆盖真实25首发/1替补、全部原固定建筑、双阵营/跨实例、移动/挂机、重启/权限与结束退场。
func TestP13UnionFightFieldActual25FullMapSourceFramesIsolationMovementRestartAndEnd(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := time.UnixMilli(clock.Load())
	// 新比赛保存完整Battle；旧比赛只从相同SHA的冻结归档恢复，损坏与版本漂移均拒绝。
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("matched roster decode")
	}
	var owner *p13UFMatchFighter
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for i := range side {
			if len(side[i].BattlePayload) == 0 {
				t.Fatal("new match omitted actual Battle bytes")
			}
			if side[i].PlayerID == ids[0] {
				owner = &side[i]
			}
		}
	}
	if owner == nil {
		t.Fatal("owner missing from actual roster")
	}
	owner.BattlePayload = nil
	restored, restoreErr := p13UFReadMatchFighter(db.GetIns().WithContext(f.ctx), *owner)
	if restoreErr != nil || len(restored.Battle) == 0 {
		t.Fatal("legacy exact-SHA archive restore", restoreErr)
	}
	where := db.GetIns().Model(&db.P13UnionFightFighter{}).Where("guild_id=? AND day=? AND player_id=?", owner.GuildID, owner.Day, owner.PlayerID)
	if err := where.Update("battle", []byte{0xff}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p13UFReadMatchFighter(db.GetIns().WithContext(f.ctx), *owner); err == nil {
		t.Fatal("legacy corrupt bytes accepted")
	}
	if err := where.Updates(map[string]any{"battle": restored.Battle, "snapshot_hash": "wrong-version"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p13UFReadMatchFighter(db.GetIns().WithContext(f.ctx), *owner); err == nil {
		t.Fatal("legacy changed archive version accepted")
	}
	if err := where.Update("snapshot_hash", restored.SnapshotHash).Error; err != nil {
		t.Fatal(err)
	}
	match.Roster, restoreErr = json.Marshal(roster)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	match.SnapshotHash = p6Digest(match.Roster)
	if err := db.GetIns().Model(match).Updates(map[string]any{"roster": match.Roster, "snapshot_hash": match.SnapshotHash}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now); err == nil {
		t.Fatal("unpublished local field policy")
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "明确本地fixture：原固定复活点1/3、非权威初始HP；原出生分配/属性变换未知"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[34], now); err == nil {
		t.Fatal("26th backup admitted without actual replacement")
	}
	var entrantIDs []int64
	entrantIDs = append(entrantIDs, ids[0])
	entrantIDs = append(entrantIDs, ids[10:34]...)
	for _, id := range entrantIDs {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
			t.Fatal(err)
		}
	}
	if f.mysql.Database != "" {
		// 八路真实行锁事务重入同一角色，实例和原HP/AP只保存一份。
		var tasks sync.WaitGroup
		failures := make(chan error, 8)
		for i := 0; i < 8; i++ {
			tasks.Add(1)
			go func() {
				defer tasks.Done()
				_, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now)
				failures <- err
			}()
		}
		tasks.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal("concurrent genuine re-entry", err)
			}
		}
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND guild_id=? AND left_at=0", match.ID, p13GuildID(t, f, ids[0], now)).Count(&count)
	if count != 25 {
		t.Fatal("original full25 first-line qualification", count)
	}
	var opponents []db.P13GuildMember
	enemy := match.RightGuildID
	if enemy == p13GuildID(t, f, ids[0], now) {
		enemy = match.LeftGuildID
	}
	db.GetIns().Where("guild_id=?", enemy).Find(&opponents)
	if len(opponents) != 1 {
		t.Fatal("real opponent guild")
	}
	other := opponents[0].PlayerID
	if _, err := f.s.p13UFFieldEnter(f.ctx, other, now); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now)
	if err != nil {
		t.Fatal(err)
	}
	position := frames[0].(*pb.SCCurrentPosition)
	transfer := frames[1].(*pb.SCUnionFightTransfer)
	if position.GetLimit() != 50 || position.Self.Position.GetScene() != "fight_union" || transfer.GetState() != 1 || transfer.GetLeftTime() != 60000 || transfer.GetTotalTime() != 300000 || transfer.GetUnionFightId() != match.ID {
		t.Fatal("original scene/ready metadata", frames)
	}
	if len(transfer.BaseCampInfos) != 2 || len(transfer.GateInfos) != 8 || len(transfer.TowerInfos) != 6 || len(transfer.FactoryInfos) != 4 || len(transfer.RevivePointInfos) != 7 || len(transfer.UnionInfos) != 2 {
		t.Fatal("complete original fixed building rows", len(transfer.BaseCampInfos), len(transfer.GateInfos), len(transfer.TowerInfos), len(transfer.FactoryInfos), len(transfer.RevivePointInfos))
	}
	var stored db.P13UnionFightFieldPlayer
	db.GetIns().First(&stored, "match_id=? AND player_id=?", match.ID, ids[0])
	battle := &pb.BattlePlayer{}
	if proto.Unmarshal(stored.Battle, battle) != nil {
		t.Fatal("actual frozen battle")
	}
	for i, pose := range transfer.Player.FightInfo.Index {
		found := false
		for _, actor := range battle.Formation {
			if actor.GetPose() == pose {
				found = true
				if transfer.Player.FightInfo.HpPos[i] != actor.GetInst().GetCurHp() || transfer.Player.FightInfo.MaxHpPos[i] != actor.GetProp().GetHp() {
					t.Fatal("Pose HP guessed or changed")
				}
			}
		}
		if !found {
			t.Fatal("missing frozen pose")
		}
	}
	encoded, err := proto.Marshal(transfer)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip := &pb.SCUnionFightTransfer{}
	if proto.Unmarshal(encoded, roundtrip) != nil || !proto.Equal(roundtrip, transfer) {
		t.Fatal("original field binary")
	}
	// 严格认证TCP的两个阵营在本场可见，第三场景的真实成员不能进入可见集合。
	received := map[int64][]proto.Message{}
	for _, id := range []int64{ids[0], other, ids[10]} {
		var row db.Profile
		db.GetIns().First(&row, id)
		playerID := id
		cleanup := f.s.RegisterP10Client(f.ctx, id, row.Session, func(m proto.Message) error {
			received[playerID] = append(received[playerID], proto.Clone(m))
			return nil
		})
		t.Cleanup(cleanup)
		if _, err = f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	peers, _, err := f.s.p13UFFieldPeers(db.GetIns().WithContext(f.ctx), match.ID, ids[0])
	if err != nil || len(peers) != 2 {
		t.Fatal("actual same-match parties", len(peers), err)
	}
	clock.Store(first - 59000)
	point := proto.Clone(position.Self.Position.Current).(*pb.PointNew)
	point.X = proto.Float32(point.GetX() + 1)
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: point, CurrentTime: proto.Int64(1700000000000)}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if len(received[other]) == 0 {
		t.Fatal("actual opponent did not receive own-instance movement")
	}
	for i := 1; i <= 6; i++ {
		step := proto.Clone(point).(*pb.PointNew)
		step.X = proto.Float32(point.GetX() + float32(i)*0.5)
		if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: step}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal("published cumulative tolerance", i, err)
		}
	}
	denied := proto.Clone(point).(*pb.PointNew)
	denied.X = proto.Float32(point.GetX() + 3.5)
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: denied}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("continuous requests received repeated free tolerance")
	}
	point.X = proto.Float32(point.GetX() + 3)
	bad := proto.Clone(point).(*pb.PointNew)
	bad.X = proto.Float32(point.GetX() + 20)
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: bad}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("instant warp accepted")
	}
	if _, err = f.s.p13UFFieldHook(f.ctx, ids[0], &pb.CSUFOnHook{OnHook: proto.Int32(1)}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: point}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("manual movement while hooked")
	}
	if _, err = f.s.p13UFFieldHook(f.ctx, ids[0], &pb.CSUFOnHook{}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	before := stored
	if err = updateProfile(f.ctx, ids[0], func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(99); return nil }); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	frames, err = f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSUFInit{}, time.UnixMilli(clock.Load()))
	if err != nil {
		t.Fatal(err)
	}
	recovered := frames[0].(*pb.SCUnionFightTransfer)
	if recovered.PlayerPos.GetX() != point.GetX() || !proto.Equal(recovered.Player.FightInfo, transfer.Player.FightInfo) {
		t.Fatal("restart reset original position/HP/AP", recovered)
	}
	db.GetIns().First(&stored, "match_id=? AND player_id=?", match.ID, ids[0])
	if string(stored.Battle) != string(before.Battle) {
		t.Fatal("live refit replaced frozen battle")
	}
	clock.Store(first + 1000)
	frames, err = f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSUFInit{}, time.UnixMilli(clock.Load()))
	if err != nil || frames[0].(*pb.SCUnionFightTransfer).GetState() != 2 {
		t.Fatal("real field fight clock", err)
	}
	clock.Store(match.EndAt + 1000)
	if err = f.s.TickP13UnionFightField(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	clock.Store(match.EndAt + 4001)
	if err = f.s.TickP13UnionFightField(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	db.GetIns().First(&profile, ids[0])
	_, game, err := decodeProfile(&profile)
	if err != nil || game.Position.GetScene() != "town_main" {
		t.Fatal("actual end4sec exit", err)
	}
	db.GetIns().Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND left_at=0", match.ID).Count(&count)
	if count != 0 {
		t.Fatal("ended actor still online", count)
	}
	var field db.P13UnionFightField
	db.GetIns().First(&field, "match_id=?", match.ID)
	var snapshot p13UFFieldSnapshot
	if json.Unmarshal(field.Snapshot, &snapshot) != nil || snapshot.Source == "" {
		t.Fatal("named local/source map missing")
	}
}
