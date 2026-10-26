package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFGateImpactHash(row *db.P13UnionFightGateImpact) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// Verify the small immutable transition. Cross-domain source identities are
// checked in bounded batches by the settlement proof consumer.
func p13UFGateImpactVerify(row *db.P13UnionFightGateImpact) (*pb.InstUFGate, *pb.InstUFGate, error) {
	reject := func() (*pb.InstUFGate, *pb.InstUFGate, error) {
		return nil, nil, errors.New(constUtil.P13UFGateImpactConflict)
	}
	if row == nil || row.MatchID == "" || row.GateGUID <= 0 || row.Ordinal <= 0 || row.ID != p6Digest([]byte(fmt.Sprintf("%s/%d/%d/gate-impact", row.MatchID, row.GateGUID, row.Ordinal))) || row.SourceHash != p13UFGateImpactHash(row) || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || len(row.OfferHash) != 64 || row.LogicalAt <= 0 || row.AppliedAt < row.LogicalAt || row.Percent == 0 || row.Percent < -100 || row.Percent > 100 {
		return reject()
	}
	switch row.Kind {
	case constUtil.P13UFGateImpactCar:
		if row.PlayerID != 0 || row.Stamp != 0 || row.ItemID != "" || row.InventoryBefore != 0 || row.InventoryAfter != 0 || row.CarGUID <= 0 || row.HitOrdinal <= 0 || row.Percent >= 0 {
			return reject()
		}
	case constUtil.P13UFGateImpactItem:
		if row.CarGUID != 0 || row.HitOrdinal != 0 || row.PlayerID <= 0 || row.Stamp < 0 || row.InventoryBefore < 1 || row.InventoryBefore > 5 || row.InventoryAfter != row.InventoryBefore-1 || row.ItemID != "ufitem_019" && row.ItemID != "ufitem_020" || row.ItemID == "ufitem_019" && row.Percent <= 0 || row.ItemID == "ufitem_020" && row.Percent >= 0 {
			return reject()
		}
	default:
		return reject()
	}
	before, after := &pb.InstUFGate{}, &pb.InstUFGate{}
	if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || before.GetGuid() != row.GateGUID || after.GetGuid() != row.GateGUID || !combatFinite(before.GetHp()) || !combatFinite(before.GetMaxHp()) || before.GetMaxHp() <= 0 || before.GetHp() < 0 || before.GetHp() > before.GetMaxHp() {
		return reject()
	}
	expected := proto.Clone(before).(*pb.InstUFGate)
	expected.Hp = proto.Float64(min(before.GetMaxHp(), max(float64(0), before.GetHp()+before.GetMaxHp()*float64(row.Percent)/100)))
	if !proto.Equal(expected, after) {
		return reject()
	}
	return before, after, nil
}

// Append under the existing gate/asset transaction before saving the new gate.
// The counter and impact are rolled back together on any later debit failure.
func p13UFGateImpactRecord(tx *gorm.DB, gate *db.P13UnionFightGateState, after *pb.InstUFGate, evidence db.P13UnionFightGateImpact) error {
	before, err := p13UFGateRead(gate)
	if err != nil {
		return err
	}
	if gate.ImpactOrdinal == math.MaxInt64 {
		return errors.New(constUtil.P13UFGateImpactConflict)
	}
	evidence.MatchID, evidence.GateGUID, evidence.Ordinal = gate.MatchID, gate.GUID, gate.ImpactOrdinal+1
	evidence.ID = p6Digest([]byte(fmt.Sprintf("%s/%d/%d/gate-impact", evidence.MatchID, evidence.GateGUID, evidence.Ordinal)))
	evidence.Before, err = proto.MarshalOptions{Deterministic: true}.Marshal(before)
	if err != nil {
		return err
	}
	evidence.After, err = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	if err != nil {
		return err
	}
	evidence.BeforeHash, evidence.AfterHash = p6Digest(evidence.Before), p6Digest(evidence.After)
	evidence.SourceHash = p13UFGateImpactHash(&evidence)
	if _, _, err = p13UFGateImpactVerify(&evidence); err != nil {
		return err
	}
	if err = tx.Create(&evidence).Error; err != nil {
		return err
	}
	gate.ImpactOrdinal = evidence.Ordinal
	return nil
}

type p13UFGateImpactLedgerData struct {
	Impacts []db.P13UnionFightGateImpact
	Gates   []db.P13UnionFightGateState
}

