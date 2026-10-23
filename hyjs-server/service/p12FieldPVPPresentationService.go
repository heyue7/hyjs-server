package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"time"
)

// ProjectP12FieldPVPSide keeps settlement authoritative while a committed PK is presented.
// It returns a wire clone and never changes the persisted terminal side.
func (s *GameService) ProjectP12FieldPVPSide(tx *gorm.DB, id int64, side *pb.PlayerSide, now time.Time) (*pb.PlayerSide, error) {
	if side == nil {
		return nil, nil
	}
	view := proto.Clone(side).(*pb.PlayerSide)
	if !s.P12FieldPVPReady {
		return view, nil
	}
	var state db.P12FieldPVPState
	err := tx.First(&state, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return view, nil
	}
	if err != nil {
		return nil, err
	}
	if state.BusyUntil <= now.UnixMilli() {
		return view, nil
	}
	var receipt db.P12FieldPVPReceipt
	if err = tx.First(&receipt, "fight_id=?", state.LastFightID).Error; err != nil {
		return nil, err
	}
	_, replay, err := s.p12FieldPVPReadReceipt(tx, &receipt)
	if err != nil {
		return nil, err
	}
	party := replay.AttPlayer
	view.State = proto.Int32(constUtil.P12FieldPVPAttackingState)
	if id == receipt.OpponentID {
		party = replay.DefPlayer
		view.State = proto.Int32(constUtil.P12FieldPVPDefendingState)
	} else if id != receipt.PlayerID {
		return nil, errors.New(constUtil.P12FieldPVPConflict)
	}
	view.Anger = proto.Float64(party.Inst.GetAnger())
	view.BattlePos3, view.BattlePos7, view.BattlePos9, view.BattlePos13 = nil, nil, nil, nil
	view.BattlePosHps = nil
	var current, maximum float64
	for _, hero := range party.Formation {
		hp, maxHP := hero.Inst.GetCurHp(), hero.Prop.GetHp()
		ratio := hp / maxHP
		view.BattlePosHps = append(view.BattlePosHps, &pb.BattlePosHp{BattlePos: proto.Int32(hero.GetPose()), HpPercent: proto.Float64(ratio)})
		switch hero.GetPose() {
		case 3:
			view.BattlePos3 = proto.Float64(ratio)
		case 7:
			view.BattlePos7 = proto.Float64(ratio)
		case 9:
			view.BattlePos9 = proto.Float64(ratio)
		case 13:
			view.BattlePos13 = proto.Float64(ratio)
		}
		current += hp
		maximum += maxHP
	}
	view.CurHp = proto.Float64(current / maximum)
	return view, nil
}

// RunP12FieldPVPPresentation owns one bounded ticker for the service lifetime.
func (s *GameService) RunP12FieldPVPPresentation(ctx context.Context) error {
	ticker := time.NewTicker(constUtil.P12FieldPVPPresentationInterval)
	defer ticker.Stop()
	for {
		if err := s.TickP12FieldPVPPresentation(ctx, LocalPlayNow(ctx)); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// TickP12FieldPVPPresentation consumes only mutable display markers, retaining immutable receipts.
// Profile locks follow the same scene-mutex/profile/state ordering as accepting PK.
func (s *GameService) TickP12FieldPVPPresentation(ctx context.Context, now time.Time) error {
	var pending []db.P12FieldPVPState
	if err := db.GetIns().WithContext(ctx).Where("busy_until>0 AND busy_until<=?", now.UnixMilli()).Order("player_id").Limit(constUtil.P12FieldPVPPresentationBatch).Find(&pending).Error; err != nil {
		return err
	}
	for _, candidate := range pending {
		var side *pb.PlayerSide
		var scene string
		var line int32
		p10SceneMutation.Lock()
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if _, err := p10LockProfiles(tx, candidate.PlayerID, candidate.PlayerID); err != nil {
				return err
			}
			var state db.P12FieldPVPState
			if err := tx.First(&state, "player_id=?", candidate.PlayerID).Error; err != nil {
				return err
			}
			if state.BusyUntil <= 0 || state.BusyUntil > now.UnixMilli() {
				return nil
			}
			var row db.Profile
			if err := tx.First(&row, candidate.PlayerID).Error; err != nil {
				return err
			}
			_, g, err := decodeProfile(&row)
			if err != nil {
				return err
			}
			if g.Field != nil && g.Field.Side != nil {
				side = proto.Clone(g.Field.Side).(*pb.PlayerSide)
				scene = p10SceneScope(g)
				var presence db.P10ScenePresence
				if e := tx.First(&presence, "player_id=?", candidate.PlayerID).Error; e == nil && presence.Scene == scene {
					line = presence.Line
				} else if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
			}
			return tx.Model(&state).Update("busy_until", 0).Error
		})
		p10SceneMutation.Unlock()
		if err != nil {
			return err
		}
		if side != nil {
			sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			message := &pb.SCFieldPlayerState{Player: []*pb.PlayerSide{side}}
			s.publishP10Private(sendCtx, candidate.PlayerID, message)
			if scene != "" {
				s.p10SceneBroadcast(sendCtx, scene, line, candidate.PlayerID, message)
			}
			cancel()
		}
	}
	return nil
}
