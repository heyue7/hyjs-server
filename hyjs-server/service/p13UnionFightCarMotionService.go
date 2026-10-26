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

func (s *GameService) InitializeP13UnionFightCarMotion(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightCarMotionSource{}, &db.P13UnionFightCarMotionOffer{}, &db.P13UnionFightCarMotionState{})
}
func p13UFCarMotionOfferHash(row *db.P13UnionFightCarMotionOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFCarMotionStateHash(row *db.P13UnionFightCarMotionState) string {
	copy := *row
	copy.StateHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p13UFCarMotionBuild 同场提前冻结原出生点及完整寻路链；未知原速度只接受具名发布。
func (s *GameService) p13UFCarMotionBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightCarMotionSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Order("factory_id").Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		if source.FactoryID < 1 || source.FactoryID > 4 || source.Speed <= 0 || source.Speed > 100 || math.IsNaN(source.Speed) || math.IsInf(source.Speed, 0) || strings.TrimSpace(source.Source) == "" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() {
			return errors.New("原战车路径速度来源尚未完整提前发布")
		}
		cfg := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(source.FactoryID, 10))
		if cfg == nil {
			return errors.New("原战车路线目录不存在")
		}
		var path []*pb.PointNew
		for _, value := range append([]string{cfg["CariotBornPos"]}, strings.Split(cfg["CariotMoveNavInfo"], ";")...) {
			point, err := fieldPoint(value)
			if err != nil {
				return err
			}
			path = append(path, point)
		}
		if len(path) < 2 {
			return errors.New("原战车完整路径缺失")
		}
		raw, err := json.Marshal(path)
		if err != nil {
			return err
		}
		row := db.P13UnionFightCarMotionOffer{MatchID: match.ID, FactoryID: source.FactoryID, Speed: source.Speed, Path: raw, Source: constUtil.P13UnionFightCarMotionSource + "；发布：" + source.Source}
		// 使用实际地图厂GUID，避免猜测实体GUID派生类别。
		var factory db.P13UnionFightFactoryState
		if err = tx.First(&factory, "match_id=? AND factory_id=?", match.ID, source.FactoryID).Error; err != nil {
			return err
		}
		if factory.ChariotHP <= 0 {
			return errors.New("行进来源需要本场实际车厂冻结初始HP")
		}
		row.FactoryGUID = factory.GUID
		row.SourceHash = p13UFCarMotionOfferHash(&row)
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func p13UFCarMotionPath(row *db.P13UnionFightCarMotionOffer) ([]*pb.PointNew, float64, error) {
	if row.SourceHash != p13UFCarMotionOfferHash(row) || row.Speed <= 0 || row.Speed > 100 || math.IsNaN(row.Speed) || math.IsInf(row.Speed, 0) {
		return nil, 0, errors.New("冻结战车路径来源SHA损坏")
	}
	var points []*pb.PointNew
	if json.Unmarshal(row.Path, &points) != nil || len(points) < 2 {
		return nil, 0, errors.New("冻结战车路径损坏")
	}
	total := float64(0)
	for i, p := range points {
		if p == nil || math.IsNaN(float64(p.GetX())) || math.IsNaN(float64(p.GetY())) || math.IsNaN(float64(p.GetZ())) || math.IsInf(float64(p.GetX()), 0) || math.IsInf(float64(p.GetY()), 0) || math.IsInf(float64(p.GetZ()), 0) {
			return nil, 0, errors.New("冻结战车路径坐标损坏")
		}
		if i > 0 {
			a := points[i-1]
			total += math.Sqrt(math.Pow(float64(p.GetX()-a.GetX()), 2) + math.Pow(float64(p.GetY()-a.GetY()), 2) + math.Pow(float64(p.GetZ()-a.GetZ()), 2))
		}
	}
	if total <= 0 || math.IsInf(total, 0) {
		return nil, 0, errors.New("冻结战车路线长度无效")
	}
	return points, total, nil
}
func p13UFCarMotionAt(points []*pb.PointNew, distance float64) *pb.PointNew {
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		length := math.Sqrt(math.Pow(float64(b.GetX()-a.GetX()), 2) + math.Pow(float64(b.GetY()-a.GetY()), 2) + math.Pow(float64(b.GetZ()-a.GetZ()), 2))
		if length > 0 && distance < length {
			r := float32(distance / length)
			return &pb.PointNew{X: proto.Float32(a.GetX() + (b.GetX()-a.GetX())*r), Y: proto.Float32(a.GetY() + (b.GetY()-a.GetY())*r), Z: proto.Float32(a.GetZ() + (b.GetZ()-a.GetZ())*r)}
		}
		distance -= length
	}
	return proto.Clone(points[len(points)-1]).(*pb.PointNew)
}

