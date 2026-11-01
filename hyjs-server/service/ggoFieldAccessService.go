package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoFieldActor struct {
	Service  *GameService
	Row      *db.Profile
	Player   *pb.Player
	Game     *model.GameState
	Presence db.P10ScenePresence
	Now      int64
}

type ggoGatherCurrentReadKey struct{}

// Gather fact admission reads settlement and activity ownership after its P1
// locks. MySQL RR could retain an earlier snapshot created before those locks.
// Other GGO transactions keep their existing isolation and SQLite keeps its
// single-writer behavior.
func ggoGatherTransactionOptions(database *gorm.DB, ctx context.Context) []*sql.TxOptions {
	if database != nil && database.Dialector.Name() == "mysql" && ctx.Value(ggoGatherCurrentReadKey{}) == true {
		return []*sql.TxOptions{{Isolation: sql.LevelReadCommitted}}
	}
	return nil
}

func ggoFieldError(code int64, text string) error { return model.NewErrBizCodeMsg(code, text) }

// ggoWithActor rechecks the current session and line while holding the player row.
// Callbacks may subsequently lock a line, preserving the shared player-before-line order.
func (s *GameService) ggoWithActor(ctx context.Context, auth model.GGOFieldAuth, session []byte, apply func(*gorm.DB, *ggoFieldActor) error) error {
	return s.ggoWithProfile(ctx, auth, session, true, true, func(tx *gorm.DB, actor *ggoFieldActor) error {
		if err := ggoCheckClientBuildLocked(tx, actor.Row.Id, session, actor.Service.GGOFieldConfig); err != nil {
			return err
		}
		return apply(tx, actor)
	})
}

// Recovery paths can read a frozen receipt after a scene switch or a publication change.
func (s *GameService) ggoWithProfile(ctx context.Context, auth model.GGOFieldAuth, session []byte, requireRuntime, requirePresence bool, apply func(*gorm.DB, *ggoFieldActor) error) error {
	if auth.PlayerID < 1 || len(session) != 16 {
		return model.NewErrParamsError()
	}
	authorized, err := AuthorizeAccount(ctx, auth.PlayerID, session)
	if err != nil {
		return ggoClientAuthError(err)
	}
	if AdminProfileServerID(*authorized) != s.OriginalServerID() {
		return constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, authorized)
	if err != nil {
		return err
	}
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return err
	}
	if requireRuntime && snapshot.ggo == nil {
		return ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig = snapshot.ggo
	runtime.GGOFieldEconomy = snapshot.ggoEconomy
	runtime.GGOFieldRevision = snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	changedChips := false
	var changedLine int32
	database := db.GetIns()
	err = database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, auth.PlayerID).Error; err != nil {
			return err
		}
		if AdminProfileServerID(row) != s.OriginalServerID() || subtle.ConstantTimeCompare(row.Session, session) != 1 {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "会话已失效")
		}
		// Marked gather transactions hold the stable activity catalog before
		// calendar decoding or any profile/period hook can read activity rows.
		if runtime.P1Ready {
			guarded, guardErr := p1AcquireGatherCatalog(tx)
			if guardErr != nil {
				return guardErr
			}
			tx = guarded
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		beforeChips, err := ggoChipCount(game)
		if err != nil {
			return err
		}
		var presence db.P10ScenePresence
		if requirePresence {
			if game.Position == nil || game.Position.GetScene() != constUtil.GGOFieldScene || game.Field == nil || game.Field.Side == nil {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请先进入沙漠")
			}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&presence, "player_id=?", auth.PlayerID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ggoFieldError(constUtil.GGOFieldInvalidLocation, "分线尚未就绪")
				}
				return err
			}
			if presence.Scene != constUtil.GGOFieldScene || presence.Line < 1 || presence.SessionHash != p6Digest(session) {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "分线或会话已变化")
			}
			sinkValue, ok := p10LiveClient(p10ClientKey{runtime.p10ClientOwner(), auth.PlayerID})
			if !ok || subtle.ConstantTimeCompare(sinkValue.(*p10ClientSink).session, session) != 1 {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请连接当前游戏场景")
			}
		}
		if err = apply(tx, &ggoFieldActor{Service: runtime, Row: &row, Player: player, Game: game, Presence: presence, Now: time.Now().UnixMilli()}); err != nil {
			return err
		}
		afterChips, err := ggoChipCount(game)
		if err != nil {
			return err
		}
		changedChips = beforeChips != afterChips
		if presence.Scene == constUtil.GGOFieldScene {
			changedLine = presence.Line
		}
		return nil
	}, ggoGatherTransactionOptions(database, ctx)...)
	if err == nil && changedChips {
		if changedLine > 0 {
			s.ggoNotifyFieldChange(ctx, changedLine)
		} else {
			s.ggoNotifyChipPlayerChange(ctx, auth.PlayerID)
		}
	}
	return err
}

