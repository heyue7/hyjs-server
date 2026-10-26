package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"strconv"
)

func p13UFCarImpactError() error { return errors.New(constUtil.P13UFCarImpactConflict) }
func p13UFCarImpactHash(row *db.P13UnionFightCarImpact) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// Movement flags can change independently. Only actual car identity and HP
// compose the cross-producer damage chain; each effect still preserves full PB.
func p13UFCarHPIdentity(car *pb.InstUFCariot) string {
	if car == nil {
		return ""
	}
	raw, _ := json.Marshal(struct {
		GUID, Factory int64
		ID, Camp      int32
		MaxHP         float64
	}{car.GetGuid(), car.GetFactoryGuid(), car.GetCariotId(), car.GetCamp(), car.GetMaxHp()})
	return p6Digest(raw)
}
func p13UFCarImpactVerify(row *db.P13UnionFightCarImpact) (*pb.InstUFCariot, *pb.InstUFCariot, error) {
	reject := func() (*pb.InstUFCariot, *pb.InstUFCariot, error) { return nil, nil, p13UFCarImpactError() }
	if row == nil || len(row.MatchID) != 64 || row.CarGUID <= 0 || row.Ordinal <= 0 || row.ID != p6Digest([]byte(fmt.Sprintf("%s/%d/%d/car-impact", row.MatchID, row.CarGUID, row.Ordinal))) || row.FactoryGUID <= 0 || row.FactoryID < 1 || row.FactoryID > 4 || row.Generation < 1 || row.Generation > 128 || row.BornAt <= 0 || row.PlayerID <= 0 || (row.Camp != 0 && row.Camp != 1) || (row.CarCamp != 0 && row.CarCamp != 1) || row.CarCamp == row.Camp || row.Stamp < 0 || (row.ItemID != "ufitem_017" && row.ItemID != "ufitem_018") || len(row.OfferHash) != 64 || len(row.FactorySourceHash) != 64 || len(row.GenerationSourceHash) != 64 || row.HPPercent < 1 || row.HPPercent > 100 || row.InventoryBefore < 1 || row.InventoryBefore > 5 || row.InventoryAfter != row.InventoryBefore-1 || row.LogicalAt < row.BornAt || row.AppliedAt < row.LogicalAt || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || row.SourceHash != p13UFCarImpactHash(row) {
		return reject()
	}
	before, after := &pb.InstUFCariot{}, &pb.InstUFCariot{}
	if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || before.GetGuid() != row.CarGUID || before.GetFactoryGuid() != row.FactoryGUID || int64(before.GetCamp()) != row.CarCamp || before.GetCariotId() <= 0 || !combatFinite(before.GetHp()) || !combatFinite(before.GetMaxHp()) || before.GetHp() <= 0 || before.GetMaxHp() <= 0 || before.GetHp() > before.GetMaxHp() || row.HPBefore != before.GetHp() || row.HPAfter != after.GetHp() || row.IdentityHash != p13UFCarHPIdentity(before) {
		return reject()
	}
	expected := proto.Clone(before).(*pb.InstUFCariot)
	expected.Hp = proto.Float64(max(float64(0), before.GetHp()-before.GetMaxHp()*(float64(row.HPPercent)/100)))
	if expected.GetHp() >= before.GetHp() || !proto.Equal(expected, after) {
		return reject()
	}
	return before, after, nil
}

