package service

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
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

type ggoPendingGather struct {
	StartRequestID string          `json:"startRequestId"`
	AttemptToken   string          `json:"attemptToken"`
	Frozen         ggoGatherFrozen `json:"frozen"`
	Rewards        string          `json:"rewards"`
}

func ggoFrozenAttempt(attempt *db.P8GatherAttempt) (*ggoGatherFrozen, error) {
	var frozen ggoGatherFrozen
	if attempt == nil || attempt.Scene != constUtil.GGOFieldScene || json.Unmarshal([]byte(attempt.Snapshot), &frozen) != nil ||
		frozen.StartRequestID == "" || frozen.Line < 1 || frozen.EconomyVersion == "" || frozen.MaterialNumerator < 1 || frozen.MaterialNumerator > 1000 {
		return nil, errors.New("沙漠采集快照无效")
	}
	if _, err := p8DecodeConfig(attempt.Snapshot); err != nil {
		return nil, err
	}
	return &frozen, nil
}

func ggoFindBusinessReceipt(tx *gorm.DB, playerID int64, operation, key string, out any) (bool, error) {
	var row db.GGOFieldReceipt
	err := tx.Where("player_id=? AND operation=? AND business_key=?", playerID, operation, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = json.Unmarshal(row.Response, out); err != nil {
		return false, err
	}
	return true, nil
}

func ggoSystemReceiptID(kind, token string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("hyjs-ggo:"+kind+":"+token)).String()
}

// ggoPrepareRewards computes the exact frozen gains and new quarter remainders
// without changing the actor. The following grant and node update commit together.
func ggoPrepareRewards(g *model.GameState, raw string, numerator int64) (string, map[string]int64, error) {
	if g.GGOField == nil {
		return "", nil, errors.New("沙漠终端缺失")
	}
	remainder := map[string]int64{}
	for k, v := range g.GGOField.MaterialRemainder {
		remainder[k] = v
	}
	totals := map[string]int64{}
	if raw != "" {
		for _, part := range strings.Split(raw, ";") {
			pair := strings.Split(part, ",")
			if len(pair) != 2 || pair[0] == "" {
				return "", nil, errors.New("沙漠采集奖励无效")
			}
			value, err := strconv.ParseInt(pair[1], 10, 32)
			if err != nil || value < 1 {
				return "", nil, errors.New("沙漠采集奖励数量无效")
			}
			if strings.HasPrefix(pair[0], "MineItem_") || strings.HasPrefix(pair[0], "FishItem_") {
				value, remainder[pair[0]], err = ggoScaleQuarter(value, remainder[pair[0]], numerator)
				if err != nil {
					return "", nil, err
				}
			}
			if value > math.MaxInt32 || value > math.MaxInt64-totals[pair[0]] {
				return "", nil, errors.New("沙漠采集奖励溢出")
			}
			totals[pair[0]] += value
		}
	}
	keys := make([]string, 0, len(totals))
	for key, value := range totals {
		if value > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s,%d", key, totals[key]))
	}
	return strings.Join(parts, ";"), remainder, nil
}

func ggoInventoryHasSpace(g *model.GameState, rewards string) (bool, error) {
	if rewards == "" {
		return true, nil
	}
	stacks := map[string]int64{}
	for _, list := range [][]*pb.SimpleItem{g.Items, g.Gems, g.Cooking} {
		for _, item := range list {
			if item == nil || item.GetId() == "" || item.GetNum() < 0 {
				return false, errors.New("库存存档无效")
			}
			if _, exists := stacks[item.GetId()]; exists {
				return false, errors.New("库存资源重复")
			}
			stacks[item.GetId()] = int64(item.GetNum())
		}
	}
	for _, part := range strings.Split(rewards, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return false, errors.New("沙漠奖励无效")
		}
		n, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || n < 1 {
			return false, errors.New("沙漠奖励数量无效")
		}
		if stacks[pair[0]] > math.MaxInt32-n {
			return false, nil
		}
	}
	return true, nil
}

