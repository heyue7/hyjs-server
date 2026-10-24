package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeFirstDouble inserts the default-off policy without reopening an
// operator stop or changing any existing product, order or lifetime receipt.
func (s *GameService) InitializeFirstDouble(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.FirstDoublePolicy{}); err != nil {
		return err
	}
	policy := db.FirstDoublePolicy{ID: 1, Version: constUtil.FirstDoubleVersion, Source: constUtil.FirstDoubleSource}
	return d.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error
}

func firstDoubleEnabled(tx *gorm.DB) (bool, error) {
	// An older fixed deployment without this optional table stays closed.
	if !gameSchemaHasTable(tx, &db.FirstDoublePolicy{}) {
		return false, nil
	}
	var policy db.FirstDoublePolicy
	if err := tx.First(&policy, "id=?", 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if policy.Version != constUtil.FirstDoubleVersion || policy.Source != constUtil.FirstDoubleSource {
		return false, errors.New(constUtil.FirstDoublePolicyInvalid)
	}
	return policy.Enabled, nil
}

// firstDoubleOriginalSKU accepts only a byte-equivalent original once SKU's
// economic fields. Local boxes/cards and modified original prices stay outside.
func (s *GameService) firstDoubleOriginalSKU(product *db.PaymentProduct) bool {
	if product == nil || product.Kind != "once" || product.Reward != "" || product.Currency != "RMB" || !product.Enabled {
		return false
	}
	for _, original := range s.Tables["Payment"] {
		if original["paymentId"] != product.ID || original["paymentType"] != "once" {
			continue
		}
		var (
			money, moneyErr   = strconv.ParseFloat(original["money"], 64)
			charge, chargeErr = strconv.ParseInt(original["charge"], 10, 64)
			free, freeErr     = strconv.ParseInt(original["free"], 10, 64)
		)
		return moneyErr == nil && chargeErr == nil && freeErr == nil && !math.IsNaN(money) && !math.IsInf(money, 0) &&
			product.Cents == int64(math.Round(money*100)) && product.Charge == charge && product.Free == free &&
			product.Currency == original["moneyType"] && product.Shop == original["shopId"] && product.Channels == original["channelIdList"]
	}
	return false
}

// firstDoubleProduct is called on the local product copy inside the original
// profile lock before the new order snapshot. Confirmation still arbitrates two
// pending orders by the existing lifetime confirmed count under that same lock.
func (s *GameService) firstDoubleProduct(tx *gorm.DB, id int64, product *db.PaymentProduct) error {
	if !s.firstDoubleOriginalSKU(product) {
		return nil
	}
	enabled, err := firstDoubleEnabled(tx)
	if err != nil || !enabled {
		return err
	}
	var count int64
	if err := tx.Model(&db.PaymentOrder{}).Where("player_id=? AND product_id=? AND status=?", id, product.ID, constUtil.P3PaymentConfirmed).Count(&count).Error; err != nil {
		return err
	}
	product.FirstDouble = count == 0
	return nil
}

// firstDoublePaymentInfo uses the original chargeType/giftRate contract. The
// original client draws the 2x badge for first+giftRate1; giftRate2 draws 3x.
func (s *GameService) firstDoublePaymentInfo(ctx context.Context, id int64, product db.PaymentProduct) (string, int32, error) {
	if err := s.firstDoubleProduct(db.GetIns().WithContext(ctx), id, &product); err != nil {
		return "", 0, err
	}
	if product.FirstDouble && product.Kind == "once" {
		var count int64
		if err := db.GetIns().WithContext(ctx).Model(&db.PaymentOrder{}).Where("player_id=? AND product_id=? AND status=?", id, product.ID, constUtil.P3PaymentConfirmed).Count(&count).Error; err != nil {
			return "", 0, err
		}
		if count == 0 {
			return "first", 1, nil
		}
	}
	return "general", 1, nil
}

// firstDoubleBoonInfo is the original readonly Type33 explanation callback.
// The shared P4 dispatcher owns activity/private-period access checks.
func (s *GameService) firstDoubleBoonInfo(ctx context.Context, id int64, request *pb.CSCopyBoonInfo, now time.Time) ([]proto.Message, error) {
	enabled, err := firstDoubleEnabled(db.GetIns().WithContext(ctx))
	if err != nil {
		return nil, err
	}
	information := constUtil.FirstDoubleInfoClosed
	if enabled {
		information = constUtil.FirstDoubleInfoOpen
	}
	return []proto.Message{&pb.SCCopyBoonInfo{ActivityId: proto.String(request.GetActivityId()), Information: proto.String(information)}}, nil
}
