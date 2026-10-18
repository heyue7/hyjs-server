package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type benefitGiftQuote struct {
	RuleID    string           `json:"ruleId"`
	ProductID string           `json:"productId"`
	Day       string           `json:"day"`
	Cost      *pb.BaseResource `json:"cost"`
	Rewards   string           `json:"rewards"`
}

// benefitGiftOffer reads the existing operator source and personal-cycle scope.
// This adapter deliberately has no independent prices, rewards or money threshold.
func (s *GameService) benefitGiftOffer(tx *gorm.DB, p *pb.Player, now time.Time) (*db.P4CommerceRule, *p4Product, *db.P4CommerceState, *p4CommerceProgress, error) {
	key := "limitSale_p4_" + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	cycle, err := localPlayEntryCycle(tx, "TimeLimitSail")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if cycle != nil {
		key = "limitSale_p4_" + cycle.ID
	}
	var live db.P4CommerceRule
	err = tx.First(&live, "id=? AND enabled=?", key, true).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, nil, err
	}
	frozen, _, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if now.UnixMilli() < frozen.StartAt || now.UnixMilli() >= frozen.EndAt {
		return nil, nil, nil, nil, nil
	}
	allowed, err := localPlayActivityVisible(tx, key)
	if err != nil || !allowed {
		return nil, nil, nil, nil, err
	}
	cfg, rules, row, state, err := s.loadP4Commerce(tx, p.GetPlayerId(), key, 0, now)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if cfg.Kind != "sale" {
		return nil, nil, nil, nil, errors.New("限时福利目录类型无效")
	}
	sort.SliceStable(rules.Products, func(i, j int) bool { return rules.Products[i].Index < rules.Products[j].Index })
	for i := range rules.Products {
		product := &rules.Products[i]
		if p.GetLevel() < product.MinLevel || p.GetVip() < product.MinVIP || !p4Visible(cfg, state, *product) {
			continue
		}
		cost, e := s.p4ParseCost(product.Cost)
		if e != nil {
			return nil, nil, nil, nil, e
		}
		if cost.GetId() != "diamond" || product.TierID {
			continue
		}
		return &cfg, product, row, state, nil
	}
	return nil, nil, nil, nil, nil
}

// benefitGiftReceipt uses the existing persistent P4 ledger with the original request/response.
// A stable domain prevents a delayed timestamp from becoming a purchase in another cycle.
func benefitGiftReceipt(tx *gorm.DB, id, stamp int64, request proto.Message) (*pb.SCBenefitGiftReward, error) {
	if stamp <= 0 {
		return nil, errors.New("限时福利请求时间戳无效")
	}
	var receipt db.P4Receipt
	err := tx.First(&receipt, "player_id=? AND domain=? AND timestamp=?", id, constUtil.BenefitGiftReceiptDomain, stamp).Error
	if err == nil {
		raw, e := proto.Marshal(request)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(raw, receipt.Request) {
			return nil, errors.New("限时福利回执请求冲突")
		}
		response := &pb.SCBenefitGiftReward{}
		if e = proto.Unmarshal(receipt.Response, response); e != nil {
			return nil, e
		}
		return response, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var newer int64
	if err = tx.Model(&db.P4Receipt{}).Where("player_id=? AND domain=? AND timestamp>?", id, constUtil.BenefitGiftReceiptDomain, stamp).Count(&newer).Error; err != nil {
		return nil, err
	}
	if newer != 0 {
		return nil, errors.New("限时福利请求已过期")
	}
	return nil, nil
}

// benefitGiftAt restores the original page while preserving generic-store quotas.
// Its callback adds Get and deletes Cost itself, so no SCPlayer accompanies this response.
func (s *GameService) benefitGiftAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	return s.benefitGiftPageAt(ctx, id, request, stamp, now, true)
}

