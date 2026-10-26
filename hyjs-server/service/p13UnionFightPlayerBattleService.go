package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

type p13UFPlayerBattleRules struct {
	Policy                 db.P13UnionFightPlayerBattlePolicy `json:"policy"`
	MatchID                string                             `json:"match_id"`
	MatchSHA               string                             `json:"match_sha"`
	Engine                 string                             `json:"engine"`
	Client                 string                             `json:"client"`
	Configuration          string                             `json:"configuration"`
	CapabilityProof        string                             `json:"capability_proof"`
	MaxRounds              int32                              `json:"max_rounds"`
	AttackerFirst          bool                               `json:"attacker_first"`
	AttackerFactor         float64                            `json:"attacker_factor"`
	DefenderFactor         float64                            `json:"defender_factor"`
	FullHPOnly             bool                               `json:"full_hp_only"`
	AttackCost             int64                              `json:"attack_cost"`
	DurationMillis         int64                              `json:"duration_millis"`
	KillScore              float64                            `json:"kill_score"`
	KillToken              float64                            `json:"kill_token"`
	ArriveRange            float64                            `json:"arrive_range"`
	ReviveSeconds          []int64                            `json:"revive_seconds"`
	PartialCapabilityProof string                             `json:"partial_capability_proof,omitempty"`
}

type p13UFPlayerBattleSide struct {
	PlayerID   int64  `json:"player_id"`
	GuildID    string `json:"guild_id"`
	Camp       int64  `json:"camp"`
	BattleHash string `json:"battle_hash"`
	Frame      []byte `json:"frame"`
}

func (s *GameService) InitializeP13UnionFightPlayerBattle(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightPlayerBattlePolicy{}, &db.P13UnionFightPlayerBattleOffer{}, &db.P13UnionFightPlayerBattleAttempt{}); err != nil {
		return err
	}
	if err := tx.Where("id=?", 1).FirstOrCreate(&db.P13UnionFightPlayerBattlePolicy{ID: 1, Version: "ordinary-player14-factor1-v1"}).Error; err != nil {
		return err
	}
	// New policy defaults off; old offers and all accepted receipts retain ID1.
	if err := tx.Where("id=?", constUtil.P13UFPlayerPartialPolicyID).FirstOrCreate(&db.P13UnionFightPlayerBattlePolicy{ID: constUtil.P13UFPlayerPartialPolicyID, Version: constUtil.P13UFPlayerPartialPolicyVersion}).Error; err != nil {
		return err
	}
	return tx.Where("id=?", constUtil.P13UFPlayerPartyPolicyID).FirstOrCreate(&db.P13UnionFightPlayerBattlePolicy{ID: constUtil.P13UFPlayerPartyPolicyID, Version: constUtil.P13UFPlayerPartyPolicyVersion}).Error
}

func p13UFPlayerBattleHash(row *db.P13UnionFightPlayerBattleAttempt) string {
	copied := *row
	copied.SourceHash = ""
	raw, _ := json.Marshal(copied)
	return p6Digest(raw)
}

func p13UFPlayerBattleRulesValid(r *p13UFPlayerBattleRules) bool {
	if r == nil {
		return false
	}
	// Validate operator-controlled values against the published admin bounds.
	if len(r.ReviveSeconds) < 1 || len(r.ReviveSeconds) > 64 {
		return false
	}
	for _, seconds := range r.ReviveSeconds {
		if seconds < 1 || seconds > 86400 {
			return false
		}
	}
	p := r.Policy
	legacy := (p.ID == 1 && r.FullHPOnly && r.PartialCapabilityProof == "" || p.ID == constUtil.P13UFPlayerPartialPolicyID && !r.FullHPOnly && r.PartialCapabilityProof != "" && r.PartialCapabilityProof == constUtil.P6UFPartialPlayerNativeProof) && r.CapabilityProof == constUtil.P6UFPlayerNativeProof && r.CapabilityProof != ""
	party := p.ID == constUtil.P13UFPlayerPartyPolicyID && p.Version == constUtil.P13UFPlayerPartyPolicyVersion && !r.FullHPOnly && r.PartialCapabilityProof == "" && r.CapabilityProof != "" && r.CapabilityProof == constUtil.P6UFPartyPlayerNativeProof
	return p.Enabled && (legacy || party) && strings.TrimSpace(p.Version) != "" && strings.TrimSpace(p.Source) != "" && p.PublishedAt > 0 && p.StartsAt >= p.PublishedAt && p.EndsAt > p.StartsAt && p.ReviveRule == constUtil.P13UFPlayerBattleReviveRule && r.MatchID != "" && r.MatchSHA != "" && r.Engine == constUtil.P6EngineVersion && r.Client == constUtil.P6OriginalClientHash && r.Configuration != "" && r.MaxRounds == 40 && r.AttackerFirst && r.AttackerFactor == 1 && r.DefenderFactor == 1 && r.AttackCost == 0 && r.DurationMillis >= 100 && r.DurationMillis <= 3600000 && p13UFGuildFiniteRange(r.KillScore, 0, 1e9) && p13UFGuildFiniteRange(r.KillToken, 0, 1e9) && p13UFGuildFiniteRange(r.ArriveRange, .1, 100)
}

func p13UFGuildFiniteRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

// Freeze only while a new field is being built, before any participant enters.
// The explicit local ordinal rule supplies the unknown private-server lookup;
// no attack currency fee or automatic healing is introduced.
func (s *GameService) p13UFPlayerBattleBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var existing db.P13UnionFightPlayerBattleOffer
	if err := tx.First(&existing, "match_id=?", match.ID).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var policy db.P13UnionFightPlayerBattlePolicy
	if err := tx.Where("id IN ?", []int64{1, constUtil.P13UFPlayerPartialPolicyID, constUtil.P13UFPlayerPartyPolicyID}).Order("enabled DESC").Order("id DESC").First(&policy).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	if policy.ID == constUtil.P13UFPlayerPartyPolicyID && constUtil.P6UFPartyPlayerNativeProof == "" {
		return nil
	}
	var count int64
	if err := tx.Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 || policy.PublishedAt > match.CreatedAt || match.CreatedAt > now.UnixMilli() || policy.StartsAt > match.StartAt || policy.EndsAt <= match.EndAt {
		return errors.New(constUtil.P13UFPlayerBattleRefusal)
	}
	read := func(key string) (float64, error) {
		return strconv.ParseFloat(original.table("GameConfigure", "StaticName", key)["NameValue"], 64)
	}
	duration, err := read("UnionFightDuration")
	if err != nil {
		return err
	}
	score, err := read("UnionFightKillPlayerRewardScore")
	if err != nil {
		return err
	}
	token, err := read("UnionFightKillPlayerRewardOffer")
	if err != nil {
		return err
	}
	distance, err := read("UnionFightArriveOther")
	if err != nil {
		return err
	}
	rules := p13UFPlayerBattleRules{Policy: policy, MatchID: match.ID, MatchSHA: match.SnapshotHash, Engine: match.EngineVersion, Client: match.ClientVersion, Configuration: match.ConfigurationHash, CapabilityProof: constUtil.P6UFPlayerNativeProof, MaxRounds: 40, AttackerFirst: true, AttackerFactor: 1, DefenderFactor: 1, FullHPOnly: true, AttackCost: 0, DurationMillis: int64(duration * 1000), KillScore: score, KillToken: token, ArriveRange: distance}
	if policy.ID == constUtil.P13UFPlayerPartialPolicyID {
		rules.FullHPOnly = false
		rules.PartialCapabilityProof = constUtil.P6UFPartialPlayerNativeProof
	}
	if policy.ID == constUtil.P13UFPlayerPartyPolicyID {
		rules.FullHPOnly = false
		rules.CapabilityProof = constUtil.P6UFPartyPlayerNativeProof
		rules.PartialCapabilityProof = ""
	}
	for _, part := range strings.Split(original.table("GameConfigure", "StaticName", "UnionFightReviveTime")["NameValue"], ",") {
		v, e := strconv.ParseInt(part, 10, 64)
		if e != nil {
			return e
		}
		rules.ReviveSeconds = append(rules.ReviveSeconds, v)
	}
	if !p13UFPlayerBattleRulesValid(&rules) {
		return errors.New(constUtil.P13UFPlayerBattleRefusal)
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	return tx.Create(&db.P13UnionFightPlayerBattleOffer{MatchID: match.ID, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}).Error
}

func p13UFPlayerBattleReadRules(tx *gorm.DB, row *db.P13UnionFightPlayerBattleAttempt) (*p13UFPlayerBattleRules, error) {
	var offer db.P13UnionFightPlayerBattleOffer
	if err := tx.First(&offer, "match_id=?", row.MatchID).Error; err != nil {
		return nil, err
	}
	return p13UFPlayerBattleRulesFromOffer(&offer, row)
}

