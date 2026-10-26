package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFPlayerRecoveryHash(row *db.P13UnionFightPlayerRecovery) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFPlayerRecoveryID(row *db.P13UnionFightPlayerRecovery) string {
	return p6Digest([]byte(fmt.Sprintf("%s/%d/%s/%s/%d/%d/%d/%s/recovery", row.MatchID, row.PlayerID, row.Kind, row.ItemID, row.Stamp, row.EffectGUID, row.LogicalAt, row.Nonce)))
}

// The producer stores the exact frame immediately around life mutation,
// before changing inventory/cooldowns owned by the same original transaction.
func p13UFPlayerRecoveryVerify(row *db.P13UnionFightPlayerRecovery) (*pb.InstUFPlayer, *pb.InstUFPlayer, error) {
	reject := func() (*pb.InstUFPlayer, *pb.InstUFPlayer, error) {
		return nil, nil, errors.New(constUtil.P13UFRecoveryConflict)
	}
	if row == nil || row.MatchID == "" || row.PlayerID <= 0 || row.CasterID <= 0 || row.Stamp < 0 || row.ID != p13UFPlayerRecoveryID(row) || row.SourceHash != p13UFPlayerRecoveryHash(row) || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || len(row.OfferHash) != 64 || row.LogicalAt <= 0 || row.AppliedAt < row.LogicalAt || row.HPPercent < 1 || row.HPPercent > 100 || row.DeadOrdinal < 0 {
		return reject()
	}
	if row.PendingID != "" && (len(row.PendingID) != 64 || row.Kind != constUtil.P13UFRecoveryAura) {
		return reject()
	}
	switch row.Kind {
	case constUtil.P13UFRecoveryItem:
		if row.CasterID != row.PlayerID || row.EffectGUID != 0 || row.ItemID != "ufitem_011" && row.ItemID != "ufitem_012" && row.ItemID != "ufitem_016" || row.InventoryBefore < 1 || row.InventoryBefore > 5 || row.InventoryAfter != row.InventoryBefore-1 {
			return reject()
		}
		if row.Stamp == 0 {
			if len(row.Nonce) != 32 {
				return reject()
			}
			if _, err := hex.DecodeString(row.Nonce); err != nil {
				return reject()
			}
		} else if row.Nonce != "" {
			return reject()
		}
	case constUtil.P13UFRecoveryAura:
		if row.EffectGUID <= 0 || row.ItemID != "ufitem_013" || row.InventoryBefore != 0 || row.InventoryAfter != 0 || row.Stamp != 0 || row.Nonce != "" {
			return reject()
		}
	default:
		return reject()
	}
	before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
	if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || before.GetGuid() != row.PlayerID || after.GetGuid() != row.PlayerID || int64(before.GetDeadNum()) != row.DeadOrdinal || after.GetDeadNum() != before.GetDeadNum() || len(before.GetFightInfo().GetIndex()) > 4 || !combatFinite(before.GetFightInfo().GetAnger()) || before.GetFightInfo().GetAnger() < 0 || before.GetFightInfo().GetAnger() > 300 {
		return reject()
	}
	expected := proto.Clone(before).(*pb.InstUFPlayer)
	if err := p13UFRecoveryLife(expected, row); err != nil {
		return reject()
	}
	if !proto.Equal(expected, after) {
		return reject()
	}
	return before, after, nil
}

func p13UFPlayerRecoveryRecord(tx *gorm.DB, before, after *pb.InstUFPlayer, evidence db.P13UnionFightPlayerRecovery) error {
	if closed, closedErr := p13UFWorldClosed(tx, evidence.MatchID); closedErr != nil {
		return closedErr
	} else if closed {
		return errors.New(constUtil.P13UFRecoveryConflict)
	}
	return p13UFPlayerRecoveryRecordOpen(tx, before, after, evidence)
}

// Caller already holds the guild gate and has performed one current-read
// closed check for the whole actor batch. No per-target source lookup is needed.
func p13UFPlayerRecoveryRecordOpen(tx *gorm.DB, before, after *pb.InstUFPlayer, evidence db.P13UnionFightPlayerRecovery) error {
	var err error
	if before == nil || after == nil {
		return errors.New(constUtil.P13UFRecoveryConflict)
	}
	evidence.PlayerID = before.GetGuid()
	evidence.DeadOrdinal = int64(before.GetDeadNum())
	if evidence.Kind == constUtil.P13UFRecoveryItem && evidence.Stamp == 0 {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return err
		}
		evidence.Nonce = hex.EncodeToString(nonce[:])
	}
	evidence.Before, err = proto.MarshalOptions{Deterministic: true}.Marshal(before)
	if err != nil {
		return err
	}
	evidence.After, err = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	if err != nil {
		return err
	}
	evidence.BeforeHash, evidence.AfterHash = p6Digest(evidence.Before), p6Digest(evidence.After)
	evidence.ID = p13UFPlayerRecoveryID(&evidence)
	evidence.SourceHash = p13UFPlayerRecoveryHash(&evidence)
	if _, _, err = p13UFPlayerRecoveryVerify(&evidence); err != nil {
		return err
	}
	return tx.Create(&evidence).Error
}

// Mining state belongs to the resource lease. New aura events may heal its
// living HP map, while preserving the complete state and collection clock.
func p13UFRecoveryLife(frame *pb.InstUFPlayer, row *db.P13UnionFightPlayerRecovery) error {
	if row.PendingID != "" && row.Kind == constUtil.P13UFRecoveryAura && row.ItemID == "ufitem_013" && frame.GetState() == 2 {
		frame.State = proto.Int32(0)
		err := p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: row.ItemID, HPPercent: row.HPPercent})
		frame.State = proto.Int32(2)
		return err
	}
	return p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: row.ItemID, HPPercent: row.HPPercent})
}
