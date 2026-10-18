package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestP14QualificationActualAutoFederation 从真实3人报名和autoPass生产证书，再冷连接另一独立库验证互信导入。
func TestP14QualificationActualAutoFederationStrictProofTwoRealmColdAndCorruption(t *testing.T) {
	f, now := p14AuditionMatchSetup(t, 3)
	if err := f.s.InitializeP14Qualification(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.PrepareP14AuditionRound(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := f.s.QueueP14Qualifications(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P14QualificationDelivery{}).Count(&count)
	if count != 0 {
		t.Fatal("default federation open")
	}
	if err := db.GetIns().Model(&db.P14QualificationFederationPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := f.s.QueueP14Qualifications(f.ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	var delivery db.P14QualificationDelivery
	if err := db.GetIns().First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.P14QualificationDelivery{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate source outbox")
	}
	var request model.P14QualificationImportRequest
	if json.Unmarshal(delivery.Payload, &request) != nil || delivery.PayloadHash != p6Digest(delivery.Payload) {
		t.Fatal("actual frozen payload")
	}
	var peer db.P14TrustedPeer
	if err := db.GetIns().First(&peer, "server_id=2").Error; err != nil {
		t.Fatal(err)
	}
	key := append([]byte(nil), peer.SigningKey...)
	sourceMysql := f.mysql
	sourcePath := f.path
	sql, _ := db.GetIns().DB()
	sql.Close()
	if sourceMysql.Database != "" {
		name := os.Getenv("HYJS_P14_QUALIFICATION_FOREIGN_DATABASE")
		if name == "" {
			t.Fatal("isolated destination schema required")
		}
		f.mysql.Database = name
	} else {
		f.path = filepath.Join(t.TempDir(), "qualified-realm2.db")
	}
	f.open(t)
	f.s.ServerID = 2
	if err := f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP14Qualification(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P14TrustedPeer{ServerID: 1, Enabled: true, Endpoint: "http://127.0.0.1:1", SigningKey: key}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "season_index": 7}).Error; err != nil {
		t.Fatal(err)
	}
	signature := P14SignPayload(key, delivery.Payload)
	if _, err := f.s.ImportP14Qualification(f.ctx, request, delivery.Payload, signature, now); err == nil {
		t.Fatal("default destination federation open")
	}
	if err := db.GetIns().Model(&db.P14QualificationFederationPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ImportP14Qualification(f.ctx, request, delivery.Payload, "00", now); err == nil {
		t.Fatal("bad HMAC accepted")
	}
	changed := request
	changed.DestinationServerID = 1
	bad, _ := json.Marshal(changed)
	if _, err := f.s.ImportP14Qualification(f.ctx, changed, bad, P14SignPayload(key, bad), now); err == nil {
		t.Fatal("wrong destination accepted")
	}
	var qualified db.P14AuditionQualification
	json.Unmarshal(request.Qualification, &qualified)
	changed = request
	qualified.RoundCount = 1
	qualified.SourceHash = p14AuditionQualificationHash(&qualified)
	changed.Qualification, _ = json.Marshal(qualified)
	changed.DeliveryID = p14QualificationID(changed, qualified.SourceHash)
	bad, _ = json.Marshal(changed)
	if _, err := f.s.ImportP14Qualification(f.ctx, changed, bad, P14SignPayload(key, bad), now); err == nil {
		t.Fatal("played results without authority proof accepted")
	}
	workers := 1
	if f.mysql.Database != "" {
		workers = 8
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply, err := f.s.ImportP14Qualification(f.ctx, request, delivery.Payload, signature, now)
			if err == nil && (!reply.Confirmed || reply.DeliveryID != request.DeliveryID) {
				t.Error("wrong exact receipt")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	db.GetIns().Model(&db.P14ForeignQualification{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate foreign certificate")
	}
	rows, err := f.s.ReadP14ForeignQualifiedEntrants(f.ctx, db.GetIns(), 7, now)
	if err != nil || len(rows) != 3 {
		t.Fatal("actual foreign frozen entrants", err)
	}
	for _, row := range rows {
		if row.ServerID != 1 || row.EngineVersion != constUtil.P6EngineVersion {
			t.Fatal("collapsed original realm identity/version")
		}
	}
	db.GetIns().Model(&db.P14Member{}).Count(&count)
	if count != 0 {
		t.Fatal("foreign certificate invented local registration")
	}
	changed = request
	json.Unmarshal(request.Qualification, &qualified)
	qualified.QualifiedAt++
	qualified.SourceHash = p14AuditionQualificationHash(&qualified)
	changed.Qualification, _ = json.Marshal(qualified)
	changed.DeliveryID = p14QualificationID(changed, qualified.SourceHash)
	bad, _ = json.Marshal(changed)
	if _, err := f.s.ImportP14Qualification(f.ctx, changed, bad, P14SignPayload(key, bad), now.Add(time.Millisecond)); err == nil {
		t.Fatal("changed cutoff certificate replaced real source")
	}
	sql, _ = db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP14Qualification(f.ctx); err != nil {
		t.Fatal(err)
	}
	cold, err := f.s.ReadP14ForeignQualifiedEntrants(f.ctx, db.GetIns(), 7, now)
	if err != nil || len(cold) != 3 || cold[0].SourceHash != rows[0].SourceHash {
		t.Fatal("cold source changed", err)
	}
	if err := db.GetIns().Model(&db.P14ForeignQualification{}).Where("season_index=7 AND server_id=1").Update("payload", []byte("corrupted")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReadP14ForeignQualifiedEntrants(f.ctx, db.GetIns(), 7, now); err == nil {
		t.Fatal("corrupt signed storage read")
	}
	sql, _ = db.GetIns().DB()
	sql.Close()
	f.mysql = sourceMysql
	f.path = sourcePath
	f.open(t)
	f.s.ServerID = 1
	var sourceAfter db.P14QualificationDelivery
	if err := db.GetIns().First(&sourceAfter, "id=?", delivery.ID).Error; err != nil || !bytes.Equal(sourceAfter.Payload, delivery.Payload) {
		t.Fatal("source outbox changed by importer", err)
	}
}
