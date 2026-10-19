package service

import (
	"bytes"
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"time"
)

// InitializeP4 初始化独立域，正式启动失败直接停止；既有P1/P2/P3规则与存档保留。
func (s *GameService) InitializeP4(ctx context.Context) error {
	if !s.P1Ready {
		return errors.New("P4需要先初始化P1公共能力")
	}
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.P4WelfareRule{}, &db.P4WelfareState{}, &db.P4Receipt{}, &db.P4PlayerStats{}, &db.P4CommerceRule{}, &db.P4CommerceState{}, &db.P4CommerceStock{}, &db.P4PopularityRankClaim{}, &db.P4PopularityRankOfferHistory{}, &db.P4RecruitRotation{}, &db.P4ActivityRegistry{}, &db.P15SeasonalBundle{}, &db.P4ResourceExchangeDirectory{}); err != nil {
		return err
	}
	if err := s.seedP4Welfare(db.GetIns().WithContext(ctx), time.Now()); err != nil {
		return err
	}
	if err := s.seedP4Commerce(db.GetIns().WithContext(ctx), time.Now()); err != nil {
		return err
	}
	if err := s.seedP4Periods(db.GetIns().WithContext(ctx), time.Now()); err != nil {
		return err
	}
	if err := s.seedP4Registry(db.GetIns().WithContext(ctx)); err != nil {
		return err
	}
	s.P4Ready = true
	return nil
}

// DispatchP4 只接管已实现的新域，支付/红装/原月签到等继续归所属域。
func (s *GameService) DispatchP4(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSCopyBoonInfo:
		if b.GetActivityId() == "RechangeDouble" {
			r, err := s.firstDoubleBoonInfo(ctx, id, b, LocalPlayNow(ctx))
			return r, true, err
		}
		r, err := s.P4BoonInfo(ctx, b, LocalPlayNow(ctx))
		return r, true, err
	case *pb.CSActivityList:
		if isP15SeasonalParent(b.GetActivityId()) {
			r, err := s.p15SeasonalList(ctx, id, b, LocalPlayNow(ctx))
			return r, true, err
		}
	case *pb.CSActivityInfo:
		if b.GetActivityId() == constUtil.P4CopyBenefitsActivityID {
			r, err := s.P4CopyBenefitsInfo(ctx, LocalPlayNow(ctx))
			return r, true, err
		}
		if strings.HasPrefix(b.GetActivityId(), "consumeRet_p4_") {
			r, e := s.p4Rebate(ctx, id, b, stamp, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSActivityReward:
		if b.GetActivityId() == constUtil.P4CopyBenefitsActivityID {
			return nil, true, errors.New("副本福利随成功副本结算发放")
		}
		if strings.HasPrefix(b.GetActivityId(), "consumeRet_p4_") {
			r, e := s.p4Rebate(ctx, id, b, stamp, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSConsumeReturnReward:
		if strings.HasPrefix(b.GetActivityId(), "consumeRet_p4_") {
			r, e := s.p4Rebate(ctx, id, b, stamp, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSMysticDiscountInfo:
		r, e := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
		return r, true, e
	case *pb.CSShopProductList:
		if b.GetShopType() == constUtil.P4GuildShopType {
			reply, err := s.p4GuildShop(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
		if b.GetShopType() == 4 || b.GetShopType() == 5 || b.GetShopType() == 7 {
			reply, err := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
	case *pb.CSShopBuy:
		if b.GetShopType() == constUtil.P4GuildShopType {
			reply, err := s.p4GuildShop(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
		if (b.GetShopType() == 1 && p4IsOrdinaryProduct(b.GetShopItemId())) || b.GetShopType() == 4 || b.GetShopType() == 5 || b.GetShopType() == 7 {
			reply, err := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
	case *pb.CSRefreshShopQuery, *pb.CSRefreshShopRefresh, *pb.CSRefreshShopBuy:
		reply, err := s.p4Commerce(ctx, id, request, stamp, LocalPlayNow(ctx))
		return reply, true, err
	case *pb.CSActivityShopInfo:
		if strings.Contains(b.GetActivityId(), "_p4_") {
			reply, err := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
	case *pb.CSGetActivityShopReward:
		if strings.Contains(b.GetActivityId(), "_p4_") {
			reply, err := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
	case *pb.CSBuyActivityShop:
		if strings.Contains(b.GetActivityId(), "_p4_") {
			reply, err := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
	case *pb.CSMysticDiscount:
		if strings.Contains(b.GetActivityId(), "_p4_") {
			reply, err := s.p4Commerce(ctx, id, b, stamp, LocalPlayNow(ctx))
			return reply, true, err
		}
	case *pb.CSLoginRewardInfo, *pb.CSGetLoginReward, *pb.CSRewardLimitSignList, *pb.CSReceiveLimitMonthSign, *pb.CSStaminaRefill, *pb.CSStaminaGet, *pb.CSFortuneCatInfo, *pb.CSFortuneCatReward:
		reply, err := s.p4Welfare(ctx, id, request, stamp, LocalPlayNow(ctx))
		return reply, true, err
	}
	return nil, false, nil
}

func p4ResponseName(request proto.Message) string {
	switch request.(type) {
	case *pb.CSConsumeReturnReward, *pb.CSActivityReward:
		return "SCActivityReward"
	case *pb.CSShopBuy:
		return "SCShopBuy"
	case *pb.CSRefreshShopRefresh:
		return "SCRefreshShopRefresh"
	case *pb.CSRefreshShopBuy:
		return "SCRefreshShopBuy"
	case *pb.CSGetActivityShopReward, *pb.CSBuyActivityShop:
		return "SCGetActivityShopReward"
	case *pb.CSMysticDiscount:
		return "SCMysticDiscount"
	case *pb.CSGetLoginReward:
		return "SCGetLoginReward"
	case *pb.CSReceiveLimitMonthSign:
		return "SCReceiveLimitMonthSign"
	case *pb.CSStaminaGet:
		return "SCStaminaGet"
	case *pb.CSFortuneCatReward:
		return "SCFortuneCatReward"
	}
	return ""
}

func p4ReadReceipt(tx *gorm.DB, id int64, domain string, stamp int64, request proto.Message) (proto.Message, error) {
	if stamp <= 0 {
		return nil, errors.New("业务请求时间戳无效")
	}
	var row db.P4Receipt
	err := tx.Where("player_id = ? AND domain = ? AND timestamp = ?", id, domain, stamp).First(&row).Error
	raw, e := proto.Marshal(request)
	if e != nil {
		return nil, e
	}
	if err == nil {
		if !bytes.Equal(raw, row.Request) {
			return nil, errors.New("业务回执时间戳冲突")
		}
		result, e := model.GameMessage(p4ResponseName(request), nil)
		if e != nil {
			return nil, e
		}
		if e = proto.Unmarshal(row.Response, result); e != nil {
			return nil, e
		}
		return result, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var newer int64
	if err = tx.Model(&db.P4Receipt{}).Where("player_id = ? AND domain = ? AND timestamp > ?", id, domain, stamp).Count(&newer).Error; err != nil {
		return nil, err
	}
	if newer > 0 {
		return nil, errors.New("业务请求已过期")
	}
	return nil, nil
}
func p4SaveReceipt(tx *gorm.DB, id int64, domain string, stamp int64, request, response proto.Message) error {
	raw, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	reply, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	return tx.Create(&db.P4Receipt{PlayerID: id, Domain: domain, Timestamp: stamp, Request: raw, Response: reply}).Error
}