func p13UFPlayerBattleRulesFromOffer(offer *db.P13UnionFightPlayerBattleOffer, row *db.P13UnionFightPlayerBattleAttempt) (*p13UFPlayerBattleRules, error) {
	var rules p13UFPlayerBattleRules
	if p6Digest([]byte(offer.Snapshot)) != offer.SourceHash || json.Unmarshal([]byte(offer.Snapshot), &rules) != nil || !p13UFPlayerBattleRulesValid(&rules) {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	return p13UFPlayerBattleRulesForRow(offer, row, &rules)
}

// Only consumes rules decoded and hash-authenticated from this exact offer in
// the same call; persisted/external callers retain RulesFromOffer above.
func p13UFPlayerBattleRulesForRow(offer *db.P13UnionFightPlayerBattleOffer, row *db.P13UnionFightPlayerBattleAttempt, rules *p13UFPlayerBattleRules) (*p13UFPlayerBattleRules, error) {
	if offer.SourceHash != row.OfferHash || rules.MatchID != row.MatchID || row.CreatedAt <= 0 || row.CreatedAt > math.MaxInt64-rules.DurationMillis || row.DueAt <= row.CreatedAt || offer.CapturedAt > row.CreatedAt || row.CreatedAt < rules.Policy.StartsAt || row.DueAt >= rules.Policy.EndsAt || row.DueAt != row.CreatedAt+rules.DurationMillis {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	return rules, nil
}

// The source uses the participant's original entry-bound Battle and exact pose
// HP/frame. New initiative1/0 is a published local response parameter; stored
// old00 entry/audit bytes remain untouched.
func p13UFPlayerBattleSource(actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, battle *pb.BattlePlayer, first int32) (*pb.BattlePlayer, error) {
	if actor == nil || frame == nil || battle == nil || actor.BattleHash != p6Digest(actor.Battle) || actor.FrameHash != p6Digest(actor.Frame) || battle.GetPlayerId() != actor.PlayerID || frame.GetGuid() != actor.PlayerID || frame.FightInfo == nil || frame.FightInfo.GetGuid() != actor.PlayerID || frame.GetCamp() != int32(actor.Camp) || battle.Inst == nil || frame.FightInfo.GetAnger() != battle.Inst.GetAnger() {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	if frame.GetKillNum() < 0 || frame.GetKillNum() == math.MaxInt32 || frame.GetDeadNum() < 0 || frame.GetDeadNum() == math.MaxInt32 || frame.GetOwnScore() < 0 || frame.GetOwnScore() > 1e15-5 || frame.GetOwnToken() < 0 || frame.GetOwnToken() > 1e15-5 || math.IsNaN(frame.GetOwnScore()) || math.IsInf(frame.GetOwnScore(), 0) || math.IsNaN(frame.GetOwnToken()) || math.IsInf(frame.GetOwnToken(), 0) {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	fight := frame.FightInfo
	if len(fight.Index) != len(battle.Formation) || len(fight.HpPos) != len(fight.Index) || len(fight.MaxHpPos) != len(fight.Index) {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	slots := map[int32]int{}
	for i, pose := range fight.Index {
		if _, ok := slots[pose]; ok {
			return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		slots[pose] = i
	}
	out := proto.Clone(battle).(*pb.BattlePlayer)
	for _, hero := range out.Formation {
		if hero == nil || hero.Prop == nil || hero.Inst == nil {
			return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		i, ok := slots[hero.GetPose()]
		if !ok || fight.HpPos[i] != fight.MaxHpPos[i] || fight.MaxHpPos[i] != hero.Prop.GetHp() || hero.Inst.GetCurHp() != fight.HpPos[i] {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
	}
	out.Inst.FirstOrder = proto.Int32(first)
	return out, nil
}

func p13UFPlayerBattleProperties(row *db.P13UnionFightPlayerBattleAttempt, before []p13UFPlayerBattleSide, typed *pb.SCUFFightRecordReplay) map[string][]string {
	applied := map[string][]string{}
	_ = json.Unmarshal(row.AppliedProperties, &applied)
	for i, side := range before {
		for _, hero := range typed.PlayerList[i].Formation {
			applied[hero.GetGuid()] = append(applied[hero.GetGuid()], fmt.Sprintf("%s;match=%s;attempt=%s;offer=%s;player=%d;camp=%d;battle=%s;frame=%s", constUtil.P13UFPlayerBattleSource, row.MatchID, row.ID, row.OfferHash, side.PlayerID, side.Camp, side.BattleHash, p6Digest(side.Frame)))
		}
	}
	return applied
}

// p13UFTryPlayerBattle is called before the legacy audit selector. Old audit
// receipts retain their route; absent/disabled new offers fall through untouched.
func (s *GameService) p13UFTryPlayerBattle(ctx context.Context, id int64, request *pb.CSUFPlayerAskFight, stamp int64, now time.Time) ([]proto.Message, bool, error) {
	if request == nil || id <= 0 || stamp < 0 || request.GetTargetGuid() <= 0 || request.GetTargetGuid() == id {
		return nil, true, errors.New(constUtil.P13UFPlayerBattleRefusal)
	}
	var output []proto.Message
	handled, fresh := false, false
	scope := ""
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var guildPolicy db.P13GuildPolicy
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guildPolicy, 1).Error; e != nil {
			return e
		}
		var old db.P13UnionFightPlayerBattleAttempt
		if e := tx.Where("? > 0", stamp).First(&old, "player_id=? AND client_stamp=?", id, stamp).Error; e == nil {
			handled = true
			if old.TargetID != request.GetTargetGuid() {
				return errors.New(constUtil.P13UFPlayerBattleConflict)
			}
			var e error
			output, e = s.p13UFPlayerBattleReceipt(tx, &old, id)
			return e
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		for _, table := range []any{&db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightGuardAuditAttempt{}} {
			var n int64
			if e := tx.Model(table).Where("player_id=? AND stamp=?", id, stamp).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return nil
			}
		}
		var live db.P13UnionFightPlayerBattlePolicy
		if e := tx.Where("id IN ?", []int64{1, constUtil.P13UFPlayerPartialPolicyID, constUtil.P13UFPlayerPartyPolicyID}).Order("enabled DESC").Order("id DESC").First(&live).Error; errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		} else if e != nil {
			return e
		}
		if !live.Enabled {
			return nil
		}
		// Global P13 policy lock precedes sorted profile and participant locks, in
		// the same order as existing field/item transactions.
		ids := []int64{id, request.GetTargetGuid()}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		games := map[int64]*model.GameState{}
		for _, pid := range ids {
			var profile db.Profile
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, pid).Error; errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			} else if e != nil {
				return e
			}
			_, game, e := decodeProfileForCalendarUpdate(tx, &profile)
			if e != nil {
				return e
			}
			if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
				return errors.New(constUtil.P13UFPlayerBattleRefusal)
			}
			games[pid] = game
		}
		actor, match, _, frame, e := s.p13UFFieldCurrent(tx, id, now)
		if e != nil {
			return e
		}
		var offer db.P13UnionFightPlayerBattleOffer
		if e = tx.First(&offer, "match_id=?", match.ID).Error; errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		} else if e != nil {
			return e
		}
		// A target outside real field players may be an old guard; it keeps audit routing.
		var targetExists int64
		if e = tx.Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND player_id=? AND left_at=0", match.ID, request.GetTargetGuid()).Count(&targetExists).Error; e != nil {
			return e
		}
		if targetExists == 0 {
			return nil
		}
		handled = true
		scope = match.ID
		target, targetMatch, _, targetFrame, e := s.p13UFFieldCurrent(tx, request.GetTargetGuid(), now)
		if e != nil {
			return e
		}
		// Original socket frames have no client timestamp. Repeated requests while
		// the same real pair is pending recover that pending calculation only.
		if stamp == 0 {
			var active db.P13UnionFightPlayerBattleAttempt
			if e = tx.First(&active, "match_id=? AND player_id=? AND target_id=? AND status=?", match.ID, id, target.PlayerID, constUtil.P13UFPlayerBattlePending).Error; e == nil {
				output, e = s.p13UFPlayerBattleReceipt(tx, &active, id)
				return e
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		// Decode the field's immutable offer before checking distance and scheduling.
		var frozenRules p13UFPlayerBattleRules
		if p6Digest([]byte(offer.Snapshot)) != offer.SourceHash || json.Unmarshal([]byte(offer.Snapshot), &frozenRules) != nil || !p13UFPlayerBattleRulesValid(&frozenRules) || frozenRules.MatchID != match.ID || now.UnixMilli() <= 0 || now.UnixMilli() > math.MaxInt64-frozenRules.DurationMillis {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		if actor.MatchID != target.MatchID || match.ID != targetMatch.ID || actor.GuildID == target.GuildID || actor.Camp == target.Camp || actor.Hooked || target.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || (targetFrame.GetState() != 0 && targetFrame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli()+frozenRules.DurationMillis > match.EndAt || offer.CapturedAt > actor.JoinedAt || offer.CapturedAt > target.JoinedAt || p13UFResourceDistance(actor, target.X, target.Y, target.Z) > frozenRules.ArriveRange {
			return errors.New(constUtil.P13UFPlayerBattleRefusal)
		}
		for _, current := range []*pb.InstUFPlayer{frame, targetFrame} {
			if e = p13UFTowerPendingLife(tx, match.ID, current.GetGuid(), current.GetDeadNum()); e != nil {
				return e
			}
		}
		var busy int64
		if e = tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("status=? AND (player_id IN ? OR target_id IN ?)", constUtil.P13UFPlayerBattlePending, ids, ids).Count(&busy).Error; e != nil {
			return e
		}
		if busy > 0 {
			return errors.New(constUtil.P13UFPlayerBattleRefusal)
		}
		for _, pid := range ids {
			var locked db.P13UnionFightFieldPlayer
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "match_id=? AND player_id=?", match.ID, pid).Error; e != nil {
				return e
			}
			source := actor
			if pid == target.PlayerID {
				source = target
			}
			if locked.FrameHash != source.FrameHash || locked.BattleHash != source.BattleHash || locked.GuildID != source.GuildID || locked.Camp != source.Camp || locked.SessionHash != source.SessionHash || locked.LeftAt != source.LeftAt || locked.Hooked != source.Hooked || locked.MapProjection != source.MapProjection || locked.X != source.X || locked.Y != source.Y || locked.Z != source.Z {
				return errors.New(constUtil.P13UFPlayerBattleConflict)
			}
			*source = locked
		}
		internalStamp := stamp
		if internalStamp == 0 {
			number, randomErr := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
			if randomErr != nil {
				return randomErr
			}
			internalStamp = number.Int64() + 1
		}
		row := db.P13UnionFightPlayerBattleAttempt{ID: p6Digest([]byte(fmt.Sprintf("%s/%d/%d", match.ID, id, internalStamp))), MatchID: match.ID, PlayerID: id, TargetID: request.GetTargetGuid(), Stamp: internalStamp, ClientStamp: stamp, Status: constUtil.P13UFPlayerBattlePending, OfferHash: offer.SourceHash, CreatedAt: now.UnixMilli(), DueAt: now.UnixMilli() + frozenRules.DurationMillis}
		row.FightID = strconv.FormatInt(p13UFFieldGUID(match.ID, "ordinary-player14", row.ID), 10)
		req, _ := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		row.RequestHash = p6Digest(req)
		rules, e := p13UFPlayerBattleRulesForRow(&offer, &row, &frozenRules)
		if e != nil {
			return e
		}
		if live.ID != rules.Policy.ID || live.Version != rules.Policy.Version || live.Source != rules.Policy.Source || live.PublishedAt != rules.Policy.PublishedAt || match.SnapshotHash != rules.MatchSHA || match.EngineVersion != rules.Engine || match.ClientVersion != rules.Client || match.ConfigurationHash != rules.Configuration {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		attacker, applied, e := s.p13UFFieldBattleLoaded(tx, actor, match, frame, games[id], now)
		if e != nil {
			return e
		}
		defender, targetApplied, e := s.p13UFFieldBattleLoaded(tx, target, targetMatch, targetFrame, games[target.PlayerID], now)
		if e != nil {
			return e
		}
		for guid, sources := range targetApplied {
			if _, exists := applied[guid]; exists {
				return errors.New(constUtil.P13UFPlayerBattleConflict)
			}
			applied[guid] = sources
		}
		attacker, e = p13UFPlayerBattleSourceForRules(actor, frame, attacker, 1, rules)
		if e != nil {
			return e
		}
		defender, e = p13UFPlayerBattleSourceForRules(target, targetFrame, defender, 0, rules)
		if e != nil {
			return e
		}
		before := []p13UFPlayerBattleSide{{PlayerID: id, GuildID: actor.GuildID, Camp: actor.Camp, BattleHash: actor.BattleHash}, {PlayerID: target.PlayerID, GuildID: target.GuildID, Camp: target.Camp, BattleHash: target.BattleHash}}
		before[0].Frame, _ = proto.Marshal(frame)
		before[1].Frame, _ = proto.Marshal(targetFrame)
		row.Before, e = json.Marshal(before)
		if e != nil {
			return e
		}
		number, e := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
		if e != nil {
			return e
		}
		seed := number.Int64() + 1
		typed := &pb.SCUFFightRecordReplay{FightId: proto.String(row.FightID), RoomId: proto.String("map_forest_002"), RandomSeed: proto.Int64(seed), PlayerList: []*pb.BattlePlayer{attacker, defender}, MaxRound: proto.Int32(40), AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)}
		original, e := s.p13UFBattleAuditFrozenEngine(tx, match, applied)
		if e != nil {
			return e
		}
		row.AppliedProperties, e = json.Marshal(applied)
		if e != nil {
			return e
		}
		row.StartPB, e = proto.MarshalOptions{Deterministic: true}.Marshal(typed)
		if e != nil {
			return e
		}
		row.StartHash = p6Digest(row.StartPB)
		properties := p13UFPlayerBattleProperties(&row, before, typed)
		var frozen *model.FrozenBattleInput
		if rules.Policy.ID == constUtil.P13UFPlayerPartyPolicyID {
			fresh, err := original.newP6UFPartyPlayerAdmission(ctx, typed, properties)
			if err != nil {
				return err
			}
			row.P6RecordID, row.InputHash, frozen, e = original.captureP6UFPartyPlayerAdmission(ctx, tx, id, row.FightID, fresh)
			if e != nil {
				return e
			}
		} else {
			input, err := original.FreezeP6UFPlayerInput(typed)
			if err != nil {
				return err
			}
			input.AppliedProperties = properties
			row.P6RecordID, e = original.CaptureBattleReplay(ctx, tx, id, constUtil.P13UFPlayerBattleSource, row.FightID, *input)
			if e != nil {
				return e
			}
			var record db.P6BattleRecord
			if e = tx.First(&record, "id=?", row.P6RecordID).Error; e != nil {
				return e
			}
			frozen, e = decodeP6Record(&record)
			if e != nil {
				return e
			}
			row.InputHash = record.InputHash
		}
		// The exact durable input still passes policy scope before TX commit/Begin.
		if e = p13UFPlayerBattleScope(frozen, rules); e != nil {
			return e
		}
		row.CalculationStage = constUtil.P13UFPlayerBattleCaptured
		row.Terminal = []byte{}
		frame.State, targetFrame.State = proto.Int32(1), proto.Int32(1)
		frame.LeftTime, targetFrame.LeftTime = proto.Float32(float32(rules.DurationMillis)/1000), proto.Float32(float32(rules.DurationMillis)/1000)
		frame.TotalTime, targetFrame.TotalTime = proto.Float32(float32(rules.DurationMillis)/1000), proto.Float32(float32(rules.DurationMillis)/1000)
		if e = p13UFFieldSaveActor(tx, actor, frame); e != nil {
			return e
		}
		if e = p13UFFieldSaveActor(tx, target, targetFrame); e != nil {
			return e
		}
		row.SourceHash = p13UFPlayerBattleHash(&row)
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		// The actual intent also excludes this match from untouched zero-fight settlement.
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightSettlementIntent{MatchID: match.ID, PlayerID: id, TargetID: target.PlayerID, Stamp: stamp, ObservedAt: now.UnixMilli(), Source: constUtil.P13UFPlayerBattleSource}).Error; e != nil {
			return e
		}
		output = []proto.Message{&pb.SCUFPlayerFightBegin{Guid: proto.Int64(id), TargetGuid: proto.Int64(target.PlayerID)}, &pb.SCUFPlayerFightBegin{Guid: proto.Int64(target.PlayerID), TargetGuid: proto.Int64(id)}}
		fresh = true
		return nil
	})
	if err == nil && fresh {
		s.p13UFFieldBroadcastBatch(ctx, scope, id, output)
	}
	return output, handled, err
}

func p13UFPlayerBattleAcceptP6(tx *gorm.DB, record *db.P6BattleRecord, input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, rules *p13UFPlayerBattleRules, now time.Time) error {
	if record == nil || record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P13UFPlayerBattleSource || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || !p13UFPlayerBattleTerminalForRules(rules, input, out) {
		return errors.New(constUtil.CombatUnsupported)
	}
	body, e := json.Marshal(out.Outcome)
	if e != nil {
		return e
	}
	operations, e := proto.MarshalOptions{Deterministic: true}.Marshal(out.Outcome.Operates)
	if e != nil {
		return e
	}
	result := p6PlaybackResult(out.Outcome.Winner)
	saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=? AND input_hash=? AND source=? AND source_fight=?", record.ID, constUtil.P6BattleCaptured, record.InputHash, constUtil.P13UFPlayerBattleSource, record.SourceFight).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": body, "outcome_hash": p6Digest(body), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
	if saved.Error != nil {
		return saved.Error
	}
	if saved.RowsAffected != 1 {
		return errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	return nil
}

// Every terminal is mapped through the unchanged input GUID/pose, with whole
// player death counted once. A timeout winner alone never creates a kill.
func p13UFPlayerBattleApply(before []*pb.InstUFPlayer, input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, rules *p13UFPlayerBattleRules) ([]*pb.InstUFPlayer, int64, int64, int64, error) {
	reject := func() ([]*pb.InstUFPlayer, int64, int64, int64, error) {
		return nil, 0, 0, 0, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	if len(before) != 2 || before[0] == nil || before[1] == nil || !p13UFPlayerBattleRulesValid(rules) || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || !p13UFPlayerBattleTerminalForRules(rules, input, out) {
		return reject()
	}
	after := []*pb.InstUFPlayer{proto.Clone(before[0]).(*pb.InstUFPlayer), proto.Clone(before[1]).(*pb.InstUFPlayer)}
	dead := []bool{true, true}
	for camp, source := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		frame := after[camp]
		fight := frame.GetFightInfo()
		if frame.GetGuid() != source.GetPlayerId() || fight == nil || fight.GetAnger() != source.GetInst().GetAnger() || fight.GetGuid() != frame.GetGuid() || len(fight.Index) != len(source.Formation) || len(fight.HpPos) != len(fight.Index) || len(fight.MaxHpPos) != len(fight.Index) {
			return reject()
		}
		initial := map[string]*pb.BattleHero{}
		poses := map[int32]int{}
		initialAlive := false
		for _, hero := range source.Formation {
			if initial[hero.GetGuid()] != nil {
				return reject()
			}
			initial[hero.GetGuid()] = hero
		}
		for i, pose := range fight.Index {
			if _, seen := poses[pose]; seen || fight.MaxHpPos[i] <= 0 || !combatFinite(fight.HpPos[i]) || fight.HpPos[i] < 0 || fight.HpPos[i] == 0 && rules.Policy.ID != constUtil.P13UFPlayerPartyPolicyID || fight.HpPos[i] > fight.MaxHpPos[i] || rules.FullHPOnly && fight.HpPos[i] != fight.MaxHpPos[i] {
				return reject()
			}
			poses[pose] = i
			initialAlive = initialAlive || fight.HpPos[i] > 0
		}
		if !initialAlive {
			return reject()
		}
		seen := map[string]bool{}
		for _, actor := range out.FinalActors {
			if actor.Camp != int32(camp) {
				continue
			}
			hero := initial[actor.GUID]
			i, ok := poses[actor.Pose]
			if hero == nil || seen[actor.GUID] || !ok || hero.GetPose() != actor.Pose || fight.HpPos[i] != hero.GetInst().GetCurHp() || fight.MaxHpPos[i] != hero.GetProp().GetHp() || actor.BloodNumber != 1 || actor.CurrentHP < 0 || actor.MaxHP <= 0 || actor.CurrentHP > actor.MaxHP || math.IsNaN(actor.CurrentHP) || math.IsInf(actor.CurrentHP, 0) {
				return reject()
			}
			seen[actor.GUID] = true
			fight.HpPos[i] = actor.CurrentHP
			fight.MaxHpPos[i] = actor.MaxHP
			dead[camp] = dead[camp] && actor.CurrentHP == 0
		}
		if len(seen) != len(source.Formation) {
			return reject()
		}
		finalPlayer := out.Outcome.Attacker
		if camp == 1 {
			finalPlayer = out.Outcome.Defender
		}
		if finalPlayer == nil || finalPlayer.Inst == nil || math.IsNaN(finalPlayer.GetInst().GetAnger()) || math.IsInf(finalPlayer.GetInst().GetAnger(), 0) || finalPlayer.GetInst().GetAnger() < 0 {
			return reject()
		}
		fight.Anger = proto.Float64(finalPlayer.GetInst().GetAnger())
		frame.State = proto.Int32(0)
		frame.LeftTime = proto.Float32(0)
		frame.TotalTime = proto.Float32(0)
	}
	if dead[0] && dead[1] {
		return reject()
	}
	deadID, rewardID, revive := int64(0), int64(0), int64(0)
	for loser, isDead := range dead {
		if !isDead {
			continue
		}
		winner := 1 - loser
		if out.Outcome.Winner != int32(winner) || after[loser].GetDeadNum() < 0 || after[loser].GetDeadNum() == math.MaxInt32 || after[winner].GetKillNum() < 0 || after[winner].GetKillNum() == math.MaxInt32 {
			return reject()
		}
		victim, killer := after[loser], after[winner]
		victim.DeadNum = proto.Int32(victim.GetDeadNum() + 1)
		killer.KillNum = proto.Int32(killer.GetKillNum() + 1)
		// This explicitly published local ordinal rule supplies the unobserved server
		// index behavior. Existing real item012 remains the only implemented revival.
		revive = rules.ReviveSeconds[min(int(victim.GetDeadNum())-1, len(rules.ReviveSeconds)-1)]
		victim.State = proto.Int32(3)
		victim.LeftTime = proto.Float32(float32(revive))
		victim.TotalTime = proto.Float32(float32(revive))
		score, token := killer.GetOwnScore()+rules.KillScore, killer.GetOwnToken()+rules.KillToken
		if score < rules.KillScore || token < rules.KillToken || score > 1e15 || token > 1e15 || math.IsNaN(score) || math.IsInf(score, 0) || math.IsNaN(token) || math.IsInf(token, 0) {
			return reject()
		}
		killer.OwnScore = proto.Float64(score)
		killer.OwnToken = proto.Float64(token)
		deadID, rewardID = victim.GetGuid(), killer.GetGuid()
	}
	return after, deadID, rewardID, revive, nil
}

// p13UFPlayerBattleScope uses the same capability gate as calculation without
// running the combat engine or inventing a terminal during admission/retry.
func p13UFPlayerBattleScope(input *model.FrozenBattleInput, rules *p13UFPlayerBattleRules) error {
	if input == nil || !p13UFPlayerBattleRulesValid(rules) {
		return errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	if rules.Policy.ID == constUtil.P13UFPlayerPartyPolicyID {
		return validateP6UFPartyPlayerScope(input)
	}
	if !rules.FullHPOnly && validateP6UFPartialPlayerScope(input) == nil {
		return nil
	}
	return validateP6UFPlayerScope(input)
}

func (s *GameService) p13UFPlayerBattleVerify(tx *gorm.DB, row *db.P13UnionFightPlayerBattleAttempt) (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, []p13UFPlayerBattleSide, *p13UFPlayerBattleRules, error) {
	if row == nil {
		return nil, nil, nil, nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	var offer db.P13UnionFightPlayerBattleOffer
	if e := tx.First(&offer, "match_id=?", row.MatchID).Error; e != nil {
		return nil, nil, nil, nil, e
	}
	var record db.P6BattleRecord
	if e := tx.First(&record, "id=?", row.P6RecordID).Error; e != nil {
		return nil, nil, nil, nil, e
	}
	return s.p13UFPlayerBattleVerifyLoaded(tx.Statement.Context, row, &record, &offer)
}

// VerifyLoaded consumes one batch-loaded immutable record/offer, without SQL.
func (s *GameService) p13UFPlayerBattleVerifyLoaded(ctx context.Context, row *db.P13UnionFightPlayerBattleAttempt, record *db.P6BattleRecord, offer *db.P13UnionFightPlayerBattleOffer) (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, []p13UFPlayerBattleSide, *p13UFPlayerBattleRules, error) {
	reject := func() (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, []p13UFPlayerBattleSide, *p13UFPlayerBattleRules, error) {
		return nil, nil, nil, nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	if row == nil || record == nil || offer == nil || record.ID != row.P6RecordID || offer.MatchID != row.MatchID || row.NextCalculationAt < 0 || row.CalculationFailures < 0 || row.ID != p6Digest([]byte(fmt.Sprintf("%s/%d/%d", row.MatchID, row.PlayerID, row.Stamp))) || row.FightID != strconv.FormatInt(p13UFFieldGUID(row.MatchID, "ordinary-player14", row.ID), 10) || row.SourceHash != p13UFPlayerBattleHash(row) || row.PlayerID <= 0 || row.TargetID <= 0 || row.PlayerID == row.TargetID || row.Stamp <= 0 || row.ClientStamp < 0 || (row.ClientStamp > 0 && row.ClientStamp != row.Stamp) || row.CreatedAt <= 0 || (row.Status != constUtil.P13UFPlayerBattlePending && row.Status != constUtil.P13UFPlayerBattleSettled) || row.StartHash != p6Digest(row.StartPB) || (row.CalculationStage != constUtil.P13UFPlayerBattleCaptured && row.TerminalHash != p6Digest(row.Terminal)) {
		return reject()
	}
	request, _ := proto.MarshalOptions{Deterministic: true}.Marshal(&pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(row.TargetID)})
	if row.RequestHash != p6Digest(request) {
		return reject()
	}
	rules, e := p13UFPlayerBattleRulesFromOffer(offer, row)
	if e != nil {
		return nil, nil, nil, nil, e
	}
	var before []p13UFPlayerBattleSide
	if json.Unmarshal(row.Before, &before) != nil || len(before) != 2 || before[0].PlayerID != row.PlayerID || before[1].PlayerID != row.TargetID || before[0].GuildID == before[1].GuildID || before[0].Camp == before[1].Camp || before[0].Camp < 0 || before[0].Camp > 1 || before[1].Camp < 0 || before[1].Camp > 1 {
		return reject()
	}
	typed := &pb.SCUFFightRecordReplay{}
	if proto.Unmarshal(row.StartPB, typed) != nil || typed.WinPlayerId != nil || len(typed.ProtoReflect().GetUnknown()) != 0 || typed.GetFightId() != row.FightID || len(typed.PlayerList) != 2 || typed.GetRoomId() != "map_forest_002" || typed.GetMaxRound() != 40 || typed.AttackerDamageFactor == nil || typed.DefenderDamageFactor == nil || typed.GetAttackerDamageFactor() != 1 || typed.GetDefenderDamageFactor() != 1 {
		return reject()
	}
	input, e := decodeP6Record(record)
	if e != nil {
		return nil, nil, nil, nil, e
	}
	if record.PlayerID != row.PlayerID || record.Source != constUtil.P13UFPlayerBattleSource || record.SourceFight != row.FightID || record.InputHash != row.InputHash || record.EngineVersion != rules.Engine || record.ClientVersion != rules.Client || input.Seed != typed.GetRandomSeed() || input.Attacker.GetPlayerId() != row.PlayerID || input.Defender.GetPlayerId() != row.TargetID || !proto.Equal(input.Attacker, typed.PlayerList[0]) || !proto.Equal(input.Defender, typed.PlayerList[1]) {
		return reject()
	}
	var actualApplied map[string][]string
	if json.Unmarshal(row.AppliedProperties, &actualApplied) != nil || len(actualApplied) == 0 {
		return reject()
	}
	expected := p13UFPlayerBattleProperties(row, before, typed)
	left, _ := json.Marshal(input.AppliedProperties)
	right, _ := json.Marshal(expected)
	if !bytes.Equal(left, right) {
		return reject()
	}
	if e = p13UFPlayerBattleScope(input, rules); e != nil {
		return nil, nil, nil, nil, e
	}
	if rules.Policy.ID == constUtil.P13UFPlayerPartyPolicyID {
		if _, e = (&GameService{Tables: input.Tables}).ValidateP6UFPartyPlayerMechanisms(ctx, input); e != nil {
			return nil, nil, nil, nil, e
		}
	}
	if row.CalculationStage == constUtil.P13UFPlayerBattleCaptured {
		if row.Status != constUtil.P13UFPlayerBattlePending || record.Status != constUtil.P6BattleCaptured || record.Authoritative || record.FinishedAt != 0 || record.OutcomeHash != "" || record.ReplayHash != "" || record.ClientResult != 0 || record.ServerResult != 0 || len(record.Outcome) != 0 || len(record.Operates) != 0 || len(row.Terminal) != 0 || row.TerminalHash != "" || row.ScopeProof != "" || row.SettledAt != 0 || len(row.After) != 0 || row.DeadPlayerID != 0 || row.RewardPlayerID != 0 || row.ReviveSeconds != 0 {
			return reject()
		}
		return input, nil, before, rules, nil
	}
	if row.CalculationStage != "" && row.CalculationStage != constUtil.P13UFPlayerBattleComputed || record.Status != constUtil.P6BattleComputed || !record.Authoritative || record.FinishedAt != row.CreatedAt {
		return reject()
	}
	var out model.CombatFinalHPOutcome
	if json.Unmarshal(row.Terminal, &out) != nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != row.ScopeProof || !p13UFPlayerBattleTerminalForRules(rules, input, &out) || out.EngineVersion != record.EngineVersion || out.ClientVersion != record.ClientVersion || out.ConfigurationHash != record.ConfigurationHash {
		return reject()
	}
	body, e := json.Marshal(out.Outcome)
	if e != nil {
		return nil, nil, nil, nil, e
	}
	operations, e := proto.MarshalOptions{Deterministic: true}.Marshal(out.Outcome.Operates)
	if e != nil {
		return nil, nil, nil, nil, e
	}
	if record.OutcomeHash != p6Digest(body) || record.OutcomeHash != p6Digest(record.Outcome) || !bytes.Equal(record.Operates, operations) || record.ClientResult != record.ServerResult || record.ServerResult != p6PlaybackResult(out.Outcome.Winner) || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) {
		return reject()
	}
	if row.Status == constUtil.P13UFPlayerBattlePending && (row.SettledAt != 0 || len(row.After) != 0 || row.DeadPlayerID != 0 || row.RewardPlayerID != 0 || row.ReviveSeconds != 0) {
		return reject()
	}
	return input, &out, before, rules, nil
}

func p13UFPlayerBattleFrames(before []p13UFPlayerBattleSide) ([]*pb.InstUFPlayer, error) {
	frames := make([]*pb.InstUFPlayer, len(before))
	for i, side := range before {
		frames[i] = &pb.InstUFPlayer{}
		if proto.Unmarshal(side.Frame, frames[i]) != nil || frames[i].GetGuid() != side.PlayerID || frames[i].GetCamp() != int32(side.Camp) {
			return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
		}
	}
	return frames, nil
}

// Only engine failures after full immutable-input verification may re-sign
// authenticated retry metadata. Storage and source failures retain their bytes.
type p13UFPlayerBattleCalculationError struct{ err error }

func (e *p13UFPlayerBattleCalculationError) Error() string { return e.err.Error() }
func (e *p13UFPlayerBattleCalculationError) Unwrap() error { return e.err }

func p13UFPlayerBattleCaptureDamaged(err error) bool {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	switch err.Error() {
	case constUtil.P13UFPlayerBattleConflict, constUtil.P6ReplayConflict, constUtil.CombatInvalid, constUtil.CombatUnsupported:
		return true
	}
	return false
}

func (s *GameService) p13UFPlayerBattleQuarantine(ctx context.Context, captured *db.P13UnionFightPlayerBattleAttempt) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stop, err := p13UFCombatCommitGate(tx, captured.MatchID, p13UFNow(ctx))
		if err != nil || stop {
			return err
		}
		return tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("id=? AND source_hash=? AND status=? AND calculation_stage=?", captured.ID, captured.SourceHash, constUtil.P13UFPlayerBattlePending, constUtil.P13UFPlayerBattleCaptured).Update("calculation_quarantined", true).Error
	})
}