// p13UFCarMotionPoint 冷恢复与炸弹使用同一已持久化位置；没有发布速度的原首车保留实际出生点。
func (s *GameService) p13UFCarMotionPoint(tx *gorm.DB, row *db.P13UnionFightFactoryState, original *GameService) (*pb.PointNew, error) {
	var offer db.P13UnionFightCarMotionOffer
	err := tx.First(&offer, "match_id=? AND factory_guid=?", row.MatchID, row.GUID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fieldPoint(original.table("UnionFightFactory", "UFFId", strconv.FormatInt(row.FactoryID, 10))["CariotBornPos"])
	}
	if err != nil {
		return nil, err
	}
	points, total, err := p13UFCarMotionPath(&offer)
	if err != nil {
		return nil, err
	}
	total, err = p13UFCarMotionLimit(tx, &offer, total)
	if err != nil {
		return nil, err
	}
	var state db.P13UnionFightCarMotionState
	err = tx.First(&state, "match_id=? AND factory_guid=?", row.MatchID, p13UFCarStateGUID(row)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return proto.Clone(points[0]).(*pb.PointNew), nil
	}
	if err != nil {
		return nil, err
	}
	car, err := p13UFCarItemRead(row)
	if err != nil {
		return nil, err
	}
	if state.StateHash != p13UFCarMotionStateHash(&state) || state.OfferHash != offer.SourceHash || state.CarGUID != car.GetGuid() || state.LastAt < row.CompletedAt || state.Distance < 0 || state.Distance > total || math.IsNaN(state.Distance) || state.Arrived != (state.Distance == total) {
		return nil, errors.New("实际战车行进SHA/实例来源损坏")
	}
	return p13UFCarMotionAt(points, state.Distance), nil
}

// TickP13UnionFightCarMotion 推进实际timer完成的存活首车；原路线终点只停留，不制造攻门或胜负事实。
func (s *GameService) TickP13UnionFightCarMotion(ctx context.Context, now time.Time) error {
	rows, err := s.p13UFCarScopeRows(db.GetIns().WithContext(ctx), "", &now)
	if err != nil {
		return err
	}
	scopes := make([]string, 0, len(rows))
	for _, row := range rows {
		scopes = append(scopes, row.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	var failures []error
	for _, candidate := range rows {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		if clocks.paused(candidate.MatchID) {
			continue
		}
		var movement *pb.SCUFElementMove
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, candidate.MatchID); err != nil {
				return err
			} else if closed {
				return nil
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
			if now.UnixMilli() < match.StartAt || now.UnixMilli() >= clocks.end(&match) {
				return nil
			}
			var count int64
			if err := tx.Model(&db.P13Guild{}).Where("id IN ?", []string{match.LeftGuildID, match.RightGuildID}).Count(&count).Error; err != nil {
				return err
			}
			if count != 2 {
				return nil
			}
			row, err := s.p13UFCarCandidate(tx, &match, &candidate)
			if err != nil {
				return err
			}
			car, err := p13UFCarItemRead(row)
			if err != nil {
				return err
			}
			if car.GetHp() <= 0 {
				return nil
			}
			var offer db.P13UnionFightCarMotionOffer
			err = tx.First(&offer, "match_id=? AND factory_guid=?", row.MatchID, row.GUID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			points, total, err := p13UFCarMotionPath(&offer)
			if err != nil {
				return err
			}
			total, err = p13UFCarMotionLimit(tx, &offer, total)
			if err != nil {
				return err
			}
			var state db.P13UnionFightCarMotionState
			err = tx.First(&state, "match_id=? AND factory_guid=?", row.MatchID, p13UFCarStateGUID(row)).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				state = db.P13UnionFightCarMotionState{MatchID: row.MatchID, FactoryGUID: p13UFCarStateGUID(row), CarGUID: car.GetGuid(), LastAt: row.CompletedAt, OfferHash: offer.SourceHash}
				state.StateHash = p13UFCarMotionStateHash(&state)
			} else if err != nil {
				return err
			}
			if state.StateHash != p13UFCarMotionStateHash(&state) || state.OfferHash != offer.SourceHash || state.CarGUID != car.GetGuid() || state.LastAt < row.CompletedAt || state.Distance < 0 || state.Distance > total || math.IsNaN(state.Distance) || state.Arrived != (state.Distance == total) {
				return errors.New("实际战车行进SHA/冻结身份损坏")
			}
			if state.Arrived || now.UnixMilli() <= state.LastAt {
				return nil
			}
			state.Distance = math.Min(total, state.Distance+float64(now.UnixMilli()-state.LastAt)*offer.Speed/1000)
			state.LastAt = now.UnixMilli()
			state.Arrived = state.Distance == total
			state.StateHash = p13UFCarMotionStateHash(&state)
			if err = tx.Save(&state).Error; err != nil {
				return err
			}
			movement = &pb.SCUFElementMove{Guid: []int64{car.GetGuid()}, Pos: []*pb.PointNew{p13UFCarMotionAt(points, state.Distance)}, Speed: []float32{float32(offer.Speed)}}
			return nil
		})
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if movement != nil {
			if err := p13UFPublishBeforeAbort(ctx, candidate.MatchID, func() {
				s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, movement)
			}); err != nil {
				failures = append(failures, err)
				continue
			}
		}
	}
	return errors.Join(failures...)
}