func ggoAwardGather(a *ggoFieldActor, frozen *ggoGatherFrozen, raw string, requestID, startID, token string) (*model.GGOFieldGatherClaimResponse, bool, error) {
	scaled, remainder, err := ggoPrepareRewards(a.Game, raw, frozen.MaterialNumerator)
	if err != nil {
		return nil, false, err
	}
	space, err := ggoInventoryHasSpace(a.Game, scaled)
	if err != nil || !space {
		return nil, !space, err
	}
	terminal := a.Game.GGOField
	_, total, err := ggoProtectedChips(terminal.Chips, 3)
	if err != nil {
		return nil, false, err
	}
	if total > 0 && (terminal.EconomyVersion != frozen.EconomyVersion || terminal.ExchangeValues != frozen.ExchangeValues) {
		return nil, false, ggoFieldError(constUtil.GGOFieldEconomyConflict, "请先结算已有版本芯片")
	}
	if terminal.Revision == math.MaxInt64 {
		return nil, false, errors.New("沙漠终端版本溢出")
	}
	preAwardTotal := total
	for i, n := range frozen.Chips {
		if n < 0 {
			return nil, false, errors.New("沙漠芯片数量无效")
		}
		if terminal.Chips[i] > math.MaxInt64-n || total > math.MaxInt64-n {
			return nil, true, nil
		}
		total += n
	}
	gain, err := a.Service.p8GrantGather(a.Player, a.Game, scaled)
	if err != nil {
		return nil, false, err
	}
	terminal = a.Game.GGOField
	terminal.MaterialRemainder = remainder
	if preAwardTotal == 0 {
		terminal.EconomyVersion = frozen.EconomyVersion
		terminal.ExchangeValues = frozen.ExchangeValues
	}
	for i, n := range frozen.Chips {
		terminal.Chips[i] += n
	}
	terminal.Revision++
	out := &model.GGOFieldGatherClaimResponse{RequestID: requestID, StartRequestID: startID, AttemptToken: token, Resources: []model.GGOFieldResourceAmount{}, Chips: frozen.Chips, TerminalRevision: terminal.Revision}
	for _, item := range gain.GetSimps() {
		out.Resources = append(out.Resources, model.GGOFieldResourceAmount{ID: item.GetId(), Amount: int64(item.GetNum())})
	}
	return out, false, nil
}

// ggoGatherCanFinish validates the same locked actor, session, position and skill
// for timer, explicit claim and cancel. A disconnected actor cannot claim a
// still-active reservation through the recovery endpoint.
func ggoGatherCanFinish(tx *gorm.DB, a *ggoFieldActor, attempt *db.P8GatherAttempt, frozen *ggoGatherFrozen) (bool, error) {
	g := a.Game
	if g == nil || g.Position == nil || g.Position.GetScene() != attempt.Scene || g.Field == nil || g.Field.Side == nil ||
		g.Field.Side.GetCurHp() <= 0 || g.Field.Fight != nil && !g.Field.Fight.Settled || g.Run != nil && !g.Run.Complete ||
		!p8Near(g.Position.GetCurrent(), &frozen.p8GatherConfig) || g.Field.Side.GetTiredness() < frozen.Fatigue {
		return false, nil
	}
	level, err := a.Service.p8LifeLevel(tx, a.Row.Id, frozen.Kind)
	if err != nil {
		return false, err
	}
	if level < frozen.Level {
		return false, nil
	}
	if err = P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
		if err.Error() == constUtil.P12FieldPVPBusy || err.Error() == constUtil.P12FieldPVPJailed {
			return false, nil
		}
		return false, err
	}
	var presence db.P10ScenePresence
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&presence, "player_id=?", a.Row.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	value, ok := p10LiveClient(p10ClientKey{a.Service.p10ClientOwner(), a.Row.Id})
	if !ok {
		return false, nil
	}
	sink := value.(*p10ClientSink)
	if presence.Scene != attempt.Scene || int64(presence.Line) != frozen.Line ||
		presence.SessionHash != p6Digest(sink.session) || subtle.ConstantTimeCompare(a.Row.Session, sink.session) != 1 {
		return false, nil
	}
	if a.Row.AccountID > 0 {
		account, err := AccountForProfile(tx, a.Row)
		if err != nil {
			return false, err
		}
		if !account.Enabled || account.AuthRevision != a.Row.AccountAuthRevision {
			return false, nil
		}
	}
	return true, nil
}

// ggoLockPeriodsBeforeLine keeps the full activity lock range ahead of the line.
// Rule bodies are loaded later only when an activity hook needs them; shared
// transactions retain their existing exclusive-retry behavior.
func ggoLockPeriodsBeforeLine(tx *gorm.DB, s *GameService) error {
	if s == nil || !s.P1Ready {
		return nil
	}
	return p1LockPeriodIDs(tx)
}

