package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p10LuckyReward struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Weight int64  `json:"weight"`
}

// InitializeP10Lucky 将原57项权重完整迁至SQL，保留已存在运营值，新玩法默认关闭。
func (s *GameService) InitializeP10Lucky(ctx context.Context) error {
	var rewards []p10LuckyReward
	for _, r := range s.Tables["ItemUse"] {
		if r["itemId"] == constUtil.P10LuckyBagItem {
			w, err := strconv.ParseInt(r["odds"], 10, 64)
			if err != nil || w < 1 || r["category"] != "22" {
				return errors.New("原福袋权重格式无效")
			}
			rewards = append(rewards, p10LuckyReward{r["id"], r["result"], w})
		}
	}
	if len(rewards) != 57 {
		return errors.New("原福袋57项奖池不完整")
	}
	raw, err := json.Marshal(rewards)
	if err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err = tx.AutoMigrate(&db.P10LuckyPolicy{}, &db.P10LuckyDaily{}, &db.P10LuckyObject{}); err != nil {
		return err
	}
	cfg := db.P10LuckyPolicy{ID: constUtil.P10LuckyPolicyID, DropDailyBatches: 10, FireworksIndex: 1, RewardJSON: string(raw), RewardHash: p6Digest(raw), Source: constUtil.P10LuckySource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}
func (s *GameService) p10LuckyConfig(tx *gorm.DB) (*db.P10LuckyPolicy, []p10LuckyReward, error) {
	var cfg db.P10LuckyPolicy
	if err := tx.First(&cfg, constUtil.P10LuckyPolicyID).Error; err != nil {
		return nil, nil, err
	}
	if cfg.Source == "" || cfg.DropDailyBatches < 1 || cfg.DropDailyBatches > 100 || cfg.FireworksIndex < 1 || cfg.FireworksIndex > 3 || p6Digest([]byte(cfg.RewardJSON)) != cfg.RewardHash {
		return nil, nil, errors.New("福袋运营发布或奖池hash无效")
	}
	var rewards []p10LuckyReward
	if err := json.Unmarshal([]byte(cfg.RewardJSON), &rewards); err != nil {
		return nil, nil, err
	}
	if len(rewards) < 1 || len(rewards) > 500 {
		return nil, nil, errors.New("福袋发布奖池数量无效")
	}
	seen := map[string]bool{}
	var total int64
	for _, r := range rewards {
		if r.ID == "" || seen[r.ID] || r.Weight < 1 || r.Weight > math.MaxInt32 || total > math.MaxInt64-r.Weight {
			return nil, nil, errors.New("福袋发布权重无效")
		}
		seen[r.ID] = true
		total += r.Weight
		pair := strings.Split(r.Result, ",")
		if len(pair) != 2 {
			return nil, nil, errors.New("福袋奖励格式无效")
		}
		n, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || n < 1 {
			return nil, nil, errors.New("福袋奖励数量无效")
		}
		if s.table("MineSkill", "MineItemID", pair[0]) == nil {
			ok, err := s.openServerRewardSupported(r.Result)
			if err != nil || !ok {
				return nil, nil, errors.New("完整福袋奖池包含未恢复资源")
			}
		}
	}
	return &cfg, rewards, nil
}
func p10LuckyDistance(a, b *pb.PointNew) float64 {
	if a == nil || b == nil {
		return math.Inf(1)
	}
	var total float64
	for _, v := range []float64{float64(a.GetX() - b.GetX()), float64(a.GetY() - b.GetY()), float64(a.GetZ() - b.GetZ())} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return math.Inf(1)
		}
		total += v * v
	}
	return math.Sqrt(total)
}
func (s *GameService) p10LuckyScope(tx *gorm.DB, id int64) (*db.P10ScenePresence, *pb.MovePlayer, error) {
	cfg, err := p10SceneConfig(tx)
	if err != nil {
		return nil, nil, err
	}
	if cfg == nil {
		return nil, nil, nil
	}
	var own db.P10ScenePresence
	if err = tx.First(&own, "player_id=?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	frame, valid := s.p10SceneLive(tx, own)
	if !valid || own.Scene != "town_main" {
		return nil, nil, nil
	}
	return &own, frame, nil
}
func p10LuckyDaily(tx *gorm.DB, id int64, now time.Time) (*db.P10LuckyDaily, error) {
	day := dailyCopyDay(now)
	out := db.P10LuckyDaily{PlayerID: id, Day: day}
	err := tx.First(&out, "player_id=? AND day=?", id, day).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var old db.P10LuckyDaily
		previousErr := tx.Where("player_id=?", id).Order("day DESC").First(&old).Error
		if previousErr != nil && !errors.Is(previousErr, gorm.ErrRecordNotFound) {
			return nil, previousErr
		}
		// UTC+8跨日清计数，20秒CD及TCP原回执继续携带，避免午夜复制扣费。
		if previousErr == nil {
			out.FireworksAt = old.FireworksAt
			out.DropAt = old.DropAt
			out.DropRequestHash = old.DropRequestHash
			out.DropResponse = append([]byte(nil), old.DropResponse...)
		}
	}
	if out.FireworksUsed < 0 || out.DropBatches < 0 || out.Snatched < 0 {
		return nil, errors.New("福袋日次数存档无效")
	}
	return &out, nil
}
func (s *GameService) p10LuckyGrant(p *pb.Player, g *model.GameState, reward string) (*pb.ResultObject, error) {
	pair := strings.Split(reward, ",")
	if len(pair) != 2 {
		return nil, errors.New("冻结福袋奖励损坏")
	}
	if s.table("MineSkill", "MineItemID", pair[0]) != nil {
		n, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || n < 1 {
			return nil, errors.New("冻结福袋矿石数量损坏")
		}
		if err = s.gemInventoryDelta(p, g, map[string]int64{pair[0]: n}); err != nil {
			return nil, err
		}
		return &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String(pair[0]), Num: proto.Int32(int32(n))}}}, nil
	}
	return s.GrantFixedReward(p, g, reward)
}
func (s *GameService) p10LuckyList(tx *gorm.DB, id int64, now time.Time) (*pb.SCCurrentLuckyBagsInfo, error) {
	out := &pb.SCCurrentLuckyBagsInfo{}
	own, _, err := s.p10LuckyScope(tx, id)
	if err != nil {
		return nil, err
	}
	if own == nil {
		return out, nil
	}
	var rows []db.P10LuckyObject
	if err = tx.Where("scene=? AND line=? AND claimed_at=0 AND expires_at>?", own.Scene, own.Line, now.UnixMilli()).Order("created_at,guid").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		blocked, err := p10Blocked(tx, id, row.OwnerID)
		if err != nil {
			return nil, err
		}
		if blocked {
			continue
		}
		frame := &pb.LuckyBag{}
		if err = proto.Unmarshal(row.Frame, frame); err != nil {
			return nil, err
		}
		out.LuckyBags = append(out.LuckyBags, frame)
	}
	return out, nil
}

