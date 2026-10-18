package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p14FinalScoreSource struct {
	ServerID          int64  `json:"serverId"`
	Revision          int64  `json:"revision"`
	VersionHash       string `json:"versionHash"`
	QualificationHash string `json:"qualificationHash"`
}

type p14FinalQualificationSnapshot struct {
	SeasonIndex   int32                        `json:"seasonIndex"`
	PolicyVersion string                       `json:"policyVersion"`
	Source        string                       `json:"source"`
	PublishedAt   int64                        `json:"publishedAt"`
	StageStart    int64                        `json:"stageStart"`
	StageEnd      int64                        `json:"stageEnd"`
	RankRulesHash string                       `json:"rankRulesHash"`
	CohortHash    string                       `json:"cohortHash"`
	Sources       []p14FinalScoreSource        `json:"sources"`
	Ranking       []p14PromotionRankProjection `json:"ranking"`
}

func p14FinalQualificationValidate(row *db.P14FinalQualification, now time.Time) (*p14FinalQualificationSnapshot, error) {
	var body p14FinalQualificationSnapshot
	if row == nil || row.ContentHash != p6Digest(row.Content) || p14PromotionRankDecode(row.Content, &body) != nil || body.SeasonIndex != row.SeasonIndex || body.SeasonIndex <= 0 || body.Source != constUtil.P14FinalQualificationSource || strings.TrimSpace(body.PolicyVersion) == "" || len(body.PolicyVersion) > 128 || body.PublishedAt <= 0 || body.PublishedAt > body.StageStart || body.StageEnd <= body.StageStart || body.StageEnd-body.StageStart > constUtil.P14PromotionMaxStageMillis || row.ObservedAt < body.StageEnd || row.ObservedAt > now.UnixMilli() || body.RankRulesHash != p6Digest([]byte(constUtil.P14PromotionRankRules)) || len(body.Sources) < 2 || len(body.Sources) > constUtil.P14PromotionRankPeerLimit || len(body.Ranking) == 0 || len(body.Ranking) > constUtil.P14PromotionRankPeerLimit*constUtil.P14AuditionPromotionLimit {
		return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
	}
	var servers []int64
	for i, source := range body.Sources {
		if source.ServerID <= 0 || i > 0 && source.ServerID <= body.Sources[i-1].ServerID || source.Revision < 1 || len(source.VersionHash) != 64 || len(source.QualificationHash) != 64 {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		servers = append(servers, source.ServerID)
	}
	raw, _ := json.Marshal(servers)
	if body.CohortHash != p6Digest(raw) {
		return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
	}
	seen := map[[2]int64]bool{}
	counts := map[int64]int{}
	for _, entry := range body.Ranking {
		key := [2]int64{entry.ServerID, entry.PlayerID}
		found := false
		for _, server := range servers {
			found = found || entry.ServerID == server
		}
		if !found || entry.PlayerID <= 0 || seen[key] || entry.Score < 1 || (entry.Score-1)%constUtil.P14PromotionRankWinDelta != 0 || entry.Score > 1+constUtil.P14PromotionRankWinDelta*constUtil.P14PromotionRankMaxAccepted || entry.Power <= 0 {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		seen[key] = true
		counts[entry.ServerID]++
		if counts[entry.ServerID] > constUtil.P14AuditionPromotionLimit {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
	}
	ordered := append([]p14PromotionRankProjection(nil), body.Ranking...)
	p14PromotionRankSort(ordered)
	if !reflect.DeepEqual(ordered, body.Ranking) {
		return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
	}
	return &body, nil
}

func (s *GameService) InitializeP14FinalQualification(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalQualificationPolicy{}, &db.P14FinalQualification{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalQualificationPolicy{ID: 1, Source: constUtil.P14FinalQualificationSource}).Error
}

// p14FinalSourcesTX reads each source's real terminal score version. A missing
// peer or an unsealed version returns pending without manufacturing a result.
// Once frozen, refs select the original exact revisions rather than a live rank.
func (s *GameService) p14FinalSourcesTX(ctx context.Context, tx *gorm.DB, own *db.P14PromotionRankOffer, refs []p14FinalScoreSource, now time.Time) ([]p14FinalScoreSource, []p14PromotionRankProjection, bool, error) {
	peers, err := p14PromotionRankValidateOffer(own, now)
	if err != nil {
		return nil, nil, false, err
	}
	if refs != nil && len(refs) != len(peers) {
		return nil, nil, false, errors.New(constUtil.P14FinalQualificationCorrupt)
	}
	var sources []p14FinalScoreSource
	var ranking []p14PromotionRankProjection
	for i, server := range peers {
		var version db.P14PromotionRankVersion
		query := tx.Where("season_index=? AND server_id=?", own.SeasonIndex, server)
		if refs != nil {
			if refs[i].ServerID != server {
				return nil, nil, false, errors.New(constUtil.P14FinalQualificationCorrupt)
			}
			query = query.Where("revision=?", refs[i].Revision)
		}
		if err = query.Order("revision DESC").First(&version).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, nil
		} else if err != nil {
			return nil, nil, false, err
		}
		body, err := p14PromotionRankReadVersion(&version, now)
		if err != nil {
			return nil, nil, false, err
		}
		if !body.Closed {
			return nil, nil, false, nil
		}
		if body.Offer.Version != own.Version || body.Offer.RulesHash != own.RulesHash || body.Offer.CohortHash != own.CohortHash || body.Offer.StageStart != own.StageStart || body.Offer.StageEnd != own.StageEnd {
			return nil, nil, false, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		roster, q, err := s.p14PromotionRankQualification(ctx, tx, own.SeasonIndex, server, now)
		if err != nil {
			return nil, nil, false, err
		}
		if err = p14PromotionRankValidateScores(body, q); err != nil {
			return nil, nil, false, err
		}
		ref := p14FinalScoreSource{ServerID: server, Revision: version.Revision, VersionHash: version.ContentHash, QualificationHash: q.SourceHash}
		if refs != nil && ref != refs[i] {
			return nil, nil, false, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		sources = append(sources, ref)
		byID := map[int64]db.P14AuditionRoster{}
		for _, entry := range roster {
			byID[entry.PlayerID] = entry
		}
		for _, entry := range body.Scores {
			row, ok := byID[entry.PlayerID]
			frame := &pb.FormationSimpleInfo{}
			if !ok || proto.Unmarshal(row.Snapshot, frame) != nil || frame.GetFightPower() != row.FightPower {
				return nil, nil, false, errors.New(constUtil.P14FinalQualificationCorrupt)
			}
			ranking = append(ranking, p14PromotionRankProjection{ServerID: server, PlayerID: entry.PlayerID, Score: entry.Score, Power: row.FightPower, Name: frame.GetName()})
		}
	}
	p14PromotionRankSort(ranking)
	return sources, ranking, true, nil
}

// p14FinalQualificationTX runs behind the existing season row lock. It creates
// a deterministic common source digest only after every frozen origin seals.
func (s *GameService) p14FinalQualificationTX(ctx context.Context, tx *gorm.DB, season int32, now time.Time) (*p14FinalQualificationSnapshot, error) {
	var saved db.P14FinalQualification
	err := tx.First(&saved, "season_index=?", season).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var frozen *p14FinalQualificationSnapshot
	if err == nil {
		frozen, err = p14FinalQualificationValidate(&saved, now)
		if err != nil {
			return nil, err
		}
	}
	var own db.P14PromotionRankOffer
	if err = tx.First(&own, "season_index=? AND server_id=?", season, s.OriginalServerID()).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if frozen != nil {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var gate db.P14FinalQualificationPolicy
	if frozen == nil {
		if err = tx.First(&gate, 1).Error; err != nil {
			return nil, err
		}
		if !gate.Enabled {
			return nil, nil
		}
		if gate.Source != constUtil.P14FinalQualificationSource || gate.PublishedAt <= 0 || gate.PublishedAt > own.StageStart || strings.TrimSpace(gate.Version) == "" || len(gate.Version) > 128 {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		// The original enum correction cannot qualify a legacy stage7 audit.
		var calendar []db.P14TimeSlot
		calendar, err = s.p14SeasonCalendar(tx, season)
		if err != nil {
			return nil, err
		}
		original := false
		for _, slot := range calendar {
			original = original || slot.Stage == constUtil.P14PromotionStage && slot.StartAt == own.StageStart && slot.EndAt == own.StageEnd
		}
		if !original {
			return nil, nil
		}
		if now.UnixMilli() < own.StageEnd {
			return nil, nil
		}
	}
	var refs []p14FinalScoreSource
	if frozen != nil {
		refs = frozen.Sources
	}
	sources, ranking, complete, err := s.p14FinalSourcesTX(ctx, tx, &own, refs, now)
	if err != nil {
		return nil, err
	}
	if !complete {
		if frozen != nil {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		return nil, nil
	}
	if frozen != nil {
		if frozen.StageStart != own.StageStart || frozen.StageEnd != own.StageEnd || frozen.CohortHash != own.CohortHash || frozen.RankRulesHash != own.RulesHash || !reflect.DeepEqual(frozen.Ranking, ranking) {
			return nil, errors.New(constUtil.P14FinalQualificationCorrupt)
		}
		return frozen, nil
	}
	body := p14FinalQualificationSnapshot{SeasonIndex: season, PolicyVersion: gate.Version, Source: gate.Source, PublishedAt: gate.PublishedAt, StageStart: own.StageStart, StageEnd: own.StageEnd, RankRulesHash: own.RulesHash, CohortHash: own.CohortHash, Sources: sources, Ranking: ranking}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	saved = db.P14FinalQualification{SeasonIndex: season, Content: raw, ContentHash: p6Digest(raw), ObservedAt: now.UnixMilli()}
	verified, err := p14FinalQualificationValidate(&saved, now)
	if err != nil {
		return nil, err
	}
	if err = tx.Create(&saved).Error; err != nil {
		return nil, err
	}
	return verified, nil
}

func p14FinalQualificationEntered(body *p14FinalQualificationSnapshot, server, player int64) int32 {
	if body == nil {
		return 0
	}
	for i, entry := range body.Ranking {
		if i >= constUtil.P14FinalQualificationLimit {
			break
		}
		if entry.ServerID == server && entry.PlayerID == player {
			return 1
		}
	}
	return 0
}
