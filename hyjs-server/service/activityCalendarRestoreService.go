package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ActivityCalendarRestoreRequest binds a request to a reviewed complete preimage.
type ActivityCalendarRestoreRequest struct {
	RequestID      string `json:"requestId"`
	ExpectedDigest string `json:"expectedDigest"`
	RealMillis     int64  `json:"realMillis"`
}

// ActivityCalendarRestoreManifest hashes private rows without exposing their payloads.
type ActivityCalendarRestoreManifest struct {
	Table  string `json:"table"`
	Rows   int    `json:"rows"`
	Digest string `json:"digest"`
}

// ActivityCalendarRestoreResult is the immutable successful operation receipt.
type ActivityCalendarRestoreResult struct {
	RequestID string                            `json:"requestId"`
	Digest    string                            `json:"digest"`
	OpeningAt int64                             `json:"openingAt"`
	Clock     *model.ServerGameClockView        `json:"clock"`
	Windows   int                               `json:"windows"`
	Carries   int                               `json:"carries"`
	Manifest  []ActivityCalendarRestoreManifest `json:"manifest"`
}
type activityCalendarRestoreSnapshot struct {
	plan     *ActivityCalendarRestorePlan
	realm    db.AdminGameServer
	clock    db.ServerGameClock
	revision db.AdminActivityScheduleRevision
	sources  []db.AdminActivityOccurrence
	issued   []db.AdminCharacterActivityInstance
	anchors  []db.PlayerOpeningCalendar
	profiles []db.Profile
	opening  db.OpenServerActivityRule
}

// InitializeActivityCalendarRestoreSchema creates only the four new carry/archive
// tables. Maintenance is checked before schema work; preview never invokes it.
func (s *GameService) InitializeActivityCalendarRestoreSchema(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var policy db.ClientUpdatePolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Maintenance {
		return constUtil.ErrAdminConfigConflict
	}
	return tx.AutoMigrate(&db.ActivityRechargeCarry{}, &db.OpeningCalendarCarry{}, &db.OpeningCalendarPaymentCarry{}, &db.ActivityCalendarPeriodArchive{})
}

// ApplyActivityCalendarRestoreCLI uses the same live administrator checks as the
// authenticated entry. CLI execution has no player-facing or notification side effect.
func (s *GameService) ApplyActivityCalendarRestoreCLI(ctx context.Context, req ActivityCalendarRestoreRequest) (*ActivityCalendarRestoreResult, error) {
	var actor db.AdminUser
	if err := db.GetIns().WithContext(ctx).Where("enabled=? AND role=?", true, constUtil.AdminRoleAdmin).Order("id").First(&actor).Error; err != nil {
		return nil, err
	}
	var clock db.ServerGameClock
	if err := db.GetIns().WithContext(ctx).First(&clock, "server_id=?", s.serverGameClockID()).Error; err != nil {
		return nil, err
	}
	if !serverGameClockValid(&clock) {
		return nil, constUtil.ErrAdminConfiguration
	}
	if err := s.InitializeActivityCalendarRestoreSchema(ctx); err != nil {
		return nil, err
	}
	s.ServerClockReady = true
	return s.AdminRestoreActivityCalendar(ctx, &model.AdminPrincipal{UserID: actor.ID, Username: actor.Username, Role: actor.Role}, req)
}

