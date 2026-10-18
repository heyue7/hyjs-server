package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFCarGenerationHash(row *db.P13UnionFightCarGeneration) string {
	raw, _ := json.Marshal(struct {
		MatchID                                  string
		CarGUID, FactoryGUID, Generation, BornAt int64
		FactorySnapshot                          []byte
		OfferHash                                string
	}{row.MatchID, row.CarGUID, row.FactoryGUID, row.Generation, row.BornAt, row.FactorySnapshot, row.OfferHash})
	return p6Digest(raw)
}

func p13UFCarGenerationStateHash(row *db.P13UnionFightCarGeneration) string {
	raw, _ := json.Marshal(struct{ SourceHash, CarHash string }{row.SourceHash, row.CarHash})
	return p6Digest(raw)
}

func p13UFCarInstanceGUID(row *db.P13UnionFightFactoryState) int64 {
	key := strconv.FormatInt(row.GUID, 10)
	if row.CarGeneration > 1 {
		key += "/" + strconv.FormatInt(row.CarGeneration, 10)
	}
	return p13UFFieldGUID(row.MatchID, "cariot", key)
}

// 首车继续兼容已发布厂GUID主键；新代独立CarGUID作为内部mutable状态键。
func p13UFCarStateGUID(row *db.P13UnionFightFactoryState) int64 {
	if row.CarGeneration > 1 {
		return p13UFCarInstanceGUID(row)
	}
	return row.GUID
}

func (s *GameService) p13UFCarGenerationRead(tx *gorm.DB, match *db.P13UnionFightMatch, carGUID int64) (*db.P13UnionFightFactoryState, error) {
	var row db.P13UnionFightCarGeneration
	if err := tx.First(&row, "match_id=? AND car_guid=?", match.ID, carGUID).Error; err != nil {
		return nil, err
	}
	if row.Generation < 2 || row.Generation > 128 || row.CarGUID != carGUID || row.BornAt < match.StartAt || row.BornAt >= match.EndAt || row.SourceHash != p13UFCarGenerationHash(&row) || row.CarHash != p6Digest(row.CarFrame) || row.StateHash != p13UFCarGenerationStateHash(&row) {
		return nil, errors.New("后续独立战车来源/时刻/帧SHA损坏")
	}
	var frozen db.P13UnionFightFactoryState
	if err := json.Unmarshal(row.FactorySnapshot, &frozen); err != nil {
		return nil, err
	}
	if _, err := p13UFFactoryFrame(&frozen); err != nil {
		return nil, err
	}
	actual, _, _, err := s.p13UFFactoryRead(tx, match, row.FactoryGUID)
	if err != nil {
		return nil, err
	}
	if frozen.MatchID != match.ID || frozen.GUID != row.FactoryGUID || frozen.CompletedAt != row.BornAt || frozen.SourceHash != actual.SourceHash || frozen.ChariotHP != actual.ChariotHP || frozen.Camp != actual.Camp || frozen.FactoryID != actual.FactoryID {
		return nil, errors.New("后续战车冻结厂身份与实际来源不符")
	}
	var offer db.P13UnionFightFactoryCycleOffer
	if err = tx.First(&offer, "match_id=? AND factory_guid=?", match.ID, row.FactoryGUID).Error; err != nil {
		return nil, err
	}
	if offer.SourceHash != p13UFFactoryCycleOfferHash(&offer) || row.OfferHash != offer.SourceHash || row.Generation > offer.MaxGenerations {
		return nil, errors.New("后续战车周期发布SHA/产能上限不符")
	}
	frozen.CarGeneration = row.Generation
	frozen.CarFrame = append([]byte(nil), row.CarFrame...)
	frozen.CarHash = row.CarHash
	frozen.StateHash = p13UFFactoryStateDigest(&frozen)
	if row.CarGUID != p13UFCarInstanceGUID(&frozen) {
		return nil, errors.New("后续战车GUID不符")
	}
	return &frozen, nil
}

func (s *GameService) p13UFCarCandidate(tx *gorm.DB, match *db.P13UnionFightMatch, candidate *db.P13UnionFightFactoryState) (*db.P13UnionFightFactoryState, error) {
	if candidate.CarGeneration > 1 {
		return s.p13UFCarGenerationRead(tx, match, p13UFCarInstanceGUID(candidate))
	}
	row, _, _, err := s.p13UFFactoryRead(tx, match, candidate.GUID)
	return row, err
}

func p13UFCarSave(tx *gorm.DB, row *db.P13UnionFightFactoryState) error {
	if row.CarGeneration <= 1 {
		frame := &pb.InstUFCariotFactory{}
		if row.FrameHash != p6Digest(row.Frame) || row.SourceHash != p13UFFactoryDigest(row) || proto.Unmarshal(row.Frame, frame) != nil {
			return errors.New("首车写回厂帧SHA损坏")
		}
		return p13UFFactorySave(tx, row, frame)
	}
	var actual db.P13UnionFightCarGeneration
	if err := tx.First(&actual, "match_id=? AND car_guid=?", row.MatchID, p13UFCarInstanceGUID(row)).Error; err != nil {
		return err
	}
	if actual.SourceHash != p13UFCarGenerationHash(&actual) || actual.StateHash != p13UFCarGenerationStateHash(&actual) {
		return errors.New("后续战车写回SHA损坏")
	}
	actual.CarFrame = append([]byte(nil), row.CarFrame...)
	actual.CarHash = p6Digest(actual.CarFrame)
	actual.StateHash = p13UFCarGenerationStateHash(&actual)
	return tx.Save(&actual).Error
}

// p13UFCarScopeRows 列出首车与后续真实已出车实体；mutable状态仍按独立CarGUID隔离。
func (s *GameService) p13UFCarScopeRows(tx *gorm.DB, scope string, now *time.Time) ([]db.P13UnionFightFactoryState, error) {
	var rows []db.P13UnionFightFactoryState
	query := tx.Where("completed_at>0")
	if scope != "" {
		query = query.Where("match_id=?", scope)
	}
	if now != nil {
		query = query.Where("match_id IN (?)", tx.Model(&db.P13UnionFightMatch{}).Select("id").Where("start_at<=? AND end_at>?", now.UnixMilli(), now.UnixMilli()))
	}
	if err := query.Order("match_id,guid").Find(&rows).Error; err != nil {
		return nil, err
	}
	var records []db.P13UnionFightCarGeneration
	query = tx.Model(&db.P13UnionFightCarGeneration{})
	if scope != "" {
		query = query.Where("match_id=?", scope)
	}
	if now != nil {
		query = query.Where("match_id IN (?)", tx.Model(&db.P13UnionFightMatch{}).Select("id").Where("start_at<=? AND end_at>?", now.UnixMilli(), now.UnixMilli()))
	}
	if err := query.Order("match_id,factory_guid,generation").Find(&records).Error; err != nil {
		return nil, err
	}
	matches := map[string]*db.P13UnionFightMatch{}
	for _, record := range records {
		match := matches[record.MatchID]
		if match == nil {
			match = &db.P13UnionFightMatch{}
			if err := tx.First(match, "id=?", record.MatchID).Error; err != nil {
				return nil, err
			}
			matches[record.MatchID] = match
		}
		row, err := s.p13UFCarGenerationRead(tx, match, record.CarGUID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, *row)
	}
	return rows, nil
}