func (s *GameService) p13UFPlayerBattleRetryCalculation(ctx context.Context, captured *db.P13UnionFightPlayerBattleAttempt, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stop, err := p13UFCombatCommitGate(tx, captured.MatchID, p13UFNow(ctx))
		if err != nil || stop {
			return err
		}
		var current db.P13UnionFightPlayerBattleAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", captured.ID).Error; err != nil {
			return err
		}
		if current.SourceHash != captured.SourceHash || current.CalculationStage != constUtil.P13UFPlayerBattleCaptured || current.CalculationQuarantined {
			return nil
		}
		if _, _, _, _, err := s.p13UFPlayerBattleVerify(tx, &current); err != nil {
			return err
		}
		delay := min(int64(1)<<min(current.CalculationFailures, int64(5)), int64(constUtil.P13UFPlayerBattleRetryMaxSeconds)) * 1000
		if now.UnixMilli() < 0 || now.UnixMilli() > math.MaxInt64-delay {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		current.NextCalculationAt = now.UnixMilli() + delay
		if current.CalculationFailures < math.MaxInt64 {
			current.CalculationFailures++
		}
		current.SourceHash = p13UFPlayerBattleHash(&current)
		updated := tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("id=? AND source_hash=? AND calculation_stage=? AND status=? AND calculation_quarantined=?", current.ID, captured.SourceHash, constUtil.P13UFPlayerBattleCaptured, constUtil.P13UFPlayerBattlePending, false).Updates(map[string]any{"next_calculation_at": current.NextCalculationAt, "calculation_failures": current.CalculationFailures, "source_hash": current.SourceHash})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		return nil
	})
}

