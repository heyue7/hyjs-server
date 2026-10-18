package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p11GrabFightRules struct {
	Policy db.P11GrabFightPolicy `json:"policy"`
	Grab   db.P11GrabPolicy      `json:"grab"`
}

// InitializeP11GrabFight adds the independent occupied-slot capability off.
// It never creates occupancy, participants, attempts or accepted battle facts.
func (s *GameService) InitializeP11GrabFight(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11GrabFightPolicy{}, &db.P11GrabFightOffer{}, &db.P11GrabFightSettlement{}); err != nil {
		return err
	}
	policy := db.P11GrabFightPolicy{ID: 1, Enabled: false, Version: "local-grab-occupied-v1", Source: constUtil.P11GrabFightPolicySource, MaxRound: constUtil.P11GrabFightMaxRound, AttackerFirst: true}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error
}

func p11GrabFightRulesValid(rules *p11GrabFightRules) bool {
	if rules == nil {
		return false
	}
	p, g := rules.Policy, rules.Grab
	return p.ID == 1 && p.Enabled && strings.TrimSpace(p.Version) == p.Version && p.Version != "" && len(p.Version) <= 64 && p.Source == constUtil.P11GrabFightPolicySource && p.PublishedAt > 0 && p.PublishedAt <= p.StartsAt && p.EndsAt > p.StartsAt && p.MaxRound == constUtil.P11GrabFightMaxRound && p.AttackerFirst && g.ID == 1 && g.Energy == 10 && g.MaxTimes == 3 && g.BuyMax == 3 && g.BuyPrice == 100 && g.MaxMillis == int64(3*time.Hour/time.Millisecond) && g.Source != "" && p6Digest([]byte(g.RewardsJSON)) == g.RewardsHash
}

// The frozen offer owns the original cost/session rules for its version.
// Current Enabled controls new use; historical receipt reads never consult it.
func (s *GameService) p11GrabFightOffer(tx *gorm.DB, grab *db.P11GrabPolicy, now time.Time) (*db.P11GrabFightOffer, *p11GrabFightRules, error) {
	var policy db.P11GrabFightPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
		return nil, nil, err
	}
	if !policy.Enabled {
		return nil, nil, errors.New(constUtil.P11GrabFightClosed)
	}
	var offer db.P11GrabFightOffer
	err := tx.First(&offer, "version=?", policy.Version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rules := p11GrabFightRules{Policy: policy, Grab: *grab}
		if !p11GrabFightRulesValid(&rules) || now.UnixMilli() < policy.StartsAt || now.UnixMilli() >= policy.EndsAt {
			return nil, nil, errors.New(constUtil.P11GrabFightClosed)
		}
		raw, e := json.Marshal(rules)
		if e != nil {
			return nil, nil, e
		}
		offer = db.P11GrabFightOffer{Version: policy.Version, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}
		if e = tx.Create(&offer).Error; e != nil {
			return nil, nil, e
		}
	} else if err != nil {
		return nil, nil, err
	}
	var rules p11GrabFightRules
	if offer.SourceHash != p6Digest([]byte(offer.Snapshot)) || json.Unmarshal([]byte(offer.Snapshot), &rules) != nil || !p11GrabFightRulesValid(&rules) || rules.Policy.Version != offer.Version || now.UnixMilli() < rules.Policy.StartsAt || now.UnixMilli() >= rules.Policy.EndsAt || offer.CapturedAt < rules.Policy.StartsAt || offer.CapturedAt >= rules.Policy.EndsAt {
		return nil, nil, errors.New(constUtil.P11GrabFightConflict)
	}
	return &offer, &rules, nil
}

func p11GrabFightProperties(row *db.P11GrabFightSettlement) map[string][]string {
	return map[string][]string{
		"source":       {constUtil.P11GrabFightSource},
		"grabRequest":  {row.RequestHash},
		"grabOffer":    {row.OfferVersion, row.OfferHash},
		"grabStart":    {row.FightID, row.StartHash},
		"grabPrevious": {strconv.FormatInt(row.Position, 10), strconv.FormatInt(row.DefenderID, 10), row.PreviousSessionID, strconv.FormatInt(row.PreviousStartedAt, 10), strconv.FormatInt(row.PreviousExpiresAt, 10)},
		"grabCost":     {strconv.FormatInt(int64(row.BeforeEnergy), 10), strconv.FormatInt(int64(row.AfterEnergy), 10), strconv.FormatInt(int64(row.BeforeRemaining), 10), strconv.FormatInt(int64(row.AfterRemaining), 10), strconv.FormatInt(int64(row.BeforeUsed), 10), strconv.FormatInt(int64(row.AfterUsed), 10)},
	}
}

