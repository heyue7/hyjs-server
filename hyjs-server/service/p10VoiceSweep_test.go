package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// TestP10VoiceSweepBounded 多于一批的真实上传PCM在禁用状态仍回收，取消不半改，未来对象保留，可并发冷重试。
func TestP10VoiceSweepBoundedActualUploadsCancellationConcurrentAndCold(t *testing.T) {
	f, _, _ := p10VoiceSetup(t)
	p10VoiceEnable(t)
	session := p10VoiceSession(t, f.id)
	raw := p10VoiceTone(200)
	total := constUtil.P10VoiceSweepBatchSize + 7
	var ids []string
	for i := 0; i < total; i++ {
		upload, err := f.s.UploadP10Voice(f.ctx, f.id, session, 3, fmt.Sprintf("sweep-real-%d", i), "person", raw)
		if err != nil {
			t.Fatal("actual uploaded PCM", i, err)
		}
		var asset db.P10VoiceAsset
		if err := db.GetIns().First(&asset, "file_name=?", upload.FileName).Error; err != nil {
			t.Fatal(err)
		}
		// 隔离运营时刻准备只提前对象到期，不替换实际上传字节、所有权或原文件名。
		asset.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
		asset.MetaHash = p10VoiceAssetHash(&asset)
		if err := db.GetIns().Save(&asset).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, asset.ID)
	}
	future, err := f.s.UploadP10Voice(f.ctx, f.id, session, 3, "sweep-keep-future", "person", raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10VoicePolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if err := f.s.TickP10Voice(ctx, time.Now()); err != context.Canceled {
		t.Fatal("canceled sweep", err)
	}
	var count int64
	db.GetIns().Model(&db.P10VoiceAsset{}).Where("id IN ? AND state=?", ids, constUtil.P10VoiceUploaded).Count(&count)
	if count != int64(total) {
		t.Fatal("canceled sweep changed blobs", count)
	}
	workers := 1
	if f.mysql.Database != "" {
		workers = 2
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.TickP10Voice(f.ctx, time.Now()) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent sweep", err)
		}
	}
	var expired []db.P10VoiceAsset
	if err := db.GetIns().Where("id IN ?", ids).Find(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if len(expired) != total {
		t.Fatal("lost tombstone")
	}
	for _, row := range expired {
		if row.State != constUtil.P10VoiceDeleted || len(row.Bytes) != 0 || row.MetaHash != p10VoiceAssetHash(&row) {
			t.Fatal("expired source not purged", row.ID)
		}
	}
	var retained db.P10VoiceAsset
	if err := db.GetIns().First(&retained, "file_name=?", future.FileName).Error; err != nil {
		t.Fatal(err)
	}
	if retained.State != constUtil.P10VoiceUploaded || p6Digest(retained.Bytes) != future.SHA256 {
		t.Fatal("future object purged")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.TickP10Voice(f.ctx, time.Now()); err != nil {
		t.Fatal("cold idempotent sweep", err)
	}
	if err := db.GetIns().First(&retained, "file_name=?", future.FileName).Error; err != nil || p6Digest(retained.Bytes) != future.SHA256 {
		t.Fatal("cold future PCM changed", err)
	}
}
