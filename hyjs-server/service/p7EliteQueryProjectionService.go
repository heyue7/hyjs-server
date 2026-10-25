package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7EliteQueryRead struct {
	outcome *model.CombatOutcome
	scoped  bool
	err     error
}

type p7EliteQueryParsed struct {
	rule                    db.P7RushStageRule
	end                     pb.CSRushFightEnd
	result                  pb.SCRushFightEnd
	ruleOK, endOK, resultOK bool
	startHash, winHash      string
}

// p7EliteQueryProjection owns one locked query's source snapshot. No snapshot or
// memo escapes this query; mutation and general settled-read invalidation stay unchanged.
type p7EliteQueryProjection struct {
	serverID           int64
	certificateRuntime *p7RushCertificateRuntime
	certificates       map[string]db.P7RushHistoryCertificate
	tx                 *gorm.DB
	state              *db.P7RushState
	progress           map[int32]map[string]bool
	fights             []db.P7RushFight
	sweeps             []db.P7ReRushSweep
	loaded             bool
	manuals            map[string]db.P7RushManualCalculation
	scoped             map[string]db.P7RushScopedCalculation
	records            map[string]db.P6BattleRecord
	parsed             map[string]*p7EliteQueryParsed
	reads              map[string]p7EliteQueryRead
	verified           int
}

func p7NewEliteQueryProjection(tx *gorm.DB, state *db.P7RushState) (*p7EliteQueryProjection, error) {
	q := &p7EliteQueryProjection{tx: tx, state: state, progress: map[int32]map[string]bool{}, reads: map[string]p7EliteQueryRead{}, parsed: map[string]*p7EliteQueryParsed{}}
	var rows []db.P7RushProgress
	if err := tx.Select("kind", "task_id").Where("player_id=?", state.PlayerID).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if q.progress[row.Kind] == nil {
			q.progress[row.Kind] = map[string]bool{}
		}
		q.progress[row.Kind][row.TaskID] = true
	}
	return q, nil
}

// Load by owner and explicit source IDs, with bounded IN queries and no joins.
// Parsing and verification remain lazy to preserve each stage's original skip/error path.
func (q *p7EliteQueryProjection) load() error {
	if q.loaded {
		return nil
	}
	if err := q.tx.Where("player_id=? AND kind=? AND status=? AND diagnostic=?", q.state.PlayerID, constUtil.P7RushTrial, constUtil.P7RushSettled, false).Find(&q.fights).Error; err != nil {
		return err
	}
	if err := q.tx.Where("player_id=? AND run_id=?", q.state.PlayerID, q.state.RunID).Find(&q.sweeps).Error; err != nil {
		return err
	}
	q.manuals = map[string]db.P7RushManualCalculation{}
	q.scoped = map[string]db.P7RushScopedCalculation{}
	q.records = map[string]db.P6BattleRecord{}
	ids := make([]string, 0, len(q.fights))
	recordIDs := map[string]bool{}
	for _, f := range q.fights {
		ids = append(ids, f.ID)
		recordIDs[f.ReplayID] = true
	}
	for from := 0; from < len(ids); from += 128 {
		to := min(from+128, len(ids))
		var manual []db.P7RushManualCalculation
		var scoped []db.P7RushScopedCalculation
		if err := q.tx.Where("fight_id IN ?", ids[from:to]).Find(&manual).Error; err != nil {
			return err
		}
		if err := q.tx.Where("fight_id IN ?", ids[from:to]).Find(&scoped).Error; err != nil {
			return err
		}
		for _, r := range manual {
			q.manuals[r.FightID] = r
			recordIDs[r.P6RecordID] = true
		}
		for _, r := range scoped {
			q.scoped[r.FightID] = r
			recordIDs[r.P6RecordID] = true
		}
	}
	ids = nil
	for id := range recordIDs {
		ids = append(ids, id)
	}
	for from := 0; from < len(ids); from += 128 {
		to := min(from+128, len(ids))
		var rows []db.P6BattleRecord
		if err := q.tx.Where("id IN ?", ids[from:to]).Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			q.records[r.ID] = r
		}
	}
	q.loadCertificates()
	q.loaded = true
	return nil
}

