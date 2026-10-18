package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
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

type p14FinalDrawEntrant struct {
	ServerID int64  `json:"serverId"`
	PlayerID int64  `json:"playerId"`
	Name     string `json:"name"`
	Image    string `json:"image"`
	Power    int32  `json:"power"`
	Group    string `json:"group"`
	Slot     int32  `json:"slot"`
}

// Pair references bind only two real ordered participants; missing opponents
// are not represented as synthetic players or automatic advances.
type p14FinalDrawPair struct {
	Group         string `json:"group"`
	Index         int32  `json:"index"`
	LeftServerID  int64  `json:"leftServerId"`
	LeftPlayerID  int64  `json:"leftPlayerId"`
	RightServerID int64  `json:"rightServerId"`
	RightPlayerID int64  `json:"rightPlayerId"`
}

type p14FinalDrawSnapshot struct {
	SeasonIndex       int32                 `json:"seasonIndex"`
	Version           string                `json:"version"`
	Source            string                `json:"source"`
	PublishedAt       int64                 `json:"publishedAt"`
	QualificationHash string                `json:"qualificationHash"`
	RulesHash         string                `json:"rulesHash"`
	Entrants          []p14FinalDrawEntrant `json:"entrants"`
	Pairs             []p14FinalDrawPair    `json:"pairs"`
}

func p14FinalDrawOrderHash(common string, entry p14FinalDrawEntrant) string {
	return p6Digest([]byte(common + "/" + strconv.FormatInt(entry.ServerID, 10) + "/" + strconv.FormatInt(entry.PlayerID, 10)))
}

// p14FinalDrawArrange implements only the prepublished local arrangement. An
// unpaired final slot remains unpaired and receives no fabricated victory.
func p14FinalDrawArrange(common string, entries []p14FinalDrawEntrant) []p14FinalDrawEntrant {
	result := append([]p14FinalDrawEntrant(nil), entries...)
	sort.Slice(result, func(i, j int) bool {
		a, b := p14FinalDrawOrderHash(common, result[i]), p14FinalDrawOrderHash(common, result[j])
		if a != b {
			return a < b
		}
		if result[i].ServerID != result[j].ServerID {
			return result[i].ServerID < result[j].ServerID
		}
		return result[i].PlayerID < result[j].PlayerID
	})
	for i := range result {
		result[i].Group = string(rune('A' + i/constUtil.P14FinalDrawGroupCapacity))
		result[i].Slot = int32(i % constUtil.P14FinalDrawGroupCapacity)
	}
	return result
}

func p14FinalDrawPairs(entries []p14FinalDrawEntrant) []p14FinalDrawPair {
	var pairs []p14FinalDrawPair
	for i := 0; i+1 < len(entries); i++ {
		a, b := entries[i], entries[i+1]
		if a.Slot%2 != 0 || a.Group != b.Group || b.Slot != a.Slot+1 {
			continue
		}
		pairs = append(pairs, p14FinalDrawPair{Group: a.Group, Index: a.Slot / 2, LeftServerID: a.ServerID, LeftPlayerID: a.PlayerID, RightServerID: b.ServerID, RightPlayerID: b.PlayerID})
	}
	return pairs
}

func p14FinalDrawValidate(row *db.P14FinalDraw, qualification *p14FinalQualificationSnapshot, common string, now time.Time) (*p14FinalDrawSnapshot, error) {
	var body p14FinalDrawSnapshot
	if row == nil || qualification == nil || row.ContentHash != p6Digest(row.Content) || p14PromotionRankDecode(row.Content, &body) != nil || body.SeasonIndex != row.SeasonIndex || body.SeasonIndex != qualification.SeasonIndex || body.Source != constUtil.P14FinalDrawSource || strings.TrimSpace(body.Version) == "" || len(body.Version) > 128 || body.PublishedAt <= 0 || body.PublishedAt > qualification.StageStart || body.QualificationHash != common || len(common) != 64 || body.RulesHash != p6Digest([]byte(constUtil.P14FinalDrawRules)) || row.ObservedAt < qualification.StageEnd || row.ObservedAt > now.UnixMilli() || len(body.Entrants) == 0 || len(body.Entrants) > constUtil.P14FinalQualificationLimit {
		return nil, errors.New(constUtil.P14FinalDrawCorrupt)
	}
	expected := len(qualification.Ranking)
	if expected > constUtil.P14FinalQualificationLimit {
		expected = constUtil.P14FinalQualificationLimit
	}
	if len(body.Entrants) != expected {
		return nil, errors.New(constUtil.P14FinalDrawCorrupt)
	}
	seen := map[[2]int64]bool{}
	for _, entry := range body.Entrants {
		key := [2]int64{entry.ServerID, entry.PlayerID}
		if seen[key] || entry.Power <= 0 || entry.Image == "" || p14FinalQualificationEntered(qualification, entry.ServerID, entry.PlayerID) != 1 {
			return nil, errors.New(constUtil.P14FinalDrawCorrupt)
		}
		seen[key] = true
		matched := false
		for _, rank := range qualification.Ranking {
			if rank.ServerID == entry.ServerID && rank.PlayerID == entry.PlayerID {
				matched = entry.Name == rank.Name && entry.Power == rank.Power
				break
			}
		}
		if !matched {
			return nil, errors.New(constUtil.P14FinalDrawCorrupt)
		}
	}
	if !reflect.DeepEqual(p14FinalDrawArrange(common, body.Entrants), body.Entrants) || !reflect.DeepEqual(p14FinalDrawPairs(body.Entrants), body.Pairs) {
		return nil, errors.New(constUtil.P14FinalDrawCorrupt)
	}
	return &body, nil
}

