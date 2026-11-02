package service

import (
	"context"
	"crypto/subtle"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	pb "hyjs-server/model/protocol"
)

type ggoNoticeTimer struct {
	mu         sync.Mutex
	timer      *time.Timer
	cancelled  bool
	dirty      bool
	lastWarn   time.Time
	suppressed int64
}

type ggoNoticeRecipient struct {
	PlayerID               int64
	PresenceHash           string
	Session                []byte
	AccountID              int64
	ProfileAccountRevision int64
	UpdatePlayerID         *int64
	UpdateHash             *string
	BuildCode              *int64
	LoginAt                *int64
	DisconnectedAt         *int64
	MaintenanceEpoch       *int64
	Connected              *bool
}

// Queue a coalesced, payload-free hint only after the domain transaction
// commits. A zero line covers the entire realm; clients reread real state.
func (s *GameService) ggoNotifyFieldChange(ctx context.Context, line int32) {
	if s == nil || !s.ggoExtractionEnabled() || line < 0 {
		return
	}
	if p10SceneDeferNotice(ctx, func(ctx context.Context) { s.ggoNotifyFieldChange(ctx, line) }) {
		return
	}
	base := s.p10ClientOwner()
	for !base.ggoNoticeStopped.Load() {
		candidate := &ggoNoticeTimer{}
		actual, loaded := base.ggoNoticeTimers.LoadOrStore(line, candidate)
		task := actual.(*ggoNoticeTimer)
		task.mu.Lock()
		if task.cancelled {
			task.mu.Unlock()
			continue
		}
		if base.ggoNoticeStopped.Load() {
			task.cancelled = true
			base.ggoNoticeTimers.CompareAndDelete(line, task)
			task.mu.Unlock()
			return
		}
		task.dirty = true
		if !loaded {
			task.timer = time.AfterFunc(50*time.Millisecond, func() { base.ggoRunFieldNotice(ctx, line, task) })
		}
		task.mu.Unlock()
		return
	}
}

// Keep the per-line task registered while sending. Mutations arriving during
// a slow send are merged into one further pass; transient SQL failures retry.
func (s *GameService) ggoRunFieldNotice(ctx context.Context, line int32, task *ggoNoticeTimer) {
	task.mu.Lock()
	if task.cancelled || s.ggoNoticeStopped.Load() {
		task.mu.Unlock()
		return
	}
	task.dirty = false
	task.mu.Unlock()
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	err := s.ggoBroadcastFieldChange(callCtx, line)
	cancel()
	task.mu.Lock()
	if task.cancelled || s.ggoNoticeStopped.Load() {
		task.mu.Unlock()
		return
	}
	var warn bool
	var suppressed int64
	if err != nil {
		task.dirty = true
		if time.Since(task.lastWarn) >= 30*time.Second {
			warn, suppressed = true, task.suppressed
			task.lastWarn, task.suppressed = time.Now(), 0
		} else {
			task.suppressed++
		}
	}
	if task.dirty {
		delay := 50 * time.Millisecond
		if err != nil {
			delay = time.Second
		}
		task.timer = time.AfterFunc(delay, func() { s.ggoRunFieldNotice(ctx, line, task) })
	} else {
		task.cancelled = true
		s.ggoNoticeTimers.CompareAndDelete(line, task)
	}
	task.mu.Unlock()
	if warn {
		elog.Warn("ggo scene change notification failed", zap.Int64("suppressed", suppressed), zap.Error(err))
	}
}

func (s *GameService) ggoStopNoticeTimers() {
	base := s.p10ClientOwner()
	base.ggoNoticeStopped.Store(true)
	base.ggoNoticeTimers.Range(func(key, value any) bool {
		task := value.(*ggoNoticeTimer)
		task.mu.Lock()
		task.cancelled = true
		if task.timer != nil {
			task.timer.Stop()
		}
		base.ggoNoticeTimers.CompareAndDelete(key, task)
		task.mu.Unlock()
		return true
	})
}