func (s *GameService) readActivityCalendarRestore(tx *gorm.DB, real time.Time) (*activityCalendarRestoreSnapshot, error) {
	x := &activityCalendarRestoreSnapshot{}
	current := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Session(&gorm.Session{})
	server := s.OriginalServerID()
	if err := current.First(&x.clock, "server_id=?", server).Error; err != nil {
		return nil, err
	}
	// Ownership includes nil saves and follows the realm's legacy-zero contract.
	owners := current.Order("id")
	if server == constUtil.GameDefaultServerID {
		owners = owners.Where("server_id=? OR server_id=0", server)
	} else {
		owners = owners.Where("server_id=?", server)
	}
	if err := owners.Find(&x.profiles).Error; err != nil {
		return nil, err
	}
	if err := current.First(&x.realm, server).Error; err != nil {
		return nil, err
	}
	if err := current.First(&x.revision, "server_id=?", server).Error; err != nil {
		return nil, err
	}
	if err := current.Where("server_id=? AND enabled=? AND archived=?", server, true, false).Order("id").Find(&x.sources).Error; err != nil {
		return nil, err
	}
	ids := []string{}
	for _, source := range x.sources {
		ids = append(ids, source.ID)
	}
	if len(ids) > 0 {
		if err := current.Where("server_id=? AND occurrence_id IN ?", server, ids).Order("player_id,occurrence_id").Find(&x.issued).Error; err != nil {
			return nil, err
		}
	}
	players := []int64{}
	for _, profile := range x.profiles {
		players = append(players, profile.Id)
	}
	if len(players) > 0 {
		if err := current.Where("player_id IN ?", players).Order("player_id").Find(&x.anchors).Error; err != nil {
			return nil, err
		}
	}
	openingMissing := !adminActivitySchemaHasTable(tx, &db.OpenServerActivityRule{})
	if !openingMissing {
		if err := current.First(&x.opening, "id=?", constUtil.OpenServerActivityID).Error; err != nil {
			return nil, err
		}
	}
	x.plan = activityCalendarRestorePlan(x.realm, x.clock, x.revision, x.sources, x.issued, x.anchors, real)
	if openingMissing {
		x.plan.Blockers = append(x.plan.Blockers, "dependency schema missing: open_server_activity_rules")
	}
	if err := activityCalendarRestoreDependencies(tx, x.plan); err != nil {
		return nil, err
	}
	var policy db.ClientUpdatePolicy
	if err := current.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if !policy.Maintenance {
		x.plan.Blockers = append(x.plan.Blockers, "maintenance: game must enter maintenance before apply")
	}
	if err := serverGameClockPendingBattles(tx, real, x.clock.OffsetDays); err != nil {
		x.plan.Blockers = append(x.plan.Blockers, "battle: "+err.Error())
	}
	for _, owner := range x.profiles {
		if owner.Game != nil {
			var game model.GameState
			if err := json.Unmarshal([]byte(*owner.Game), &game); err != nil {
				return nil, err
			}
			if game.Run != nil {
				x.plan.Blockers = append(x.plan.Blockers, fmt.Sprintf("battle: player %d has active run", owner.Id))
			}
		}
	}
	// Bounded administrative snapshot: all rows in each consumer/ledger table are
	// current-read and canonicalized. Including unrelated retained history is
	// conservative: a concurrent change requires a fresh preview, never partial CAS.
	models := []any{&db.Profile{}, &db.GameAccount{}, &db.ClientUpdatePolicy{}, &db.AdminActivityOccurrence{}, &db.AdminActivityDefinition{}, &db.AdminActivityTemplateOwner{}, &db.AdminCharacterActivityInstance{}, &db.PlayerOpeningCalendar{}, &db.PeriodInstance{}, &db.PeriodProgress{}, &db.PeriodSettlement{}, &db.RewardDelivery{}, &db.GameplayPool{}, &db.GameplayPoolReward{}, &db.TurntableRules{}, &db.TurntableState{}, &db.TurntablePersonal{}, &db.TurntableGlobal{}, &db.TurntableDrawReceipt{}, &db.KingRecruitActivitySnapshot{}, &db.KingRecruitState{}, &db.KingRecruitSelection{}, &db.KingRecruitRules{}, &db.P4CommerceRule{}, &db.P4CommerceState{}, &db.P4CommerceStock{}, &db.PaymentOrder{}, &db.LocalPlayPaymentTime{}, &db.LocalPlayActivityBinding{}, &db.LocalPlayActivityCycle{}, &db.OpenServerActivityRule{}, &db.OpenServerActivityState{}, &db.OpenServerActivityCounter{}, &db.OpenServerActivityReceipt{}, &db.OpenServerActivityTask{}, &db.AdminOpeningState{}, &db.AdminOpeningCounter{}, &db.AdminOpeningStock{}, &db.AdminOpeningReceipt{}, &db.ActivityRechargeCarry{}, &db.OpeningCalendarCarry{}, &db.OpeningCalendarPaymentCarry{}, &db.ActivityCalendarPeriodArchive{}}
	for _, entity := range models {
		statement := &gorm.Statement{DB: tx}
		if err := statement.Parse(entity); err != nil {
			return nil, err
		}
		rows := []map[string]any{}
		if adminActivitySchemaHasTable(tx, entity) {
			if err := current.Model(entity).Limit(100001).Find(&rows).Error; err != nil {
				return nil, err
			}
		}
		if len(rows) > 100000 {
			return nil, fmt.Errorf("restore snapshot exceeds bound: %s", statement.Schema.Table)
		}
		encoded := make([]string, 0, len(rows))
		for _, row := range rows {
			raw, err := json.Marshal(row)
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, string(raw))
		}
		sort.Strings(encoded)
		x.plan.Manifest = append(x.plan.Manifest, ActivityCalendarRestoreManifest{statement.Schema.Table, len(rows), p6Digest(adminJSON(encoded))})
	}
	x.plan.Digest = p6Digest(adminJSON([]any{x.plan.Digest, x.plan.Manifest, x.opening}))
	return x, nil
}