// computeP13UnionFightPlayerBattles reads durable captures without guild or
// profile locks. Calculation may be cancelled; only the short CAS transaction
// commits a certified terminal, so crashes retain a retryable captured input.
func (s *GameService) computeP13UnionFightPlayerBattles(ctx context.Context, now time.Time) error {
	return s.computeP13UnionFightPlayerBattlesWith(ctx, now, s.computeP13UnionFightPlayerBattle)
}

func (s *GameService) computeP13UnionFightPlayerBattlesWith(ctx context.Context, now time.Time, compute func(context.Context, *db.P13UnionFightPlayerBattleAttempt) error) error {
	var rows []db.P13UnionFightPlayerBattleAttempt
	tx := p13UFCombatOpenCandidates(db.GetIns().WithContext(ctx), "p13_union_fight_player_battle_attempts", now)
	if err := tx.Where("status=? AND calculation_stage=? AND calculation_quarantined=? AND next_calculation_at<=?", constUtil.P13UFPlayerBattlePending, constUtil.P13UFPlayerBattleCaptured, false, now.UnixMilli()).Order("next_calculation_at,created_at,id").Limit(constUtil.P13UFPlayerBattleComputeBatch).Find(&rows).Error; err != nil {
		return err
	}
	var failures []error
	for i := range rows {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		if err := compute(ctx, &rows[i]); err != nil {
			failures = append(failures, fmt.Errorf("ordinary14 calculate %s: %w", rows[i].ID, err))
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				continue
			}
			var calculation *p13UFPlayerBattleCalculationError
			if errors.As(err, &calculation) {
				if retryErr := s.p13UFPlayerBattleRetryCalculation(ctx, &rows[i], now); retryErr != nil {
					failures = append(failures, retryErr)
				}
			} else if p13UFPlayerBattleCaptureDamaged(err) {
				// Keep the damaged bytes/hash intact. This independent scheduler
				// flag prevents poison rows starving later accepted captures.
				if quarantineErr := s.p13UFPlayerBattleQuarantine(ctx, &rows[i]); quarantineErr != nil {
					failures = append(failures, quarantineErr)
				}
			}
		}
	}
	return errors.Join(failures...)
}

