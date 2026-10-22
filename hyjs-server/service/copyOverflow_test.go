package service

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
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

// copyOverflowWire 保留原HTTP二进制帧、签名、会话校验、Dispatch和二进制响应编解码。
func copyOverflowWire(t *testing.T, f *p1Fixture, body proto.Message) []proto.Message {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var opcode int32
	for code, name := range model.OpcodeNames {
		if name == string(body.ProtoReflect().Descriptor().Name()) {
			opcode = code
			break
		}
	}
	payload, err := proto.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixMilli()
	zone := int32(f.s.OriginalServerID())
	raw := make([]byte, 68+len(payload))
	binary.BigEndian.PutUint32(raw[0:4], uint32(len(raw)))
	binary.BigEndian.PutUint32(raw[4:8], 10000)
	binary.BigEndian.PutUint32(raw[8:12], uint32(opcode))
	binary.BigEndian.PutUint64(raw[12:20], uint64(f.id))
	binary.BigEndian.PutUint32(raw[20:24], uint32(zone))
	binary.BigEndian.PutUint32(raw[24:28], uint32(len(payload)))
	copy(raw[28:44], row.Session)
	copy(raw[68:], payload)
	binary.BigEndian.PutUint64(raw[60:68], uint64(stamp))
	sign := md5.New()
	fmt.Fprintf(sign, "%d%d%d%s", f.id, zone, stamp, constUtil.SigningSuffix)
	sign.Write(payload)
	copy(raw[44:60], sign.Sum(nil))
	packet, err := model.ParseGamePacket(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	response := f.s.Dispatch(f.ctx, packet, "127.0.0.1:fixture")
	encoded, err := model.EncodeGameResponses(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []proto.Message
	for len(encoded) > 0 {
		if len(encoded) < 8 {
			t.Fatal("truncated response header")
		}
		length := int(binary.BigEndian.Uint32(encoded[:4]))
		code := int32(binary.BigEndian.Uint32(encoded[4:8]))
		if length > len(encoded)-8 {
			t.Fatal("truncated response body")
		}
		message, err := model.GameMessage(model.OpcodeNames[code], nil)
		if err != nil || proto.Unmarshal(encoded[8:8+length], message) != nil {
			t.Fatal("invalid original response", err)
		}
		decoded = append(decoded, message)
		encoded = encoded[8+length:]
	}
	return decoded
}

func TestCopyOverflowOriginalWireFullBagColdReplayAndClaim(t *testing.T) {
	f := p1Setup(t)
	for _, initialize := range []func(context.Context) error{f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitializeGameplayPools, f.s.InitializeShop, f.s.InitializeP4, f.s.InitializeP4CopyBenefits, f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	// 仅隔离库启用2倍代表窗口，覆盖暂存掉落的原福利倍率，不改生产运营配置。
	now := time.Now()
	if err := db.GetIns().Model(&db.P4CopyBenefitRule{}).Where("1=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	cfg := db.P4CopyBenefitRule{ID: "overflow-fixture", Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), WeekdayMask: 127, StartMinute: 0, EndMinute: 1440, CurrencyFactor: 1, DropFactor: 2, Source: "isolated overflow transaction fixture"}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		for len(g.Warriors) < int(p.GetWarriorBagSize()) {
			g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String("hero_020"), Guid: proto.String(fmt.Sprintf("fixture-owned-%d", len(g.Warriors))), Level: proto.Int32(1)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, beforeGame := f.profile(t)
	ownedBefore, _ := json.Marshal(beforeGame.Warriors)
	entered := copyOverflowWire(t, f, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")})
	if _, bad := entered[0].(*pb.ErrorMessage); bad {
		t.Fatal("original enter rejected", entered)
	}
	var firstEnd *pb.CSCopyFightEnd
	var firstReceipt *pb.SCCopyFightEnd
	for _, taskID := range []string{"1", "4"} {
		started := copyOverflowWire(t, f, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String(taskID)})
		start, ok := started[0].(*pb.SCCopyFightStart)
		if !ok {
			t.Fatal("original fight start failed", started)
		}
		if taskID == "1" {
			// 首个战斗模拟升级前无掉落快照的旧存档，Boss继续覆盖新冻结配置分支。
			if err := db.GetIns().Where("player_id=? AND fight_id=?", f.id, start.GetFightId()).Delete(&db.CopyDropSnapshot{}).Error; err != nil {
				t.Fatal(err)
			}
		}
		end := &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: start.BattlePlayer}
		// 胜负使用隔离原协议结果夹具；这项测试证明事务/回包，未声称原引擎胜利。
		ended := copyOverflowWire(t, f, end)
		receipt, ok := ended[0].(*pb.SCCopyFightEnd)
		if !ok || len(receipt.GetGain().GetWars()) != 0 {
			t.Fatal("full bag blocked battle or returned phantom heroes", ended)
		}
		p, g := f.profile(t)
		ownedAfter, _ := json.Marshal(g.Warriors)
		if p.GetWarriorBagSize() != 1000 || len(g.Warriors) != 1000 || string(ownedAfter) != string(ownedBefore) || !g.Run.Fight.Settled {
			t.Fatal("settlement altered full inventory or stayed pending")
		}
		if taskID == "1" {
			firstEnd, firstReceipt = end, receipt
			f.restart(t)
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					repeated := copyOverflowWire(t, f, firstEnd)
					if r, ok := repeated[0].(*pb.SCCopyFightEnd); !ok || !proto.Equal(r, firstReceipt) {
						t.Error("cold concurrent battle replay changed result")
					}
				}()
			}
			wg.Wait()
		} else if receipt.GetIsCopyComplete() != 1 {
			t.Fatal("full bag did not allow mission completion")
		}
	}
	var rewards []db.RewardDelivery
	if err := db.GetIns().Where("player_id=? AND source_key LIKE ?", f.id, "copy-overflow:%").Order("id").Find(&rewards).Error; err != nil {
		t.Fatal(err)
	}
	if len(rewards) != 5 || p1Count(t, &db.MailMessage{}, "player_id=? AND source_key LIKE ?", f.id, "copy-overflow:%") != 0 {
		t.Fatal("first/ordinary/bonus deliveries were lost or replayed", len(rewards))
	}
	var ids []int64
	var pending int
	for _, reward := range rewards {
		if reward.ExpiresAt != 0 || reward.ClaimedAt != 0 {
			t.Fatal("deferred original reward expired or was falsely claimed")
		}
		ids = append(ids, reward.ID)
		for _, part := range strings.Split(reward.Rewards, ";") {
			pair := strings.Split(part, ",")
			amount, err := p1PositiveAmount(pair[1])
			if err != nil || pair[0] != "hero_exp_006" {
				t.Fatal("invented source in deferred reward", reward.Rewards)
			}
			pending += int(amount)
		}
	}
	if pending < 10 || pending > 18 || pending%2 != 0 {
		t.Fatal("original first/ordinary quantities or double benefit changed", pending)
	}
	query := copyOverflowWire(t, f, &pb.CSQueryUserReward{})
	if _, ok := query[0].(*pb.SCQueryUserReward); !ok {
		t.Fatal("original reward center cannot show deferred content", query)
	}
	claim := &pb.CSAddRewardsToPlayer{RewardIds: ids}
	full := copyOverflowWire(t, f, claim)
	if _, ok := full[0].(*pb.ErrorMessage); !ok {
		t.Fatal("reward center bypassed full-bag protection")
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key LIKE ? AND claimed_at=0", f.id, "copy-overflow:%") != 5 {
		t.Fatal("failed full-bag claim consumed pending attachments")
	}
	// 仅删除隔离测试的末尾填充实例，模拟用户整理；保留首位实际出战主角。
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Warriors = g.Warriors[:980]; return nil }); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	claimed := copyOverflowWire(t, f, claim)
	result, ok := claimed[0].(*pb.SCAddRewardsToPlayer)
	if !ok || len(result.GetResult().GetWars()) != pending {
		t.Fatal("cold claim lost deferred instances", claimed)
	}
	again := copyOverflowWire(t, f, claim)
	if result, ok := again[0].(*pb.SCAddRewardsToPlayer); !ok || len(result.GetResult().GetWars()) != 0 {
		t.Fatal("claimed attachments were issued twice", again)
	}
	p, g := f.profile(t)
	if len(g.Warriors) != 980+pending || p.GetWarriorBagSize() != 1000 || !g.Run.Complete {
		t.Fatal("cold claim changed capacity or completed mission")
	}
	t.Logf("original signed frame/dispatch/response: full capacity1000, two original encounters completed, 5 permanent deliveries, %d deferred instances claimed exactly once", pending)
}

