package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

func (s *GameService) InitializeP13UnionFightFactory(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightFactorySource{}, &db.P13UnionFightFactoryState{}, &db.P13UnionFightFactoryCycleSource{}, &db.P13UnionFightFactoryCycleOffer{}, &db.P13UnionFightFactoryCycleState{}, &db.P13UnionFightCarGeneration{})
}
func p13UFFactoryDigest(row *db.P13UnionFightFactoryState) string {
	raw, _ := json.Marshal(struct {
		MatchID               string
		GUID, FactoryID, Camp int64
		HP                    float64
		Source                string
	}{row.MatchID, row.GUID, row.FactoryID, row.Camp, row.ChariotHP, row.Source})
	return p6Digest(raw)
}
func p13UFFactoryStateDigest(row *db.P13UnionFightFactoryState) string {
	raw, _ := json.Marshal(struct {
		MatchID                                                    string
		GUID, OpenedAt, StartedAt, AnimationAt, DueAt, CompletedAt int64
		FrameHash, CarHash                                         string
	}{row.MatchID, row.GUID, row.OpenedAt, row.StartedAt, row.AnimationAt, row.DueAt, row.CompletedAt, row.FrameHash, row.CarHash})
	return p6Digest(raw)
}
func p13UFFactoryFrame(row *db.P13UnionFightFactoryState) (*pb.InstUFCariotFactory, error) {
	if p13UFFactoryDigest(row) != row.SourceHash || p6Digest(row.Frame) != row.FrameHash || p13UFFactoryStateDigest(row) != row.StateHash {
		return nil, errors.New("车厂持久来源/原帧SHA损坏")
	}
	frame := &pb.InstUFCariotFactory{}
	if proto.Unmarshal(row.Frame, frame) != nil || frame.GetGuid() != row.GUID || int64(frame.GetFactoryId()) != row.FactoryID {
		return nil, errors.New("车厂原实例身份损坏")
	}
	return frame, nil
}
func p13UFFactorySave(tx *gorm.DB, row *db.P13UnionFightFactoryState, frame *pb.InstUFCariotFactory) error {
	raw, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	row.Frame = raw
	row.FrameHash = p6Digest(raw)
	row.StateHash = p13UFFactoryStateDigest(row)
	return tx.Save(row).Error
}

