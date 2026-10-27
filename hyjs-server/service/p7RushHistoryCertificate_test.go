package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func TestP7RushHistoryCertificateRejectsForgeryAndWrongScope(t *testing.T) {
	r := &p7RushCertificateRuntime{key: []byte("dedicated-test-key-with-32-bytes!!"), validator: "binary-v1"}
	raw, _ := json.Marshal(&model.CombatOutcome{Authoritative: true, Winner: 0})
	record := &db.P6BattleRecord{PlayerID: 24, SourceFight: "fight-a", Source: constUtil.P7RushReplaySource, Status: constUtil.P6BattleComputed, Authoritative: true, Outcome: raw, OutcomeHash: p6Digest(raw)}
	c := db.P7RushHistoryCertificate{ServerID: 1, PlayerID: 24, FightID: "fight-a", Format: constUtil.P7RushHistoryCertificateFormat, Validator: r.validator, Fingerprint: p6Digest([]byte("complete actual sources")), OutcomeHash: record.OutcomeHash, VerifiedAt: time.Now().UnixMilli()}
	c.MAC = r.sign(c)
	valid := func(value db.P7RushHistoryCertificate, realm, owner int64, fingerprint string) bool {
		return r.accepted(value, realm, owner, "fight-a", fingerprint, record) != nil
	}
	if !valid(c, 1, 24, c.Fingerprint) {
		t.Fatal("authentic certificate rejected")
	}
	for _, field := range []string{"mac", "version", "fingerprint", "outcome", "fight", "owner", "realm", "format", "time", "winner"} {
		t.Run(field, func(t *testing.T) {
			bad := c
			switch field {
			case "mac":
				bad.MAC = p6Digest([]byte("forged"))
			case "version":
				bad.Validator = "other-binary"
			case "fingerprint":
				bad.Fingerprint = p6Digest([]byte("other-source"))
			case "outcome":
				bad.OutcomeHash = p6Digest([]byte("other-outcome"))
			case "fight":
				bad.FightID = "fight-b"
			case "owner":
				bad.PlayerID++
			case "realm":
				bad.ServerID++
			case "format":
				bad.Format++
			case "winner":
				bad.Winner++
			case "time":
				bad.VerifiedAt++
			}
			if valid(bad, 1, 24, c.Fingerprint) {
				t.Fatal("forged certificate accepted")
			}
		})
	}
	if valid(c, 2, 24, c.Fingerprint) || valid(c, 1, 25, c.Fingerprint) || valid(c, 1, 24, p6Digest([]byte("changed source"))) {
		t.Fatal("cross-scope certificate accepted")
	}
	record.Outcome = append(record.Outcome, ' ')
	if valid(c, 1, 24, c.Fingerprint) {
		t.Fatal("actual outcome tamper accepted")
	}
}
