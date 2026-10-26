package service

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// P7ElitePage bounds catalogue work before reading stage blobs. Only the chosen
// enemy needs battle-mechanism admission; pending rows never claim to be open.
func (s *GameService) P7ElitePage(ctx context.Context, id int64, session []byte, offset *int64, limit int64, bossID, selectedID string) (*model.P7ElitePageResponse, error) {
	owner, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	ctx, err = s.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	ctx = p7WithRushHistoricalQuery(p7WithRushReadRequestCache(ctx))
	out := &model.P7ElitePageResponse{NextOffset: -1, PendingIDs: []string{}}
	out.SelectedBossID = selectedID
	if offset != nil {
		out.Offset = *offset
	}
	if bossID != "" {
		out.Offset = -1
		out.SelectedBossID = bossID
	}
	err = updateProfileForP7QueryWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var current db.Profile
		if e := tx.Select("id", "session", "account_id", "account_auth_revision").First(&current, id).Error; e != nil {
			return e
		}
		if subtle.ConstantTimeCompare(current.Session, session) != 1 || current.AccountID != owner.AccountID || current.AccountAuthRevision != owner.AccountAuthRevision {
			return errors.New(constUtil.NoviceGuideSessionInvalid)
		}
		if current.AccountID > 0 {
			account, e := AccountForProfile(tx, &current)
			if e != nil {
				return e
			}
			if !account.Enabled || account.AuthRevision != current.AccountAuthRevision {
				return errors.New(constUtil.NoviceGuideSessionInvalid)
			}
		}
		cfg, e := s.p7ReadPolicy(tx)
		if e != nil {
			return e
		}
		now := time.Now()
		state, e := p7State(tx, id, now)
		if e != nil {
			return e
		}
		authority, e := p7AuthorityAvailable(tx)
		if e != nil {
			return e
		}
		free, e := s.p7VIP(p, "numPerks24")
		if e != nil {
			return e
		}
		paid, e := s.p7VIP(p, "numPerks25")
		if e != nil {
			return e
		}
		if state.EliteBought > paid || state.EliteUsed > free+state.EliteBought {
			return errors.New("精英日次数存档无效")
		}
		packet := &pb.SCGetRushEliteBossInfo{LeftTime: proto.Int32(free + state.EliteBought - state.EliteUsed), TotalTime: proto.Int32(free + state.EliteBought), DiamondLeftTime: proto.Int32(paid - state.EliteBought), DiamondLeftTimeCost: proto.Int32(cfg.EliteBuyCost)}
		query := tx.Model(&db.P7RushStageRule{}).Where("kind=?", constUtil.P7RushElite)
		if e = query.Count(&out.Total).Error; e != nil {
			return e
		}
		var stages []db.P7RushStageRule
		if bossID != "" {
			e = tx.Where("kind=? AND task_id=?", constUtil.P7RushElite, bossID).Order("task_id,id").Limit(1).Find(&stages).Error
			if e == nil && len(stages) == 0 {
				return errors.New(constUtil.P7RushContentMissing)
			}
		} else {
			e = tx.Where("kind=?", constUtil.P7RushElite).Order("task_id,id").Offset(int(out.Offset)).Limit(int(limit)).Find(&stages).Error
			if out.Offset+int64(len(stages)) < out.Total {
				out.NextOffset = out.Offset + int64(len(stages))
			}
		}
		if e != nil {
			return e
		}
		projection, e := p7NewEliteQueryProjection(tx, state)
		if e != nil {
			return e
		}
		projection.serverID = s.OriginalServerID()
		candidates := map[string]bool{}
		for _, stage := range stages {
			first, e := p1RewardResources(stage.FirstReward)
			if e != nil {
				return e
			}
			general, e := p1RewardResources(stage.Reward)
			if e != nil {
				return e
			}
			passed, e := projection.prerequisite(s, stage)
			if e != nil {
				return e
			}
			if cfg.Enabled && stage.Enabled && passed && p.GetLevel() >= stage.MinLevel {
				if _, e = s.validateP7Stage(stage); e == nil {
					candidates[stage.TaskID] = true
					if selectedID == "" && bossID == "" {
						out.SelectedBossID = stage.TaskID
					}
				}
			}
			won := projection.progress[constUtil.P7RushElite][stage.TaskID]
			if won {
				first = nil
			}
			packet.BossInfoList = append(packet.BossInfoList, &pb.RushEliteBossInfo{EliteTaskId: proto.String(stage.TaskID), IsOpen: proto.Int32(0), FirstRewardList: first, GeneralRewardList: general, IsSuccess: proto.Int32(0)})
			if won {
				packet.BossInfoList[len(packet.BossInfoList)-1].IsSuccess = proto.Int32(1)
			}
		}
		if out.SelectedBossID == "" && len(stages) > 0 {
			out.SelectedBossID = stages[0].TaskID
		}
		for i, stage := range stages {
			if !candidates[stage.TaskID] {
				continue
			}
			if authority {
				packet.BossInfoList[i].IsOpen = proto.Int32(1)
			} else if stage.TaskID == out.SelectedBossID {
				allowed, e := projection.scopeAvailable(s, ctx, p, g, stage, now)
				if e != nil {
					return e
				}
				if allowed {
					packet.BossInfoList[i].IsOpen = proto.Int32(1)
				}
			} else {
				out.PendingIDs = append(out.PendingIDs, stage.TaskID)
			}
		}
		raw, e := proto.MarshalOptions{Deterministic: true}.Marshal(packet)
		if e != nil {
			return e
		}
		out.Packet = base64.StdEncoding.EncodeToString(raw)
		return nil
	})
	return out, err
}