// Append before the actual car save. Caller holds the existing policy/target
// transaction, so stamp0 still allocates a real irreversible damage ordinal.
func p13UFCarImpactRecord(tx *gorm.DB, car *db.P13UnionFightFactoryState, after *pb.InstUFCariot, evidence db.P13UnionFightCarImpact) error {
	before, err := p13UFCarItemRead(car)
	if err != nil {
		return err
	}
	evidence.MatchID, evidence.CarGUID, evidence.FactoryGUID, evidence.FactoryID, evidence.CarCamp, evidence.Generation, evidence.BornAt = car.MatchID, before.GetGuid(), car.GUID, car.FactoryID, car.Camp, max(int64(1), car.CarGeneration), car.CompletedAt
	evidence.FactorySourceHash, evidence.GenerationSourceHash = car.SourceHash, car.SourceHash
	if evidence.Generation > 1 {
		var generation db.P13UnionFightCarGeneration
		if err = tx.Select("match_id,car_guid,factory_guid,generation,born_at,source_hash,car_hash").First(&generation, "match_id=? AND car_guid=?", car.MatchID, before.GetGuid()).Error; err != nil {
			return err
		}
		if generation.Generation != evidence.Generation || generation.FactoryGUID != car.GUID || generation.BornAt != car.CompletedAt || generation.CarHash != p6Digest(car.CarFrame) || len(generation.SourceHash) != 64 {
			return p13UFCarImpactError()
		}
		evidence.GenerationSourceHash = generation.SourceHash
	}
	var previous db.P13UnionFightCarImpact
	err = tx.Select("ordinal").Where("match_id=? AND car_guid=?", car.MatchID, before.GetGuid()).Order("ordinal DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if previous.Ordinal == math.MaxInt64 {
		return p13UFCarImpactError()
	}
	evidence.Ordinal = previous.Ordinal + 1
	evidence.ID = p6Digest([]byte(fmt.Sprintf("%s/%d/%d/car-impact", evidence.MatchID, evidence.CarGUID, evidence.Ordinal)))
	evidence.Before, err = proto.MarshalOptions{Deterministic: true}.Marshal(before)
	if err != nil {
		return err
	}
	evidence.After, err = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	if err != nil {
		return err
	}
	evidence.BeforeHash, evidence.AfterHash = p6Digest(evidence.Before), p6Digest(evidence.After)
	evidence.HPBefore, evidence.HPAfter, evidence.IdentityHash = before.GetHp(), after.GetHp(), p13UFCarHPIdentity(before)
	evidence.SourceHash = p13UFCarImpactHash(&evidence)
	if _, _, err = p13UFCarImpactVerify(&evidence); err != nil {
		return err
	}
	return tx.Create(&evidence).Error
}

type P13UnionFightCarImpactMeta struct {
	ID, MatchID, ItemID, OfferHash, FactorySourceHash, GenerationSourceHash, IdentityHash, BeforeHash, AfterHash, SourceHash                                       string
	CarGUID, Ordinal, FactoryGUID, FactoryID, Generation, BornAt, CarCamp, PlayerID, Camp, Stamp, HPPercent, InventoryBefore, InventoryAfter, LogicalAt, AppliedAt int64
	HPBefore, HPAfter                                                                                                                                              float64
}

func P13UnionFightCarImpactMetadata(row *db.P13UnionFightCarImpact) P13UnionFightCarImpactMeta {
	return P13UnionFightCarImpactMeta{row.ID, row.MatchID, row.ItemID, row.OfferHash, row.FactorySourceHash, row.GenerationSourceHash, row.IdentityHash, row.BeforeHash, row.AfterHash, row.SourceHash, row.CarGUID, row.Ordinal, row.FactoryGUID, row.FactoryID, row.Generation, row.BornAt, row.CarCamp, row.PlayerID, row.Camp, row.Stamp, row.HPPercent, row.InventoryBefore, row.InventoryAfter, row.LogicalAt, row.AppliedAt, row.HPBefore, row.HPAfter}
}

type p13UFCarImpactCar struct {
	CarGUID, FactoryGUID, FactoryID, Generation, BornAt, Camp      int64
	MaxHP, CurrentHP                                               float64
	FactorySourceHash, GenerationSourceHash, IdentityHash, CarHash string
}
type p13UFCarImpactLedgerData struct {
	Impacts []db.P13UnionFightCarImpact
	Cars    []p13UFCarImpactCar
}