func (q *p7EliteQueryProjection) record(id string) (*db.P6BattleRecord, error) {
	r, ok := q.records[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &r, nil
}

func (q *p7EliteQueryProjection) read(fight *db.P7RushFight, end *pb.CSRushFightEnd) (*model.CombatOutcome, bool, error) {
	if r, ok := q.reads[fight.ID]; ok {
		return r.outcome, r.scoped, r.err
	}
	q.verified++
	var r p7EliteQueryRead
	if manual, ok := q.manuals[fight.ID]; ok {
		// Reuse cross-request accepted history only after hashing all actual source
		// rows loaded for this query, including hidden blobs and the supplied End.
		var key p7RushHistoricalKey
		fingerprint := ""
		record, recordErr := q.record(manual.P6RecordID)
		if recordErr == nil && manual.Status == constUtil.P7RushManualAccepted && manual.AcceptedAt > 0 && record.Status == constUtil.P6BattleComputed && record.Authoritative {
			fingerprint, _ = p7RushCertificateFingerprint(fight, &manual, record, end)
			if fingerprint != "" {
				if q.certificateRuntime != nil {
					if cert, ok := q.certificates[fight.ID]; ok {
						if outcome := q.certificateRuntime.accepted(cert, q.serverID, q.state.PlayerID, fight.ID, fingerprint, record); outcome != nil {
							r.outcome, r.scoped = outcome, true
							q.reads[fight.ID] = r
							return r.outcome, r.scoped, nil
						}
					}
				}
				if pool, e := q.tx.DB(); e == nil {
					key = p7RushHistoricalKey{database: pool, fingerprint: fingerprint}
				}
			}
		}
		if key.database != nil {
			if raw := p7RushHistoryCache.get(key, time.Now()); raw != nil {
				r.outcome, r.err = p7DecodeHistoricalOutcome(raw)
				r.scoped = r.err == nil
			}
		}
		if r.outcome == nil || r.err != nil {
			r.outcome, r.scoped, r.err = p7ReadManualRushRows(q.tx, fight, end, manual, q.record)
			if key.database != nil && r.err == nil && r.scoped && r.outcome != nil && r.outcome.Authoritative {
				if raw, e := p7EncodeHistoricalOutcome(r.outcome); e == nil {
					p7RushHistoryCache.put(key, raw, time.Now())
				}
			}
		}
		if r.err == nil && r.scoped && r.outcome != nil && r.outcome.Authoritative && recordErr == nil {
			// A derived-cache write failure cannot invalidate an otherwise valid
			// directory read. The dedicated warmer reports persistence failures.
			_ = q.certify(fight, record, fingerprint, r.outcome)
		}
	} else if scoped, ok := q.scoped[fight.ID]; ok {
		r.outcome, r.scoped, r.err = p7ReadScopedRushRows(fight, end, scoped, q.record)
	}
	q.reads[fight.ID] = r
	return r.outcome, r.scoped, r.err
}

func (q *p7EliteQueryProjection) parse(fight *db.P7RushFight) *p7EliteQueryParsed {
	if p, ok := q.parsed[fight.ID]; ok {
		return p
	}
	p := &p7EliteQueryParsed{}
	p.ruleOK = p6Digest(fight.Rule) == fight.RuleHash && json.Unmarshal(fight.Rule, &p.rule) == nil
	p.endOK = proto.Unmarshal(fight.EndRequest, &p.end) == nil
	p.resultOK = proto.Unmarshal(fight.Result, &p.result) == nil
	p.startHash = p6Digest(fight.Start)
	p.winHash = p6Digest(append(append([]byte(fight.StartHash+fight.RuleHash), fight.Result...), fight.EndRequest...))
	q.parsed[fight.ID] = p
	return p
}

func (q *p7EliteQueryProjection) acceptedWins(runID int64, from, to int32, source *string) (map[int32]p7ReRushWin, error) {
	wins := map[int32]p7ReRushWin{}
	tasks := map[string]bool{}
	for floor := from; floor <= to; floor++ {
		tasks[fmt.Sprintf("local_rush_%d", floor)] = true
	}
	for i := range q.fights {
		fight := &q.fights[i]
		if runID != 0 && fight.RunID != runID || source != nil && fight.ID != *source {
			continue
		}
		if !tasks[fight.TaskID] {
			continue
		}
		parsed := q.parse(fight)
		stage := parsed.rule
		if !parsed.ruleOK || parsed.startHash != fight.StartHash || stage.Source != constUtil.P7ReRushSource {
			return nil, errors.New("Re试炼历史战斗来源损坏")
		}
		end, result := &parsed.end, &parsed.result
		if !parsed.endOK || !parsed.resultOK || end.GetFightId() != fight.ID || end.GetResult() != result.GetResult() || stage.Floor < from || stage.Floor > to {
			return nil, errors.New("Re试炼历史结算回执损坏")
		}
		if result.GetResult() != 1 {
			continue
		}
		outcome, scoped, err := q.read(fight, end)
		if err != nil {
			return nil, err
		}
		if !scoped {
			record, e := q.record(fight.ReplayID)
			if e != nil {
				return nil, e
			}
			outcome = &model.CombatOutcome{}
			if record.PlayerID != q.state.PlayerID || record.SourceFight != fight.ID || record.Source != constUtil.P7RushReplaySource || !record.Authoritative || p6Digest(record.Outcome) != record.OutcomeHash || json.Unmarshal(record.Outcome, outcome) != nil {
				return nil, errors.New("Re试炼历史权威记录损坏")
			}
		}
		if outcome == nil || !outcome.Authoritative || outcome.Winner != 0 || stage.Star < 1 || stage.Star > 3 || result.GetStar() != stage.Star {
			return nil, errors.New("Re试炼历史胜利与实际计算冲突")
		}
		if wins[stage.Floor].Star < stage.Star {
			wins[stage.Floor] = p7ReRushWin{Floor: stage.Floor, Star: stage.Star, Fight: fight.ID, Hash: parsed.winHash}
		}
	}
	return wins, nil
}

func (q *p7EliteQueryProjection) runWins(floor int32) (map[int32]p7ReRushWin, error) {
	wins, err := q.acceptedWins(q.state.RunID, 1, floor, nil)
	if err != nil {
		return nil, err
	}
	for _, sweep := range q.sweeps {
		if sweep.Floor < 1 || sweep.Floor > floor {
			continue
		}
		history, e := q.acceptedWins(0, sweep.Floor, sweep.Floor, &sweep.SourceFight)
		if e != nil {
			return nil, e
		}
		win := history[sweep.Floor]
		if win.Star != sweep.Star || win.Fight != sweep.SourceFight || win.Hash != sweep.SourceHash || wins[sweep.Floor].Star != 0 {
			return nil, errors.New("Re试炼扫荡资格来源冲突")
		}
		wins[sweep.Floor] = win
	}
	return wins, nil
}

func (q *p7EliteQueryProjection) prerequisite(s *GameService, stage db.P7RushStageRule) (bool, error) {
	passed := stage.ConditionTaskID == "" || q.progress[constUtil.P7RushTrial][stage.ConditionTaskID]
	if stage.Kind != constUtil.P7RushElite || passed && stage.Source != constUtil.P7ReRushSource {
		return passed, nil
	}
	original := s.table("RushEliteBoss", "EliteTaskId", stage.TaskID)
	if original == nil || original["ConditionTaskId"] != stage.ConditionTaskID {
		return false, nil
	}
	floor, err := p7Integer(stage.ConditionTaskID, 1, math.MaxInt32)
	if err != nil || q.state.HighFloor < floor {
		return false, nil
	}
	if err := q.load(); err != nil {
		return false, err
	}
	// Preserve the original current-run Re proof before looking at fallback
	// history. Unrelated higher-floor damage must not affect a lower stage.
	if stage.Source == constUtil.P7ReRushSource {
		wins, e := q.runWins(floor)
		if e != nil {
			return false, e
		}
		if len(wins) == int(floor) {
			return true, nil
		}
	}
	completed := map[int64]map[int32]bool{}
	for i := range q.fights {
		fight := &q.fights[i]
		if fight.RunID <= 0 {
			continue
		}
		parsed := q.parse(fight)
		rule := parsed.rule
		if !parsed.ruleOK || rule.Kind != constUtil.P7RushTrial || rule.TaskID != fight.TaskID || rule.Floor < 1 || rule.Floor > floor || rule.Star < 1 || rule.Star > 3 {
			continue
		}
		if rule.Source != constUtil.P7SectionStageSource && rule.Source != constUtil.P7RushContinuationSource && rule.Source != constUtil.P7ReRushSource {
			continue
		}
		end, result := &parsed.end, &parsed.result
		if !parsed.endOK || !parsed.resultOK || end.GetFightId() != fight.ID || end.GetResult() != 1 || result.GetResult() != 1 || result.GetStar() != rule.Star {
			continue
		}
		outcome, scoped, e := q.read(fight, end)
		if e != nil {
			return false, e
		}
		if !scoped || outcome == nil || !outcome.Authoritative || outcome.Winner != 0 {
			continue
		}
		if completed[fight.RunID] == nil {
			completed[fight.RunID] = map[int32]bool{}
		}
		completed[fight.RunID][rule.Floor] = true
	}
	for _, run := range completed {
		if len(run) == int(floor) {
			return true, nil
		}
	}
	return false, nil
}
