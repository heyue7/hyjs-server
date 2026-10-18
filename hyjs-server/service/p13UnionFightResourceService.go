package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"time"
)

// p13UFResourceOverlayCamp 在静态建筑快照上投影本場实际已上交资源，不改历史地图SHA。
func (s *GameService) p13UFResourceOverlayCamp(tx *gorm.DB, scope string, buildings *pb.SCUFBuildingRefresh) error {
	var camps []db.P13UnionFightCampResource
	if err := tx.Where("match_id=?", scope).Find(&camps).Error; err != nil {
		return err
	}
	for _, camp := range camps {
		if camp.Amount < 0 || camp.Amount > 9007199254740991 {
			return errors.New("公会战已上交资源无效")
		}
		for _, union := range buildings.UnionInfos {
			if union.GetUnionId() == camp.GuildID && int64(union.GetCamp()) == camp.Camp {
				union.UnionResource = &pb.InstUFResource{ResourceNum: proto.Float64(float64(camp.Amount))}
			}
		}
	}
	return nil
}

func (s *GameService) p13UFResourceDeposit(ctx context.Context, id int64, request *pb.CSUFPlayerUpResource, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerUpResource
	var update *pb.SCUFCampResourceUpdate
	var missionPush *pb.SCUFMissionPush
	var scope string
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		scope = match.ID
		raw := append(append([]byte(scope), 0), []byte(request.GetNpcId())...)
		if stamp > 0 {
			var receipt db.P13GuildReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, constUtil.P13UnionFightResourceDepositOperation, stamp).Error
			if err == nil {
				if !bytes.Equal(raw, receipt.Request) {
					return errors.New("公会战回收请求时间戳冲突")
				}
				reply = &pb.SCUFPlayerUpResource{}
				return proto.Unmarshal(receipt.Response, reply)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err = p13UFPlayerBattleAction(tx, actor, frame); err != nil {
			return err
		}
		if now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt || actor.Hooked || frame.GetState() == 2 || frame.GetState() == 4 || frame.GetState() == 12 {
			return errors.New("原Fight阶段且非挂机/采集中才可回收资源")
		}
		if snapshot.ResourceConfig == nil {
			return errors.New("本场资源供应未冻结")
		}
		var profile db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return errors.New("原新场景Current握手尚未完成")
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		npc := original.table("UnionFightNpc", "UFNId", request.GetNpcId())
		if npc == nil || npc["Building"] != "recycle_bin" || npc["Camp"] != strconv.FormatInt(actor.Camp, 10) {
			return errors.New("原回收NPC必须属于本场本阵营")
		}
		position, err := fieldPoint(npc["Position"])
		if err != nil {
			return err
		}
		if p13UFResourceDistance(actor, position.GetX(), position.GetY(), position.GetZ()) > snapshot.ResourceConfig.RecycleDistance {
			return errors.New("原回收NPC验证距离超过3米")
		}
		owned := frame.GetOwnRes().GetResourceNum()
		if owned <= 0 || owned != math.Trunc(owned) || owned > float64(snapshot.ResourceConfig.Capacity) {
			return errors.New("原背包没有可上交资源")
		}
		offer := owned * snapshot.ResourceConfig.OfferFactor
		if offer <= 0 || offer != math.Trunc(offer) || offer > math.MaxInt32 {
			return errors.New("原本次战争贡献换算不能精确表示")
		}
		var camp db.P13UnionFightCampResource
		err = tx.First(&camp, "match_id=? AND guild_id=?", scope, actor.GuildID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			camp = db.P13UnionFightCampResource{MatchID: scope, GuildID: actor.GuildID, Camp: actor.Camp, Source: constUtil.P13UnionFightResourceSource}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if camp.Camp != actor.Camp || camp.Amount < 0 || float64(camp.Amount)+owned > 9007199254740991 {
			return errors.New("公会战阵营资源归属/总量无效")
		}
		camp.Amount += int64(owned)
		if err = tx.Save(&camp).Error; err != nil {
			return err
		}
		previousMissionScore, err := p13UFMissionScore(tx, match, actor)
		if err != nil {
			return err
		}
		if err = p13UFTransportDeposit(tx, actor, int64(owned), now); err != nil {
			return err
		}
		missionPush, err = p13UFMissionSync(tx, match, actor, frame, previousMissionScore)
		if err != nil {
			return err
		}
		frame.OwnRes = &pb.InstUFResource{ResourceNum: proto.Float64(0)}
		frame.OwnToken = proto.Float64(frame.GetOwnToken() + offer)
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		reply = &pb.SCUFPlayerUpResource{Player: frame, ResourceNum: proto.Int32(int32(owned)), PlayerReward: proto.Int32(int32(offer))}
		update = &pb.SCUFCampResourceUpdate{Camp: proto.Int32(int32(actor.Camp)), Resource: &pb.InstUFResource{ResourceNum: proto.Float64(float64(camp.Amount))}}
		fresh = true
		if stamp > 0 {
			encoded, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			return tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: constUtil.P13UnionFightResourceDepositOperation, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p13UFFieldBroadcast(ctx, scope, 0, update)
	}
	messages := []proto.Message{reply}
	if fresh && missionPush != nil {
		messages = append(messages, missionPush)
	}
	return messages, nil
}

// p13UFResourceConfig 保存本场冻结的原容量、距离与战争贡献换算；本地发布矿点写入其独立来源。
type p13UFResourceConfig struct {
	Capacity        int64
	ArriveDistance  float64
	RecycleDistance float64
	OfferFactor     float64
}

func (s *GameService) InitializeP13UnionFightResource(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightResourceSource{}, &db.P13UnionFightResourceNode{}, &db.P13UnionFightResourceTask{}, &db.P13UnionFightCampResource{}, &db.P13UnionFightTransportScope{}, &db.P13UnionFightTransportStat{})
}

// p13UFTransportDeposit is called after authentic ore was verified and before
// the same transaction clears it. HTTP retries return their old receipt before
// this point; TCP0 retries see an empty backpack and cannot increment twice.
func p13UFTransportDeposit(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, ore int64, now time.Time) error {
	var scope db.P13UnionFightTransportScope
	err := tx.First(&scope, "match_id=?", actor.MatchID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // Existing legacy fields retain gameplay without invented history.
	} else if err != nil {
		return err
	}
	if scope.Source != constUtil.P13UnionFightTransportSource || now.UnixMilli() < scope.StartedAt || ore <= 0 {
		return errors.New("公会战实际矿运统计来源无效")
	}
	var stat db.P13UnionFightTransportStat
	err = tx.First(&stat, "match_id=? AND player_id=?", actor.MatchID, actor.PlayerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		stat = db.P13UnionFightTransportStat{MatchID: actor.MatchID, PlayerID: actor.PlayerID, GuildID: actor.GuildID}
	} else if err != nil {
		return err
	}
	if stat.GuildID != actor.GuildID || stat.Ore < 0 || stat.Ore > math.MaxInt64-ore || stat.Deposits < 0 || stat.Deposits == math.MaxInt64 {
		return errors.New("公会战实际矿运累计无效")
	}
	stat.Ore += ore
	stat.Deposits++
	stat.UpdatedAt = now.UnixMilli()
	return tx.Save(&stat).Error
}

func p13UFResourceSourceDigest(row *db.P13UnionFightResourceNode, point *pb.InstUFResourcePoint) string {
	raw, _ := json.Marshal(struct {
		MatchID                 string
		GUID                    int64
		ResourceID              string
		Reward                  float64
		X, Y, Z                 float32
		TotalUses, GatherMillis int64
		Source                  string
	}{row.MatchID, row.GUID, point.GetResourceId(), point.GetRewardRes().GetResourceNum(), row.X, row.Y, row.Z, row.TotalUses, row.GatherMillis, row.Source})
	return p6Digest(raw)
}

func p13UFResourcePoint(row *db.P13UnionFightResourceNode) (*pb.InstUFResourcePoint, error) {
	if p6Digest(row.Frame) != row.FrameHash {
		return nil, errors.New("公会战矿点快照损坏")
	}
	point := &pb.InstUFResourcePoint{}
	if err := proto.Unmarshal(row.Frame, point); err != nil {
		return nil, err
	}
	if point.GetGuid() != row.GUID || point.GetResourceId() == "" || point.GetRewardRes().GetResourceNum() <= 0 || math.IsNaN(point.GetRewardRes().GetResourceNum()) || math.IsInf(point.GetRewardRes().GetResourceNum(), 0) || point.GetRewardRes().GetResourceNum() != math.Trunc(point.GetRewardRes().GetResourceNum()) || row.RemainingUses < 0 || row.RemainingUses > row.TotalUses || row.GatherMillis <= 0 {
		return nil, errors.New("公会战矿点原身份损坏")
	}
	if row.SourceHash == "" || row.SourceHash != p13UFResourceSourceDigest(row, point) {
		return nil, errors.New("公会战矿点原类型/固定位置/产量来源SHA损坏")
	}
	return point, nil
}

func p13UFResourceSaveNode(tx *gorm.DB, row *db.P13UnionFightResourceNode, frame *pb.InstUFResourcePoint) error {
	raw, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	row.Frame = raw
	row.FrameHash = p6Digest(raw)
	return tx.Save(row).Error
}

// p13UFResourceBuild 冻结已显式发布的矿点，原spawn/再生无证据时没有隐式供应。
func (s *GameService) p13UFResourceBuild(tx *gorm.DB, match *db.P13UnionFightMatch, snapshot *p13UFFieldSnapshot, original *GameService, now time.Time) error {
	// Only initial field creation creates the complete-observation marker. Cold
	// loading an old field never backfills a plausible but incomplete history.
	if err := tx.Create(&db.P13UnionFightTransportScope{MatchID: match.ID, StartedAt: now.UnixMilli(), Source: constUtil.P13UnionFightTransportSource}).Error; err != nil {
		return err
	}
	read := func(name string) (float64, error) {
		value, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", name)["NameValue"], 64)
		if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errors.New("公会战原资源配置缺失：" + name)
		}
		return value, nil
	}
	capacity, err := read("UnionFightResNum")
	if err != nil {
		return err
	}
	arrive, err := read("UnionFightArriveResourcePointDistance")
	if err != nil {
		return err
	}
	recycle, err := read("UnionFightResMinDistance")
	if err != nil {
		return err
	}
	factor, err := read("UnionFightResConvertOfferFactor")
	if err != nil {
		return err
	}
	if capacity != math.Trunc(capacity) || capacity > math.MaxInt32 {
		return errors.New("公会战原矿石容量无效")
	}
	snapshot.ResourceConfig = &p13UFResourceConfig{Capacity: int64(capacity), ArriveDistance: arrive, RecycleDistance: recycle, OfferFactor: factor}
	var sources []db.P13UnionFightResourceSource
	if err = tx.Where("day=? AND enabled=1", match.Day).Order("source_key").Find(&sources).Error; err != nil {
		return err
	}
	if len(sources) > 100 {
		return errors.New("明确本地矿点发布超过本实例100点上限")
	}
	for _, source := range sources {
		if source.SourceKey == "" || source.Source == "" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || source.RewardRes <= 0 || source.RewardRes > int64(capacity) {
			return errors.New("公会战本地矿点位置/产量来源未完整发布")
		}
		for i, value := range []float32{source.X, source.Y, source.Z} {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || float64(value) < float64(snapshot.Minimum[i])-snapshot.MoveTolerance || float64(value) > float64(snapshot.Maximum[i])+snapshot.MoveTolerance {
				return errors.New("发布矿点超出本場固定地图范围")
			}
		}
		row := original.table("UnionFightResource", "UFRId", source.ResourceID)
		if row == nil {
			return errors.New("发布矿点缺少原资源类型")
		}
		seconds, err := strconv.ParseFloat(row["GatherCostTime"], 64)
		if err != nil || seconds <= 0 || seconds > 86400 {
			return errors.New("原矿点采集时长缺失")
		}
		uses, err := strconv.ParseInt(row["GatherTime"], 10, 64)
		if err != nil || uses <= 0 || uses > 100000 {
			return errors.New("原矿点可采集次数缺失")
		}
		guid := p13UFFieldGUID(match.ID, "resource", source.SourceKey)
		frame := &pb.InstUFResourcePoint{Guid: proto.Int64(guid), ResourceId: proto.String(source.ResourceID), State: proto.Int32(0), RewardRes: &pb.InstUFResource{ResourceNum: proto.Float64(float64(source.RewardRes))}}
		node := db.P13UnionFightResourceNode{MatchID: match.ID, GUID: guid, X: source.X, Y: source.Y, Z: source.Z, TotalUses: uses, RemainingUses: uses, GatherMillis: int64(seconds * 1000), Source: constUtil.P13UnionFightResourceSource + "；发布：" + source.Source}
		node.SourceHash = p13UFResourceSourceDigest(&node, frame)
		if err = p13UFResourceSaveNode(tx, &node, frame); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) p13UFResourceFrames(tx *gorm.DB, scope string, guids map[int64]bool) ([]proto.Message, error) {
	var nodes []db.P13UnionFightResourceNode
	if err := tx.Where("match_id=?", scope).Order("guid").Find(&nodes).Error; err != nil {
		return nil, err
	}
	response := &pb.SCUFResourcePoint{}
	for _, node := range nodes {
		if len(guids) > 0 && !guids[node.GUID] {
			continue
		}
		frame, err := p13UFResourcePoint(&node)
		if err != nil {
			return nil, err
		}
		response.Point = append(response.Point, frame)
		response.Local = append(response.Local, &pb.PointNew{X: proto.Float32(node.X), Y: proto.Float32(node.Y), Z: proto.Float32(node.Z)})
	}
	if len(response.Point) == 0 {
		return nil, nil
	}
	return []proto.Message{response}, nil
}

func p13UFResourceDistance(actor *db.P13UnionFightFieldPlayer, x, y, z float32) float64 {
	dx, dy, dz := float64(actor.X-x), float64(actor.Y-y), float64(actor.Z-z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func (s *GameService) p13UFResourceBegin(ctx context.Context, id int64, request *pb.CSUFPlayerCollect, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerCollect
	var scope string
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		if err = p13UFPlayerBattleAction(tx, actor, frame); err != nil {
			return err
		}
		if now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt || actor.Hooked {
			return errors.New("原Fight阶段且非挂机才可采矿")
		}
		if snapshot.ResourceConfig == nil {
			return errors.New("本场没有已冻结的真实矿点供应")
		}
		if frame.GetState() == 4 || frame.GetState() == 12 {
			return errors.New("原传送尚未完成，不能开始采矿")
		}
		var profile db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if game.PositionLoading {
			return errors.New("原新场景Current握手尚未完成")
		}
		var active db.P13UnionFightResourceTask
		err = tx.First(&active, "match_id=? AND player_id=? AND status=?", match.ID, id, constUtil.P13UnionFightResourcePending).Error
		if err == nil {
			if active.NodeGUID != request.GetResGuid() {
				return errors.New("本角色已有采集尚未结束")
			}
			reply = &pb.SCUFPlayerCollect{PlayerGuid: []int64{id}, ResGuid: []int64{active.NodeGUID}, CollectTime: []float64{float64(max(int64(0), active.DueAt-now.UnixMilli())) / 1000}}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var node db.P13UnionFightResourceNode
		if err = tx.First(&node, "match_id=? AND guid=?", match.ID, request.GetResGuid()).Error; err != nil {
			return errors.New("本场没有请求矿点")
		}
		point, err := p13UFResourcePoint(&node)
		if err != nil {
			return err
		}
		if node.RemainingUses <= 0 || point.GetState() == 2 {
			return errors.New("原矿点已耗尽")
		}
		if node.BusyTaskID != "" {
			return errors.New("本地单点仲裁：已有实际采集者")
		}
		if p13UFResourceDistance(actor, node.X, node.Y, node.Z) > snapshot.ResourceConfig.ArriveDistance {
			return errors.New("原采矿距离不足以到达矿点")
		}
		owned := frame.GetOwnRes().GetResourceNum()
		if owned < 0 || owned != math.Trunc(owned) || owned+point.GetRewardRes().GetResourceNum() > float64(snapshot.ResourceConfig.Capacity) {
			return errors.New("本地全额容量预留：原矿石背包不能容纳本次资源")
		}
		if now.UnixMilli()+node.GatherMillis > match.EndAt {
			return errors.New("本场剩余时间不足一次完整采集")
		}
		var ordinal int64
		if err = tx.Model(&db.P13UnionFightResourceTask{}).Where("match_id=? AND player_id=?", match.ID, id).Count(&ordinal).Error; err != nil {
			return err
		}
		task := db.P13UnionFightResourceTask{ID: p6Digest([]byte(fmt.Sprintf("%s/%d/%d/%d", match.ID, id, now.UnixMilli(), ordinal+1))), MatchID: match.ID, PlayerID: id, NodeGUID: node.GUID, StartedAt: now.UnixMilli(), DueAt: now.UnixMilli() + node.GatherMillis, Source: node.Source}
		if err = tx.Create(&task).Error; err != nil {
			return err
		}
		node.BusyTaskID = task.ID
		point.State = proto.Int32(1)
		if err = p13UFResourceSaveNode(tx, &node, point); err != nil {
			return err
		}
		frame.State = proto.Int32(2)
		frame.LeftTime = proto.Float32(float32(node.GatherMillis) / 1000)
		frame.TotalTime = proto.Float32(float32(node.GatherMillis) / 1000)
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		scope = match.ID
		fresh = true
		reply = &pb.SCUFPlayerCollect{PlayerGuid: []int64{id}, ResGuid: []int64{node.GUID}, CollectTime: []float64{float64(node.GatherMillis) / 1000}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p13UFFieldBroadcast(ctx, scope, id, reply)
	}
	return []proto.Message{reply}, nil
}

// p13UFResourceCancel 只撤销已开始但未取得的租约；旧loading移动由调用方先过滤，不进入此函数。
func (s *GameService) p13UFResourceCancel(tx *gorm.DB, scope string, id int64, now time.Time) (int64, bool, error) {
	var task db.P13UnionFightResourceTask
	err := tx.First(&task, "match_id=? AND player_id=? AND status=?", scope, id, constUtil.P13UnionFightResourcePending).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var node db.P13UnionFightResourceNode
	if err = tx.First(&node, "match_id=? AND guid=?", scope, task.NodeGUID).Error; err != nil {
		return 0, false, err
	}
	point, err := p13UFResourcePoint(&node)
	if err != nil {
		return 0, false, err
	}
	if node.BusyTaskID != task.ID {
		return 0, false, errors.New("公会战采集租约损坏")
	}
	node.BusyTaskID = ""
	point.State = proto.Int32(0)
	if node.RemainingUses == 0 {
		point.State = proto.Int32(2)
	}
	if err = p13UFResourceSaveNode(tx, &node, point); err != nil {
		return 0, false, err
	}
	if err = tx.Model(&task).Updates(map[string]any{"status": constUtil.P13UnionFightResourceCanceled, "finished_at": now.UnixMilli()}).Error; err != nil {
		return 0, false, err
	}
	var actor db.P13UnionFightFieldPlayer
	if err = tx.First(&actor, "match_id=? AND player_id=?", scope, id).Error; err != nil {
		return 0, false, err
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		return 0, false, err
	}
	frame.State, frame.LeftTime, frame.TotalTime = proto.Int32(0), proto.Float32(0), proto.Float32(0)
	if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
		return 0, false, err
	}
	return task.NodeGUID, true, nil
}

// TickP13UnionFightResource 在真实计时/资格/租约同事务下完成或撤销；停服越过比赛结束仍按本场合法到期记录完成，未完成/已撤销任务没有资源。
func (s *GameService) TickP13UnionFightResource(ctx context.Context, now time.Time) error {
	var tasks []db.P13UnionFightResourceTask
	if err := db.GetIns().WithContext(ctx).Where("status=?", constUtil.P13UnionFightResourcePending).Order("due_at,id").Find(&tasks).Error; err != nil {
		return err
	}
	for _, candidate := range tasks {
		var detail *pb.SCUFPlayerCollectEndDetail
		var ended *pb.SCUFPlayerCollectEnd
		var missionPush *pb.SCUFMissionPush
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			var task db.P13UnionFightResourceTask
			if err := tx.First(&task, "id=?", candidate.ID).Error; err != nil {
				return err
			}
			if task.Status != constUtil.P13UnionFightResourcePending {
				return nil
			}
			var actor db.P13UnionFightFieldPlayer
			if err := tx.First(&actor, "match_id=? AND player_id=?", task.MatchID, task.PlayerID).Error; err != nil {
				return err
			}
			frame, err := p13UFFieldReadActor(&actor)
			if err != nil {
				return err
			}
			var node db.P13UnionFightResourceNode
			if err = tx.First(&node, "match_id=? AND guid=?", task.MatchID, task.NodeGUID).Error; err != nil {
				return err
			}
			point, err := p13UFResourcePoint(&node)
			if err != nil {
				return err
			}
			if node.BusyTaskID != task.ID {
				return errors.New("公会战待结矿点租约损坏")
			}
			var match db.P13UnionFightMatch
			if err = tx.First(&match, "id=?", task.MatchID).Error; err != nil {
				return err
			}
			var field db.P13UnionFightFieldPolicy
			if err = tx.First(&field, 1).Error; err != nil {
				return err
			}
			var enrollment db.P13UnionFightPolicy
			if err = tx.First(&enrollment, 1).Error; err != nil {
				return err
			}
			member, err := p13GuildMember(tx, task.PlayerID)
			if err != nil {
				return err
			}
			valid := gate.ManagementEnabled && field.Enabled && enrollment.EnrollmentEnabled && enrollment.MatchEnabled && actor.LeftAt == 0 && member.GuildID == actor.GuildID && task.StartedAt >= match.StartAt && task.DueAt <= match.EndAt
			if valid && now.UnixMilli() < task.DueAt {
				return nil
			}
			previousMissionScore, err := p13UFMissionScore(tx, &match, &actor)
			if err != nil {
				return err
			}
			grant := int64(0)
			if valid {
				grant = int64(point.GetRewardRes().GetResourceNum())
				if node.RemainingUses <= 0 {
					return errors.New("公会战采集剩余次数损坏")
				}
				frame.OwnRes = &pb.InstUFResource{ResourceNum: proto.Float64(frame.GetOwnRes().GetResourceNum() + float64(grant))}
				node.RemainingUses--
				task.Status = constUtil.P13UnionFightResourceDone
			} else {
				task.Status = constUtil.P13UnionFightResourceCanceled
			}
			node.BusyTaskID = ""
			point.State = proto.Int32(0)
			exhausted := int32(0)
			if node.RemainingUses == 0 {
				exhausted = 1
				point.State = proto.Int32(2)
			}
			if err = p13UFResourceSaveNode(tx, &node, point); err != nil {
				return err
			}
			frame.State = proto.Int32(0)
			frame.LeftTime = proto.Float32(0)
			frame.TotalTime = proto.Float32(0)
			if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
				return err
			}
			detail = &pb.SCUFPlayerCollectEndDetail{Player: frame, ResGuid: proto.Int64(node.GUID), IsResExhausted: proto.Int32(exhausted), AddResourceNum: proto.Int32(int32(grant))}
			ended = &pb.SCUFPlayerCollectEnd{PlayerGuid: []int64{task.PlayerID}, ResGuid: []int64{node.GUID}, IsResExhausted: []int32{exhausted}}
			task.FinishedAt = now.UnixMilli()
			task.Granted = grant
			task.Result, err = proto.Marshal(detail)
			if err != nil {
				return err
			}
			task.ResultHash = p6Digest(task.Result)
			if err = tx.Save(&task).Error; err != nil {
				return err
			}
			missionPush, err = p13UFMissionSync(tx, &match, &actor, frame, previousMissionScore)
			if err != nil {
				return err
			}
			// Persist the final reply after the same transaction applies real mission points.
			task.Result, err = proto.Marshal(detail)
			if err != nil {
				return err
			}
			task.ResultHash = p6Digest(task.Result)
			return tx.Save(&task).Error
		})
		if err != nil {
			return err
		}
		if ended != nil {
			s.p13UFFieldBroadcast(ctx, candidate.MatchID, candidate.PlayerID, ended)
			s.publishP10Private(ctx, candidate.PlayerID, detail)
			if missionPush != nil {
				s.publishP10Private(ctx, candidate.PlayerID, missionPush)
			}
		}
	}
	return nil
}

func (s *GameService) p13UFResourceRecovery(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, now time.Time) ([]proto.Message, error) {
	var task db.P13UnionFightResourceTask
	err := tx.First(&task, "match_id=? AND player_id=? AND status=?", actor.MatchID, actor.PlayerID, constUtil.P13UnionFightResourcePending).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCUFPlayerCollect{PlayerGuid: []int64{actor.PlayerID}, ResGuid: []int64{task.NodeGUID}, CollectTime: []float64{float64(max(int64(0), task.DueAt-now.UnixMilli())) / 1000}}}, nil
}
