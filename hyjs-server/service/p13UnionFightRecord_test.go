package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 使用已有有限真实宣战/匹配/原入场夹具，一场真实ordinary14战斗；仅推进业务时钟5秒。
func TestP13UFRecordActualTerminalProjectionReplayIsolationAndCold(t *testing.T) {
	f, id, enemy, clock, match := p13UFPlayerBattle172ActualSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, pid := range []int64{id, enemy} {
		var profile db.Profile
		if err := db.GetIns().First(&profile, pid).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, profile.Session, func(proto.Message) error { return nil }))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, pid, false); err != nil {
			t.Fatal(err)
		}
	}
	query := func(player int64, request proto.Message) ([]proto.Message, error) {
		messages, handled, err := f.s.DispatchP13UnionFight(f.ctx, player, request, 0)
		if !handled {
			t.Fatal("原80087/80100入口未接入")
		}
		return messages, err
	}
	if _, handled, err := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0); !handled || err != nil {
		t.Fatal("真实原Start未受理", err)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	fightID, err := strconv.ParseInt(attempt.FightID, 10, 64)
	if err != nil || fightID <= 0 {
		t.Fatal("原可回放FightID", err)
	}
	list, err := query(id, &pb.CSUFFightRecord{})
	if err != nil || len(list) != 1 || len(list[0].(*pb.SCUFFightRecord).GetRecords()) != 0 {
		t.Fatal("未结算战斗提前成为战绩", err)
	}
	replayRequest := &pb.CSUFFightRecordReplay{FightId: proto.Int64(fightID)}
	if _, err = query(id, replayRequest); err == nil {
		t.Fatal("未结算战斗伪装成录像")
	}
	clock.Add(5000)
	if err = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil || attempt.Status != constUtil.P13UFPlayerBattleSettled {
		t.Fatal("真实终末未结算", err)
	}
	// 读取前后逐表核对，战绩不再次运算、不投影回血、不发奖或重写终末。
	snapshot := func() []byte {
		var profiles []db.Profile
		var actors []db.P13UnionFightFieldPlayer
		var attempts []db.P13UnionFightPlayerBattleAttempt
		var records []db.P6BattleRecord
		for _, lookup := range []struct {
			dest  any
			where string
			args  []any
		}{
			{&profiles, "id IN ?", []any{[]int64{id, enemy}}},
			{&actors, "match_id=?", []any{match.ID}},
			{&attempts, "match_id=?", []any{match.ID}},
			{&records, "source=?", []any{constUtil.P13UFPlayerBattleSource}},
		} {
			if err := db.GetIns().Where(lookup.where, lookup.args...).Find(lookup.dest).Error; err != nil {
				t.Fatal(err)
			}
		}
		var saves []any
		for i := range profiles {
			player, game, err := decodeProfile(&profiles[i])
			if err != nil {
				t.Fatal(err)
			}
			saves = append(saves, player, game)
		}
		value, _ := json.Marshal([]any{profiles, saves, actors, attempts, records})
		return value
	}
	before := snapshot()
	list, err = query(id, &pb.CSUFFightRecord{})
	if err != nil {
		t.Fatal(err)
	}
	frame := list[0].(*pb.SCUFFightRecord)
	if len(frame.GetRecords()) != 1 || len(frame.GetUnionInfos()) != 0 || frame.GetAppVersion() != constUtil.P13UFRecordOriginalAppVersion || frame.GetResVersion() != "" {
		t.Fatal("实际战绩子集/未知汇总边界错误", frame)
	}
	record := frame.GetRecords()[0]
	if record.GetFightId() != fightID || record.GetWinId() != attempt.RewardPlayerID || record.GetTime() != attempt.CreatedAt || len(record.GetPlayerInfos()) != 2 {
		t.Fatal("列表未来自真实终末", record)
	}
	for _, side := range record.GetPlayerInfos() {
		if side.GetType() != 0 || (side.GetGuid() != id && side.GetGuid() != enemy) || side.GetMainHeroId() == "" || side.GetNickName() == "" {
			t.Fatal("列表制造不存在角色或非玩家类型", side)
		}
	}
	replay, err := query(enemy, replayRequest)
	if err != nil || len(replay) != 1 {
		t.Fatal("同场对手无法看真实录像", err)
	}
	var original pb.SCUFFightRecordReplay
	if err = proto.Unmarshal(attempt.StartPB, &original); err != nil {
		t.Fatal(err)
	}
	original.WinPlayerId = proto.Int64(attempt.RewardPlayerID)
	if !proto.Equal(&original, replay[0]) {
		t.Fatal("原Start帧/真实胜者被改写")
	}
	var foreign db.Profile
	if err = db.GetIns().Where("id NOT IN ?", []int64{id, enemy}).First(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = query(foreign.Id, replayRequest); err == nil {
		t.Fatal("未入当前真实场次者读取录像")
	}
	if _, err = query(id, &pb.CSUFFightRecordReplay{FightId: proto.Int64(fightID + 1)}); err == nil {
		t.Fatal("未知录像可读")
	}
	if !bytes.Equal(before, snapshot()) {
		t.Fatal("列表/录像查询改变战斗或钱包")
	}
	f.open(t)
	cold, err := query(id, replayRequest)
	if err != nil || !proto.Equal(cold[0], replay[0]) || !bytes.Equal(before, snapshot()) {
		t.Fatal("冷读取重复计算/扣发/改帧", err)
	}
	// 损坏来源必须拒绝读取，不能把未验证Start直接包装成原录像。
	originalStartHash := attempt.StartHash
	if err = db.GetIns().Model(&attempt).Update("start_hash", "invalid").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = query(id, replayRequest); err == nil {
		t.Fatal("损坏Start来源仍可播")
	}
	if err = db.GetIns().Model(&attempt).Update("start_hash", originalStartHash).Error; err != nil {
		t.Fatal(err)
	}
}