func (s *GameService) benefitGiftPageAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time, freezeQuote bool) ([]proto.Message, error) {
	if !s.P4Ready {
		return nil, errors.New("限时福利目录尚未初始化")
	}
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, purchase := request.(*pb.CSBenefitGiftReward)
		if purchase {
			prior, err := benefitGiftReceipt(tx, id, stamp, request)
			if err != nil {
				return err
			}
			if prior != nil {
				response = prior
				return nil
			}
		}
		cfg, product, row, state, err := s.benefitGiftOffer(tx, p, now)
		if err != nil {
			return err
		}
		if cfg == nil {
			if purchase {
				return errors.New("限时福利未开放或已结束")
			}
			if _, ok := request.(*pb.CSBenefitGiftList); ok {
				response = &pb.SCBenefitGiftList{}
			} else {
				response = &pb.SCBenefitGiftInfo{RewardState: proto.Int32(2), CloseTime: proto.Int64(now.UnixMilli())}
			}
			return nil
		}
		key := p4SlotKey(*cfg, state, *product)
		bought := state.Bought[key]
		total := state.Total[product.ID]
		if bought < 0 || total < 0 {
			return errors.New("限时福利购买进度无效")
		}
		status := int32(0)
		if bought >= product.DailyLimit || product.TotalLimit > 0 && total >= product.TotalLimit {
			status = 2
		}
		stock := db.P4CommerceStock{RuleID: cfg.ID, ProductID: product.ID}
		if product.Stock >= 0 {
			e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stock, "rule_id=? AND product_id=?", cfg.ID, product.ID).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if stock.Used < 0 {
				return errors.New("限时福利库存进度无效")
			}
			if stock.Used >= product.Stock {
				status = 2
			}
		}
		base, err := s.p4ParseCost(product.Cost)
		if err != nil {
			return err
		}
		cost, err := s.p4Price(*product, state, bought+1, 1)
		if err != nil {
			return err
		}
		resources, err := p1RewardResources(product.Rewards)
		if err != nil {
			return err
		}
		quote := benefitGiftQuote{RuleID: cfg.ID, ProductID: product.ID, Day: p1Day(now), Cost: cost, Rewards: product.Rewards}
		if purchase {
			var shown benefitGiftQuote
			if json.Unmarshal([]byte(p5KV(g, constUtil.BenefitGiftQuoteKey)), &shown) != nil || shown.RuleID != quote.RuleID || shown.ProductID != quote.ProductID || shown.Day != quote.Day || shown.Rewards != quote.Rewards || !proto.Equal(shown.Cost, quote.Cost) {
				return errors.New("请重新打开限时福利确认当前报价")
			}
			if status == 2 {
				return errors.New("限时福利限购已用完")
			}
			if product.Stock >= 0 {
				stock.Used++
				if err = tx.Save(&stock).Error; err != nil {
					return err
				}
			}
			source := fmt.Sprintf("p4:benefit-gift:%s:%d", cfg.ID, stamp)
			if err = s.p4Deduct(tx, id, p, g, cost, source, now); err != nil {
				return err
			}
			gain, e := s.GrantFixedReward(p, g, product.Rewards)
			if e != nil {
				return e
			}
			state.Bought[key]++
			state.Total[product.ID]++
			if err = s.RecordP1Event(tx, id, source, constUtil.P1EventPurchase, 1, now); err != nil {
				return err
			}
			response = &pb.SCBenefitGiftReward{Get: gain, Cost: []*pb.BaseResource{cost}, RewardState: proto.Int32(2)}
			if err = p4SaveReceipt(tx, id, constUtil.BenefitGiftReceiptDomain, stamp, request, response); err != nil {
				return err
			}
		} else {
			switch request.(type) {
			case *pb.CSBenefitGiftList:
				response = &pb.SCBenefitGiftList{ItemList: []*pb.ActivityTab{{ActivityId: proto.String(cfg.ID), Name: proto.String("限时特惠"), Status: proto.Int32(status), Order: proto.Int32(1)}}}
			case *pb.CSBenefitGiftInfo:
				// Only the actual page response confirms its quotation. A HUD refresh
				// must never silently authorize a different cycle behind an open page.
				if freezeQuote {
					raw, e := json.Marshal(quote)
					if e != nil {
						return e
					}
					p5SetKV(g, constUtil.BenefitGiftQuoteKey, string(raw))
				}
				response = &pb.SCBenefitGiftInfo{TargetId: proto.String(product.ID), GiftTitle: proto.String("限时特惠"), BuyType: proto.String("diamond"), OpenTime: proto.Int64(cfg.StartAt), CloseTime: proto.Int64(cfg.EndAt), Price: base.Num, DiscountPrice: cost.Num, Reward: resources, RewardState: proto.Int32(status)}
			default:
				return errors.New("限时福利请求类型无效")
			}
		}
		return saveP4Commerce(tx, row, state)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// BenefitGiftOpenState derives the native town icon solely from the current real offer.
// Initialization-off callers retain their old behavior without reading uninitialized tables.
func (s *GameService) BenefitGiftOpenState(ctx context.Context, id int64, now time.Time) (*pb.OpenState, error) {
	state := &pb.OpenState{Type: proto.String(constUtil.BenefitGiftActivityType), IsOpen: proto.Int32(0)}
	if !s.P4Ready {
		return state, nil
	}
	result, err := s.benefitGiftPageAt(ctx, id, &pb.CSBenefitGiftInfo{}, 0, now, false)
	if err != nil {
		return nil, err
	}
	info := result[0].(*pb.SCBenefitGiftInfo)
	if info.GetTargetId() != "" && info.GetRewardState() != 2 {
		state.IsOpen = proto.Int32(1)
	}
	return state, nil
}