// ggoEndAttempt consumes a reserved generation exactly once. If inventory cannot
// accept its frozen materials, the durable ready receipt allows later delivery.
func ggoEndAttempt(tx *gorm.DB, a *ggoFieldActor, attempt *db.P8GatherAttempt, requestID string) (*model.GGOFieldGatherClaimResponse, bool, error) {
	if attempt == nil || !attempt.Active {
		return nil, false, errors.New("沙漠采集已结束")
	}
	if a.Now < attempt.ReadyAt {
		return nil, false, ggoFieldError(constUtil.GGOFieldNotReady, "采集尚未完成")
	}
	frozen, err := ggoFrozenAttempt(attempt)
	if err != nil {
		return nil, false, err
	}
	canFinish, err := ggoGatherCanFinish(tx, a, attempt, frozen)
	if err != nil {
		return nil, false, err
	}
	if !canFinish {
		return nil, false, ggoFieldError(constUtil.GGOFieldInvalidLocation, "采集中断或资格已变化")
	}
	if err = ggoLockPeriodsBeforeLine(tx, a.Service); err != nil {
		return nil, false, err
	}
	var row db.GGOFieldLine
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=? AND line=?", a.Service.OriginalServerID(), frozen.Line).Error; err != nil {
		return nil, false, err
	}
	var state ggoLineState
	if json.Unmarshal(row.Payload, &state) != nil {
		return nil, false, errors.New("沙漠场景状态损坏")
	}
	node, detached, released, err := ggoResolveGatherReservation(&state, attempt, frozen)
	if err != nil || node != nil && node.Remaining < 1 {
		return nil, false, ggoFieldError(constUtil.GGOFieldStaleGeneration, "采集资源代次已变化")
	}
	claim, pending, err := ggoAwardGather(a, frozen, attempt.Rewards, requestID, frozen.StartRequestID, attempt.Token)
	if err != nil {
		return nil, false, err
	}
	if node != nil {
		node.Remaining--
		if !detached {
			node.ReservedBy, node.AttemptToken, node.ReadyAtMS = 0, "", 0
		}
		if node.Remaining == 0 {
			node.RefreshAtMS = attempt.ReadyAt + frozen.CooldownMS
		}
	}
	attempt.Active = false
	a.Game.Field.Side.Tiredness = proto.Int32(a.Game.Field.Side.GetTiredness() - frozen.Fatigue)
	a.Game.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	if !detached || released {
		if err = ggoLineSave(tx, &row, &state, a.Now); err != nil {
			return nil, false, err
		}
	}
	if err = tx.Save(attempt).Error; err != nil {
		return nil, false, err
	}
	if err = a.Service.recordP1GatherEvent(tx, a.Row.Id, attempt,
		map[bool]string{true: constUtil.P8EventFishGather, false: constUtil.P8EventMineGather}[frozen.Kind == "fish"], true,
		LocalPlayAt(tx.Statement.Context, time.UnixMilli(a.Now))); err != nil {
		return nil, false, err
	}
	if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
		return nil, false, err
	}
	key := attempt.Token
	if pending {
		ready := ggoPendingGather{StartRequestID: frozen.StartRequestID, AttemptToken: attempt.Token, Frozen: *frozen, Rewards: attempt.Rewards}
		err = ggoSaveReceipt(tx, a.Row.Id, ggoSystemReceiptID("ready", attempt.Token), constUtil.GGOFieldOpGatherReady, p6Digest([]byte(attempt.Token)), frozen.EconomyVersion, &key, [6]int64{}, 0, a.Now, ready)
		return nil, true, err
	}
	err = ggoSaveReceipt(tx, a.Row.Id, requestID, constUtil.GGOFieldOpGatherClaim, ggoClaimHash(frozen.StartRequestID, attempt.Token), frozen.EconomyVersion, &key, frozen.Chips, 0, a.Now, claim)
	return claim, false, err
}

func ggoDeliverPending(tx *gorm.DB, a *ggoFieldActor, pending *ggoPendingGather, requestID string) (*model.GGOFieldGatherClaimResponse, bool, error) {
	if pending == nil || pending.AttemptToken == "" {
		return nil, false, errors.New("沙漠待领奖励无效")
	}
	out, full, err := ggoAwardGather(a, &pending.Frozen, pending.Rewards, requestID, pending.StartRequestID, pending.AttemptToken)
	if err != nil || full {
		return nil, full, err
	}
	if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
		return nil, false, err
	}
	key := pending.AttemptToken
	if err = ggoSaveReceipt(tx, a.Row.Id, requestID, constUtil.GGOFieldOpGatherClaim, ggoClaimHash(pending.StartRequestID, key), pending.Frozen.EconomyVersion, &key, pending.Frozen.Chips, 0, a.Now, out); err != nil {
		return nil, false, err
	}
	return out, false, nil
}