func (s *GameService) computeP13UnionFightPlayerBattle(ctx context.Context, captured *db.P13UnionFightPlayerBattleAttempt) error {
	return s.computeP13UnionFightPlayerBattleWith(ctx, captured, func(ctx context.Context, input *model.FrozenBattleInput, rules *p13UFPlayerBattleRules) (*model.CombatFinalHPOutcome, error) {
		if rules.Policy.ID == constUtil.P13UFPlayerPartyPolicyID {
			return s.ComputeP6UFPartyPlayerSettlement(ctx, input)
		}
		if !rules.FullHPOnly && validateP6UFPartialPlayerScope(input) == nil {
			return s.ComputeP6UFPartialPlayerSettlement(ctx, input)
		}
		return s.ComputeP6UFPlayerSettlement(ctx, input)
	})
}

// The calculator boundary makes lock ownership and cancellation explicit. The
// production caller supplies the same certified engine used by legacy receipts.
func (s *GameService) computeP13UnionFightPlayerBattleWith(ctx context.Context, captured *db.P13UnionFightPlayerBattleAttempt, calculate func(context.Context, *model.FrozenBattleInput, *p13UFPlayerBattleRules) (*model.CombatFinalHPOutcome, error)) error {
	input, _, _, rules, err := s.p13UFPlayerBattleVerify(db.GetIns().WithContext(ctx), captured)
	if err != nil {
		return err
	}
	if captured.CalculationStage != constUtil.P13UFPlayerBattleCaptured {
		return errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	terminal, err := calculate(ctx, input, rules)
	if err != nil {
		return &p13UFPlayerBattleCalculationError{err: err}
	}
	if !p13UFPlayerBattleTerminalForRules(rules, input, terminal) {
		return &p13UFPlayerBattleCalculationError{err: errors.New(constUtil.CombatUnsupported)}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stop, gateErr := p13UFCombatCommitGate(tx, captured.MatchID, p13UFNow(ctx))
		if gateErr != nil || stop {
			return gateErr
		}
		// The guild gate precedes the attempt. No current policy/profile lookup
		// can change the immutable accepted scope or charge a second request.
		var current db.P13UnionFightPlayerBattleAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", captured.ID).Error; err != nil {
			return err
		}
		if current.CalculationStage != constUtil.P13UFPlayerBattleCaptured {
			_, _, _, _, err := s.p13UFPlayerBattleVerify(tx, &current)
			return err
		}
		if current.SourceHash != captured.SourceHash {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		verified, _, _, frozenRules, err := s.p13UFPlayerBattleVerify(tx, &current)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", current.P6RecordID).Error; err != nil {
			return err
		}
		if record.InputHash != current.InputHash || record.Source != constUtil.P13UFPlayerBattleSource || record.SourceFight != current.FightID || record.PlayerID != current.PlayerID {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		// Preserve legacy FinishedAt semantics: it binds the accepted source
		// clock, while the new stage carries the durable computation boundary.
		if err = p13UFPlayerBattleAcceptP6(tx, &record, verified, terminal, frozenRules, time.UnixMilli(current.CreatedAt)); err != nil {
			return err
		}
		current.Terminal, err = json.Marshal(terminal)
		if err != nil {
			return err
		}
		current.TerminalHash = p6Digest(current.Terminal)
		current.ScopeProof = terminal.Outcome.CapabilityHash
		current.CalculationStage = constUtil.P13UFPlayerBattleComputed
		current.NextCalculationAt = 0
		current.SourceHash = p13UFPlayerBattleHash(&current)
		updated := tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("id=? AND source_hash=? AND calculation_stage=? AND status=?", current.ID, captured.SourceHash, constUtil.P13UFPlayerBattleCaptured, constUtil.P13UFPlayerBattlePending).Updates(map[string]any{"terminal": current.Terminal, "terminal_hash": current.TerminalHash, "scope_proof": current.ScopeProof, "calculation_stage": current.CalculationStage, "next_calculation_at": current.NextCalculationAt, "source_hash": current.SourceHash})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		return nil
	})
}

