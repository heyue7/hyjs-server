package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP13UnionFightCarAttack(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarAttackOffer{}, &db.P13UnionFightCarAttackState{})
}
func p13UFCarAttackOfferHash(row *db.P13UnionFightCarAttackOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFCarAttackStateHash(row *db.P13UnionFightCarAttackState) string {
	copy := *row
	copy.StateHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p13UFCarAttackIntersection 将原完整路线上首次进入提前发布范围的实际距离冻结，不假设路径终点就是城门。
func p13UFCarAttackIntersection(points []*pb.PointNew, gate *pb.PointNew, radius float64) (float64, error) {
	distance := float64(0)
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		x, y, z := float64(a.GetX()-gate.GetX()), float64(a.GetY()-gate.GetY()), float64(a.GetZ()-gate.GetZ())
		dx, dy, dz := float64(b.GetX()-a.GetX()), float64(b.GetY()-a.GetY()), float64(b.GetZ()-a.GetZ())
		length2 := dx*dx + dy*dy + dz*dz
		c := x*x + y*y + z*z - radius*radius
		if c <= 0 {
			return distance, nil
		}
		if length2 > 0 {
			linear := 2 * (x*dx + y*dy + z*dz)
			disc := linear*linear - 4*length2*c
			if disc >= 0 {
				at := (-linear - math.Sqrt(disc)) / (2 * length2)
				if at >= 0 && at <= 1 {
					return distance + at*math.Sqrt(length2), nil
				}
			}
		}
		distance += math.Sqrt(length2)
	}
	return 0, errors.New("原战车路线未进入明确发布的本场目标门攻击范围")
}

// p13UFCarAttackBuild 冻结原指定敌门、原攻击间隔/释放点和具名本地伤害及范围。
func (s *GameService) p13UFCarAttackBuild(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightCarAttackSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Order("factory_id").Find(&sources).Error; err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	if err := p13UFGateFreeze(tx, match, buildings); err != nil {
		return err
	}
	for _, source := range sources {
		if source.FactoryID < 1 || source.FactoryID > 4 || source.HPPercent < 1 || source.HPPercent > 100 || source.Radius <= 0 || source.Radius > 10 || math.IsNaN(source.Radius) || math.IsInf(source.Radius, 0) || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("战车原未知攻门范围/比例必须提前具名发布")
		}
		var factory db.P13UnionFightFactoryState
		if err := tx.First(&factory, "match_id=? AND factory_id=?", match.ID, source.FactoryID).Error; err != nil {
			return err
		}
		var motion db.P13UnionFightCarMotionOffer
		if err := tx.First(&motion, "match_id=? AND factory_guid=?", match.ID, factory.GUID).Error; err != nil {
			return errors.New("攻门依赖本场提前发布的实际路径速度")
		}
		points, _, err := p13UFCarMotionPath(&motion)
		if err != nil {
			return err
		}
		cfg := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(factory.FactoryID, 10))
		gateID, err := strconv.ParseInt(cfg["CariotTargetGateId"], 10, 64)
		if err != nil {
			return err
		}
		gateCfg := original.table("UnionFightGate", "UFGId", strconv.FormatInt(gateID, 10))
		if gateCfg == nil || gateCfg["Camp"] == strconv.FormatInt(factory.Camp, 10) {
			return errors.New("原战车指定敌门目录/阵营损坏")
		}
		gatePoint, err := fieldPoint(gateCfg["Position"])
		if err != nil {
			return err
		}
		carCfg := original.table("UnionFightCariot", "UFCId", cfg["CariotId"])
		if carCfg == nil || carCfg["AttackCountDown"] != "3.5" || carCfg["AttackRelease"] != "0.6" || carCfg["AttackDuration"] != "1.0" {
			return errors.New("原战车3.5秒间隔/0.6秒释放/1秒动画缺失")
		}
		stop, err := p13UFCarAttackIntersection(points, gatePoint, source.Radius)
		if err != nil {
			return err
		}
		row := db.P13UnionFightCarAttackOffer{MatchID: match.ID, FactoryGUID: factory.GUID, GateGUID: p13UFFieldGUID(match.ID, "gate", strconv.FormatInt(gateID, 10)), GateID: gateID, HPPercent: source.HPPercent, Radius: source.Radius, StopDistance: stop, CooldownMillis: 3500, ReleaseMillis: 600, MotionHash: motion.SourceHash, Source: constUtil.P13UnionFightCarAttackSource + "；发布：" + source.Source}
		row.SourceHash = p13UFCarAttackOfferHash(&row)
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func p13UFCarAttackOffer(tx *gorm.DB, motion *db.P13UnionFightCarMotionOffer, total float64) (*db.P13UnionFightCarAttackOffer, error) {
	var row db.P13UnionFightCarAttackOffer
	err := tx.First(&row, "match_id=? AND factory_guid=?", motion.MatchID, motion.FactoryGUID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.SourceHash != p13UFCarAttackOfferHash(&row) || row.MotionHash != motion.SourceHash || row.StopDistance < 0 || row.StopDistance > total || math.IsNaN(row.StopDistance) || math.IsInf(row.StopDistance, 0) || row.GateGUID != p13UFFieldGUID(row.MatchID, "gate", strconv.FormatInt(row.GateID, 10)) || row.HPPercent < 1 || row.HPPercent > 100 || row.CooldownMillis != 3500 || row.ReleaseMillis != 600 || row.Radius <= 0 || row.Radius > 10 {
		return nil, errors.New("实际战车攻门来源SHA/原目标/时序损坏")
	}
	return &row, nil
}
func p13UFCarMotionLimit(tx *gorm.DB, offer *db.P13UnionFightCarMotionOffer, total float64) (float64, error) {
	row, err := p13UFCarAttackOffer(tx, offer, total)
	if err != nil {
		return 0, err
	}
	if row != nil {
		return row.StopDistance, nil
	}
	return total, nil
}

// p13UFCarAttackFrames 原CarRefresh先切移动，因此冷恢复需要紧随已保存的原攻击/完成状态。
func p13UFCarAttackFrames(tx *gorm.DB, factory *db.P13UnionFightFactoryState) ([]proto.Message, error) {
	var state db.P13UnionFightCarAttackState
	err := tx.First(&state, "match_id=? AND factory_guid=?", factory.MatchID, p13UFCarStateGUID(factory)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	car, err := p13UFCarItemRead(factory)
	if err != nil {
		return nil, err
	}
	var motion db.P13UnionFightCarMotionOffer
	if err = tx.First(&motion, "match_id=? AND factory_guid=?", factory.MatchID, factory.GUID).Error; err != nil {
		return nil, err
	}
	_, total, err := p13UFCarMotionPath(&motion)
	if err != nil {
		return nil, err
	}
	offer, err := p13UFCarAttackOffer(tx, &motion, total)
	if err != nil {
		return nil, err
	}
	if offer == nil || state.StateHash != p13UFCarAttackStateHash(&state) || state.OfferHash != offer.SourceHash || state.CarGUID != car.GetGuid() || state.Hits < 0 || state.NextAttackAt < factory.CompletedAt || state.PendingReleaseAt < 0 {
		return nil, errors.New("实际战车攻门计时SHA/身份损坏")
	}
	if car.GetHp() <= 0 {
		return nil, nil
	}
	if state.Completed {
		return []proto.Message{&pb.SCUFCariotComplete{Guid: car.Guid}}, nil
	}
	return []proto.Message{&pb.SCUFCariotAttackGate{Guid: car.Guid, GateGuid: proto.Int64(offer.GateGUID)}}, nil
}

// TickP13UnionFightCarAttack 到达当前真实范围后先播攻击，实际释放时才原子扣门HP；冷启动沿同一待释放时钟。
func (s *GameService) TickP13UnionFightCarAttack(ctx context.Context, now time.Time) error {
	rows, err := s.p13UFCarScopeRows(db.GetIns().WithContext(ctx), "", &now)
	if err != nil {
		return err
	}
	for _, candidate := range rows {
		var frames []proto.Message
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			var policy db.P13UnionFightPolicy
			if err := tx.First(&policy, 1).Error; err != nil {
				return err
			}
			var field db.P13UnionFightFieldPolicy
			if err := tx.First(&field, 1).Error; err != nil {
				return err
			}
			if !gate.ManagementEnabled || !policy.EnrollmentEnabled || !policy.MatchEnabled || !field.Enabled {
				return nil
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", candidate.MatchID).Error; err != nil {
				return err
			}
			if now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
				return nil
			}
			var count int64
			if err := tx.Model(&db.P13Guild{}).Where("id IN ?", []string{match.LeftGuildID, match.RightGuildID}).Count(&count).Error; err != nil {
				return err
			}
			if count != 2 {
				return nil
			}
			factory, err := s.p13UFCarCandidate(tx, &match, &candidate)
			if err != nil {
				return err
			}
			car, err := p13UFCarItemRead(factory)
			if err != nil {
				return err
			}
			if car.GetHp() <= 0 {
				return nil
			}
			var motion db.P13UnionFightCarMotionOffer
			err = tx.First(&motion, "match_id=? AND factory_guid=?", factory.MatchID, factory.GUID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			_, total, err := p13UFCarMotionPath(&motion)
			if err != nil {
				return err
			}
			offer, err := p13UFCarAttackOffer(tx, &motion, total)
			if err != nil || offer == nil {
				return err
			}
			var moving db.P13UnionFightCarMotionState
			err = tx.First(&moving, "match_id=? AND factory_guid=?", factory.MatchID, p13UFCarStateGUID(factory)).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if moving.StateHash != p13UFCarMotionStateHash(&moving) || moving.OfferHash != motion.SourceHash || moving.CarGUID != car.GetGuid() || moving.Distance < 0 || moving.Distance > offer.StopDistance || moving.Arrived != (moving.Distance == offer.StopDistance) {
				return errors.New("实际战车攻门位置状态损坏")
			}
			if !moving.Arrived {
				return nil
			}
			var target db.P13UnionFightGateState
			if err = tx.First(&target, "match_id=? AND guid=?", factory.MatchID, offer.GateGUID).Error; err != nil {
				return err
			}
			door, err := p13UFGateRead(&target)
			if err != nil {
				return err
			}
			if target.GateID != offer.GateID || target.Camp == factory.Camp {
				return errors.New("原战车攻击当前敌门身份损坏")
			}
			var state db.P13UnionFightCarAttackState
			err = tx.First(&state, "match_id=? AND factory_guid=?", factory.MatchID, p13UFCarStateGUID(factory)).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				state = db.P13UnionFightCarAttackState{MatchID: factory.MatchID, FactoryGUID: p13UFCarStateGUID(factory), CarGUID: car.GetGuid(), NextAttackAt: now.UnixMilli(), OfferHash: offer.SourceHash}
				state.StateHash = p13UFCarAttackStateHash(&state)
			} else if err != nil {
				return err
			}
			if state.StateHash != p13UFCarAttackStateHash(&state) || state.OfferHash != offer.SourceHash || state.CarGUID != car.GetGuid() || state.NextAttackAt < factory.CompletedAt || state.PendingReleaseAt < 0 || state.Hits < 0 {
				return errors.New("原战车实际攻击计时SHA损坏")
			}
			if state.Completed {
				return nil
			}
			if door.GetHp() == 0 {
				state.Completed = true
				state.PendingReleaseAt = 0
				frames = append(frames, &pb.SCUFCariotComplete{Guid: car.Guid})
			} else if state.PendingReleaseAt > 0 {
				if now.UnixMilli() < state.PendingReleaseAt {
					return nil
				}
				old := door.GetHp()
				door.Hp = proto.Float64(math.Max(0, old-door.GetMaxHp()*float64(offer.HPPercent)/100))
				if err = p13UFGateSave(tx, &target, door); err != nil {
					return err
				}
				state.PendingReleaseAt = 0
				state.Hits++
				frames = append(frames, &pb.SCUFElementTickExport{TargetGuid: []int64{target.GUID}, SourceGuid: []int64{car.GetGuid()}, HpDelta: []float64{door.GetHp() - old}}, &pb.SCUFGate{Gate: []*pb.InstUFGate{door}})
				if door.GetHp() == 0 {
					state.Completed = true
					frames = append(frames, &pb.SCUFGateCrash{GateId: proto.Int64(target.GateID)}, &pb.SCUFCariotComplete{Guid: car.Guid})
				}
			} else if now.UnixMilli() >= state.NextAttackAt {
				state.PendingReleaseAt = now.UnixMilli() + offer.ReleaseMillis
				state.NextAttackAt = now.UnixMilli() + offer.CooldownMillis
				frames = append(frames, &pb.SCUFCariotAttackGate{Guid: car.Guid, GateGuid: proto.Int64(target.GUID)})
			} else {
				return nil
			}
			state.StateHash = p13UFCarAttackStateHash(&state)
			return tx.Save(&state).Error
		})
		if err != nil {
			return err
		}
		for _, frame := range frames {
			s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, frame)
		}
	}
	return nil
}