func (s *GameService) InitializeP14FinalDraw(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalDrawPolicy{}, &db.P14FinalDraw{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalDrawPolicy{ID: 1, Source: constUtil.P14FinalDrawSource}).Error
}

// p14FinalDrawTX uses the same genuine final source as admission and formation.
// Its common content has no local observation clock, so both realms derive the
// same real draw without one realm inventing remote players or result facts.
func (s *GameService) p14FinalDrawTX(ctx context.Context, tx *gorm.DB, season *db.P14SeasonPolicy, now time.Time) (*p14FinalDrawSnapshot, error) {
	qualification, err := s.p14FinalQualificationTX(ctx, tx, season.SeasonIndex, now)
	if err != nil {
		return nil, err
	}
	if qualification == nil {
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	var certificate db.P14FinalQualification
	if err = tx.First(&certificate, "season_index=?", season.SeasonIndex).Error; err != nil {
		return nil, err
	}
	var row db.P14FinalDraw
	err = tx.First(&row, "season_index=?", season.SeasonIndex).Error
	if err == nil {
		return p14FinalDrawValidate(&row, qualification, certificate.ContentHash, now)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var gate db.P14FinalDrawPolicy
	if err = tx.First(&gate, 1).Error; err != nil {
		return nil, err
	}
	if !season.Enabled || !gate.Enabled {
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	var entries []p14FinalDrawEntrant
	for _, rank := range qualification.Ranking {
		if len(entries) == constUtil.P14FinalQualificationLimit {
			break
		}
		roster, _, err := s.p14PromotionRankQualification(ctx, tx, season.SeasonIndex, rank.ServerID, now)
		if err != nil {
			return nil, err
		}
		var image string
		for _, source := range roster {
			if source.ServerID != rank.ServerID || source.PlayerID != rank.PlayerID {
				continue
			}
			frame := &pb.FormationSimpleInfo{}
			if proto.Unmarshal(source.Snapshot, frame) != nil || source.FightPower != rank.Power || frame.GetName() != rank.Name {
				return nil, errors.New(constUtil.P14FinalDrawCorrupt)
			}
			for _, slot := range frame.GetFormationItemInfoList() {
				if slot.GetWarInfo().GetHeroId() != "" {
					image = slot.GetWarInfo().GetHeroId()
					break
				}
			}
			break
		}
		if image == "" {
			return nil, errors.New(constUtil.P14FinalDrawCorrupt)
		}
		entries = append(entries, p14FinalDrawEntrant{ServerID: rank.ServerID, PlayerID: rank.PlayerID, Name: rank.Name, Image: image, Power: rank.Power})
	}
	body := p14FinalDrawSnapshot{SeasonIndex: season.SeasonIndex, Version: gate.Version, Source: gate.Source, PublishedAt: gate.PublishedAt, QualificationHash: certificate.ContentHash, RulesHash: p6Digest([]byte(constUtil.P14FinalDrawRules)), Entrants: p14FinalDrawArrange(certificate.ContentHash, entries)}
	body.Pairs = p14FinalDrawPairs(body.Entrants)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	row = db.P14FinalDraw{SeasonIndex: season.SeasonIndex, Content: raw, ContentHash: p6Digest(raw), ObservedAt: now.UnixMilli()}
	if _, err = p14FinalDrawValidate(&row, qualification, certificate.ContentHash, now); err != nil {
		return nil, err
	}
	if err = tx.Create(&row).Error; err != nil {
		return nil, err
	}
	return &body, nil
}

// p14FinalDrawProject keeps the original empty-group FinalFour query distinct
// even after complete first-round entrants exist. An actual empty A-D group is
// a valid first-round view and contains no fabricated player message.
func p14FinalDrawProject(body *p14FinalDrawSnapshot, group string) (*pb.KFMatchAllRandList, error) {
	if body == nil || group == "" {
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	list := &pb.KFMatchAllRandList{}
	for _, entry := range body.Entrants {
		if entry.Group != group {
			continue
		}
		// Original list entries are actual players. Zero displayed wins and no
		// promotion flag express no certified game result; future lists stay empty.
		list.KFMatchFirstList = append(list.KFMatchFirstList, &pb.KFMatchFightRandList{ServerId: proto.Int32(int32(entry.ServerID)), PlayerId: proto.Int64(entry.PlayerID), PlayerName: proto.String(entry.Name), Image: proto.String(entry.Image), FightNum: proto.Int32(entry.Power), WinTime: proto.Int32(0), PromotionInfo: proto.Int32(0), GuessType: proto.Int32(0)})
	}
	return list, nil
}

func (s *GameService) p14FinalDrawReadAt(ctx context.Context, request *pb.CSKFMatchFrightList, now time.Time) ([]proto.Message, error) {
	var response *pb.SCKFMatchFrightList
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		stage, end, err := s.p14SeasonStage(tx, &season, now)
		if err != nil {
			return err
		}
		if request.GetReplayFlag() != 0 || stage != constUtil.P14Final16BeforeStage && stage != constUtil.P14Final16Stage {
			return errors.New(constUtil.P14ResultsUnverified)
		}
		body, err := s.p14FinalDrawTX(ctx, tx, &season, now)
		if err != nil {
			return err
		}
		list, err := p14FinalDrawProject(body, request.GetGroupId())
		if err != nil {
			return err
		}
		if err = s.P14FinalScoreProjection(ctx, tx, season.SeasonIndex, body, list); err != nil {
			return err
		}
		response = &pb.SCKFMatchFrightList{KfMatchAllRandList: list, StageType: proto.Int32(stage), CurrentTime: proto.Int64(now.UnixMilli()), EndTime: proto.Int64(end)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