func ggoPointInsideSafe(p *pb.PointNew, area *model.AdminFieldGGOSafeArea) bool {
	if p == nil || area == nil {
		return false
	}
	x, z := float64(p.GetX()), float64(p.GetZ())
	if math.IsNaN(x) || math.IsNaN(z) || math.IsInf(x, 0) || math.IsInf(z, 0) {
		return false
	}
	return x >= area.MinX && x <= area.MaxX && z >= area.MinZ && z <= area.MaxZ
}

func (s *GameService) GGOFieldSnapshot(ctx context.Context, auth model.GGOFieldAuth, session []byte) (*model.GGOFieldSnapshotResponse, error) {
	// Collect frozen awards before acquiring the scene/profile locks. Its own
	// transaction grants the original reward once and may update the terminal.
	profile, err := AuthorizeAccount(ctx, auth.PlayerID, session)
	if err != nil {
		return nil, ggoClientAuthError(err)
	}
	if AdminProfileServerID(*profile) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, profile)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return nil, err
	}
	if snapshot.ggo == nil {
		return nil, ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig, runtime.GGOFieldEconomy, runtime.GGOFieldRevision = snapshot.ggo, snapshot.ggoEconomy, snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	pending, err := runtime.CollectFieldBossPendingRewardsWithSession(ctx, auth.PlayerID, session)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		s.p10SceneSend(ctx, auth.PlayerID, &pb.SCAddRewardsToPlayer{Result: pending})
	}
	var out *model.GGOFieldSnapshotResponse
	err = s.ggoWithActor(ctx, auth, session, func(tx *gorm.DB, a *ggoFieldActor) error {
		cfg := a.Service.GGOFieldConfig
		if err := ggoEnsureChipProjection(tx, a.Row, a.Game); err != nil {
			return err
		}
		row, state, _, err := ggoLoadLine(tx, a.Service.OriginalServerID(), int64(a.Presence.Line), cfg, a.Service.GGOFieldRevision, a.Now)
		if err != nil {
			return err
		}
		// Timestamp and patrol projection share one instant after acquiring the
		// public line lock, minimizing queueing drift in client interpolation.
		projectionNow := time.Now().UnixMilli()
		out = &model.GGOFieldSnapshotResponse{ServerTimeMS: projectionNow, Scene: constUtil.GGOFieldScene, Line: int64(a.Presence.Line), EconomyVersion: cfg.Runtime.EconomyVersion, Revision: row.Revision, ResourceMineCount: cfg.Runtime.Resource.MineCount, ResourceFishCount: cfg.Runtime.Resource.FishCount,
			Safe:      ggoPointInsideSafe(a.Game.Position.GetCurrent(), cfg.Runtime.SafeArea),
			Resources: []model.GGOFieldResourceView{}, Monsters: []model.GGOFieldMonsterView{}, Exits: []model.GGOFieldExitView{}}
		out.PlayerChipCounts, err = a.Service.ggoFieldChipCounts(tx, a)
		if err != nil {
			return err
		}
		if a.Game.GGOField != nil {
			out.Terminal.Chips = a.Game.GGOField.Chips
			out.Terminal.Revision = a.Game.GGOField.Revision
			out.ProtectionUntilMS = a.Game.GGOField.ProtectionUntil
		}
		protected, held, err := ggoProtectedChips(out.Terminal.Chips, cfg.Runtime.ProtectedChips)
		if err != nil {
			return err
		}
		out.Terminal.Protected = protected
		if held > 0 {
			// Settlement uses the values frozen when these chips were acquired.
			out.Terminal.ExchangeValues = a.Game.GGOField.ExchangeValues
		} else {
			// No chips have a frozen price yet. A bad optional quote must not
			// make the rest of the scene snapshot unavailable.
			out.Terminal.ExchangeValues, _ = ggoChipValues(cfg)
		}
		out.Terminal.CreditBalance = ggoCreditBalance(a.Game)
		if out.Terminal.CreditBalance < 0 {
			return errors.New("信用余额无效")
		}
		out.Fatigue = int64(a.Game.Field.Side.GetTiredness())
		amounts := map[string]int64{}
		for _, tier := range cfg.Runtime.Resource.Tiers {
			for _, drops := range [][]model.AdminFieldDrop{tier.MineDrops, tier.FishDrops} {
				for _, drop := range drops {
					amounts[drop.ResourceID] = 0
				}
			}
		}
		for _, inventory := range [][]*pb.SimpleItem{a.Game.Gems, a.Game.Cooking} {
			for _, item := range inventory {
				if item != nil {
					if _, ok := amounts[item.GetId()]; ok {
						amounts[item.GetId()] = int64(item.GetNum())
					}
				}
			}
		}
		keys := make([]string, 0, len(amounts))
		for key := range amounts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.Materials = make([]model.GGOFieldResourceAmount, 0, len(keys))
		for _, key := range keys {
			out.Materials = append(out.Materials, model.GGOFieldResourceAmount{ID: key, Amount: amounts[key]})
		}
		exitBinding, ok := model.MatchAdminGGOExitNPCs(cfg)
		if !ok {
			return errors.New("沙漠接应点与NPC位置不匹配")
		}
		out.SafeArea = cfg.Runtime.SafeArea
		out.Boss, err = a.Service.ggoBossSnapshotLocked(tx, cfg, projectionNow, int64(a.Presence.Line))
		if err != nil {
			return err
		}
		out.NPCs = make([]model.GGOFieldNPCView, 0, len(cfg.NPCs))
		for _, npc := range cfg.NPCs {
			view := model.GGOFieldNPCView{ID: npc.ID, Name: npc.Name, HeroID: npc.HeroID, Role: npc.Role, Position: npc.Position, InteractionPosition: npc.Position, Radius: 3}
			if npc.Role == "extraction" {
				view.PointID = exitBinding[npc.ID]
				for _, p := range cfg.Runtime.Points {
					if p.ID == view.PointID {
						view.InteractionPosition = p.Position
						view.Radius = p.Radius
						break
					}
				}
			}
			out.NPCs = append(out.NPCs, view)
		}
		points := map[string]model.AdminFieldGGOPoint{}
		for _, p := range cfg.Runtime.Points {
			points[p.ID] = p
			if p.Kind == "exit" {
				out.Exits = append(out.Exits, model.GGOFieldExitView{PointID: p.ID, Position: p.Position, Radius: p.Radius})
			}
		}
		for _, n := range state.Resources {
			p := points[n.PointID]
			stand := p.StandPosition
			if stand == nil {
				stand = p.Position
			}
			out.Resources = append(out.Resources, model.GGOFieldResourceView{PointID: n.PointID, Kind: n.Kind, Level: n.Level, Remaining: n.Remaining, Generation: n.Generation, Position: p.Position, StandPosition: stand, Radius: p.Radius, RefreshAtMS: n.RefreshAtMS})
		}
		for _, tier := range cfg.Runtime.MonsterTiers {
			out.MonsterCount += tier.Count
		}
		for _, n := range state.Monsters {
			p := points[n.PointID]
			hp := float64(1)
			if n.RefreshAtMS > 0 {
				hp = 0
			}
			position, err := ggoPatrolPosition(&n, &p, projectionNow)
			if err != nil {
				return err
			}
			var patrol *model.GGOFieldPatrolView
			path, err := ggoPatrolPath(&p)
			if err != nil {
				return err
			}
			if path != nil {
				patrol = &model.GGOFieldPatrolView{Waypoints: path.Waypoints, Speed: path.Speed, EpochMS: n.PatrolEpochMS, PausedAtMS: n.PatrolPausedAtMS}
			}
			out.Monsters = append(out.Monsters, model.GGOFieldMonsterView{UUID: n.UUID, MonID: n.MonID, PointID: n.PointID, Level: n.Level, HeroID: n.HeroID, Generation: n.Generation, Position: position, HPRatio: hp, Available: n.ReservedBy == 0 && n.RefreshAtMS == 0 && !ggoLegacyBlocked(state, n.PointID), RefreshAtMS: n.RefreshAtMS, Patrol: patrol})
		}
		return nil
	})
	return out, err
}

