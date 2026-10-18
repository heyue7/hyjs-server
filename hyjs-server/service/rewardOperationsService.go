package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	"io"
	"os"
	"sync"
	"time"
)

// 进程版本仅在首次初始化读取，运维查询不会反复读取文件或误报磁盘新版本。
var p1Process struct {
	once         sync.Once
	startedAt    int64
	binarySHA256 string
	err          error
}

func initializeP1Runtime() error {
	p1Process.once.Do(func() {
		executable, err := os.Executable()
		if err != nil {
			p1Process.err = err
			return
		}
		file, err := os.Open(executable)
		if err != nil {
			p1Process.err = err
			return
		}
		defer file.Close()
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err != nil {
			p1Process.err = err
			return
		}
		p1Process.binarySHA256 = hex.EncodeToString(hash.Sum(nil))
		p1Process.startedAt = time.Now().UnixMilli()
	})
	return p1Process.err
}

// P1Operations 只读查询有限运行记录，供本机运维入口使用，不接受原始SQL或返回敏感存档。
func (s *GameService) P1Operations(ctx context.Context, playerID int64, instanceID string) (*model.P1OperationsSnapshot, error) {
	if playerID < 0 || len(instanceID) > 64 {
		return nil, errors.New("运维筛选参数无效")
	}
	tx := db.GetIns().WithContext(ctx)
	now := time.Now().UnixMilli()
	result := &model.P1OperationsSnapshot{PlayerID: playerID, InstanceID: instanceID, BinarySHA256: p1Process.binarySHA256, StartedAt: p1Process.startedAt, Status: "ready", Database: tx.Dialector.Name(), ServerTime: now}
	rewards := func() *gorm.DB {
		q := tx.Model(&db.RewardDelivery{})
		if playerID > 0 {
			q = q.Where("player_id = ?", playerID)
		}
		if instanceID != "" {
			q = q.Where("source_key IN ?", []string{"activity:" + instanceID, "rank:" + instanceID})
		}
		return q
	}
	if err := rewards().Where("claimed_at = 0 AND (expires_at = 0 OR expires_at > ?)", now).Count(&result.PendingRewards).Error; err != nil {
		return nil, err
	}
	if err := rewards().Where("claimed_at > 0").Count(&result.ClaimedRewards).Error; err != nil {
		return nil, err
	}
	if err := rewards().Where("claimed_at = 0 AND expires_at > 0 AND expires_at <= ?", now).Count(&result.ExpiredRewards).Error; err != nil {
		return nil, err
	}
	mail := tx.Model(&db.MailMessage{}).Where("deleted_at = 0 AND read_at = 0 AND (expires_at = 0 OR expires_at > ?)", now)
	if playerID > 0 {
		mail = mail.Where("player_id = ?", playerID)
	}
	if err := mail.Count(&result.UnreadMail).Error; err != nil {
		return nil, err
	}
	facts := tx.Model(&db.EventFact{})
	if playerID > 0 {
		facts = facts.Where("player_id = ?", playerID)
	}
	if err := facts.Count(&result.EventFacts).Error; err != nil {
		return nil, err
	}
	periods := tx.Model(&db.PeriodSettlement{})
	if instanceID != "" {
		periods = periods.Where("instance_id = ?", instanceID)
	}
	if err := periods.Count(&result.SettledPeriods).Error; err != nil {
		return nil, err
	}
	if err := rewards().Select("id,player_id,source_key,created_at,expires_at,claimed_at").Order("id DESC").Limit(20).Find(&result.Recent).Error; err != nil {
		return nil, err
	}
	return result, nil
}