// p11GrabFightLocked uses the caller's single transaction and already ordered
// profile locks. It never locks a third profile or opens a nested transaction.
// The caller persists these same player/game/daily pointers in its legacy tail.
func (s *GameService) p11GrabFightLocked(ctx context.Context, tx *gorm.DB, own, defender *db.Profile, player *pb.Player, game *model.GameState, cfg *db.P11GrabPolicy, slot *db.P11GrabSlot, daily *db.P11GrabDaily, request *pb.CSGrab, stamp int64, now time.Time) (*pb.SCGrab, error) {
	reject := func() (*pb.SCGrab, error) { return nil, errors.New(constUtil.P11GrabFightConflict) }
	if ctx == nil || tx == nil || own == nil || defender == nil || player == nil || game == nil || cfg == nil || slot == nil || daily == nil || request == nil || stamp <= 0 || own.Id <= 0 || own.Id == defender.Id || player.GetPlayerId() != own.Id || slot.OwnerID == nil || *slot.OwnerID != defender.Id || slot.Position != int64(request.GetIndex()) || slot.Position < 1 || slot.Position > 18 || slot.SessionID == "" || slot.StartedAt <= 0 || slot.StartedAt > now.UnixMilli() || slot.ExpiresAt <= now.UnixMilli() || daily.PlayerID != own.Id || len(request.ProtoReflect().GetUnknown()) != 0 {
		return reject()
	}
	if s.P12WantedReady || s.P12FieldPVPReady {
		for _, pid := range []int64{own.Id, defender.Id} {
			// This helper runs only for a new challenge after the caller's receipt
			// recovery. Both locked participants must be free of existing modes.
			if err := s.ensureNoActiveDailyCopy(tx, pid, now); err != nil {
				return nil, err
			}
		}
	}
	if !cfg.Enabled {
		return nil, errors.New(constUtil.P11GrabFightClosed)
	}
	if now.UnixMilli() <= 0 || cfg.MaxMillis <= 0 || now.UnixMilli() > math.MaxInt64-cfg.MaxMillis || daily.Used < 0 || daily.Bought < 0 || daily.Bought > cfg.BuyMax || daily.Used >= math.MaxInt32 || cfg.MaxTimes+daily.Bought-daily.Used < 1 || player.GetEnergy() < cfg.Energy {
		return reject()
	}
	day := now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	if daily.Day != day {
		return reject()
	}
	var old db.P11GrabFightSettlement
	if err := tx.First(&old, "player_id=? AND stamp=?", own.Id, stamp).Error; err == nil {
		return reject()
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	offer, rules, err := s.p11GrabFightOffer(tx, cfg, now)
	if err != nil {
		return nil, err
	}
	if cfg.Energy != rules.Grab.Energy || cfg.MaxMillis != rules.Grab.MaxMillis || cfg.MaxTimes != rules.Grab.MaxTimes || cfg.BuyMax != rules.Grab.BuyMax || player.GetEnergy() < rules.Grab.Energy {
		return reject()
	}
	var previous db.P11GrabSession
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, "id=?", slot.SessionID).Error; err != nil {
		return nil, err
	}
	if previous.PlayerID != defender.Id || previous.Position != slot.Position || previous.StartedAt != slot.StartedAt || previous.ExpiresAt != slot.ExpiresAt || previous.EndedAt != 0 {
		return reject()
	}
	// A fresh same-TX P5 projection retains every actor, talent, skill and source.
	_, attacker, err := s.p11ArenaProjection(own, tx)
	if err != nil {
		return nil, err
	}
	_, enemy, err := s.p11ArenaProjection(defender, tx)
	if err != nil {
		return nil, err
	}
	peer, _, err := decodeProfile(defender)
	if err != nil {
		return nil, err
	}
	minimum, err := s.p10Setting("resPlunderLevelLimit")
	if err != nil {
		return nil, err
	}
	if minimum != 30 || player.GetLevel() < minimum || peer.GetLevel() < minimum || attacker.GetPlayerId() != own.Id || enemy.GetPlayerId() != defender.Id {
		return reject()
	}
	if err = SetP6FirstOrder(attacker, enemy, false); err != nil {
		return nil, err
	}
	seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
	if err != nil {
		return nil, err
	}
	start := &pb.SCGrab{FightId: proto.String(uuid.NewString()), RandSeed: proto.Int64(seed.Int64() + 1), MaxRoundNum: proto.Int32(rules.Policy.MaxRound), AttPlayer: attacker, DefPlayer: enemy}
	startPB, err := proto.MarshalOptions{Deterministic: true}.Marshal(start)
	if err != nil {
		return nil, err
	}
	requestPB, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return nil, err
	}
	beforeRemaining := cfg.MaxTimes + daily.Bought - daily.Used
	row := db.P11GrabFightSettlement{PlayerID: own.Id, Stamp: stamp, DefenderID: defender.Id, Position: slot.Position, Day: daily.Day, RequestHash: p6Digest(requestPB), PreviousSessionID: slot.SessionID, PreviousStartedAt: slot.StartedAt, PreviousExpiresAt: slot.ExpiresAt, OfferVersion: offer.Version, OfferHash: offer.SourceHash, FightID: start.GetFightId(), BeforeEnergy: player.GetEnergy(), AfterEnergy: player.GetEnergy() - rules.Grab.Energy, BeforeRemaining: beforeRemaining, AfterRemaining: beforeRemaining - 1, BeforeUsed: daily.Used, AfterUsed: daily.Used + 1, StartPB: startPB, StartHash: p6Digest(startPB), CreatedAt: now.UnixMilli()}
	frozen, err := s.FreezeP6GrabInput(start)
	if err != nil {
		return nil, err
	}
	frozen.AppliedProperties = p11GrabFightProperties(&row)
	row.P6RecordID, err = s.CaptureBattleReplay(ctx, tx, own.Id, constUtil.P11GrabFightSource, row.FightID, *frozen)
	if err != nil {
		return nil, err
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	terminal, err := s.ComputeP6GrabSettlement(ctx, input)
	if err != nil {
		return nil, err
	}
	if terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownGrabSettlementProof(input, terminal.Outcome.CapabilityHash) || validateP6GrabTerminal(input, terminal) != nil {
		return nil, errors.New(constUtil.P11GrabFightClosed)
	}
	if terminal.Outcome.Winner != 0 && terminal.Outcome.Winner != 1 {
		return reject()
	}
	row.Won = terminal.Outcome.Winner == 0
	row.ScopeProof = terminal.Outcome.CapabilityHash
	// Both accepted results pay once. Only a real challenger win replaces the
	// old session; losses retain the previous owner and exact original clock.
	player.Energy = proto.Int32(row.AfterEnergy)
	daily.Used = row.AfterUsed
	winner := defender.Id
	if row.Won {
		winner = own.Id
		if err = p11GrabClose(tx, slot, now.UnixMilli(), constUtil.P11GrabFightSource); err != nil {
			return nil, err
		}
		row.NewSessionID = uuid.NewString()
		session := db.P11GrabSession{ID: row.NewSessionID, PlayerID: own.Id, Position: slot.Position, StartedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + rules.Grab.MaxMillis}
		if err = tx.Create(&session).Error; err != nil {
			return nil, err
		}
		ownerID := own.Id
		slot.OwnerID = &ownerID
		slot.SessionID = session.ID
		slot.StartedAt = session.StartedAt
		slot.ExpiresAt = session.ExpiresAt
		saved := tx.Model(&db.P11GrabSlot{}).Where("position=? AND owner_id IS NULL", slot.Position).Updates(map[string]any{"owner_id": own.Id, "session_id": slot.SessionID, "started_at": slot.StartedAt, "expires_at": slot.ExpiresAt})
		if saved.Error != nil {
			return nil, saved.Error
		}
		if saved.RowsAffected != 1 {
			return reject()
		}
	}
	frame, err := p11GrabFrame(tx, *slot)
	if err != nil {
		return nil, err
	}
	response := proto.Clone(start).(*pb.SCGrab)
	response.WinPlayerId = proto.Int64(winner)
	response.GrabData = frame
	response.CurrEnergy = proto.Int32(row.AfterEnergy)
	response.GrabTimes = proto.Int32(row.AfterRemaining)
	row.Response, err = proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		return nil, err
	}
	row.ResponseHash = p6Digest(row.Response)
	row.Terminal, err = json.Marshal(terminal)
	if err != nil {
		return nil, err
	}
	row.TerminalHash = p6Digest(row.Terminal)
	if err = p11StoreGrabFightP6(tx, &record, input, terminal, now); err != nil {
		return nil, err
	}
	if err = tx.Create(&row).Error; err != nil {
		return nil, err
	}
	if s.P1Ready {
		source := constUtil.P11GrabFightSource + ":" + row.FightID
		if err = s.RecordP1Cost(tx, own.Id, source, []*pb.BaseResource{{Id: proto.String("energy"), Num: proto.Int32(rules.Grab.Energy)}}, now); err != nil {
			return nil, err
		}
		if err = s.RecordP1Event(tx, own.Id, source, constUtil.P11GrabFightAcceptedEvent, 1, now); err != nil {
			return nil, err
		}
		if row.Won {
			if err = s.RecordP1Event(tx, own.Id, source+":win", constUtil.P11GrabFightWonEvent, 1, now); err != nil {
				return nil, err
			}
		}
	}
	return response, nil
}

