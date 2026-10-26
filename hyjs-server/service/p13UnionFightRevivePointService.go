package service

import (
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

func (s *GameService) InitializeP13UnionFightRevivePoint(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightRevivePointSource{}, &db.P13UnionFightRevivePointOffer{}, &db.P13UnionFightRevivePointState{})
}
func p13UFRevivePointOfferHash(row *db.P13UnionFightRevivePointOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFRevivePointStateHash(row *db.P13UnionFightRevivePointState) string {
	copy := *row
	copy.StateHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFRevivePointQuoteValid(row *db.P13UnionFightRevivePointOffer) bool {
	return row.SourceHash == p13UFRevivePointOfferHash(row) && row.PointID == 5 && !math.IsNaN(float64(row.X)) && !math.IsInf(float64(row.X), 0) && !math.IsNaN(float64(row.Z)) && !math.IsInf(float64(row.Z), 0) && row.PointGUID == p13UFFieldGUID(row.MatchID, "revive", "5") && row.Radius == 3 && p13UFGuildFiniteRange(row.Speed, .01, 100) && row.MaxStepMillis > 0 && row.MaxStepMillis <= 1000 && row.MaxGapMillis >= 2000 && row.MaxGapMillis <= 60000 && strings.TrimSpace(row.Source) != ""
}
func (s *GameService) p13UFRevivePointBuild(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightRevivePointSource
	if err := tx.Find(&sources, "day=? AND enabled=1", match.Day).Error; err != nil {
		return err
	}
	for _, source := range sources {
		cfg := original.table("UnionFightRevivePoint", "UFRPId", strconv.FormatInt(source.PointID, 10))
		speed, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightRevivePointOccupySpeed")["NameValue"], 64)
		if cfg == nil || source.PointID != 5 || cfg["InitCamp"] != "2" || cfg["IsCanOccupy"] != "1" || cfg["OccupyRange"] != "3.0" || err != nil || !p13UFGuildFiniteRange(speed, .01, 100) || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || source.MaxStepMillis <= 0 || source.MaxStepMillis > 1000 || source.MaxGapMillis < 2000 || source.MaxGapMillis > 60000 || strings.TrimSpace(source.Source) == "" {
			return errors.New("原中立可占领点/水平半径/速度及提前具名人数规则缺失")
		}
		point, err := fieldPoint(cfg["Position"])
		if err != nil {
			return err
		}
		offer := db.P13UnionFightRevivePointOffer{MatchID: match.ID, PointGUID: p13UFFieldGUID(match.ID, "revive", "5"), PointID: 5, X: point.GetX(), Z: point.GetZ(), Radius: 3, Speed: speed, MaxStepMillis: source.MaxStepMillis, MaxGapMillis: source.MaxGapMillis, Source: constUtil.P13UnionFightRevivePointSource + "；发布：" + source.Source}
		offer.SourceHash = p13UFRevivePointOfferHash(&offer)
		var frame *pb.InstUFRevivePoint
		for _, unit := range buildings.GetPointInfos() {
			if unit.GetGuid() == offer.PointGUID {
				frame = unit
			}
		}
		if frame == nil || frame.GetCamp() != 2 || frame.GetId() != 5 {
			return errors.New("原中立复活点冻结帧损坏")
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(frame)
		if err != nil {
			return err
		}
		state := db.P13UnionFightRevivePointState{MatchID: match.ID, PointGUID: offer.PointGUID, Point: raw, Participants: []byte("{}"), LastAt: match.StartAt, OfferHash: offer.SourceHash}
		state.StateHash = p13UFRevivePointStateHash(&state)
		if err = tx.Create(&offer).Error; err != nil {
			return err
		}
		if err = tx.Create(&state).Error; err != nil {
			return err
		}
	}
	return nil
}

type p13UFRevivePointParticipant struct {
	Camp      int64
	JoinedAt  int64
	UpdatedAt int64
	X         float32
	Z         float32
}

func p13UFRevivePointRead(state *db.P13UnionFightRevivePointState, quote *db.P13UnionFightRevivePointOffer) (*pb.InstUFRevivePoint, map[int64]p13UFRevivePointParticipant, error) {
	frame := &pb.InstUFRevivePoint{}
	participants := map[int64]p13UFRevivePointParticipant{}
	if !p13UFRevivePointQuoteValid(quote) || state.StateHash != p13UFRevivePointStateHash(state) || state.OfferHash != quote.SourceHash || state.MatchID != quote.MatchID || state.PointGUID != quote.PointGUID || proto.Unmarshal(state.Point, frame) != nil || frame.GetGuid() != quote.PointGUID || int64(frame.GetId()) != quote.PointID || frame.GetCamp() < 0 || frame.GetCamp() > 2 || json.Unmarshal(state.Participants, &participants) != nil {
		return nil, nil, errors.New("中立复活点原帧/人数/来源SHA损坏")
	}
	for _, value := range []float64{frame.GetLeftOccupyProgress(), frame.GetRightOccupyProgress()} {
		if value < 0 || value > 1 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, nil, errors.New("原占领进度越界")
		}
	}
	for id, row := range participants {
		if id <= 0 || row.Camp < 0 || row.Camp > 1 || row.JoinedAt <= 0 || math.IsNaN(float64(row.X)) || math.IsNaN(float64(row.Z)) || math.IsInf(float64(row.X), 0) || math.IsInf(float64(row.Z), 0) {
			return nil, nil, errors.New("占领参与者冻结身份损坏")
		}
	}
	return frame, participants, nil
}
func (s *GameService) p13UFRevivePointParticipants(tx *gorm.DB, match *db.P13UnionFightMatch, quote *db.P13UnionFightRevivePointOffer) (map[int64]p13UFRevivePointParticipant, error) {
	actors, frames, err := s.p13UFFieldPeers(tx, match.ID, 0)
	if err != nil {
		return nil, err
	}
	result := map[int64]p13UFRevivePointParticipant{}
	for i, actor := range actors {
		frame := frames[i]
		if actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || actor.Camp < 0 || actor.Camp > 1 || math.Hypot(float64(actor.X-quote.X), float64(actor.Z-quote.Z)) > quote.Radius {
			continue
		}
		alive := false
		for _, hp := range frame.GetFightInfo().GetHpPos() {
			if math.IsNaN(hp) || math.IsInf(hp, 0) || hp < 0 {
				return nil, errors.New("真实占领角色生命损坏")
			}
			if hp > 0 {
				alive = true
			}
		}
		if !alive {
			continue
		}
		var profile db.Profile
		if err = tx.First(&profile, actor.PlayerID).Error; err != nil {
			return nil, err
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			return nil, err
		}
		if game.PositionLoading {
			continue
		}
		_, camp, err := p13UFFieldFighter(tx, match, actor.PlayerID, actor.GuildID)
		if err != nil || camp != actor.Camp {
			continue
		}
		result[actor.PlayerID] = p13UFRevivePointParticipant{Camp: actor.Camp, JoinedAt: actor.JoinedAt, UpdatedAt: actor.UpdatedAt, X: actor.X, Z: actor.Z}
	}
	return result, nil
}

// 每个时间边界只结算连续观察的真实在场成员。第一次观察、退出重入和离线长跨度不会追补占领。
func (s *GameService) TickP13UnionFightRevivePoint(ctx context.Context, now time.Time) error {
	var quotes []db.P13UnionFightRevivePointOffer
	if err := db.GetIns().WithContext(ctx).Find(&quotes).Error; err != nil {
		return err
	}
	scopes := make([]string, 0, len(quotes))
	for _, candidate := range quotes {
		scopes = append(scopes, candidate.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	for _, quote := range quotes {
		if clocks.paused(quote.MatchID) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var update *pb.SCUFRevivePointUpdate
		var occupied *pb.SCUFRevivePointOccupy
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var policy db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, quote.MatchID); err != nil {
				return err
			} else if closed {
				return nil
			}
			var enrollment db.P13UnionFightPolicy
			if err := tx.First(&enrollment, 1).Error; err != nil {
				return err
			}
			var fieldPolicy db.P13UnionFightFieldPolicy
			if err := tx.First(&fieldPolicy, 1).Error; err != nil {
				return err
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", quote.MatchID).Error; err != nil {
				return err
			}
			if now.UnixMilli() < match.StartAt || now.UnixMilli() >= clocks.end(&match) {
				return nil
			}
			var state db.P13UnionFightRevivePointState
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "match_id=? AND point_guid=?", quote.MatchID, quote.PointGUID).Error; err != nil {
				return err
			}
			frame, previous, err := p13UFRevivePointRead(&state, &quote)
			if err != nil {
				return err
			}
			if now.UnixMilli() <= state.LastAt {
				return nil
			}
			if !policy.ManagementEnabled || !enrollment.EnrollmentEnabled || !enrollment.MatchEnabled || !fieldPolicy.Enabled {
				state.Participants = []byte("{}")
				state.LastAt = now.UnixMilli()
				state.StateHash = p13UFRevivePointStateHash(&state)
				return tx.Save(&state).Error
			}
			participants, err := s.p13UFRevivePointParticipants(tx, &match, &quote)
			if err != nil {
				return err
			}
			counts := [2]int32{}
			continuous := [2]int32{}
			for id, row := range participants {
				counts[row.Camp]++
				prior, ok := previous[id]
				if ok && prior.JoinedAt == row.JoinedAt && prior.Camp == row.Camp && prior.X == row.X && prior.Z == row.Z && row.UpdatedAt <= state.LastAt {
					continuous[row.Camp]++
				}
			}
			step := min(now.UnixMilli()-state.LastAt, quote.MaxStepMillis)
			difference := continuous[0] - continuous[1]
			if now.UnixMilli()-state.LastAt > quote.MaxGapMillis {
				difference = 0
			}
			left, right := frame.GetLeftOccupyProgress(), frame.GetRightOccupyProgress()
			oldCamp := frame.GetCamp()
			change := quote.Speed * float64(step) / 1000 * math.Abs(float64(difference))
			if difference > 0 {
				decrease := min(right, change)
				right -= decrease
				left = min(1, left+change-decrease)
				if left >= 1 {
					frame.Camp = proto.Int32(0)
				}
			} else if difference < 0 {
				decrease := min(left, change)
				left -= decrease
				right = min(1, right+change-decrease)
				if right >= 1 {
					frame.Camp = proto.Int32(1)
				}
			}
			frame.LeftOccupyProgress = proto.Float64(left)
			frame.RightOccupyProgress = proto.Float64(right)
			state.Point, err = proto.MarshalOptions{Deterministic: true}.Marshal(frame)
			if err != nil {
				return err
			}
			state.Participants, err = json.Marshal(participants)
			if err != nil {
				return err
			}
			state.LastAt = now.UnixMilli()
			state.StateHash = p13UFRevivePointStateHash(&state)
			if err = tx.Save(&state).Error; err != nil {
				return err
			}
			update = &pb.SCUFRevivePointUpdate{Point: frame, RedPlayerCount: proto.Int32(counts[0]), BluePlayercount: proto.Int32(counts[1])}
			if frame.GetCamp() != oldCamp {
				occupied = &pb.SCUFRevivePointOccupy{Id: proto.Int64(quote.PointID), Camp: frame.Camp}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if update != nil || occupied != nil {
			if err := p13UFPublishBeforeAbort(ctx, quote.MatchID, func() {
				if update != nil {
					s.p13UFFieldBroadcast(ctx, quote.MatchID, 0, update)
				}
				if occupied != nil {
					s.p13UFFieldBroadcast(ctx, quote.MatchID, 0, occupied)
				}
			}); err != nil {
				return err
			}
		}

	}
	return nil
}
func p13UFRevivePointApply(tx *gorm.DB, matchID string, buildings *pb.SCUFBuildingRefresh) error {
	var states []db.P13UnionFightRevivePointState
	if err := tx.Find(&states, "match_id=?", matchID).Error; err != nil {
		return err
	}
	for _, state := range states {
		var quote db.P13UnionFightRevivePointOffer
		if err := tx.First(&quote, "match_id=? AND point_guid=?", state.MatchID, state.PointGUID).Error; err != nil {
			return err
		}
		frame, _, err := p13UFRevivePointRead(&state, &quote)
		if err != nil {
			return err
		}
		found := false
		for i, unit := range buildings.GetPointInfos() {
			if unit.GetGuid() == state.PointGUID {
				buildings.PointInfos[i] = frame
				found = true
			}
		}
		if !found {
			return errors.New("原建筑快照不存在占领点")
		}
	}
	return nil
}