func ggoReadReceipt(tx *gorm.DB, playerID int64, requestID, operation, hash string, out any) (bool, error) {
	var row db.GGOFieldReceipt
	err := tx.First(&row, "player_id=? AND request_id=?", playerID, requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Operation != operation || row.RequestHash != hash {
		return false, ggoFieldError(constUtil.GGOFieldRequestConflict, "请求编号已用于其他操作")
	}
	if err = json.Unmarshal(row.Response, out); err != nil {
		return false, err
	}
	return true, nil
}

func ggoSaveReceipt(tx *gorm.DB, playerID int64, requestID, operation, hash, version string, businessKey *string, delta [6]int64, otherID, now int64, response any) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	chips, err := json.Marshal(delta)
	if err != nil {
		return err
	}
	return tx.Create(&db.GGOFieldReceipt{PlayerID: playerID, RequestID: requestID, Operation: operation, BusinessKey: businessKey, RequestHash: hash, Response: data, CounterpartyID: otherID, EconomyVersion: version, ChipDelta: chips, CreatedAt: now}).Error
}

func (s *GameService) GGOFieldProtectionRelease(ctx context.Context, req model.GGOFieldProtectionReleaseRequest, session []byte) (*model.GGOFieldProtectionReleaseResponse, error) {
	var out model.GGOFieldProtectionReleaseResponse
	err := s.ggoWithActor(ctx, req.GGOFieldAuth, session, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := p6Digest([]byte(constUtil.GGOFieldOpProtectionRelease))
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpProtectionRelease, hash, &out); err != nil || found {
			return err
		}
		if a.Game.Field.Fight != nil && !a.Game.Field.Fight.Settled {
			return ggoFieldError(constUtil.GGOFieldBusy, "战斗中无法解除保护")
		}
		var attempt db.P8GatherAttempt
		err := tx.Where("player_id=? AND active=?", a.Row.Id, true).First(&attempt).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "采集中无法解除保护")
		}
		if err = P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
			return err
		}
		out = model.GGOFieldProtectionReleaseResponse{RequestID: req.RequestID, ServerTimeMS: a.Now}
		if a.Game.GGOField != nil && a.Game.GGOField.ProtectionUntil > 0 {
			out.Released = a.Game.GGOField.ProtectionUntil > a.Now
			a.Game.GGOField.ProtectionUntil = 0
			if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
				return err
			}
		}
		return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpProtectionRelease, hash, a.Service.GGOFieldConfig.Runtime.EconomyVersion, nil, [6]int64{}, 0, a.Now, out)
	})
	return &out, err
}
