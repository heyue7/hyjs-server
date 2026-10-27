package service

import (
	"context"
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

// p7EliteQueryProjection owns one profile transaction's accepted fight sources.
// No source snapshot escapes it; sweep writes reread their current-run facts.
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
	refreshSweeps      bool
	manuals            map[string]db.P7RushManualCalculation
	scoped             map[string]db.P7RushScopedCalculation
	records            map[string]db.P6BattleRecord
	parsed             map[string]*p7EliteQueryParsed
	reads              map[string]p7EliteQueryRead
	scopePoliciesRead  bool
	scopeEnabled       bool
	scopePolicyErr     error
	basePrepared       bool
	baseAttacker       *pb.BattlePlayer
	baseErr            error
	rePrepared         bool
	reAttacker         *pb.BattlePlayer
	reErr              error
	manualBlobs        map[string]map[string]p7RushBlobDigest
	scopedBlobs        map[string]map[string]p7RushBlobDigest
	recordBlobs        map[string]map[string]p7RushBlobDigest
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

// Bind settled-source proof reuse to one owning profile transaction. Section
// choice/reward changes no fight source. Sweep requests also reread new sweep
// facts before proving a section; paid Start and battle End never bind it.
func (s *GameService) p7BindSettledQueryProjection(tx *gorm.DB, state *db.P7RushState) (*gorm.DB, error) {
	q, err := p7NewEliteQueryProjection(tx, state)
	if err != nil {
		return nil, err
	}
	q.serverID = s.OriginalServerID()
	return tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.P7EliteQueryProjectionKey{}, q)), nil
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
		if err := q.loadManualRows(ids[from:to], &manual); err != nil {
			return err
		}
		if err := q.loadScopedRows(ids[from:to], &scoped); err != nil {
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
		if err := q.loadRecordRows(ids[from:to], &rows); err != nil {
			return err
		}
		for _, r := range rows {
			q.records[r.ID] = r
		}
	}
	q.loadCertificates()
	if err := q.loadProofMissBlobs(); err != nil {
		return err
	}
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
	var stored pb.CSRushFightEnd
	if err := proto.Unmarshal(fight.EndRequest, &stored); err != nil {
		return nil, false, err
	}
	if end == nil {
		end = &stored
	} else if !proto.Equal(end, &stored) {
		return nil, true, errors.New(constUtil.P7ScopedRushConflict)
	}
	if r, ok := q.reads[fight.ID]; ok {
		return p7RushDurableCloneOutcome(r.outcome), r.scoped, r.err
	}
	q.verified++
	var r p7EliteQueryRead
	if manual, ok := q.manuals[fight.ID]; ok {
		fingerprint := ""
		record, recordErr := q.record(manual.P6RecordID)
		if recordErr == nil {
			fingerprint, _ = p7RushDurableFingerprint(fight, &manual, record, end, q.manualBlobs[fight.ID], q.recordBlobs[record.ID])
			if q.certificateRuntime != nil {
				if outcome := q.certificateRuntime.acceptedProjection(q.certificates[fight.ID], q.serverID, q.state.PlayerID, fight.ID, fingerprint, record); outcome != nil {
					// 严格校验接受终局时已绑定 ScopeProof 和 CapabilityHash；
					// 证书的指纹覆盖该源行，旧分段摘要需要相同的能力标识。
					outcome.CapabilityHash = manual.ScopeProof
					r.outcome, r.scoped = outcome, true
					q.reads[fight.ID] = r
					return p7RushDurableCloneOutcome(outcome), true, nil
				}
			}
		}
		r.outcome, r.scoped, r.err = p7ReadManualRushRows(q.tx, fight, end, manual, q.record)
		if r.err == nil && r.scoped && r.outcome != nil && r.outcome.Authoritative && recordErr == nil {
			// Report proof persistence failures so bounded migration is resumable.
			r.err = q.certify(fight, record, fingerprint, r.outcome)
		}
	} else if scoped, ok := q.scoped[fight.ID]; ok {
		record, recordErr := q.record(scoped.P6RecordID)
		fingerprint := ""
		if recordErr == nil {
			fingerprint, _ = p7RushDurableFingerprint(fight, &scoped, record, end, q.scopedBlobs[fight.ID], q.recordBlobs[record.ID])
			if q.certificateRuntime != nil {
				r.outcome = q.certificateRuntime.acceptedProjection(q.certificates[fight.ID], q.serverID, q.state.PlayerID, fight.ID, fingerprint, record)
			}
		}
		if r.outcome != nil {
			// 证书覆盖完整 scoped 源行，恢复严格读取时的终局能力标识。
			r.outcome.CapabilityHash = scoped.ScopeProof
			r.scoped = true
		} else {
			r.outcome, r.scoped, r.err = p7ReadScopedRushRows(fight, end, scoped, q.record)
			if r.err == nil && r.scoped && recordErr == nil {
				r.err = q.certify(fight, record, fingerprint, r.outcome)
			}
		}
	}
	q.reads[fight.ID] = r
	return p7RushDurableCloneOutcome(r.outcome), r.scoped, r.err
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
	return q.runWinsBetween(1, floor)
}