// AdminRestoreActivityCalendar commits every native consumer, carry, source,
// anchor, timer and operation receipt in one caller-visible transaction.
func (s *GameService) AdminRestoreActivityCalendar(ctx context.Context, p *model.AdminPrincipal, req ActivityCalendarRestoreRequest) (*ActivityCalendarRestoreResult, error) {
	if s == nil || !s.ServerClockReady || p == nil || req.RequestID == "" || len(req.RequestID) > 36 || len(req.ExpectedDigest) != 64 || req.RealMillis <= 0 {
		return nil, constUtil.ErrAdminInput
	}
	var result *ActivityCalendarRestoreResult
	committed := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var clock db.ServerGameClock
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&clock, "server_id=?", s.serverGameClockID()).Error; err != nil {
			return err
		}
		if err := adminOperationUser(tx.Clauses(clause.Locking{Strength: "SHARE"}), p, true); err != nil {
			return err
		}
		var actor db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&actor, p.UserID).Error; err != nil {
			return err
		}
		if actor.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		var prior db.ServerGameClockOperation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, "server_id=? AND request_id=?", clock.ServerID, req.RequestID).Error
		if err == nil {
			if prior.ActorID != p.UserID {
				return constUtil.ErrAdminConfigConflict
			}
			var old ActivityCalendarRestoreResult
			if json.Unmarshal(prior.Response, &old) != nil || old.RequestID != req.RequestID || old.Digest != req.ExpectedDigest || old.Clock == nil {
				return constUtil.ErrAdminConfigConflict
			}
			result = &old
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		real := time.Now()
		previewReal := time.UnixMilli(req.RealMillis)
		if p1Day(real) != p1Day(previewReal) {
			return constUtil.ErrAdminConfigConflict
		}
		x, err := s.readActivityCalendarRestore(tx, previewReal)
		if err != nil {
			return err
		}
		if x.plan.Digest != req.ExpectedDigest || len(x.plan.Blockers) > 0 {
			return fmt.Errorf("restore preimage/blockers: %v: %w", x.plan.Blockers, constUtil.ErrAdminConfigConflict)
		}
		if !serverGameClockValid(&clock) || clock.Revision == math.MaxInt64 || clock.Revision != x.clock.Revision {
			return constUtil.ErrAdminConfiguration
		}
		oldNow := real.AddDate(0, 0, int(clock.OffsetDays)).UnixMilli()
		if err := validateActivityCalendarRestoreOwners(tx, x); err != nil {
			return err
		}
		count, err := s.prepareActivityCalendarRestoreCarries(tx, x, req.RequestID, oldNow, real.UnixMilli())
		if err != nil {
			return err
		}
		seen := map[string]ActivityCalendarRestoreWindow{}
		for _, w := range x.plan.Windows {
			domain, key, ok := strings.Cut(w.ConsumerID, ":")
			if !ok || key == "" {
				return constUtil.ErrAdminConfiguration
			}
			if domain == "character-plan" {
				if w.PlayerID != 0 {
					return constUtil.ErrAdminConfiguration
				}
				continue
			}
			if old, ok := seen[w.ConsumerID]; ok {
				if old.Before != w.Before || old.After != w.After || old.Mode != w.Mode {
					return constUtil.ErrAdminConfigConflict
				}
				continue
			}
			seen[w.ConsumerID] = w
			before, after := FrozenJSONWindow{w.Before[0], w.Before[1], w.Before[2]}, FrozenJSONWindow{w.After[0], w.After[1], w.After[2]}
			switch domain {
			case "period":
				err = RestoreActivityCalendarPeriodWindow(tx, key, req.RequestID, before, after)
			case "pool":
				if adminKingRecruitPoolID(key) {
					err = s.RestoreActivityCalendarKingRecruitWindow(tx, key, before, after)
				} else if w.Mode == "turntable" {
					err = RestoreActivityCalendarTurntableWindow(tx, key, before, after)
				} else {
					return constUtil.ErrAdminConfiguration
				}
			case "commerce", "shop-gate":
				err = s.RestoreCommerceCalendarWindow(tx, key, CommerceCalendarWindow{before.StartAt, before.EndAt, before.ClaimEndAt}, CommerceCalendarWindow{after.StartAt, after.EndAt, after.ClaimEndAt})
			case "opening":
				if w.Mode != "opening-14" || w.PlayerID <= 0 {
					return constUtil.ErrAdminConfiguration
				}
			default:
				return constUtil.ErrAdminConfiguration
			}
			if err != nil {
				return fmt.Errorf("consumer %s: %w", w.ConsumerID, err)
			}
		}
		if err := RestoreActivityCalendarScheduleWindows(tx, x.plan, x.sources, x.issued); err != nil {
			return err
		}
		before := FrozenJSONWindow{x.opening.StartAt, x.opening.EndAt, x.opening.RewardEndAt}
		after := FrozenJSONWindow{x.plan.OpeningAt, x.plan.OpeningAt + 14*86400000, x.plan.OpeningAt + 14*86400000 + before.ClaimEndAt - before.EndAt}
		if err := RestoreActivityCalendarOpeningWindows(tx, x.realm, x.anchors, x.opening, x.plan.OpeningAt, before, after); err != nil {
			return err
		}
		if x.revision.Revision == math.MaxInt64 {
			return constUtil.ErrAdminConfiguration
		}
		changed := tx.Model(&db.AdminActivityScheduleRevision{}).Where("server_id=? AND revision=?", x.plan.ServerID, x.revision.Revision).Update("revision", x.revision.Revision+1)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
		if err := captureFieldCalendar75BeforeShift(tx, x.profiles, time.UnixMilli(oldNow)); err != nil {
			return err
		}
		if err := expireCalendarWanted75(tx, s.OriginalServerID(), oldNow); err != nil {
			return err
		}
		if err := s.shiftCalendarDurationTimers75(tx, x.profiles, -clock.OffsetDays); err != nil {
			return err
		}
		delta := -clock.OffsetDays
		clock.OffsetDays = 0
		clock.Revision++
		clock.UpdatedAtMillis = real.UnixMilli()
		if clock.HighDays < 0 {
			clock.HighDays = 0
			clock.HighReachedAt = real.UnixMilli()
		}
		if err := tx.Save(&clock).Error; err != nil {
			return err
		}
		result = &ActivityCalendarRestoreResult{RequestID: req.RequestID, Digest: req.ExpectedDigest, OpeningAt: x.plan.OpeningAt, Clock: serverGameClockView(&clock, real), Windows: len(x.plan.Windows), Carries: count, Manifest: x.plan.Manifest}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := tx.Create(&db.ServerGameClockOperation{ServerID: clock.ServerID, RequestID: req.RequestID, ActorID: p.UserID, DeltaDays: delta, Response: raw, CreatedAt: real.UnixMilli()}).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, p, "activity.calendar.restore", fmt.Sprint(clock.ServerID), fmt.Sprintf("request=%s; digest=%s; windows=%d; carries=%d", req.RequestID, req.ExpectedDigest, result.Windows, result.Carries)); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err == nil && committed {
		s.cacheServerGameClock(result.Clock)
		s.adminFieldRuntimeCache.Store(nil)
	}
	return result, err
}