// Advance calculates durable captures, then applies certified terminals only
// after their original due time. Disabled new-start policy preserves accepted work.
func (s *GameService) AdvanceP13UnionFightPlayerBattles(ctx context.Context, now time.Time) error {
	var failures []error
	if err := s.computeP13UnionFightPlayerBattles(ctx, now); err != nil {
		failures = append(failures, err)
	}
	var ids []string
	if e := p13UFCombatOpenCandidates(db.GetIns().WithContext(ctx), "p13_union_fight_player_battle_attempts", now).Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("status=? AND calculation_stage<>? AND due_at<=?", constUtil.P13UFPlayerBattlePending, constUtil.P13UFPlayerBattleCaptured, now.UnixMilli()).Order("due_at,id").Limit(64).Pluck("id", &ids).Error; e != nil {
		return e
	}
	for _, id := range ids {
		var changed *db.P13UnionFightPlayerBattleAttempt
		var environmentalFrames []proto.Message
		e := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var lock db.P13GuildPolicy
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, 1).Error; e != nil {
				return e
			}
			var row db.P13UnionFightPlayerBattleAttempt
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; e != nil {
				return e
			}
			if row.Status != constUtil.P13UFPlayerBattlePending || row.DueAt > now.UnixMilli() {
				return nil
			}
			stop, gateErr := p13UFCombatCommitGate(tx, row.MatchID, p13UFNow(ctx))
			if gateErr != nil || stop {
				return gateErr
			}
			input, out, before, rules, e := s.p13UFPlayerBattleVerify(tx, &row)
			if e != nil {
				return e
			}
			frames, e := p13UFPlayerBattleFrames(before)
			if e != nil {
				return e
			}
			after, dead, reward, revive, e := p13UFPlayerBattleApply(frames, input, out, rules)
			if e != nil {
				return e
			}
			ids := []int64{row.PlayerID, row.TargetID}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			for _, pid := range ids {
				var locked db.P13UnionFightFieldPlayer
				if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "match_id=? AND player_id=?", row.MatchID, pid).Error; e != nil {
					return e
				}
			}
			for i, side := range before {
				var actor db.P13UnionFightFieldPlayer
				if e = tx.First(&actor, "match_id=? AND player_id=?", row.MatchID, side.PlayerID).Error; e != nil {
					return e
				}
				current, e := p13UFFieldReadActor(&actor)
				if e != nil {
					return e
				}
				if actor.BattleHash != side.BattleHash || actor.GuildID != side.GuildID || actor.Camp != side.Camp || (!actor.Hooked && actor.LeftAt == 0 && current.GetState() != 1) || !proto.Equal(current.FightInfo, frames[i].FightInfo) || current.GetKillNum() != frames[i].GetKillNum() || current.GetDeadNum() != frames[i].GetDeadNum() || current.GetOwnToken() != frames[i].GetOwnToken() || current.GetOwnScore() != frames[i].GetOwnScore() {
					return errors.New(constUtil.P13UFPlayerBattleConflict)
				}
				// Preserve inventory, cooldown, membership and effect changes made by other
				// real field operations while applying only the bound battle fields.
				current.FightInfo = after[i].FightInfo
				current.KillNum = after[i].KillNum
				current.DeadNum = after[i].DeadNum
				current.OwnToken = after[i].OwnToken
				current.OwnScore = after[i].OwnScore
				current.State = after[i].State
				current.LeftTime = after[i].LeftTime
				current.TotalTime = after[i].TotalTime
				if side.PlayerID == dead {
					current.LeftTime = proto.Float32(float32(max(int64(0), row.DueAt+revive*1000-now.UnixMilli())) / 1000)
				}
				if e = p13UFFieldSaveActor(tx, &actor, current); e != nil {
					return e
				}
				after[i] = current
			}
			row.DeadPlayerID, row.RewardPlayerID, row.ReviveSeconds = dead, reward, revive
			row.Status = constUtil.P13UFPlayerBattleSettled
			row.SettledAt = now.UnixMilli()
			row.After, e = json.Marshal(after)
			if e != nil {
				return e
			}
			row.SourceHash = p13UFPlayerBattleHash(&row)
			if e = tx.Save(&row).Error; e != nil {
				return e
			}
			// Keep the sealed P6 terminal separate from environmental hits queued
			// during this occupancy. Apply those hits before either actor is free.
			for i, side := range before {
				var actor db.P13UnionFightFieldPlayer
				if e = tx.First(&actor, "match_id=? AND player_id=?", row.MatchID, side.PlayerID).Error; e != nil {
					return e
				}
				scene, messages, drainErr := p13UFTowerDrain(tx, &actor, after[i], constUtil.P13UnionFightTowerOccupancyOrdinary, row.ID, row.InputHash, row.DueAt, now)
				if drainErr != nil {
					return drainErr
				}
				if e = p13UFFieldSaveActor(tx, &actor, scene); e != nil {
					return e
				}
				environmentalFrames = append(environmentalFrames, messages...)
			}
			changed = &row
			return nil
		})
		if e != nil {
			failures = append(failures, fmt.Errorf("ordinary14 pending %s: %w", id, e))
			continue
		}
		if changed != nil {
			publishErr := p13UFPublishBeforeAbort(ctx, changed.MatchID, func() {
				s.p13UFPlayerBattlePublish(ctx, changed)
				for _, message := range environmentalFrames {
					if death, private := message.(*pb.SCUFPlayerDead); private && len(death.Guid) == 1 {
						s.publishP10Private(ctx, death.Guid[0], message)
					} else {
						s.p13UFFieldBroadcast(ctx, changed.MatchID, 0, message)
					}
				}
			})
			if publishErr != nil {
				failures = append(failures, publishErr)
			}
		}
	}
	return errors.Join(failures...)
}