// p13UFFactoryBuild 冻结原四个厂实体；只有提前发布的具名初始HP允许产车，没有隐含HP/产车规则。
func (s *GameService) p13UFFactoryBuild(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightFactorySource
	if err := tx.Where("day=? AND enabled=1", match.Day).Find(&sources).Error; err != nil {
		return err
	}
	known := map[int64]db.P13UnionFightFactorySource{}
	for _, source := range sources {
		if original.table("UnionFightFactory", "UFFId", strconv.FormatInt(source.FactoryID, 10)) == nil || source.ChariotHP <= 0 || math.IsNaN(source.ChariotHP) || math.IsInf(source.ChariotHP, 0) || strings.TrimSpace(source.Source) == "" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() {
			return errors.New("实际战车HP来源尚未完整发布")
		}
		known[source.FactoryID] = source
	}
	for _, frame := range buildings.FactoryInfos {
		definition := original.table("UnionFightFactory", "UFFId", strconv.Itoa(int(frame.GetFactoryId())))
		camp, err := strconv.ParseInt(definition["Camp"], 10, 64)
		if err != nil {
			return err
		}
		row := db.P13UnionFightFactoryState{MatchID: match.ID, GUID: frame.GetGuid(), FactoryID: int64(frame.GetFactoryId()), Camp: camp, Source: constUtil.P13UnionFightFactorySource + "；初始HP尚未发布"}
		if source, ok := known[row.FactoryID]; ok {
			row.ChariotHP = source.ChariotHP
			row.Source = constUtil.P13UnionFightFactorySource + "；发布：" + source.Source
		}
		row.SourceHash = p13UFFactoryDigest(&row)
		if row.ChariotHP > 0 && frame.GetIsOpen() == 1 {
			if err = s.p13UFFactoryStart(&row, definition, match.StartAt); err != nil {
				return err
			}
		}
		if err = p13UFFactorySave(tx, &row, frame); err != nil {
			return err
		}
	}
	return s.p13UFFactoryCycleBuild(tx, match, original, now)
}
func (s *GameService) p13UFFactoryStart(row *db.P13UnionFightFactoryState, definition map[string]string, at int64) error {
	craft, err := strconv.ParseFloat(definition["CraftTime"], 64)
	if err != nil || craft <= 0 {
		return errors.New("原造车时间缺失")
	}
	animation, err := strconv.ParseFloat(definition["factoryBuildAnimationTime"], 64)
	if err != nil || animation < 0 || animation > craft {
		return errors.New("原出车动画时间缺失")
	}
	row.StartedAt = at
	row.DueAt = at + int64(math.Round(craft*1000))
	row.AnimationAt = row.DueAt - int64(math.Round(animation*1000))
	return nil
}
func (s *GameService) p13UFFactoryRead(tx *gorm.DB, match *db.P13UnionFightMatch, guid int64) (*db.P13UnionFightFactoryState, *pb.InstUFCariotFactory, map[string]string, error) {
	var row db.P13UnionFightFactoryState
	if err := tx.First(&row, "match_id=? AND guid=?", match.ID, guid).Error; err != nil {
		return nil, nil, nil, errors.New("本场原车厂GUID不存在")
	}
	frame, err := p13UFFactoryFrame(&row)
	if err != nil {
		return nil, nil, nil, err
	}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, nil, nil, err
	}
	definition := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(row.FactoryID, 10))
	if definition == nil || definition["Camp"] != strconv.FormatInt(row.Camp, 10) {
		return nil, nil, nil, errors.New("车厂原表与持久身份不符")
	}
	return &row, frame, definition, nil
}
func (s *GameService) p13UFFactoryCost(ctx context.Context, id int64, request *pb.CSUFOpenFactoryCost, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFOpenFactoryCost
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		row, factory, definition, err := s.p13UFFactoryRead(tx, match, request.GetFactoryId())
		if err != nil {
			return err
		}
		if row.Camp != actor.Camp || definition["IsField"] != "1" || frame.GetUnionAuthority() == 0 || factory.GetIsOpen() != 0 || now.UnixMilli() >= match.EndAt {
			return errors.New("原本阵营未开野外车厂仅副会长/会长可查询")
		}
		cost, err := strconv.ParseInt(definition["NeedResource"], 10, 64)
		if err != nil || cost <= 0 {
			return errors.New("原野外车厂资源消耗缺失")
		}
		reply = &pb.SCUFOpenFactoryCost{Cost: &pb.InstUFResource{ResourceNum: proto.Float64(float64(cost))}}
		return nil
	})
	return []proto.Message{reply}, err
}