// Read one match directory, then stream bounded immutable transitions. This is
// settlement work, never a movement or Current request. No query runs per gate
// or hit, and the first before-frame must match the frozen original snapshot.
func p13UFGateImpactLedger(tx *gorm.DB, match *db.P13UnionFightMatch) (*p13UFGateImpactLedgerData, error) {
	reject := func() (*p13UFGateImpactLedgerData, error) { return nil, errors.New(constUtil.P13UFGateImpactConflict) }
	result := &p13UFGateImpactLedgerData{}
	if err := tx.Where("match_id=?", match.ID).Order("guid").Find(&result.Gates).Error; err != nil {
		return nil, err
	}
	var count int64
	if err := tx.Model(&db.P13UnionFightGateImpact{}).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	if len(result.Gates) == 0 && count == 0 {
		return result, nil
	}
	if len(result.Gates) != 8 {
		return reject()
	}
	snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
	if err != nil {
		return nil, err
	}
	buildings := &pb.SCUFBuildingRefresh{}
	if proto.Unmarshal(snapshot.Buildings, buildings) != nil || len(buildings.GateInfos) != 8 {
		return reject()
	}
	initial := map[int64]*pb.InstUFGate{}
	for _, frame := range buildings.GateInfos {
		initial[frame.GetGuid()] = frame
	}
	states := map[int64]*db.P13UnionFightGateState{}
	lastHash := map[int64]string{}
	ordinal := map[int64]int64{}
	lastAt := map[int64]int64{}
	for i := range result.Gates {
		state := &result.Gates[i]
		frame, readErr := p13UFGateRead(state)
		first := initial[state.GUID]
		if readErr != nil || first == nil || state.ImpactOrdinal < 0 || frame.GetMaxHp() != first.GetMaxHp() {
			return reject()
		}
		raw, marshalErr := proto.Marshal(first)
		if marshalErr != nil || p6Digest(raw) != state.InitialHash {
			return reject()
		}
		states[state.GUID] = state
		lastHash[state.GUID] = state.InitialHash
	}
	var carOffers []db.P13UnionFightCarAttackOffer
	if err = tx.Where("match_id=?", match.ID).Find(&carOffers).Error; err != nil {
		return nil, err
	}
	carByHash := map[string]*db.P13UnionFightCarAttackOffer{}
	for i := range carOffers {
		o := &carOffers[i]
		if o.SourceHash != p13UFCarAttackOfferHash(o) {
			return reject()
		}
		carByHash[o.SourceHash] = o
	}
	var itemOffers []db.P13UnionFightGateItemOffer
	if err = tx.Where("match_id=?", match.ID).Find(&itemOffers).Error; err != nil {
		return nil, err
	}
	itemByHash := map[string]*db.P13UnionFightGateItemOffer{}
	for i := range itemOffers {
		o := &itemOffers[i]
		if o.SourceHash != p13UFGateItemOfferHash(o) {
			return reject()
		}
		itemByHash[o.SourceHash] = o
	}
	var carClocks []db.P13UnionFightCarAttackState
	if err = tx.Where("match_id=?", match.ID).Find(&carClocks).Error; err != nil {
		return nil, err
	}
	clocks := map[int64]*db.P13UnionFightCarAttackState{}
	for i := range carClocks {
		clock := &carClocks[i]
		if clock.StateHash != p13UFCarAttackStateHash(clock) || clock.Hits < 0 || clocks[clock.CarGUID] != nil {
			return reject()
		}
		clocks[clock.CarGUID] = clock
	}
	carHits := map[int64]int64{}
	cursor := ""
	for {
		var batch []db.P13UnionFightGateImpact
		if err = tx.Where("match_id=? AND id>?", match.ID, cursor).Order("id").Limit(128).Find(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		result.Impacts = append(result.Impacts, batch...)
		cursor = batch[len(batch)-1].ID
	}
	if int64(len(result.Impacts)) != count {
		return reject()
	}
	// Ordinal owns each gate's atomic update order. Independent gates need no
	// invented global ordering when worker transactions share a millisecond.
	sort.Slice(result.Impacts, func(i, j int) bool {
		a, b := result.Impacts[i], result.Impacts[j]
		if a.GateGUID != b.GateGUID {
			return a.GateGUID < b.GateGUID
		}
		return a.Ordinal < b.Ordinal
	})
	for i := range result.Impacts {
		hit := &result.Impacts[i]
		_, after, verifyErr := p13UFGateImpactVerify(hit)
		state := states[hit.GateGUID]
		if verifyErr != nil || state == nil || hit.Ordinal != ordinal[hit.GateGUID]+1 || hit.BeforeHash != lastHash[hit.GateGUID] || hit.LogicalAt < match.StartAt || hit.LogicalAt > match.EndAt || hit.LogicalAt < lastAt[hit.GateGUID] {
			return reject()
		}
		if hit.Kind == constUtil.P13UFGateImpactCar {
			offer, clock := carByHash[hit.OfferHash], clocks[hit.CarGUID]
			if offer == nil || clock == nil || offer.GateGUID != hit.GateGUID || offer.HPPercent != -hit.Percent || clock.OfferHash != hit.OfferHash || hit.HitOrdinal != carHits[hit.CarGUID]+1 {
				return reject()
			}
			carHits[hit.CarGUID] = hit.HitOrdinal
		} else {
			offer := itemByHash[hit.OfferHash]
			if offer == nil || offer.ItemID != hit.ItemID || offer.HPPercent != int64(math.Abs(float64(hit.Percent))) || offer.Enemy != (hit.Percent < 0) {
				return reject()
			}
		}
		if after.GetCamp() != int32(state.Camp) {
			return reject()
		}
		ordinal[hit.GateGUID], lastHash[hit.GateGUID], lastAt[hit.GateGUID] = hit.Ordinal, hit.AfterHash, hit.LogicalAt
	}
	for _, state := range result.Gates {
		if ordinal[state.GUID] != state.ImpactOrdinal || lastHash[state.GUID] != state.FrameHash {
			return reject()
		}
	}
	for _, clock := range carClocks {
		if carHits[clock.CarGUID] != clock.Hits {
			return reject()
		}
	}
	return result, nil
}
