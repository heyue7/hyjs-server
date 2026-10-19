package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeAdminCDK adds policy beside the original code/claim tables without issuing gifts.
func (s *GameService) InitializeAdminCDK(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminCDKCode{})
}
func (s *GameService) adminCDKScope(ctx context.Context, tx *gorm.DB, actor *model.AdminPrincipal, write bool) (int64, error) {
	if err := adminOperationUser(tx, actor, write); err != nil {
		return 0, err
	}
	id := AdminSelectedServerID(ctx)
	if id != s.OriginalServerID() {
		return 0, constUtil.ErrAdminPermission
	}
	// Keep permission and realm lookup on the caller's transaction connection.
	var realm db.AdminGameServer
	if err := tx.First(&realm, "id=?", id).Error; err != nil {
		return 0, err
	}
	return id, nil
}
func adminCDKProjection(meta db.AdminCDKCode, code db.PaymentGiftCode) model.AdminCDKDTO {
	return model.AdminCDKDTO{ID: meta.Hash, Name: meta.Name, CodeHint: meta.CodeHint, Enabled: meta.Enabled, Unlimited: meta.Unlimited, StartAt: meta.StartAt, EndAt: meta.EndAt, TotalLimit: code.Limit, Used: code.Used, Rewards: adminTypedResources(code.Reward), Version: adminDigest(string(adminJSON(struct {
		Meta db.AdminCDKCode
		Code db.PaymentGiftCode
	}{meta, code}))), CreatedAt: meta.CreatedAt}
}