// p10LuckyAt 持有实际角色和对象行锁，扣袋、冻结对象或抢取资源、次数、回执在同一InnoDB事务提交。
func (s *GameService) p10LuckyAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	if _, ok := request.(*pb.CSCurrentLuckyBagsInfo); ok {
		out, err := s.p10LuckyList(db.GetIns().WithContext(ctx), id, now)
		return []proto.Message{out}, err
	}
	var response proto.Message
	domain := ""
	switch request.(type) {
	case *pb.CSItemUse:
		response = &pb.SCItemUse{}
		domain = "lucky-fireworks"
	case *pb.CSThrowLuckyBags:
		response = &pb.SCThrowLuckyBags{}
		domain = "lucky-throw"
	case *pb.CSSnatchOneLuckyBag:
		response = &pb.SCSnatchOneLuckyBag{}
		domain = "lucky-snatch"
	default:
		return nil, errors.New("福袋协议尚未恢复")
	}
	var push proto.Message
	var scope *db.P10ScenePresence
	visibleOwner := id
	fresh := false
	var eventKind, eventSource string
	var eventAmount int64
	var costs []*pb.BaseResource
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		if stamp > 0 {
			if found, err := p10ReadReceipt(tx, id, domain, stamp, request, response); err != nil || found {
				return err
			}
		}
		// 同一实际对象已经领取时，原TCP以GUID恢复首笔原回执；其他角色永远不能复用。
		var object db.P10LuckyObject
		if b, ok := request.(*pb.CSSnatchOneLuckyBag); ok {
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&object, "guid=?", b.GetGuid()).Error; err != nil {
				return errors.New("福袋对象不存在")
			}
			if object.ClaimedAt > 0 {
				if object.ClaimedBy != id {
					return errors.New("福袋已被其他玩家抢取")
				}
				if err = proto.Unmarshal(object.ClaimResponse, response); err != nil {
					return err
				}
				if stamp > 0 {
					return p10SaveReceipt(tx, id, domain, stamp, request, response)
				}
				return nil
			}
		}
		daily, err := p10LuckyDaily(tx, id, now)
		if err != nil {
			return err
		}
		if b, ok := request.(*pb.CSThrowLuckyBags); ok && stamp == 0 && daily.DropAt > 0 {
			cd, err := s.p10Setting("luckyBagItemCD")
			if err != nil {
				return err
			}
			if now.UnixMilli()-daily.DropAt < int64(cd) {
				raw, err := proto.Marshal(b)
				if err != nil {
					return err
				}
				if daily.DropRequestHash == p6Digest(raw) {
					return proto.Unmarshal(daily.DropResponse, response)
				}
				return errors.New("原福袋20秒投放CD未到")
			}
		}
		var cfg *db.P10LuckyPolicy
		var rewards []p10LuckyReward
		if _, isClaim := request.(*pb.CSSnatchOneLuckyBag); !isClaim {
			cfg, rewards, err = s.p10LuckyConfig(tx)
			if err != nil {
				return err
			}
			if domain == "lucky-fireworks" && !cfg.FireworksEnabled || domain == "lucky-throw" && !cfg.DropEnabled {
				return errors.New(constUtil.P10LuckyClosed)
			}
		}
		daily, err = p10LuckyDaily(tx, id, now)
		if err != nil {
			return err
		}
		var frame *pb.MovePlayer
		scope, frame, err = s.p10LuckyScope(tx, id)
		if err != nil {
			return err
		}
		if scope == nil {
			return errors.New("福袋仅原主城真实公共同线TCP可用")
		}
		switch b := request.(type) {
		case *pb.CSItemUse:
			if stamp <= 0 {
				return errors.New("原烟花HTTP使用缺少回执时间戳")
			}
			if !cfg.FireworksEnabled {
				return errors.New(constUtil.P10LuckyClosed)
			}
			if b.GetCostItem().GetId() != constUtil.P10FireworksItem || b.GetCostItem().GetNum() != 1 || b.GetCostItem().GetGuid() != "" || b.GetGetItemIndex() != 0 {
				return errors.New("原烟花单次需1份且无选择索引")
			}
			level, err := s.p10Setting("fireworksLevelLimit")
			if err != nil {
				return err
			}
			limit, err := s.p10Setting("fireworksUseTimes")
			if err != nil {
				return err
			}
			cd, err := s.p10Setting("fireworksItemCD")
			if err != nil {
				return err
			}
			if p.GetLevel() < level || daily.FireworksUsed >= limit || daily.FireworksAt > 0 && now.UnixMilli()-daily.FireworksAt < int64(cd) {
				return errors.New("烟花原等级/日次数/CD未满足")
			}
			var channel db.P10ChannelPolicy
			if err = tx.First(&channel, 1).Error; err != nil {
				return err
			}
			if !channel.WorldEnabled {
				return errors.New("请先开放原世界文字后使用烟花")
			}
			var original map[string]string
			for _, r := range s.Tables["ItemUse"] {
				if r["itemId"] == constUtil.P10FireworksItem {
					if original != nil {
						return errors.New("原烟花兑换不唯一")
					}
					original = r
				}
			}
			if original == nil || original["category"] != "21" || original["result"] != "item_349,20" {
				return errors.New("原烟花20福袋兑换损坏")
			}
			if err = deductPoolCost(p, g, b.GetCostItem()); err != nil {
				return err
			}
			gain, err := s.GrantFixedReward(p, g, original["result"])
			if err != nil {
				return err
			}
			costs = []*pb.BaseResource{proto.Clone(b.GetCostItem()).(*pb.BaseResource)}
			response = &pb.SCItemUse{Cost: proto.Clone(b.GetCostItem()).(*pb.BaseResource), Get: gain}
			eventKind = "fireworks_used"
			eventSource = fmt.Sprintf("p10-fireworks:%d:%d", id, stamp)
			eventAmount = 1
			daily.FireworksUsed++
			daily.FireworksAt = now.UnixMilli()
			push = &pb.SCSpecialEfficiency{Index: proto.Int32(cfg.FireworksIndex), PlayerId: proto.Int64(id)}
		case *pb.CSThrowLuckyBags:
			if !cfg.DropEnabled {
				return errors.New(constUtil.P10LuckyClosed)
			}
			raw, err := proto.Marshal(b)
			if err != nil {
				return err
			}
			hash := p6Digest(raw)
			cd, err := s.p10Setting("luckyBagItemCD")
			if err != nil {
				return err
			}
			if daily.DropAt > 0 && now.UnixMilli()-daily.DropAt < int64(cd) {
				if stamp == 0 && daily.DropRequestHash == hash {
					return proto.Unmarshal(daily.DropResponse, response)
				}
				return errors.New("原福袋20秒投放CD未到")
			}
			maxNum, err := s.p10Setting("maxDropLuckyBagsNum")
			if err != nil {
				return err
			}
			level, err := s.p10Setting("ChatLevel")
			if err != nil {
				return err
			}
			expiry, err := s.p10Setting("throwItemOverdueTime")
			if err != nil {
				return err
			}
			rangeRow := s.table("GameConfigure", "StaticName", "throwItemRange")
			radius, err := strconv.ParseFloat(rangeRow["NameValue"], 64)
			if err != nil || radius <= 0 {
				return errors.New("原福袋投放半径无效")
			}
			if b.GetNum() < 1 || b.GetNum() > maxNum || len(b.GetPosArray()) != int(b.GetNum()) || p.GetLevel() < level || daily.DropBatches >= cfg.DropDailyBatches {
				return errors.New("福袋投放原数量/等级或本地日批次未满足")
			}
			for _, point := range b.GetPosArray() {
				if p10LuckyDistance(frame.GetPosition().GetCurrent(), point) > radius {
					return errors.New("福袋投放超出原3米范围")
				}
			}
			var active int64
			if err = tx.Model(&db.P10LuckyObject{}).Where("scene=? AND line=? AND claimed_at=0 AND expires_at>?", scope.Scene, scope.Line, now.UnixMilli()).Count(&active).Error; err != nil {
				return err
			}
			if active+int64(b.GetNum()) > constUtil.P10LuckyActiveScopeCap {
				return errors.New("本地同线有效福袋1000上限")
			}
			cost := &pb.BaseResource{Id: proto.String(constUtil.P10LuckyBagItem), Num: b.Num}
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
			weights := []db.GameplayPoolReward{}
			for i, r := range rewards {
				weights = append(weights, db.GameplayPoolReward{ID: int32(i), Weight: r.Weight, Amount: 1})
			}
			appear := &pb.SCCurrentLuckyBagsInfo{ThrowPlayerId: proto.Int64(id)}
			for _, point := range b.GetPosArray() {
				picked, err := choosePoolReward(weights, false)
				if err != nil {
					return err
				}
				reward := rewards[int(picked.ID)].Result
				bag := &pb.LuckyBag{Guid: proto.String(uuid.NewString()), ItemId: proto.String(constUtil.P10LuckyBagItem), PlayerId: proto.Int64(id), CreateTime: proto.Int64(now.UnixMilli()), Pos: proto.Clone(point).(*pb.PointNew)}
				blob, err := proto.Marshal(bag)
				if err != nil {
					return err
				}
				obj := db.P10LuckyObject{GUID: bag.GetGuid(), OwnerID: id, OwnerName: p.GetName(), Scene: scope.Scene, Line: scope.Line, CreatedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + int64(expiry), Frame: blob, Reward: reward, RewardHash: cfg.RewardHash}
				if err = tx.Create(&obj).Error; err != nil {
					return err
				}
				appear.LuckyBags = append(appear.LuckyBags, bag)
			}
			costs = []*pb.BaseResource{cost}
			response = &pb.SCThrowLuckyBags{CostResource: []*pb.BaseResource{cost}}
			daily.DropBatches++
			daily.DropAt = now.UnixMilli()
			daily.DropRequestHash = hash
			blob, err := proto.Marshal(response)
			if err != nil {
				return err
			}
			daily.DropResponse = blob
			eventKind = "lucky_bags_dropped"
			eventSource = "p10-lucky-drop:" + appear.LuckyBags[0].GetGuid()
			eventAmount = int64(b.GetNum())
			push = appear
		case *pb.CSSnatchOneLuckyBag:
			// 已支付来源的真实冻结对象继续兑现，关停新投放不使既有1小时资格失效。
			if object.Scene != scope.Scene || object.Line != scope.Line || object.ExpiresAt <= now.UnixMilli() {
				return errors.New("福袋不在当前同线或已过期")
			}
			blocked, err := p10Blocked(tx, id, object.OwnerID)
			if err != nil {
				return err
			}
			if blocked {
				return errors.New("黑名单阻止双方福袋共享")
			}
			distance, err := s.p10Setting("snatchLuckyBagDistanceLimit")
			if err != nil {
				return err
			}
			limit, err := s.p10Setting("maxSnatchLuckyBagNum")
			if err != nil {
				return err
			}
			bag := &pb.LuckyBag{}
			if err = proto.Unmarshal(object.Frame, bag); err != nil {
				return err
			}
			if p10LuckyDistance(frame.GetPosition().GetCurrent(), bag.GetPos()) > float64(distance) || daily.Snatched >= limit {
				return errors.New("福袋超出原5米抢距或日100上限")
			}
			gain, err := s.p10LuckyGrant(p, g, object.Reward)
			if err != nil {
				return err
			}
			response = &pb.SCSnatchOneLuckyBag{Get: gain, ThrowPlayerName: proto.String(object.OwnerName)}
			blob, err := proto.Marshal(response)
			if err != nil {
				return err
			}
			object.ClaimedBy = id
			object.ClaimedAt = now.UnixMilli()
			object.ClaimResponse = blob
			if err = tx.Save(&object).Error; err != nil {
				return err
			}
			eventKind = "lucky_bag_claimed"
			eventSource = "p10-lucky-snatch:" + object.GUID
			eventAmount = 1
			daily.Snatched++
			visibleOwner = object.OwnerID
			push = &pb.SCRemoveOneLuckyBag{Guid: proto.String(object.GUID)}
		}
		if s.P1Ready {
			if len(costs) > 0 {
				if err = s.RecordP1Cost(tx, id, eventSource, costs, now); err != nil {
					return err
				}
			}
			if err = s.RecordP1Event(tx, id, eventSource, eventKind, eventAmount, now); err != nil {
				return err
			}
		}
		if err = tx.Save(daily).Error; err != nil {
			return err
		}
		if err = saveProfile(tx, &profile, p, g); err != nil {
			return err
		}
		fresh = true
		if stamp > 0 {
			return p10SaveReceipt(tx, id, domain, stamp, request, response)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := []proto.Message{response}
	if fresh && push != nil {
		// 原抢取移除/投放动画在提交后发给同线当前真实角色；单次回执重试不重复广播。
		s.p10LuckyBroadcast(ctx, scope.Scene, scope.Line, id, visibleOwner, push)
		if _, ok := push.(*pb.SCSpecialEfficiency); ok {
			s.publishP10Private(ctx, id, push)
		} else {
			result = append(result, push)
		}
	}
	return result, nil
}

// p10LuckyBroadcast 沿投放者共享权限广播对象，移除依然以投放者定域，避免留下已领取的幽灵模型。
func (s *GameService) p10LuckyBroadcast(ctx context.Context, scene string, line int32, exclude, owner int64, push proto.Message) {
	tx := db.GetIns().WithContext(ctx)
	peers, err := s.p10ScenePeers(tx, scene, line, exclude)
	if err != nil {
		return
	}
	for _, peer := range peers {
		target := peer.GetInfo().GetPlayerId()
		blocked, err := p10Blocked(tx, target, owner)
		if err != nil || blocked {
			continue
		}
		s.publishP10Private(ctx, target, push)
	}
}

func (s *GameService) DispatchP10Lucky(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSItemUse:
		if b.GetCostItem().GetId() != constUtil.P10FireworksItem {
			return nil, false, nil
		}
	case *pb.CSThrowLuckyBags, *pb.CSSnatchOneLuckyBag, *pb.CSCurrentLuckyBagsInfo:
	default:
		return nil, false, nil
	}
	result, err := s.p10LuckyAt(ctx, id, request, stamp, time.Now())
	return result, true, err
}