// Indexed batches read presence and authorization; no per-player
// SQL is issued. Hints contain no assets and every HTTP reread reauthorizes.
func (s *GameService) ggoBroadcastFieldChange(ctx context.Context, line int32) error {
	base := s.p10ClientOwner()
	var ids []int64
	p10ClientSinks.Range(func(key, value any) bool {
		if client, ok := key.(p10ClientKey); ok && client.service == base && value.(*p10ClientSink).disconnectedAt.Load() == 0 {
			ids = append(ids, client.playerID)
		}
		return true
	})
	if len(ids) == 0 {
		return nil
	}
	tx := db.GetIns().WithContext(ctx)
	query := tx.Table("p10_scene_presence AS p").
		Select("p.player_id, p.session_hash AS presence_hash, pr.session, pr.account_id, pr.account_auth_revision AS profile_account_revision").
		Joins("JOIN profiles AS pr ON pr.id=p.player_id").
		Where("p.scene=? AND p.session_hash<>?", constUtil.GGOFieldScene, "").
		Where("(pr.server_id=? OR (?=? AND pr.server_id<=0))", s.OriginalServerID(), s.OriginalServerID(), constUtil.GameDefaultServerID)
	if line > 0 {
		query = query.Where("p.line=?", line)
	}
	var policy db.ClientUpdatePolicy
	if clientUpdatesEnabled() {
		var err error
		policy, err = clientUpdatePolicy(ctx)
		if err != nil {
			return err
		}
		query = query.Select("p.player_id, p.session_hash AS presence_hash, pr.session, pr.account_id, pr.account_auth_revision AS profile_account_revision, cu.player_id AS update_player_id, cu.session_hash AS update_hash, cu.build_code, cu.login_at, cu.disconnected_at, cu.maintenance_epoch, cu.connected").Joins("LEFT JOIN client_update_sessions AS cu ON cu.player_id=p.player_id")
	}
	for offset := 0; offset < len(ids); offset += 512 {
		end := min(offset+512, len(ids))
		var recipients []ggoNoticeRecipient
		if err := query.Session(&gorm.Session{}).Where("p.player_id IN ?", ids[offset:end]).Order("p.player_id").Scan(&recipients).Error; err != nil {
			return err
		}
		accountIDs := make([]int64, 0, len(recipients))
		unique := map[int64]bool{}
		for _, recipient := range recipients {
			if recipient.AccountID > 0 && !unique[recipient.AccountID] {
				accountIDs = append(accountIDs, recipient.AccountID)
				unique[recipient.AccountID] = true
			}
		}
		accounts := map[int64]db.GameAccount{}
		if len(accountIDs) > 0 {
			var rows []db.GameAccount
			if err := db.AccountsTable(tx, "game_accounts").Select("id", "enabled", "auth_revision").Where("id IN ?", accountIDs).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				accounts[row.ID] = row
			}
		}
		for _, recipient := range recipients {
			v, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), recipient.PlayerID})
			if !ok {
				continue
			}
			sink := v.(*p10ClientSink)
			if len(sink.session) != 16 || subtle.ConstantTimeCompare(sink.session, recipient.Session) != 1 || recipient.PresenceHash != p6Digest(sink.session) {
				continue
			}
			if recipient.AccountID > 0 {
				account, found := accounts[recipient.AccountID]
				if !found || !account.Enabled || account.AuthRevision != recipient.ProfileAccountRevision {
					continue
				}
			}
			if clientUpdatesEnabled() {
				binding := db.ClientUpdateSession{}
				if recipient.UpdatePlayerID != nil && recipient.UpdateHash != nil && *recipient.UpdateHash == clientUpdateSessionHash(sink.session) {
					binding.PlayerID = *recipient.UpdatePlayerID
					if recipient.BuildCode != nil {
						binding.BuildCode = *recipient.BuildCode
					}
					if recipient.LoginAt != nil {
						binding.LoginAt = *recipient.LoginAt
					}
					if recipient.DisconnectedAt != nil {
						binding.DisconnectedAt = *recipient.DisconnectedAt
					}
					if recipient.MaintenanceEpoch != nil {
						binding.MaintenanceEpoch = *recipient.MaintenanceEpoch
					}
					if recipient.Connected != nil {
						binding.Connected = *recipient.Connected
					}
				}
				if clientUpdateRunError(policy, binding) != nil || binding.BuildCode < policy.MinLoginBuild && (binding.PlayerID == 0 || binding.LoginAt > policy.LoginGateAt || !binding.Connected && (binding.DisconnectedAt == 0 || binding.DisconnectedAt < policy.LoginGateAt || time.Now().UnixMilli()-binding.DisconnectedAt > constUtil.ClientUpdateReconnectMS)) {
					continue
				}
			}
			key := p10ClientKey{s.p10ClientOwner(), recipient.PlayerID}
			if current, ok := p10LiveClient(key); !ok || current != sink {
				continue
			}
			if err := sink.send(&pb.SCFieldResourceInfo{}); err != nil {
				s.p10RetireFailedPush(ctx, recipient.PlayerID, sink)
			}
		}
	}
	return nil
}