func (s *GameService) p13UFPlayerBattlePublish(ctx context.Context, row *db.P13UnionFightPlayerBattleAttempt) {
	var after []*pb.InstUFPlayer
	if json.Unmarshal(row.After, &after) != nil {
		return
	}
	// Cold state is the durable fallback for lost pushes. No old push is queued
	// for delivery after a later fight, preventing stale HP from replacing new HP.
	full := &pb.SCUFPlayer{}
	for _, frame := range after {
		s.p13UFFieldBroadcast(ctx, row.MatchID, 0, &pb.SCUFPlayerFightEnd{Guid: frame.Guid})
		var actor db.P13UnionFightFieldPlayer
		if db.GetIns().WithContext(ctx).First(&actor, "match_id=? AND player_id=? AND left_at=0", row.MatchID, frame.GetGuid()).Error == nil {
			full.Player = append(full.Player, frame)
			full.Local = append(full.Local, &pb.PointNew{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)})
			if _, live := s.p13UFFieldLive(db.GetIns().WithContext(ctx), actor); live {
				s.publishP10Private(ctx, frame.GetGuid(), &pb.SCUFPlayerFightEndDetail{Player: frame})
			}
		}
	}
	if len(full.Player) > 0 {
		s.p13UFFieldBroadcast(ctx, row.MatchID, 0, full)
	}
	if dead := p13UFPlayerBattleDeadPacket(row, after); dead != nil {
		s.p13UFFieldBroadcast(ctx, row.MatchID, 0, dead)
	}
}