// AdminCDKList reads the selected realm's operator-generated codes, never plaintext.
func (s *GameService) AdminCDKList(ctx context.Context, actor *model.AdminPrincipal, q string, page, size int64) (*model.AdminCDKListResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	id, err := s.adminCDKScope(ctx, tx, actor, false)
	if err != nil {
		return nil, err
	}
	query := tx.Model(&db.AdminCDKCode{}).Where("server_id=?", id)
	if q != "" {
		query = query.Where("name LIKE ? OR code_hint LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	out := &model.AdminCDKListResponse{Items: []model.AdminCDKDTO{}}
	if err = query.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.AdminCDKCode
	if err = query.Order("created_at DESC,hash").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, meta := range rows {
		var code db.PaymentGiftCode
		if err = tx.First(&code, "hash=?", meta.Hash).Error; err != nil {
			return nil, err
		}
		out.Items = append(out.Items, adminCDKProjection(meta, code))
	}
	return out, nil
}

// AdminCreateCDK fixes the original reward/limit and returns plaintext once after an atomic batch commit.
func (s *GameService) AdminCreateCDK(ctx context.Context, actor *model.AdminPrincipal, req model.AdminCDKCreateRequest) (*model.AdminCDKCreateResponse, error) {
	reward, err := s.adminTypedResourceSpec(req.Rewards, false)
	if err != nil {
		return nil, err
	}
	out := &model.AdminCDKCreateResponse{Items: []model.AdminCDKDTO{}, Codes: []string{}}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		id, e := s.adminCDKScope(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		now := time.Now().UnixMilli()
		for i := int64(0); i < req.Count; i++ {
			plaintext := req.Code
			if plaintext == "" {
				buf := make([]byte, 12)
				if _, e = rand.Read(buf); e != nil {
					return e
				}
				plaintext = req.Prefix + strings.ToUpper(hex.EncodeToString(buf))
			}
			hash := p3Hash([]byte(plaintext))
			expires := req.EndAt
			if req.Unlimited {
				expires = math.MaxInt64
			}
			code := db.PaymentGiftCode{Hash: hash, Reward: reward, ExpiresAt: expires, Limit: req.TotalLimit, Source: fmt.Sprintf("admin-cdk:%d", id)}
			// A duplicate manual code fails the entire batch instead of overwriting a redeemed code.
			var count int64
			if e = tx.Model(&db.PaymentGiftCode{}).Where("hash=?", hash).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				return constUtil.ErrAdminConfigConflict
			}
			if e = tx.Create(&code).Error; e != nil {
				return e
			}
			hint := "****"
			if len(plaintext) > 8 {
				hint = "…" + plaintext[len(plaintext)-4:]
			} else if len(plaintext) > 4 {
				hint = "…" + plaintext[len(plaintext)-2:]
			}
			meta := db.AdminCDKCode{Hash: hash, ServerID: id, Name: req.Name, CodeHint: hint, Enabled: req.Enabled, Unlimited: req.Unlimited, StartAt: req.StartAt, EndAt: req.EndAt, Revision: 1, CreatedAt: now, UpdatedAt: now}
			if e = tx.Create(&meta).Error; e != nil {
				return e
			}
			out.Codes = append(out.Codes, plaintext)
			out.Items = append(out.Items, adminCDKProjection(meta, code))
		}
		return adminAudit(tx, actor, "cdk.create", fmt.Sprint(id), fmt.Sprintf("%s; count=%d; %s", req.Name, req.Count, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdminUpdateCDK serializes with redemption by locking the original code before policy.
func (s *GameService) AdminUpdateCDK(ctx context.Context, actor *model.AdminPrincipal, hash string, req model.AdminCDKUpdateRequest) (*model.AdminCDKDTO, error) {
	reward, err := s.adminTypedResourceSpec(req.Rewards, false)
	if err != nil {
		return nil, err
	}
	var out model.AdminCDKDTO
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		id, e := s.adminCDKScope(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		var code db.PaymentGiftCode
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&code, "hash=?", hash).Error; e != nil {
			return e
		}
		var meta db.AdminCDKCode
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&meta, "hash=? AND server_id=?", hash, id).Error; e != nil {
			return e
		}
		if adminCDKProjection(meta, code).Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.TotalLimit > 0 && req.TotalLimit < code.Used || code.Used > 0 && reward != code.Reward {
			return constUtil.ErrAdminConfigConflict
		}
		code.Reward = reward
		code.Limit = req.TotalLimit
		code.ExpiresAt = req.EndAt
		if req.Unlimited {
			code.ExpiresAt = math.MaxInt64
		}
		meta.Name = req.Name
		meta.Enabled = req.Enabled
		meta.Unlimited = req.Unlimited
		meta.StartAt = req.StartAt
		meta.EndAt = req.EndAt
		meta.Revision++
		meta.UpdatedAt = time.Now().UnixMilli()
		if e = tx.Save(&code).Error; e != nil {
			return e
		}
		if e = tx.Save(&meta).Error; e != nil {
			return e
		}
		out = adminCDKProjection(meta, code)
		return adminAudit(tx, actor, "cdk.update", hash, req.Reason)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminCDKClaims reads the original lifetime claim ledger; no duplicate redemption table is introduced.
func (s *GameService) AdminCDKClaims(ctx context.Context, actor *model.AdminPrincipal, hash string, page, size int64) (*model.AdminCDKClaimsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	id, err := s.adminCDKScope(ctx, tx, actor, false)
	if err != nil {
		return nil, err
	}
	var meta db.AdminCDKCode
	if err = tx.First(&meta, "hash=? AND server_id=?", hash, id).Error; err != nil {
		return nil, err
	}
	query := tx.Model(&db.MembershipClaim{}).Where("kind=? AND period=?", "gift:"+hash, "lifetime")
	out := &model.AdminCDKClaimsResponse{Items: []model.AdminCDKClaimDTO{}}
	if err = query.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var claims []db.MembershipClaim
	if err = query.Order("at DESC,player_id").Offset(int((page - 1) * size)).Limit(int(size)).Find(&claims).Error; err != nil {
		return nil, err
	}
	for _, claim := range claims {
		v := model.AdminCDKClaimDTO{PlayerID: claim.PlayerID, ClaimedAt: claim.At}
		var profile db.Profile
		if err = tx.First(&profile, claim.PlayerID).Error; err != nil {
			return nil, err
		}
		if AdminProfileServerID(profile) != id {
			return nil, constUtil.ErrAdminPermission
		}
		if profile.Player != nil {
			var name struct {
				Name string `json:"name"`
			}
			if err = json.Unmarshal([]byte(*profile.Player), &name); err != nil {
				return nil, err
			}
			v.PlayerName = name.Name
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// adminCDKEligible gates only new operator codes. Historical non-admin gifts retain their original rules.
func (s *GameService) adminCDKEligible(tx *gorm.DB, code db.PaymentGiftCode, now time.Time) error {
	if !strings.HasPrefix(code.Source, "admin-cdk:") {
		return nil
	}
	var meta db.AdminCDKCode
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&meta, "hash=?", code.Hash).Error; err != nil {
		return err
	}
	if !meta.Enabled || meta.ServerID != s.OriginalServerID() || (!meta.Unlimited && (now.UnixMilli() < meta.StartAt || now.UnixMilli() >= meta.EndAt)) {
		return errors.New("兑换码未开放或已过期")
	}
	return nil
}