func (q *p7EliteQueryProjection) runWinsBetween(from, floor int32) (map[int32]p7ReRushWin, error) {
	if q.refreshSweeps {
		// The same sweep request may have just appended up to three facts.
		// Keep accepted battle proofs while rereading every current-run sweep.
		if err := q.tx.Where("player_id=? AND run_id=?", q.state.PlayerID, q.state.RunID).Find(&q.sweeps).Error; err != nil {
			return nil, err
		}
	}
	wins, err := q.acceptedWins(q.state.RunID, from, floor, nil)
	if err != nil {
		return nil, err
	}
	for _, sweep := range q.sweeps {
		if sweep.Floor < from || sweep.Floor > floor {
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

// The frozen current formation and selected trial bonus are identical for all
// elite directory rows. Each enemy and admission remains checked separately.
func (q *p7EliteQueryProjection) scopeAvailable(s *GameService, ctx context.Context, p *pb.Player, g *model.GameState, cfg db.P7RushStageRule, now time.Time) (bool, error) {
	if !q.scopePoliciesRead {
		q.scopePoliciesRead = true
		allowed, err := p7ManualRushStageAllowed(q.tx, constUtil.P7RushElite)
		q.scopePolicyErr = err
		if err == nil && allowed {
			q.scopeEnabled, q.scopePolicyErr = p7ScopedRushAvailable(q.tx)
		}
	}
	if q.scopePolicyErr != nil || !q.scopeEnabled {
		return false, q.scopePolicyErr
	}
	monster, err := s.validateP7Stage(cfg)
	if err != nil {
		return false, nil
	}
	if !q.basePrepared {
		q.basePrepared = true
		player := proto.Clone(p).(*pb.Player)
		raw, e := json.Marshal(g)
		if e != nil {
			return false, e
		}
		var game model.GameState
		if e = json.Unmarshal(raw, &game); e != nil {
			return false, e
		}
		if e = s.PrepareP5GuildPrayerProperties(ctx, q.tx, player, &game, now); e != nil {
			return false, e
		}
		q.baseAttacker, q.baseErr = s.battlePlayer(player, &game)
	}
	if q.baseErr != nil {
		return false, nil
	}
	attacker := q.baseAttacker
	if cfg.Source == constUtil.P7ReRushSource {
		if !q.rePrepared {
			q.rePrepared = true
			readTx := q.tx.WithContext(context.WithValue(q.tx.Statement.Context, constUtil.P7EliteQueryProjectionKey{}, q))
			q.reAttacker, q.reErr = s.p7ApplyReRushBonus(readTx, q.state, q.baseAttacker)
		}
		if q.reErr != nil {
			return false, q.reErr
		}
		attacker = q.reAttacker
	}
	start := &pb.SCRushFightStart{FightId: proto.String("scope-query"), RandSeed: proto.Int64(0), BattlePlayer: proto.Clone(attacker).(*pb.BattlePlayer), BattleMonsterList: []*pb.BattleMonster{monster}, FightScene: proto.String(cfg.Scene), MaxRound: proto.Int32(cfg.MaxRound), WinKey: proto.Int32(cfg.WinKey), WinValue: proto.Int32(cfg.WinValue), RushType: proto.Int32(cfg.Kind)}
	return s.p7FreshEliteDirectoryScope(start)
}

// The input series never escapes this fresh server-source qualification boundary.
func (s *GameService) p7FreshEliteDirectoryScope(start *pb.SCRushFightStart) (bool, error) {
	return s.p7FreshRushDirectoryScope(start, false)
}

func (s *GameService) p7FreshRushDirectoryScope(start *pb.SCRushFightStart, trial bool) (bool, error) {
	series, err := s.FreezeP6RushSeriesInput(start, trial, true, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		return false, nil
	}
	if validateP6RushStarterScope(series) == nil {
		return true, nil
	}
	if p7PartyRushSourceAvailable(series) || p7NaturalRushSourceAvailable(series) {
		return true, nil
	}
	// This series was just frozen from the current server tables in this function.
	// Reuse the same sealed-source qualification path as freezeAndQualifyP6GenericMechanisms;
	// persisted or supplied series still use the public full source verifier.
	view := *series
	view.PlayerAutomatic = false
	if p7GenericMechanismRushShape(&view) != nil {
		return false, nil
	}
	validator := &GameService{Tables: series.Template.Tables}
	_, err = qualifyP6GenericMechanisms(validator, series.Template, series.Template)
	return err == nil, nil
}

func p7EliteProjectionFor(tx *gorm.DB) *p7EliteQueryProjection {
	q, _ := tx.Statement.Context.Value(constUtil.P7EliteQueryProjectionKey{}).(*p7EliteQueryProjection)
	if q == nil || q.tx.Statement.ConnPool != tx.Statement.ConnPool {
		return nil
	}
	return q
}