// Cold and duplicate receipts verify the immutable P6/source/terminal/HP/reward
// facts without computing another fight or reading the current enabled policy.
func (s *GameService) p13UFPlayerBattleReceipt(tx *gorm.DB, row *db.P13UnionFightPlayerBattleAttempt, id int64) ([]proto.Message, error) {
	input, out, before, rules, e := s.p13UFPlayerBattleVerify(tx, row)
	if e != nil {
		return nil, e
	}
	if id != row.PlayerID && id != row.TargetID {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	if row.Status == constUtil.P13UFPlayerBattlePending {
		return []proto.Message{&pb.SCUFPlayerFightBegin{Guid: proto.Int64(row.PlayerID), TargetGuid: proto.Int64(row.TargetID)}, &pb.SCUFPlayerFightBegin{Guid: proto.Int64(row.TargetID), TargetGuid: proto.Int64(row.PlayerID)}}, nil
	}
	beforeFrames, e := p13UFPlayerBattleFrames(before)
	if e != nil {
		return nil, e
	}
	expected, dead, reward, revive, e := p13UFPlayerBattleApply(beforeFrames, input, out, rules)
	if e != nil {
		return nil, e
	}
	var after []*pb.InstUFPlayer
	if row.SettledAt < row.DueAt || json.Unmarshal(row.After, &after) != nil || len(after) != 2 || row.DeadPlayerID != dead || row.RewardPlayerID != reward || row.ReviveSeconds != revive {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	var own *pb.InstUFPlayer
	for i, frame := range after {
		wanted := expected[i]
		left := float32(0)
		if frame.GetGuid() == dead {
			left = float32(max(int64(0), row.DueAt+revive*1000-row.SettledAt)) / 1000
		}
		if frame.GetGuid() != wanted.GetGuid() || frame.GetCamp() != wanted.GetCamp() || !proto.Equal(frame.FightInfo, wanted.FightInfo) || frame.GetKillNum() != wanted.GetKillNum() || frame.GetDeadNum() != wanted.GetDeadNum() || frame.GetOwnScore() != wanted.GetOwnScore() || frame.GetOwnToken() != wanted.GetOwnToken() || frame.GetState() != wanted.GetState() || frame.GetTotalTime() != wanted.GetTotalTime() || frame.GetLeftTime() != left {
			return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		if frame.GetGuid() == id {
			own = frame
		}
	}
	messages := []proto.Message{&pb.SCUFPlayerFightEnd{Guid: proto.Int64(row.PlayerID)}, &pb.SCUFPlayerFightEnd{Guid: proto.Int64(row.TargetID)}, &pb.SCUFPlayerFightEndDetail{Player: own}}
	if dead := p13UFPlayerBattleDeadPacket(row, after); dead != nil {
		messages = append(messages, dead)
	}
	return messages, nil
}

// A real dead player's countdown derives from the accepted death timestamp.
// This projection never revives/heals a player or restarts the countdown.
func p13UFPlayerBattleProject(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, now time.Time) error {
	if frame.GetState() == 1 {
		var active db.P13UnionFightPlayerBattleAttempt
		err := tx.Where("match_id=? AND status=? AND (player_id=? OR target_id=?)", actor.MatchID, constUtil.P13UFPlayerBattlePending, actor.PlayerID, actor.PlayerID).First(&active).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		rules, rulesErr := p13UFPlayerBattleReadRules(tx, &active)
		if active.SourceHash != p13UFPlayerBattleHash(&active) || rulesErr != nil {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		frame.LeftTime = proto.Float32(float32(max(int64(0), active.DueAt-now.UnixMilli())) / 1000)
		frame.TotalTime = proto.Float32(float32(rules.DurationMillis) / 1000)
		return nil
	}
	if frame.GetState() != 3 {
		return nil
	}
	var row db.P13UnionFightPlayerBattleAttempt
	e := tx.Where("match_id=? AND dead_player_id=? AND status=?", actor.MatchID, actor.PlayerID, constUtil.P13UFPlayerBattleSettled).Order("due_at DESC,id DESC").First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if row.SourceHash != p13UFPlayerBattleHash(&row) || row.ReviveSeconds <= 0 || row.SettledAt < row.DueAt {
		return errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	frame.LeftTime = proto.Float32(float32(max(int64(0), row.DueAt+row.ReviveSeconds*1000-now.UnixMilli())) / 1000)
	frame.TotalTime = proto.Float32(float32(row.ReviveSeconds))
	return nil
}

// A positive-stamp historical retry is restored before observing a new intent
// in the caller's current match. Socket0 has no historical request identity.
func (s *GameService) p13UFPlayerBattleRetry(ctx context.Context, id int64, request *pb.CSUFPlayerAskFight, stamp int64) ([]proto.Message, bool, error) {
	if stamp <= 0 {
		return nil, false, nil
	}
	var row db.P13UnionFightPlayerBattleAttempt
	e := db.GetIns().WithContext(ctx).First(&row, "player_id=? AND client_stamp=?", id, stamp).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if e != nil {
		return nil, true, e
	}
	if request == nil || row.TargetID != request.GetTargetGuid() {
		return nil, true, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	var messages []proto.Message
	e = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// Use the same outer lock as Begin/items/resource actions before any
		// current-state read; a repeatable-read snapshot must not predate a
		// concurrent legal new battle or actual item012 revival.
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := tx.First(&row, "id=?", row.ID).Error; err != nil {
			return err
		}
		// Verify the historical immutable result even when no current-scene
		// response can be emitted. No old detail/death packet may roll back a
		// revived player, a later fight or the current war.
		if _, err := s.p13UFPlayerBattleReceipt(tx, &row, id); err != nil {
			return err
		}
		var actor db.P13UnionFightFieldPlayer
		err := tx.First(&actor, "match_id=? AND player_id=? AND left_at=0", row.MatchID, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var profile db.Profile
		if err = tx.First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return nil
		}
		member, err := p13GuildMember(tx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if member.GuildID != actor.GuildID {
			return nil
		}
		var latest db.P13UnionFightPlayerBattleAttempt
		if err = tx.Where("player_id=? OR target_id=?", id, id).Order("created_at DESC,id DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.ID != row.ID {
			return nil
		}
		var match db.P13UnionFightMatch
		if err = tx.First(&match, "id=?", row.MatchID).Error; err != nil {
			return err
		}
		now := p13UFNow(ctx)
		if now.UnixMilli() >= match.EndAt {
			return nil
		}
		current, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return err
		}
		if row.Status == constUtil.P13UFPlayerBattlePending {
			if current.GetState() == 1 {
				messages = []proto.Message{&pb.SCUFPlayerFightBegin{Guid: proto.Int64(row.PlayerID), TargetGuid: proto.Int64(row.TargetID)}}
			}
			return nil
		}
		var after []*pb.InstUFPlayer
		if json.Unmarshal(row.After, &after) != nil {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		var expected *pb.InstUFPlayer
		for _, frame := range after {
			if frame.GetGuid() == id {
				expected = frame
			}
		}
		if expected == nil {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		if current.GetState() != expected.GetState() || current.GetKillNum() != expected.GetKillNum() || current.GetDeadNum() != expected.GetDeadNum() || !proto.Equal(current.FightInfo, expected.FightInfo) {
			return nil
		}
		if err = p13UFPlayerBattleProject(tx, &actor, current, now); err != nil {
			return err
		}
		current.UnionAuthority = proto.Int32(min(current.GetUnionAuthority(), member.Role))
		// Only this recipient's current frame is restored. Sending the old
		// opponent End would terminate an unrelated newer fight on that actor.
		messages = []proto.Message{&pb.SCUFPlayerFightEnd{Guid: current.Guid}, &pb.SCUFPlayerFightEndDetail{Player: current}}
		if row.DeadPlayerID == id {
			timed := row
			timed.SettledAt = now.UnixMilli()
			if dead := p13UFPlayerBattleDeadPacket(&timed, []*pb.InstUFPlayer{current}); dead != nil {
				messages = append(messages, dead)
			}
		}
		return nil
	})
	return messages, true, e
}

// Restrict movement only for participants owned by this new ordinary battle
// lifecycle. Legacy unsupported/audit states keep their existing behavior.
func p13UFPlayerBattleMovement(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer) error {
	return p13UFPlayerBattleAction(tx, actor, frame)
}

// Only this new lifecycle reserves a participant across all mutable field
// actions. Legacy audit/mining/transfer states keep their original admission.
func p13UFPlayerBattleAction(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer) error {
	if err := p13UFGuardBattleAction(tx, actor, frame); err != nil {
		return err
	}
	var n int64
	if err := tx.Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("match_id=? AND status=? AND (player_id=? OR target_id=?)", actor.MatchID, constUtil.P13UFPlayerBattlePending, actor.PlayerID, actor.PlayerID).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return errors.New("原普通战斗5秒状态尚未结束，不能开始其他场景操作")
	}
	if frame.GetState() == 3 {
		return errors.New("死亡状态不能开始其他场景操作")
	}
	return nil
}

// Original OpenReviveTipPage matches the war bag InstUFItem.id. An unavailable
// ID silently falls back to slot0, so absent real012 stock must use empty ID.
func p13UFPlayerBattleDeadPacket(row *db.P13UnionFightPlayerBattleAttempt, after []*pb.InstUFPlayer) *pb.SCUFPlayerDead {
	if row.DeadPlayerID <= 0 {
		return nil
	}
	item := ""
	for _, frame := range after {
		if frame.GetGuid() != row.DeadPlayerID {
			continue
		}
		for _, stock := range frame.Items {
			if stock.GetId() == "ufitem_012" && stock.GetLeftUseNum() > 0 {
				item = "ufitem_012"
			}
		}
	}
	return &pb.SCUFPlayerDead{Guid: []int64{row.DeadPlayerID}, ReviveTime: []float64{float64(max(int64(0), row.DueAt+row.ReviveSeconds*1000-row.SettledAt)) / 1000}, ReviveItemId: []string{item}}
}

// Current HP/AP is selected only by a pre-entry ID2/ID3 offer. Eligibility
// normalization stays private; the returned calculation uses exact live data.
// ID3 alone retains already-dead partners, with at least one living actor.
func p13UFPlayerBattleSourceForRules(actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, battle *pb.BattlePlayer, first int32, rules *p13UFPlayerBattleRules) (*pb.BattlePlayer, error) {
	if !p13UFPlayerBattleRulesValid(rules) {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	if rules.FullHPOnly {
		return p13UFPlayerBattleSource(actor, frame, battle, first)
	}
	return p13UFPlayerBattleCurrentSource(actor, frame, battle, first, rules.Policy.ID == constUtil.P13UFPlayerPartyPolicyID)
}

// Pure producer binding, independent of authority. The offer gate above is
// always checked before this helper is used by a real accepted request.
func p13UFPlayerBattleCurrentSource(actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, battle *pb.BattlePlayer, first int32, allowDead bool) (*pb.BattlePlayer, error) {
	if frame == nil || frame.FightInfo == nil || battle == nil || battle.Inst == nil || !combatFinite(frame.FightInfo.GetAnger()) || frame.FightInfo.GetAnger() < 0 || frame.FightInfo.GetAnger() > 300 {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if len(frame.FightInfo.Index) != len(battle.Formation) || len(frame.FightInfo.HpPos) != len(frame.FightInfo.Index) || len(frame.FightInfo.MaxHpPos) != len(frame.FightInfo.Index) {
		return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	eligibleFrame := proto.Clone(frame).(*pb.InstUFPlayer)
	eligibleBattle := proto.Clone(battle).(*pb.BattlePlayer)
	eligibleFrame.FightInfo.Anger = proto.Float64(0)
	eligibleBattle.Inst.Anger = proto.Float64(0)
	for i := range eligibleFrame.FightInfo.HpPos {
		eligibleFrame.FightInfo.HpPos[i] = eligibleFrame.FightInfo.MaxHpPos[i]
	}
	slots := map[int32]int{}
	for i, pose := range frame.FightInfo.Index {
		if _, exists := slots[pose]; exists {
			return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		slots[pose] = i
	}
	alive := false
	for _, hero := range eligibleBattle.Formation {
		if hero == nil || hero.Prop == nil || hero.Inst == nil {
			return nil, errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		index, exists := slots[hero.GetPose()]
		if !exists || !combatFinite(frame.FightInfo.HpPos[index]) || frame.FightInfo.HpPos[index] < 0 || frame.FightInfo.HpPos[index] == 0 && !allowDead || frame.FightInfo.HpPos[index] > frame.FightInfo.MaxHpPos[index] || hero.Inst.GetCurHp() != frame.FightInfo.HpPos[index] {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
		alive = alive || frame.FightInfo.HpPos[index] > 0
		hero.Inst.CurHp = proto.Float64(hero.Prop.GetHp())
	}
	if !alive {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if _, err := p13UFPlayerBattleSource(actor, eligibleFrame, eligibleBattle, first); err != nil {
		return nil, err
	}
	out := proto.Clone(battle).(*pb.BattlePlayer)
	out.Inst.FirstOrder = proto.Int32(first)
	out.Inst.Anger = proto.Float64(frame.FightInfo.GetAnger())
	return out, nil
}

// The immutable offer authorizes its precise capability; Authoritative alone
// never transfers an old partial audit or another wrapper into war effects.
func p13UFPlayerBattleTerminalForRules(rules *p13UFPlayerBattleRules, input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) bool {
	if !p13UFPlayerBattleRulesValid(rules) || out == nil || out.Outcome == nil || !out.Outcome.Authoritative {
		return false
	}
	proof := out.Outcome.CapabilityHash
	if rules.Policy.ID == constUtil.P13UFPlayerPartyPolicyID {
		return proof == rules.CapabilityProof && p6KnownUFPartyPlayerSettlementProof(input, proof) && validateP6UFPartyPlayerTerminal(input, out) == nil
	}
	if proof == rules.CapabilityProof {
		return p6KnownUFPlayerSettlementProof(input, proof) && validateP6UFPlayerTerminal(input, out) == nil
	}
	return !rules.FullHPOnly && proof == rules.PartialCapabilityProof && p6KnownUFPartialPlayerSettlementProof(input, proof) && validateP6UFPartialPlayerTerminal(input, out) == nil
}