func activityCalendarRestoreProtected(tx *gorm.DB, owner db.Profile) (bool, error) {
	if owner.AccountID == 0 {
		return owner.Id <= 1, nil
	}
	account, err := AccountForProfile(tx.Clauses(clause.Locking{Strength: "SHARE"}), &owner)
	if err != nil {
		return false, err
	}
	return account.Protected, nil
}

func (s *GameService) prepareActivityCalendarRestoreCarries(tx *gorm.DB, x *activityCalendarRestoreSnapshot, request string, oldNow, cutoff int64) (int, error) {
	owners := map[int64]db.Profile{}
	legal := map[int64]bool{}
	for _, owner := range x.profiles {
		if AdminProfileServerID(owner) != x.plan.ServerID {
			return 0, constUtil.ErrAdminConfiguration
		}
		owners[owner.Id] = owner
		protected, err := activityCalendarRestoreProtected(tx, owner)
		if err != nil {
			return 0, err
		}
		legal[owner.Id] = !protected
	}
	anchors := map[int64]db.PlayerOpeningCalendar{}
	for _, anchor := range x.anchors {
		if _, ok := owners[anchor.PlayerID]; !ok {
			return 0, constUtil.ErrAdminConfiguration
		}
		anchors[anchor.PlayerID] = anchor
	}
	count := 0
	periodOwners := map[string]map[int64]bool{}
	periodTargets := map[string]ActivityCalendarRestoreWindow{}
	openingWindows := map[int64][]ActivityCalendarRestoreWindow{}
	for _, w := range x.plan.Windows {
		domain, key, ok := strings.Cut(w.ConsumerID, ":")
		if !ok {
			return 0, constUtil.ErrAdminConfiguration
		}
		if w.PlayerID > 0 {
			if _, ok := owners[w.PlayerID]; !ok || !legal[w.PlayerID] {
				return 0, constUtil.ErrAdminPermission
			}
		}
		if domain == "period" {
			if old, exists := periodTargets[key]; exists && (old.Before != w.Before || old.After != w.After) {
				return 0, constUtil.ErrAdminConfigConflict
			}
			periodTargets[key] = w
			if periodOwners[key] == nil {
				periodOwners[key] = map[int64]bool{}
			}
			if w.PlayerID > 0 {
				periodOwners[key][w.PlayerID] = true
			} else {
				for id, yes := range legal {
					if yes {
						periodOwners[key][id] = true
					}
				}
			}
		}
		if domain == "opening" {
			openingWindows[w.PlayerID] = append(openingWindows[w.PlayerID], w)
		}
	}
	periods := []string{}
	for id := range periodOwners {
		periods = append(periods, id)
	}
	sort.Strings(periods)
	for _, id := range periods {
		var cfg db.PeriodInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, "id=?", id).Error; err != nil {
			return 0, err
		}
		if cfg.Kind != "payment_amount" {
			continue
		}
		players := []int64{}
		for player := range periodOwners[id] {
			players = append(players, player)
		}
		sort.Slice(players, func(i, j int) bool { return players[i] < players[j] })
		for _, player := range players {
			var bindings []db.LocalPlayActivityBinding
			if gameSchemaHasTable(tx, &db.LocalPlayActivityBinding{}) {
				if err := tx.Where("player_id=? AND activity_id=?", player, id).Find(&bindings).Error; err != nil {
					return 0, err
				}
				if len(bindings) > 0 {
					continue
				}
			}
			carry, err := PrepareActivityRechargeCarry(tx, player, cfg, request, oldNow, cutoff, periodTargets[id].After[0])
			if err != nil {
				return 0, err
			}
			if err := SaveActivityRechargeCarry(tx, carry); err != nil {
				return 0, err
			}
			count++
		}
	}
	for _, owner := range x.profiles {
		if !legal[owner.Id] {
			continue
		}
		// Private-cycle qualification retains its independently frozen calendar.
		var bindings []db.LocalPlayActivityBinding
		if gameSchemaHasTable(tx, &db.LocalPlayActivityBinding{}) {
			if err := tx.Where("player_id=? AND activity_id=?", owner.Id, constUtil.OpenServerActivityID).Find(&bindings).Error; err != nil {
				return 0, err
			}
			if len(bindings) > 0 {
				continue
			}
		}
		windows := openingWindows[owner.Id]
		if anchor, ownsAnchor := anchors[owner.Id]; len(windows) == 0 && ownsAnchor {
			duration := x.opening.EndAt - x.opening.StartAt
			grace := x.opening.RewardEndAt - x.opening.EndAt
			before := FrozenJSONWindow{anchor.StartAt, anchor.StartAt + duration, anchor.StartAt + duration + grace}
			windows = []ActivityCalendarRestoreWindow{{PlayerID: owner.Id, Before: [3]int64{before.StartAt, before.EndAt, before.ClaimEndAt}, After: [3]int64{x.plan.OpeningAt, x.plan.OpeningAt + 14*86400000, x.plan.OpeningAt + 14*86400000 + before.ClaimEndAt - before.EndAt}}}
		}
		for _, w := range windows {
			scoped := tx.Session(&gorm.Session{NewDB: true}).WithContext(tx.Statement.Context)
			if w.OccurrenceID != "" {
				found := false
				for _, issued := range x.issued {
					if issued.PlayerID == owner.Id && issued.OccurrenceID == w.OccurrenceID {
						adminBindOpeningPeriod(scoped, issued)
						found = true
						break
					}
				}
				if !found {
					return 0, constUtil.ErrAdminConfiguration
				}
			}
			before, after := FrozenJSONWindow{w.Before[0], w.Before[1], w.Before[2]}, FrozenJSONWindow{w.After[0], w.After[1], w.After[2]}
			carry, err := PrepareOpeningCalendarCarry(scoped, owner.Id, request, before, after, oldNow, cutoff)
			if err != nil {
				return 0, err
			}
			prior, next := x.opening, x.opening
			prior.StartAt, prior.EndAt, prior.RewardEndAt = before.StartAt, before.EndAt, before.ClaimEndAt
			next.StartAt, next.EndAt, next.RewardEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
			money, err := PrepareOpeningCalendarPaymentCarry(scoped, owner.Id, prior, next, request, oldNow, cutoff)
			if err != nil {
				return 0, err
			}
			if err := SaveOpeningCalendarCarry(scoped, carry); err != nil {
				return 0, err
			}
			if err := SaveOpeningCalendarPaymentCarry(scoped, money); err != nil {
				return 0, err
			}
			count += 2
		}
	}
	return count, nil
}

