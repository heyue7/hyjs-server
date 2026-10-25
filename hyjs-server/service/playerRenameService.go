package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type playerRenameState struct {
	Count        int32  `json:"count"`
	Stamp        int64  `json:"stamp"`
	Name         string `json:"name"`
	AdminName    string `json:"adminName,omitempty"`
	RequestHash  string `json:"requestHash"`
	Request      []byte `json:"request"`
	Response     []byte `json:"response"`
	ResponseHash string `json:"responseHash"`
}

// playerRenameNameValid mirrors the original C# char[] width rule. Rejecting
// surrounding whitespace preserves the exact nickname the callback will display.
// The original font's HasCharacter remains a client-side rendering check.
func playerRenameNameValid(name string) bool {
	if !utf8.ValidString(name) || name == "" || name != strings.TrimSpace(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	width := 0
	for _, unit := range utf16.Encode([]rune(name)) {
		if unit <= 256 {
			width++
		} else {
			width += 2
		}
	}
	return width <= 12
}

func readPlayerRenameState(p *pb.Player, g *model.GameState) (*playerRenameState, error) {
	raw := ""
	found := false
	for _, entry := range g.KV {
		if entry.GetKey() == constUtil.PlayerRenameStateKey {
			if found {
				return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
			}
			found = true
			raw = entry.GetValue()
		}
	}
	if !found {
		return &playerRenameState{}, nil
	}
	var state playerRenameState
	if raw == "" || json.Unmarshal([]byte(raw), &state) != nil {
		return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
	}
	expectedName := state.Name
	if state.AdminName != "" {
		if !playerRenameNameValid(state.AdminName) {
			return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
		}
		expectedName = state.AdminName
	}
	if state.Count < 1 || state.Stamp <= 0 || expectedName != p.GetName() || len(state.RequestHash) != 64 || state.ResponseHash != p6Digest(state.Response) {
		return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
	}
	var request pb.CSChangePlayerName
	if p6Digest(state.Request) != state.RequestHash || proto.Unmarshal(state.Request, &request) != nil || request.GetPlayerName() != state.Name {
		return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
	}
	var response pb.SCChangePlayerName
	if proto.Unmarshal(state.Response, &response) != nil {
		return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
	}
	// The first successful rename is free; all subsequent persisted responses
	// contain one positive diamond charge frozen at that actual request.
	if state.Count == 1 && len(response.Cost) != 0 {
		return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
	}
	if state.Count > 1 && (len(response.Cost) != 1 || response.Cost[0].GetId() != "diamond" || response.Cost[0].GetNum() <= 0) {
		return nil, errors.New(constUtil.PlayerRenameReceiptInvalid)
	}
	return &state, nil
}

func (s *GameService) changeNameTimes(ctx context.Context, id int64) ([]proto.Message, error) {
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	state, err := readPlayerRenameState(p, g)
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCChangeNameTimes{ChangeNameTimes: proto.Int32(state.Count)}}, nil
}

// changePlayerName returns only the original result. The native callback applies
// Delete(cost), updates its exact input nickname, then sends original TCP30037;
// an immediate SCPlayer balance would cause it to deduct that charge twice.
func (s *GameService) changePlayerName(ctx context.Context, id int64, request *pb.CSChangePlayerName, stamp int64) ([]proto.Message, error) {
	if !s.P1Ready {
		return nil, errors.New(constUtil.PlayerRenameUnavailable)
	}
	if stamp <= 0 || !playerRenameNameValid(request.GetPlayerName()) {
		return nil, errors.New(constUtil.PlayerRenameInvalid)
	}
	if err := validateNameContent(request.GetPlayerName()); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	var response *pb.SCChangePlayerName
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		state, err := readPlayerRenameState(p, g)
		if err != nil {
			return err
		}
		if state.Count > 0 && stamp <= state.Stamp {
			if state.AdminName != "" {
				return errors.New(constUtil.PlayerRenameRequestExpired)
			}
			if stamp != state.Stamp || state.RequestHash != p6Digest(raw) {
				return errors.New(constUtil.PlayerRenameRequestExpired)
			}
			response = &pb.SCChangePlayerName{}
			return proto.Unmarshal(state.Response, response)
		}
		if request.GetPlayerName() == p.GetName() {
			return errors.New(constUtil.PlayerRenameUnchanged)
		}
		if state.Count == math.MaxInt32 {
			return errors.New(constUtil.PlayerRenameReceiptInvalid)
		}
		if err = lockProfilePeriods(tx); err != nil {
			return err
		}
		response = &pb.SCChangePlayerName{}
		if state.Count > 0 {
			cost, err := parsePoolCost(s.table("GameConfigure", "StaticName", "EditNameCost")["NameValue"])
			if err != nil || cost.GetId() != "diamond" || cost.GetNum() <= 0 {
				return errors.New(constUtil.PlayerRenameReceiptInvalid)
			}
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
			response.Cost = []*pb.BaseResource{cost}
			if err = s.RecordP1Cost(tx, id, fmt.Sprintf("player-rename:%d", stamp), response.Cost, LocalPlayNow(ctx)); err != nil {
				return err
			}
		}
		p.Name = proto.String(request.GetPlayerName())
		state.Count++
		state.Stamp = stamp
		state.Name = p.GetName()
		state.AdminName = ""
		state.RequestHash = p6Digest(raw)
		state.Request = append([]byte(nil), raw...)
		state.Response, err = proto.Marshal(response)
		if err != nil {
			return err
		}
		state.ResponseHash = p6Digest(state.Response)
		encoded, err := json.Marshal(state)
		if err != nil {
			return err
		}
		p5SetKV(g, constUtil.PlayerRenameStateKey, string(encoded))
		return saveProfile(tx, &row, p, g)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
