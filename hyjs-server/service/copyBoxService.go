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
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"slices"
	"strconv"
	"strings"
)

type copyBoxInput struct {
	Chapter string          `json:"chapter"`
	Mission string          `json:"mission"`
	Box     *pb.CopyBoxInfo `json:"box"`
	Reward  string          `json:"reward"`
	Source  string          `json:"source"`
}

func (s *GameService) initializeCopyBoxes(tx *gorm.DB) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.CopyBoxRule{ID: constUtil.CopyBoxRuleID, Enabled: true, Source: constUtil.CopyBoxSource}).Error
}
func (s *GameService) copyBoxInput(row map[string]string) (*copyBoxInput, error) {
	mission := s.table("CopyMission", "MissionId", row["MissionId"])
	if mission == nil || row["BoxId"] == "" || row["BoxRes"] == "" {
		return nil, errors.New(constUtil.CopyBoxError)
	}
	values := map[string]float64{}
	for _, key := range []string{"BoxX", "BoxY", "BoxZ", "BoxAngle"} {
		n, e := strconv.ParseFloat(row[key], 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, errors.New(constUtil.CopyBoxError)
		}
		values[key] = n
	}
	// Validate all fixed original entries before freezing or exposing the scene chest.
	for _, part := range strings.Split(row["DropAward"], ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return nil, errors.New(constUtil.CopyBoxError)
		}
		n, e := strconv.ParseInt(pair[1], 10, 32)
		if e != nil || n < 1 {
			return nil, errors.New(constUtil.CopyBoxError)
		}
		ok, e := s.openServerRewardSupported(part)
		if e != nil || !ok {
			return nil, errors.New(constUtil.CopyBoxError)
		}
	}
	return &copyBoxInput{Chapter: mission["ChapterId"], Mission: row["MissionId"], Reward: row["DropAward"], Source: constUtil.CopyBoxSource, Box: &pb.CopyBoxInfo{BoxId: proto.String(row["BoxId"]), X: proto.Float64(values["BoxX"]), Y: proto.Float64(values["BoxY"]), Z: proto.Float64(values["BoxZ"]), Angle: proto.Float64(values["BoxAngle"]), Res: proto.String(row["BoxRes"]), IsOpen: proto.Int32(0)}}, nil
}
func decodeCopyBox(row *db.CopyBoxClaim) (*copyBoxInput, error) {
	input := &copyBoxInput{}
	if json.Unmarshal(row.Input, input) != nil || p6Digest(row.Input) != row.InputHash || input.Source == "" || input.Box == nil || input.Box.GetBoxId() != row.BoxID || input.Box.GetIsOpen() != 0 || input.Chapter == "" || input.Mission == "" || input.Reward == "" {
		return nil, errors.New(constUtil.CopyBoxError)
	}
	return input, nil
}

// CaptureCopyBoxes runs under the existing entry/profile transaction and preserves claimed state.
func (s *GameService) CaptureCopyBoxes(tx *gorm.DB, id int64, chapter, mission string) ([]*pb.CopyBoxInfo, error) {
	if len(s.Tables["CopyBox"]) == 0 {
		return nil, nil
	}
	var rule db.CopyBoxRule
	if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&rule, constUtil.CopyBoxRuleID).Error; e != nil {
		return nil, e
	}
	if !rule.Enabled {
		return nil, nil
	}
	var boxes []*pb.CopyBoxInfo
	for _, cfg := range s.Tables["CopyBox"] {
		if cfg["MissionId"] != mission {
			continue
		}
		row := db.CopyBoxClaim{PlayerID: id, BoxID: cfg["BoxId"]}
		e := tx.First(&row, "player_id=? AND box_id=?", id, row.BoxID).Error
		var input *copyBoxInput
		if errors.Is(e, gorm.ErrRecordNotFound) {
			input, e = s.copyBoxInput(cfg)
			if e != nil {
				return nil, e
			}
			input.Source = rule.Source
			row.Input, e = json.Marshal(input)
			if e != nil {
				return nil, e
			}
			row.InputHash = p6Digest(row.Input)
			if e = tx.Create(&row).Error; e != nil {
				return nil, e
			}
		} else if e != nil {
			return nil, e
		} else {
			input, e = decodeCopyBox(&row)
			if e != nil {
				return nil, e
			}
		}
		if input.Chapter != chapter || input.Mission != mission || input.Source == "" {
			return nil, errors.New(constUtil.CopyBoxError)
		}
		box := proto.Clone(input.Box).(*pb.CopyBoxInfo)
		if row.Claimed {
			box.IsOpen = proto.Int32(1)
		}
		boxes = append(boxes, box)
	}
	return boxes, nil
}

// copyBoxOpen grants the frozen original fixed contents only after the current server run completed.
func (s *GameService) copyBoxOpen(ctx context.Context, id int64, b *pb.CSCopyBox, stamp int64) ([]proto.Message, error) {
	out := &pb.SCCopyBox{}
	return s.p5Mutation(ctx, id, "copy-box", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var rule db.CopyBoxRule
		if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&rule, constUtil.CopyBoxRuleID).Error; e != nil {
			return e
		}
		if !rule.Enabled {
			return errors.New(constUtil.CopyBoxError)
		}
		var row db.CopyBoxClaim
		if e := tx.First(&row, "player_id=? AND box_id=?", id, b.GetBoxId()).Error; e != nil {
			return errors.New(constUtil.CopyBoxError)
		}
		input, e := decodeCopyBox(&row)
		if e != nil {
			return e
		}
		run := g.Run
		if row.Claimed || run == nil || !run.Complete || run.Chapter != input.Chapter || run.Mission != input.Mission || run.Fight == nil || !run.Fight.Settled || run.Fight.Result.GetFightResult() != 1 {
			return errors.New(constUtil.CopyBoxError)
		}
		for _, task := range s.tasks(run.Mission) {
			if !slices.Contains(run.Passed, task["TaskId"]) {
				return errors.New(constUtil.CopyBoxError)
			}
		}
		gain, e := s.openServerGain(p, g, input.Reward)
		if e != nil {
			return e
		}
		for _, v := range gain.GetSimps() {
			out.CurrentResource = append(out.CurrentResource, &pb.BaseResource{Id: v.Id, Num: v.Num})
		}
		for _, v := range gain.GetEquips() {
			out.CurrentResource = append(out.CurrentResource, &pb.BaseResource{Id: v.EquipId, Guid: v.Guid, Num: proto.Int32(1)})
		}
		for _, v := range gain.GetWars() {
			out.CurrentResource = append(out.CurrentResource, &pb.BaseResource{Id: v.Id, Guid: v.Guid, Num: proto.Int32(1)})
		}
		row.Claimed = true
		return tx.Save(&row).Error
	})
}
