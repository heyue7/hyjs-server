package service

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The original click constructs season_server_player. It is not an invented
// opaque record ID. Generic syntax is validated by the handler; this extracts
// the joint identity needed to locate one actual frozen first-round series.
func p14FinalReplayRegistration(raw string) (int32, int64, int64, error) {
	values := strings.Split(raw, "_")
	if len(values) != 3 {
		return 0, 0, 0, errors.New(constUtil.P14FinalReplayPending)
	}
	season, e1 := strconv.ParseInt(values[0], 10, 32)
	server, e2 := strconv.ParseInt(values[1], 10, 32)
	player, e3 := strconv.ParseInt(values[2], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || season <= 0 || server <= 0 || player <= 0 {
		return 0, 0, 0, errors.New(constUtil.P14FinalReplayPending)
	}
	return int32(season), server, player, nil
}

// The caller's transaction authenticates the entire genuine prefix of games.
// Slot certificate and input bytes remain linked to the exact source result;
// no caller winner, current mutable profile or engine recomputation is used.
func p14FinalReplayHistoryTX(tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest) ([]int32, error) {
	count := series.LeftWins + series.RightWins
	if count == 0 {
		return nil, nil
	}
	winners, _, err := p14ContinuationHistoryTX(tx, series, manifest, count)
	if err != nil {
		return nil, err
	}
	left, right, next, state, err := p14FinalSeriesProgress(winners)
	if err != nil || left != series.LeftWins || right != series.RightWins || next != series.CurrentGame || (state == constUtil.P14FinalSeriesComplete) != (series.State == constUtil.P14FinalSeriesComplete) {
		return nil, errors.New(constUtil.P14FinalReplayCorrupt)
	}
	for index := int32(1); index <= count; index++ {
		var slot db.P14FinalSeriesSlot
		if err = tx.First(&slot, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			return nil, err
		}
		var frozen *model.FrozenBattleInput
		var sourceID, sourceHash, provenanceHash string
		if index == 1 {
			var local db.P14FinalSettlement
			err = tx.First(&local, "series_id=? AND game_index=1", series.ID).Error
			if err == nil {
				var record db.P6BattleRecord
				if err = tx.First(&record, "id=?", local.P6RecordID).Error; err == nil {
					frozen, err = decodeP6Record(&record)
				}
				sourceID, sourceHash, provenanceHash = local.InputID, local.InputHash, local.ProvenanceHash
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				var remote db.P14FinalResultReceipt
				if err = tx.First(&remote, "series_id=? AND game_index=1", series.ID).Error; err == nil {
					frozen, _, err = p14ReadFinalResultReceipt(&remote)
					var source db.P14FinalSettlement
					if err == nil {
						err = p14PromotionRankDecode(remote.SourceResult, &source)
					}
					sourceID, sourceHash, provenanceHash = source.InputID, source.InputHash, source.ProvenanceHash
				}
			}
		} else {
			var local db.P14FinalContinuationResult
			err = tx.First(&local, "series_id=? AND game_index=?", series.ID, index).Error
			if err == nil {
				var record db.P6BattleRecord
				if err = tx.First(&record, "id=?", local.P6RecordID).Error; err == nil {
					frozen, err = decodeP6Record(&record)
				}
				sourceID, sourceHash, provenanceHash = local.InputID, local.InputHash, local.ProvenanceHash
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				var remote db.P14FinalContinuationReceipt
				if err = tx.First(&remote, "series_id=? AND game_index=?", series.ID, index).Error; err == nil {
					frozen, _, err = p14ReadContinuationReceipt(&remote)
					var source db.P14FinalContinuationResult
					if err == nil {
						err = p14PromotionRankDecode(remote.SourceResult, &source)
					}
					sourceID, sourceHash, provenanceHash = source.InputID, source.InputHash, source.ProvenanceHash
				}
			}
		}
		replay := &pb.KFReplayData{}
		if err != nil {
			return nil, err
		}
		if frozen == nil || sourceID != slot.InputID || sourceHash != slot.InputHash || provenanceHash != slot.ProvenanceHash || proto.Unmarshal(slot.Input, replay) != nil || !proto.Equal(replay, frozen.KFReplay) {
			return nil, errors.New(constUtil.P14FinalReplayCorrupt)
		}
	}
	if state == constUtil.P14FinalSeriesComplete {
		server, player := manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID
		if right == 3 {
			server, player = manifest.Pair.RightServerID, manifest.Pair.RightPlayerID
		}
		if series.WinnerServerID != server || series.WinnerPlayerID != player {
			return nil, errors.New(constUtil.P14FinalReplayCorrupt)
		}
	} else if series.WinnerServerID != 0 || series.WinnerPlayerID != 0 {
		return nil, errors.New(constUtil.P14FinalReplayCorrupt)
	}
	return winners, nil
}

func p14FinalReplaySeriesTX(ctx context.Context, tx *gorm.DB, season int32, server, player int64) (*db.P14FinalSeries, *p14FinalSeriesManifest, error) {
	var rows []db.P14FinalSeries
	if err := tx.WithContext(ctx).Where("season_index=? AND round_stage=?", season, constUtil.P14Final16Stage).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	var found *db.P14FinalSeries
	var manifest *p14FinalSeriesManifest
	for index := range rows {
		candidate, err := p14PerGameReadSeries(&rows[index])
		if err != nil {
			return nil, nil, err
		}
		if !p14PerGameMember(candidate, server, player) {
			continue
		}
		if found != nil {
			return nil, nil, errors.New(constUtil.P14FinalReplayCorrupt)
		}
		found, manifest = &rows[index], candidate
	}
	if found == nil {
		return nil, nil, errors.New(constUtil.P14FinalReplayPending)
	}
	return found, manifest, nil
}

// P14FinalReplayInfoAt restores original type3/subtype8 using the click's exact
// joint identity. Only a certified game is state0/played. A planned time is
// state1 and cannot expose a movie; unneeded games after three wins are omitted.
func (s *GameService) P14FinalReplayInfoAt(ctx context.Context, id int64, request *pb.CSKFReplayInfo, now time.Time) ([]proto.Message, error) {
	if request.GetType() != 3 || request.GetSubType() != constUtil.P14Final16Stage || request.GetPageIndex() != 0 || request.GetReplayFlag() != 0 {
		return nil, errors.New(constUtil.P14FinalReplayPending)
	}
	season, server, player, err := p14FinalReplayRegistration(request.GetRegId())
	if err != nil {
		return nil, err
	}
	var response *pb.SCKFReplayInfo
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// Authentication/session checking happens at the existing game boundary.
		// Spectators may read unambiguous original logs without owning a P6 record.
		var current db.P14SeasonPolicy
		if err := tx.First(&current, 1).Error; err != nil {
			return err
		}
		if current.SeasonIndex != season {
			return errors.New(constUtil.P14FinalReplayPending)
		}
		series, manifest, err := p14FinalReplaySeriesTX(ctx, tx, season, server, player)
		if err != nil {
			return err
		}
		if manifest.Pair.LeftPlayerID == manifest.Pair.RightPlayerID {
			return errors.New(constUtil.P14FinalReplayPending)
		}
		winners, err := p14FinalReplayHistoryTX(tx, series, manifest)
		if err != nil {
			return err
		}
		var slots []db.P14FinalSeriesSlot
		if err = tx.Where("series_id=?", series.ID).Order("game_index").Find(&slots).Error; err != nil {
			return err
		}
		if len(slots) != 5 {
			return errors.New(constUtil.P14FinalReplayCorrupt)
		}
		first := &pb.KFReplayData{}
		if proto.Unmarshal(slots[0].Input, first) != nil || len(first.PlayerList) != 2 {
			return errors.New(constUtil.P14FinalReplayCorrupt)
		}
		response = &pb.SCKFReplayInfo{Type: proto.Int32(3), SubType: proto.Int32(8), PageIndex: proto.Int32(0), PageNum: proto.Int32(1), Flag: proto.Int32(0)}
		for _, slot := range slots {
			if slot.State == constUtil.P14FinalSeriesSlotUnneeded {
				continue
			}
			replay := first
			if len(slot.Input) > 0 {
				replay = &pb.KFReplayData{}
				if proto.Unmarshal(slot.Input, replay) != nil || len(replay.PlayerList) != 2 || slot.InputHash != p6Digest(slot.Input) {
					return errors.New(constUtil.P14FinalReplayCorrupt)
				}
			}
			left, right := replay.PlayerList[0], replay.PlayerList[1]
			if len(left.Formation) == 0 || len(right.Formation) == 0 {
				return errors.New(constUtil.P14FinalReplayCorrupt)
			}
			row := &pb.KFReplayItem{Time: proto.Int64(slot.ScheduledAt), State: proto.Int32(1), AttServerId: left.ServerId, AttPlayerId: left.PlayerId, AttPlayerName: left.NickName, AttMainHeroId: left.Formation[0].Id, DefServerId: right.ServerId, DefPlayerId: right.PlayerId, DefPlayerName: right.NickName, DefMainHeroId: right.Formation[0].Id}
			if slot.State == constUtil.P14FinalSeriesSlotCertified {
				if slot.GameIndex < 1 || int(slot.GameIndex) > len(winners) || slot.ScheduledAt > now.UnixMilli() {
					return errors.New(constUtil.P14FinalReplayCorrupt)
				}
				winner := manifest.Pair.LeftPlayerID
				if winners[slot.GameIndex-1] == 1 {
					winner = manifest.Pair.RightPlayerID
				}
				row.State = proto.Int32(0)
				row.WarId = proto.String(slot.InputID)
				row.WinPlayerId = proto.Int64(winner)
			}
			response.Replays = append(response.Replays, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// P14FinalReplayAt returns the exact already-certified original input without
// injecting a winner or changing any ID/HP/AP/seed. The current controlled
// wrapper proves a canonical replay only for actual source0's joint viewer.
// Opposite/spectator views remain pending until a separate display adapter is proved.
func (s *GameService) P14FinalReplayAt(ctx context.Context, id int64, request *pb.CSKFReplay, now time.Time) ([]proto.Message, error) {
	if request.GetType() != 3 {
		return nil, errors.New(constUtil.P14FinalReplayPending)
	}
	var response *pb.SCKFReplay
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var slot db.P14FinalSeriesSlot
		if err := tx.First(&slot, "input_id=? AND state=?", request.GetWarId(), constUtil.P14FinalSeriesSlotCertified).Error; err != nil {
			return err
		}
		var series db.P14FinalSeries
		if err := tx.First(&series, "id=?", slot.SeriesID).Error; err != nil {
			return err
		}
		manifest, err := p14PerGameReadSeries(&series)
		if err != nil {
			return err
		}
		var current db.P14SeasonPolicy
		if err = tx.First(&current, 1).Error; err != nil {
			return err
		}
		if current.SeasonIndex != series.SeasonIndex {
			return errors.New(constUtil.P14FinalReplayPending)
		}
		if s.OriginalServerID() != manifest.Pair.LeftServerID || id != manifest.Pair.LeftPlayerID {
			return errors.New(constUtil.P14FinalReplayPending)
		}
		if _, err = p14FinalReplayHistoryTX(tx, &series, manifest); err != nil {
			return err
		}
		replay := &pb.KFReplayData{}
		if slot.ScheduledAt > now.UnixMilli() || proto.Unmarshal(slot.Input, replay) != nil || replay.GetWarId() != request.GetWarId() {
			return errors.New(constUtil.P14FinalReplayCorrupt)
		}
		original, err := proto.Marshal(replay)
		if err != nil || !bytes.Equal(original, slot.Input) {
			return errors.New(constUtil.P14FinalReplayCorrupt)
		}
		response = &pb.SCKFReplay{Type: proto.Int32(3), Data: replay}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
