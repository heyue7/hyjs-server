package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// P14FinalScoreProjection reads already certified games under the caller's
// season transaction. Original81046 receives only actual per-side win counts;
// background/promotion/bracket fields keep their independent original rules.
func (s *GameService) P14FinalScoreProjection(ctx context.Context, tx *gorm.DB, season int32, body *p14FinalDrawSnapshot, list *pb.KFMatchAllRandList) error {
	if body == nil || list == nil {
		return errors.New(constUtil.P14FinalSeriesCorrupt)
	}
	if !tx.Migrator().HasTable(&db.P14FinalSeries{}) {
		return nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var rows []db.P14FinalSeries
	if err = tx.WithContext(ctx).Where("season_index=? AND round_stage=?", season, constUtil.P14Final16Stage).Order("group_id,pair_index").Find(&rows).Error; err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, row := range rows {
		manifest, err := p14PerGameReadSeries(&row)
		if err != nil {
			return err
		}
		if manifest.DrawHash != p6Digest(raw) {
			return errors.New(constUtil.P14FinalSeriesCorrupt)
		}
		count := row.LeftWins + row.RightWins
		if count == 0 {
			continue
		}
		winners, _, err := p14ContinuationHistoryTX(tx, &row, manifest, count)
		if err != nil {
			return err
		}
		left, right, next, state, err := p14FinalSeriesProgress(winners)
		if err != nil || left != row.LeftWins || right != row.RightWins || next != row.CurrentGame || (state == constUtil.P14FinalSeriesComplete) != (row.State == constUtil.P14FinalSeriesComplete) {
			return errors.New(constUtil.P14FinalSeriesCorrupt)
		}
		if state == constUtil.P14FinalSeriesComplete {
			server, player := manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID
			if right == constUtil.P14FinalSeriesWinsRequired {
				server, player = manifest.Pair.RightServerID, manifest.Pair.RightPlayerID
			}
			if row.WinnerServerID != server || row.WinnerPlayerID != player {
				return errors.New(constUtil.P14FinalSeriesCorrupt)
			}
		} else if row.WinnerServerID != 0 || row.WinnerPlayerID != 0 {
			return errors.New(constUtil.P14FinalSeriesCorrupt)
		}
		if err = p14FinalScoreApplyPair(list, manifest.Pair, left, right, seen); err != nil {
			return err
		}
	}
	return nil
}

// Apply joint identity after certificate validation. Identical local player IDs
// in different realms remain distinct original81046 rows.
func p14FinalScoreApplyPair(list *pb.KFMatchAllRandList, pair p14FinalDrawPair, left, right int32, seen map[string]bool) error {
	for _, entry := range list.KFMatchFirstList {
		server, player := int64(entry.GetServerId()), entry.GetPlayerId()
		if !(server == pair.LeftServerID && player == pair.LeftPlayerID || server == pair.RightServerID && player == pair.RightPlayerID) {
			continue
		}
		key := strconv.FormatInt(server, 10) + "/" + strconv.FormatInt(player, 10)
		if seen[key] {
			return errors.New(constUtil.P14FinalSeriesCorrupt)
		}
		seen[key] = true
		wins := left
		if server == pair.RightServerID && player == pair.RightPlayerID {
			wins = right
		}
		entry.WinTime = proto.Int32(wins)
	}
	return nil
}