// p13UFFactoryOpen 真实本会采矿上交资源支付原400，原身份/NPC/Current握手和冻结HP资格同事务核对。
func (s *GameService) p13UFFactoryOpen(ctx context.Context, id int64, request *pb.CSUFFieldCariotFactoryOpen, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFCariotFactoryOpen
	var update *pb.SCUFCampResourceUpdate
	scope := ""
	var missionGuild string
	missionChanged := false
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
		raw := []byte(scope + "\x00" + strconv.FormatInt(request.GetFactoryGuid(), 10) + "\x00" + request.GetNpcId())
		if stamp > 0 {
			var receipt db.P13GuildReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, constUtil.P13UnionFightFactoryOpenOperation, stamp).Error
			if err == nil {
				if !bytes.Equal(raw, receipt.Request) {
					return errors.New("车厂开启请求时间戳冲突")
				}
				reply = &pb.SCUFCariotFactoryOpen{}
				return proto.Unmarshal(receipt.Response, reply)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		row, factory, definition, err := s.p13UFFactoryRead(tx, match, request.GetFactoryGuid())
		if err != nil {
			return err
		}
		if row.Camp != actor.Camp || definition["IsField"] != "1" || definition["NPCID"] != request.GetNpcId() || frame.GetUnionAuthority() == 0 || factory.GetIsOpen() != 0 || actor.Hooked || frame.GetState() != 0 || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("原开启的当前职位/本阵营未开厂/NPC/空闲开赛资格不符")
		}
		if row.ChariotHP <= 0 {
			return errors.New("本场尚未发布具名战车HP，不能消费资源启动未知实例")
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
			return errors.New("原Current加载握手尚未完成")
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		npc := original.table("UnionFightNpc", "UFNId", request.GetNpcId())
		point, err := fieldPoint(npc["Position"])
		if err != nil {
			return err
		}
		if snapshot.ShopDistance == nil || npc["Building"] != "field_factory" || p13UFResourceDistance(actor, point.GetX(), point.GetY(), point.GetZ()) > *snapshot.ShopDistance {
			return errors.New("角色尚未到达本阵营原车厂NPC")
		}
		cost, err := strconv.ParseInt(definition["NeedResource"], 10, 64)
		if err != nil || cost <= 0 {
			return errors.New("原开启资源缺失")
		}
		var camp db.P13UnionFightCampResource
		if err = tx.First(&camp, "match_id=? AND guild_id=?", scope, actor.GuildID).Error; err != nil || camp.Camp != row.Camp || camp.Amount < cost {
			return errors.New("实际本会采矿上交资源不足")
		}
		camp.Amount -= cost
		if err = tx.Save(&camp).Error; err != nil {
			return err
		}
		factory.IsOpen = proto.Int32(1)
		factory.State = proto.Int32(0)
		row.OpenedAt = now.UnixMilli()
		if err = s.p13UFFactoryStart(row, definition, row.OpenedAt); err != nil {
			return err
		}
		if err = p13UFFactorySave(tx, row, factory); err != nil {
			return err
		}
		missionChanged, err = p13UFGuildMissionFactoryOpened(tx, match, actor, row, now)
		if err != nil {
			return err
		}
		missionGuild = actor.GuildID
		reply = &pb.SCUFCariotFactoryOpen{Factory: factory}
		update = &pb.SCUFCampResourceUpdate{Camp: proto.Int32(int32(actor.Camp)), Resource: &pb.InstUFResource{ResourceNum: proto.Float64(float64(camp.Amount))}}
		fresh = true
		if stamp > 0 {
			encoded, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			return tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: constUtil.P13UnionFightFactoryOpenOperation, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		if missionChanged {
			s.p13UFGuildMissionNotify(ctx, scope, missionGuild)
		}
		s.p13UFFieldBroadcast(ctx, scope, id, reply)
		s.p13UFFieldBroadcast(ctx, scope, id, update)
		return []proto.Message{reply, update}, nil
	}
	return []proto.Message{reply}, nil
}

func (s *GameService) p13UFFactoryOverlay(tx *gorm.DB, scope string, buildings *pb.SCUFBuildingRefresh) error {
	var rows []db.P13UnionFightFactoryState
	if err := tx.Where("match_id=?", scope).Find(&rows).Error; err != nil {
		return err
	}
	byGUID := map[int64]*pb.InstUFCariotFactory{}
	for _, row := range rows {
		frame, err := p13UFFactoryFrame(&row)
		if err != nil {
			return err
		}
		byGUID[row.GUID] = frame
	}
	for i, frame := range buildings.FactoryInfos {
		if live, ok := byGUID[frame.GetGuid()]; ok {
			buildings.FactoryInfos[i] = live
		}
	}
	return nil
}
func (s *GameService) p13UFFactoryCars(tx *gorm.DB, match *db.P13UnionFightMatch, wanted map[int64]bool) ([]proto.Message, error) {
	rows, err := s.p13UFCarScopeRows(tx, match.ID, nil)
	if err != nil {
		return nil, err
	}
	reply := &pb.SCUFCaropt{}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, err = p13UFFactoryFrame(&row); err != nil {
			return nil, err
		}
		if p6Digest(row.CarFrame) != row.CarHash {
			return nil, errors.New("实际已产战车SHA损坏")
		}
		car := &pb.InstUFCariot{}
		if err = proto.Unmarshal(row.CarFrame, car); err != nil {
			return nil, err
		}
		if car.GetFactoryGuid() != row.GUID || int64(car.GetCamp()) != row.Camp || car.GetMaxHp() != row.ChariotHP {
			return nil, errors.New("战车实例与冻结实际来源不符")
		}
		if len(wanted) > 0 && !wanted[car.GetGuid()] {
			continue
		}
		point, err := s.p13UFCarMotionPoint(tx, &row, original)
		if err != nil {
			return nil, err
		}
		reply.Cariot = append(reply.Cariot, car)
		reply.Local = append(reply.Local, point)
	}
	if len(reply.Cariot) == 0 {
		return nil, nil
	}
	messages := []proto.Message{reply}
	// 原CarRefresh总先创建/切Move；实际HP0车需紧随原Crash恢复死亡，不能在cold重新站起。
	for _, car := range reply.Cariot {
		if car.GetHp() == 0 {
			messages = append(messages, &pb.SCUFCariotCrash{Guid: car.Guid})
		} else {
			var actual *db.P13UnionFightFactoryState
			for i := range rows {
				if p13UFCarInstanceGUID(&rows[i]) == car.GetGuid() {
					actual = &rows[i]
					break
				}
			}
			if actual == nil {
				return nil, errors.New("本场原冷车GUID缺失")
			}
			stateFrames, stateErr := p13UFCarAttackFrames(tx, actual)
			if stateErr != nil {
				return nil, stateErr
			}
			messages = append(messages, stateFrames...)
		}
	}
	return messages, nil
}