func TestCopyOverflowSpecialStageKeepsOriginalCounterAndMail(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeCopyDrops(f.ctx); err != nil {
		t.Fatal(err)
	}
	// 有限状态夹具覆盖可配置特殊奖励的角色类型，未在生产创建新投放。
	input := &copySpecialInput{Enabled: true, Cycle: 1, TaskID: "fixture-special", Source: "isolated special overflow fixture", Entries: []copySpecialEntry{{Reward: "hero_exp_006,2"}}}
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.WarriorBagSize = proto.Int32(int32(len(g.Warriors)) + 1)
		g.Run = &model.CopyRun{Fight: &model.CopyFight{Start: &pb.SCCopyFightStart{FightId: proto.String("isolated-special-overflow")}}}
		tx = beginCopyOverflow(tx, f.id, g.Run.Fight.Start.GetFightId())
		gain, _, err := f.s.applyCopySpecialInput(tx, f.id, p, g, input)
		if err == nil && (len(gain.GetWars()) != 1 || len(g.Warriors) != int(p.GetWarriorBagSize())) {
			t.Fatal("partial special reward did not fill exactly the available slot")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var state db.CopySpecialState
	if err := db.GetIns().First(&state, "player_id=? AND task_id=?", f.id, input.TaskID).Error; err != nil || state.Granted != 1 || state.Credit != 0 || state.Spent != 1 || state.Successes != 1 {
		t.Fatal("special cadence did not atomically preserve the pending reward", err, state)
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key LIKE ? AND claimed_at=0", f.id, "copy-overflow:%") != 1 {
		t.Fatal("special deferred delivery absent")
	}
	var reward db.RewardDelivery
	if err := db.GetIns().First(&reward, "player_id=? AND source_key LIKE ?", f.id, "copy-overflow:%").Error; err != nil || reward.Rewards != "hero_exp_006,1" {
		t.Fatal("partial capacity lost or duplicated the remaining special reward", err)
	}
}