// This settlement reader validates identity/quote closures in batches. It does
// not assert a bomb-only full-frame chain: tower damage and moving flags may
// intervene, and the mixed consumer composes their HP transitions explicitly.
func p13UFCarImpactLedger(tx *gorm.DB, match *db.P13UnionFightMatch) (*p13UFCarImpactLedgerData, error) {
	result := &p13UFCarImpactLedgerData{Impacts: []db.P13UnionFightCarImpact{}, Cars: []p13UFCarImpactCar{}}
	var factories []db.P13UnionFightFactoryState
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&factories).Error; err != nil {
		return nil, err
	}
	if len(factories) > 4 {
		return nil, p13UFCarImpactError()
	}
	var original *GameService
	initialFactories := map[int64]*pb.InstUFCariotFactory{}
	if len(factories) > 0 {
		var err error
		original, err = (&GameService{}).p13UFFieldOriginal(match, tx)
		if err != nil {
			return nil, err
		}
		snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
		if err != nil {
			return nil, err
		}
		var buildings pb.SCUFBuildingRefresh
		if proto.Unmarshal(snapshot.Buildings, &buildings) != nil || len(buildings.FactoryInfos) != 4 {
			return nil, p13UFCarImpactError()
		}
		for _, frame := range buildings.FactoryInfos {
			initialFactories[frame.GetGuid()] = frame
		}
	}
	byFactory := map[int64]*db.P13UnionFightFactoryState{}
	byCar := map[int64]p13UFCarImpactCar{}
	for i := range factories {
		row := &factories[i]
		initial := initialFactories[row.GUID]
		if row.FactoryID < 1 || row.FactoryID > 4 || row.GUID != p13UFFieldGUID(match.ID, "factory", strconv.FormatInt(row.FactoryID, 10)) || (row.Camp != 0 && row.Camp != 1) || initial == nil || int64(initial.GetFactoryId()) != row.FactoryID || initial.GetGuard() != nil && int64(initial.GetGuard().GetCamp()) != row.Camp {
			return nil, p13UFCarImpactError()
		}
		cfg := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(row.FactoryID, 10))
		if cfg == nil || cfg["Camp"] != strconv.FormatInt(row.Camp, 10) {
			return nil, p13UFCarImpactError()
		}
		if _, err := p13UFFactoryFrame(row); err != nil {
			return nil, err
		}
		byFactory[row.GUID] = row
		if row.CompletedAt > 0 {
			car, err := p13UFCarItemRead(row)
			if err != nil {
				return nil, err
			}
			if cfg["CariotId"] != strconv.FormatInt(int64(car.GetCariotId()), 10) {
				return nil, p13UFCarImpactError()
			}
			meta := p13UFCarImpactCar{car.GetGuid(), row.GUID, row.FactoryID, 1, row.CompletedAt, row.Camp, row.ChariotHP, car.GetHp(), row.SourceHash, row.SourceHash, p13UFCarHPIdentity(car), row.CarHash}
			result.Cars = append(result.Cars, meta)
			byCar[meta.CarGUID] = meta
		}
	}
	var offers []db.P13UnionFightFactoryCycleOffer
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&offers).Error; err != nil {
		return nil, err
	}
	if len(offers) > 4 {
		return nil, p13UFCarImpactError()
	}
	cycles := map[int64]*db.P13UnionFightFactoryCycleOffer{}
	for i := range offers {
		row := &offers[i]
		if row.SourceHash != p13UFFactoryCycleOfferHash(row) {
			return nil, p13UFCarImpactError()
		}
		cycles[row.FactoryGUID] = row
	}
	var generations []db.P13UnionFightCarGeneration
	generationCursor := int64(0)
	for {
		if err := tx.Statement.Context.Err(); err != nil {
			return nil, err
		}
		var page []db.P13UnionFightCarGeneration
		if err := tx.Where("match_id=? AND car_guid>?", match.ID, generationCursor).Order("car_guid").Limit(128).Find(&page).Error; err != nil {
			return nil, err
		}
		if len(generations)+len(page) > 512 {
			return nil, p13UFCarImpactError()
		}
		generations = append(generations, page...)
		if len(page) < 128 {
			break
		}
		generationCursor = page[len(page)-1].CarGUID
	}
	for _, row := range generations {
		factory, offer := byFactory[row.FactoryGUID], cycles[row.FactoryGUID]
		var frozen db.P13UnionFightFactoryState
		if factory == nil || offer == nil || row.Generation < 2 || row.Generation > 128 || offer.MaxGenerations < 2 || offer.MaxGenerations > 128 || row.Generation > offer.MaxGenerations || row.SourceHash != p13UFCarGenerationHash(&row) || row.StateHash != p13UFCarGenerationStateHash(&row) || row.CarHash != p6Digest(row.CarFrame) || row.OfferHash != offer.SourceHash || row.BornAt < match.StartAt || row.BornAt >= match.EndAt || json.Unmarshal(row.FactorySnapshot, &frozen) != nil {
			return nil, p13UFCarImpactError()
		}
		if _, err := p13UFFactoryFrame(&frozen); err != nil {
			return nil, err
		}
		if frozen.MatchID != match.ID || frozen.GUID != factory.GUID || frozen.FactoryID != factory.FactoryID || frozen.SourceHash != factory.SourceHash || frozen.Camp != factory.Camp || frozen.ChariotHP != factory.ChariotHP || frozen.CompletedAt != row.BornAt {
			return nil, p13UFCarImpactError()
		}
		if err := p13UFCarGenerationClockValid(&row, &frozen, offer); err != nil {
			return nil, err
		}
		frozen.CarGeneration = row.Generation
		birth, birthErr := p13UFCarItemRead(&frozen)
		if birthErr != nil {
			return nil, birthErr
		}
		if birth.GetHp() != birth.GetMaxHp() {
			return nil, p13UFCarImpactError()
		}
		frozen.CarFrame = row.CarFrame
		frozen.CarHash = row.CarHash
		frozen.StateHash = p13UFFactoryStateDigest(&frozen)
		car, err := p13UFCarItemRead(&frozen)
		if err != nil {
			return nil, err
		}
		if car.GetGuid() != row.CarGUID || p13UFCarHPIdentity(car) != p13UFCarHPIdentity(birth) || byCar[row.CarGUID].CarGUID != 0 {
			return nil, p13UFCarImpactError()
		}
		meta := p13UFCarImpactCar{row.CarGUID, factory.GUID, factory.FactoryID, row.Generation, row.BornAt, factory.Camp, factory.ChariotHP, car.GetHp(), factory.SourceHash, row.SourceHash, p13UFCarHPIdentity(car), row.CarHash}
		result.Cars = append(result.Cars, meta)
		byCar[row.CarGUID] = meta
	}
	var itemOffers []db.P13UnionFightCarItemOffer
	if err := tx.Where("match_id=?", match.ID).Limit(3).Find(&itemOffers).Error; err != nil {
		return nil, err
	}
	if len(itemOffers) > 2 {
		return nil, p13UFCarImpactError()
	}
	byOffer := map[string]*db.P13UnionFightCarItemOffer{}
	for i := range itemOffers {
		row := &itemOffers[i]
		if row.SourceHash != p13UFCarItemOfferHash(row) || p13UFItemBudgetValidate(row.ItemID, p13UFItemBudgetConfig{row.BudgetRule, row.TeamUseLimit, row.TargetCooldownMillis, row.EffectFamily}) != nil {
			return nil, p13UFCarImpactError()
		}
		byOffer[row.SourceHash] = row
	}
	var count int64
	if err := tx.Model(&db.P13UnionFightCarImpact{}).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	cursor := ""
	ordinals := map[int64]int64{}
	lastAt := map[int64]int64{}
	for {
		if err := tx.Statement.Context.Err(); err != nil {
			return nil, err
		}
		var rows []db.P13UnionFightCarImpact
		if err := tx.Where("match_id=? AND id>?", match.ID, cursor).Order("id").Limit(128).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			row := &rows[i]
			before, _, err := p13UFCarImpactVerify(row)
			meta, ok := byCar[row.CarGUID]
			offer := byOffer[row.OfferHash]
			if err != nil || !ok || offer == nil || offer.ItemID != row.ItemID || offer.HPPercent != row.HPPercent || meta.Generation != row.Generation || meta.FactoryGUID != row.FactoryGUID || meta.FactoryID != row.FactoryID || meta.BornAt != row.BornAt || meta.Camp != row.CarCamp || meta.FactorySourceHash != row.FactorySourceHash || meta.GenerationSourceHash != row.GenerationSourceHash || meta.IdentityHash != row.IdentityHash || row.IdentityHash != p13UFCarHPIdentity(before) || row.LogicalAt < match.StartAt || row.LogicalAt > match.EndAt {
				return nil, p13UFCarImpactError()
			}
		}
		result.Impacts = append(result.Impacts, rows...)
		cursor = rows[len(rows)-1].ID
	}
	if int64(len(result.Impacts)) != count {
		return nil, p13UFCarImpactError()
	}
	// Ordinal continuity is independent of other producers' HP changes.
	sort.Slice(result.Impacts, func(i, j int) bool {
		a, b := result.Impacts[i], result.Impacts[j]
		if a.CarGUID != b.CarGUID {
			return a.CarGUID < b.CarGUID
		}
		return a.Ordinal < b.Ordinal
	})
	for _, row := range result.Impacts {
		if row.Ordinal != ordinals[row.CarGUID]+1 || row.LogicalAt < lastAt[row.CarGUID] {
			return nil, p13UFCarImpactError()
		}
		ordinals[row.CarGUID] = row.Ordinal
		lastAt[row.CarGUID] = row.LogicalAt
	}
	return result, nil
}