func p11StoreGrabFightP6(tx *gorm.DB, record *db.P6BattleRecord, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, now time.Time) error {
	if record == nil || record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P11GrabFightSource || terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownGrabSettlementProof(input, terminal.Outcome.CapabilityHash) || validateP6GrabTerminal(input, terminal) != nil {
		return errors.New(constUtil.P11GrabFightConflict)
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return err
	}
	body, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return err
	}
	result := p6PlaybackResult(terminal.Outcome.Winner)
	saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=?", record.ID, constUtil.P6BattleCaptured).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": body, "outcome_hash": p6Digest(body), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
	if saved.Error != nil {
		return saved.Error
	}
	if saved.RowsAffected != 1 {
		return errors.New(constUtil.P11GrabFightConflict)
	}
	return nil
}

// p11ReadGrabFightReceipt checks immutable ownership/session/cost/P6 facts.
// It never recalculates a historical fight or reads a live operator policy/slot.
func (s *GameService) p11ReadGrabFightReceipt(tx *gorm.DB, row *db.P11GrabFightSettlement) (*pb.SCGrab, error) {
	reject := func() (*pb.SCGrab, error) { return nil, errors.New(constUtil.P11GrabFightConflict) }
	if tx == nil || row == nil || row.PlayerID <= 0 || row.DefenderID <= 0 || row.PlayerID == row.DefenderID || row.Position < 1 || row.Position > 18 || row.Stamp <= 0 || row.FightID == "" || row.P6RecordID == "" || row.PreviousSessionID == "" || row.PreviousStartedAt <= 0 || row.PreviousStartedAt > row.CreatedAt || row.PreviousExpiresAt <= row.CreatedAt || row.BeforeEnergy < 10 || row.AfterEnergy != row.BeforeEnergy-10 || row.BeforeUsed < 0 || row.AfterUsed != row.BeforeUsed+1 || row.BeforeRemaining < 1 || row.AfterRemaining != row.BeforeRemaining-1 || row.BeforeRemaining+row.BeforeUsed < 3 || row.BeforeRemaining+row.BeforeUsed > 6 || row.StartHash != p6Digest(row.StartPB) || row.ResponseHash != p6Digest(row.Response) || row.TerminalHash != p6Digest(row.Terminal) {
		return reject()
	}
	if row.Day != time.UnixMilli(row.CreatedAt).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02") {
		return reject()
	}
	requestPB, err := proto.MarshalOptions{Deterministic: true}.Marshal(&pb.CSGrab{Index: proto.Int32(int32(row.Position))})
	if err != nil || p6Digest(requestPB) != row.RequestHash {
		return reject()
	}
	var offer db.P11GrabFightOffer
	if err = tx.First(&offer, "version=?", row.OfferVersion).Error; err != nil {
		return nil, err
	}
	var rules p11GrabFightRules
	if offer.SourceHash != row.OfferHash || offer.SourceHash != p6Digest([]byte(offer.Snapshot)) || json.Unmarshal([]byte(offer.Snapshot), &rules) != nil || !p11GrabFightRulesValid(&rules) || rules.Policy.Version != row.OfferVersion || offer.CapturedAt < rules.Policy.StartsAt || offer.CapturedAt > row.CreatedAt || row.CreatedAt < rules.Policy.StartsAt || row.CreatedAt >= rules.Policy.EndsAt || row.CreatedAt > math.MaxInt64-rules.Grab.MaxMillis {
		return reject()
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	start, response := &pb.SCGrab{}, &pb.SCGrab{}
	var terminal model.CombatFinalHPOutcome
	if proto.Unmarshal(row.StartPB, start) != nil || proto.Unmarshal(row.Response, response) != nil || json.Unmarshal(row.Terminal, &terminal) != nil || terminal.Outcome == nil || start.WinPlayerId != nil || start.GrabData != nil || start.CurrEnergy != nil || start.GrabTimes != nil || start.GetFightId() != row.FightID || start.GetRandSeed() != input.Seed || start.GetMaxRoundNum() != 40 || !proto.Equal(start.AttPlayer, input.Attacker) || !proto.Equal(start.DefPlayer, input.Defender) || !reflect.DeepEqual(input.AppliedProperties, p11GrabFightProperties(row)) {
		return reject()
	}
	if record.PlayerID != row.PlayerID || record.Source != constUtil.P11GrabFightSource || record.SourceFight != row.FightID || record.Status != constUtil.P6BattleComputed || !record.Authoritative || input.Attacker.GetPlayerId() != row.PlayerID || input.Defender.GetPlayerId() != row.DefenderID || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != row.ScopeProof || !p6KnownGrabSettlementProof(input, row.ScopeProof) || validateP6GrabTerminal(input, &terminal) != nil || terminal.EngineVersion != record.EngineVersion || terminal.ClientVersion != record.ClientVersion || terminal.ConfigurationHash != record.ConfigurationHash {
		return reject()
	}
	body, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return nil, err
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return nil, err
	}
	if record.OutcomeHash != p6Digest(body) || record.OutcomeHash != p6Digest(record.Outcome) || !bytes.Equal(operations, record.Operates) || record.ClientResult != record.ServerResult || record.ServerResult != p6PlaybackResult(terminal.Outcome.Winner) || record.FinishedAt != row.CreatedAt || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) || row.Won != (terminal.Outcome.Winner == 0) {
		return reject()
	}
	var previous db.P11GrabSession
	if err = tx.First(&previous, "id=?", row.PreviousSessionID).Error; err != nil {
		return nil, err
	}
	if previous.PlayerID != row.DefenderID || previous.Position != row.Position || previous.StartedAt != row.PreviousStartedAt || previous.ExpiresAt != row.PreviousExpiresAt || previous.EndedAt != 0 && (previous.EndedAt < row.CreatedAt || previous.EndedAt > previous.ExpiresAt) {
		return reject()
	}
	winner, startAt := row.DefenderID, row.PreviousStartedAt
	winnerName := input.Defender.GetNickName()
	if row.Won {
		if row.NewSessionID == "" || row.NewSessionID == row.PreviousSessionID || previous.EndedAt != row.CreatedAt || previous.EndReason != constUtil.P11GrabFightSource {
			return reject()
		}
		var created db.P11GrabSession
		if err = tx.First(&created, "id=?", row.NewSessionID).Error; err != nil {
			return nil, err
		}
		if created.PlayerID != row.PlayerID || created.Position != row.Position || created.StartedAt != row.CreatedAt || created.ExpiresAt != row.CreatedAt+rules.Grab.MaxMillis || created.EndedAt != 0 && (created.EndedAt < row.CreatedAt || created.EndedAt > created.ExpiresAt) {
			return reject()
		}
		winner, startAt = row.PlayerID, row.CreatedAt
		winnerName = input.Attacker.GetNickName()
	} else if row.NewSessionID != "" {
		return reject()
	}
	frame := response.GetGrabData()
	if response.GetFightId() != row.FightID || response.GetRandSeed() != input.Seed || response.GetMaxRoundNum() != 40 || response.GetWinPlayerId() != winner || !proto.Equal(response.AttPlayer, input.Attacker) || !proto.Equal(response.DefPlayer, input.Defender) || response.GetCurrEnergy() != row.AfterEnergy || response.GetGrabTimes() != row.AfterRemaining || frame == nil || frame.GetPosition() != int32(row.Position) || frame.GetPlayerId() != winner || frame.GetGrabTime() != startAt || frame.GetPlayerNam() != winnerName || !frame.GetIsGrab() {
		return reject()
	}
	return response, nil
}