// Frozen native progress must belong to the same accepted consumer owners. A
// foreign row is configuration drift, never permission to migrate another role.
func validateActivityCalendarRestoreOwners(tx *gorm.DB, x *activityCalendarRestoreSnapshot) error {
	owners := map[int64]bool{}
	for _, owner := range x.profiles {
		owners[owner.Id] = true
	}
	allowed := map[string]map[int64]bool{}
	for _, w := range x.plan.Windows {
		domain, _, ok := strings.Cut(w.ConsumerID, ":")
		if !ok {
			return constUtil.ErrAdminConfiguration
		}
		if domain == "character-plan" || domain == "opening" {
			continue
		}
		if allowed[w.ConsumerID] == nil {
			allowed[w.ConsumerID] = map[int64]bool{}
		}
		if w.PlayerID == 0 {
			for id := range owners {
				allowed[w.ConsumerID][id] = true
			}
		} else {
			allowed[w.ConsumerID][w.PlayerID] = true
		}
	}
	checks := []struct {
		domain, column string
		entity         any
	}{{"period", "instance_id", &db.PeriodProgress{}}, {"pool", "pool_id", &db.TurntableState{}}, {"pool", "pool_id", &db.TurntablePersonal{}}, {"pool", "pool_id", &db.TurntableDrawReceipt{}}, {"commerce", "rule_id", &db.P4CommerceState{}}, {"shop-gate", "rule_id", &db.P4CommerceState{}}}
	keys := []string{}
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		domain, id, _ := strings.Cut(key, ":")
		for _, check := range checks {
			if check.domain != domain || !adminActivitySchemaHasTable(tx, check.entity) {
				continue
			}
			var players []int64
			if err := tx.Model(check.entity).Clauses(clause.Locking{Strength: "SHARE"}).Where(check.column+"=?", id).Distinct("player_id").Pluck("player_id", &players).Error; err != nil {
				return err
			}
			for _, player := range players {
				if !allowed[key][player] {
					return fmt.Errorf("consumer %s has foreign player %d: %w", key, player, constUtil.ErrAdminConfiguration)
				}
			}
		}
	}
	return nil
}