// TickP13UnionFightFactory 只推进有来源的首车原动画/出车；重复worker和冷启动不重造实体，不猜战车行进或战争胜负。
func (s *GameService) TickP13UnionFightFactory(ctx context.Context, now time.Time) error {
	var pending []db.P13UnionFightFactoryState
	if err := db.GetIns().WithContext(ctx).Where("completed_at=0 AND due_at>0 AND animation_at<=?", now.UnixMilli()).Order("due_at,guid").Find(&pending).Error; err != nil {
		return err
	}
	for _, candidate := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		var frames []proto.Message
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			var enroll db.P13UnionFightPolicy
			if err := tx.First(&enroll, 1).Error; err != nil {
				return err
			}
			var field db.P13UnionFightFieldPolicy
			if err := tx.First(&field, 1).Error; err != nil {
				return err
			}
			if !gate.ManagementEnabled || !enroll.EnrollmentEnabled || !enroll.MatchEnabled || !field.Enabled {
				return nil
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", candidate.MatchID).Error; err != nil {
				return err
			}
			row, factory, definition, err := s.p13UFFactoryRead(tx, &match, candidate.GUID)
			if err != nil {
				return err
			}
			if row.CompletedAt != 0 || row.DueAt == 0 {
				return nil
			}
			if row.StartedAt < match.StartAt || row.StartedAt >= match.EndAt || row.DueAt > match.EndAt {
				if now.UnixMilli() >= match.EndAt {
					row.CompletedAt = constUtil.P13UnionFightFactoryCanceled
					return p13UFFactorySave(tx, row, factory)
				}
				return nil
			}
			guildID := match.LeftGuildID
			if row.Camp == 1 {
				guildID = match.RightGuildID
			}
			var count int64
			if err = tx.Model(&db.P13Guild{}).Where("id=?", guildID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				row.CompletedAt = constUtil.P13UnionFightFactoryCanceled
				return p13UFFactorySave(tx, row, factory)
			}
			if now.UnixMilli() < row.DueAt {
				if factory.GetState() == 0 {
					factory.State = proto.Int32(1)
					frames = append(frames, &pb.SCUFFactoryBuildPlay{Guid: proto.Int64(row.GUID)})
					return p13UFFactorySave(tx, row, factory)
				}
				return nil
			}
			carID, err := strconv.ParseInt(definition["CariotId"], 10, 32)
			if err != nil {
				return err
			}
			car := &pb.InstUFCariot{Guid: proto.Int64(p13UFFieldGUID(match.ID, "cariot", strconv.FormatInt(row.GUID, 10))), CariotId: proto.Int32(int32(carID)), Camp: proto.Int32(int32(row.Camp)), FactoryGuid: proto.Int64(row.GUID), Hp: proto.Float64(row.ChariotHP), MaxHp: proto.Float64(row.ChariotHP), IsSlowDown: proto.Int32(0)}
			raw, err := proto.Marshal(car)
			if err != nil {
				return err
			}
			row.CarFrame = raw
			row.CarHash = p6Digest(raw)
			row.CompletedAt = row.DueAt
			factory.State = proto.Int32(3)
			factory.CariotGuid = car.Guid
			if err = p13UFFactorySave(tx, row, factory); err != nil {
				return err
			}
			point, err := fieldPoint(definition["CariotBornPos"])
			if err != nil {
				return err
			}
			frames = append(frames, &pb.SCUFCariotFactoryBuildComplete{FactoryGuid: proto.Int64(row.GUID), Cariot: car, Pos: point})
			return nil
		})
		if err != nil {
			return err
		}
		for _, frame := range frames {
			s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, frame)
		}
	}
	return s.TickP13UnionFightFactoryCycle(ctx, now)
}
