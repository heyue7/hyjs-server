package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p3Error() error             { return errors.New(constUtil.P3PaymentError) }
func p3Day(now time.Time) string { return now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02") }
func p3Hash(data []byte) string  { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// p3PaymentChannel 只兼容已核实原客户端AND0003。目录与下单共同使用，
// 不改写商品Channels原表来源，也不放开未知渠道。
func p3PaymentChannel(channel string) string {
	if channel == "AND0003" {
		return "gf_default"
	}
	return channel
}

// LoadPaymentTables 读取支付域三张APK表，供统一加载器调用；不重置其他域已加载表。
func (s *GameService) LoadPaymentTables(root string) error {
	if s.Tables == nil {
		s.Tables = map[string][]map[string]string{}
	}
	for _, name := range []string{"Payment", "VipPerks", "LuxuryEquip"} {
		file, err := os.Open(filepath.Join(root, "Table"+name+".tsv"))
		if err != nil {
			return err
		}
		reader := csv.NewReader(file)
		reader.Comma = '\t'
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		file.Close()
		if err != nil {
			return err
		}
		if len(rows) < 5 {
			return errors.New(constUtil.P3PaymentConfigError)
		}
		var result []map[string]string
		for _, row := range rows[5:] {
			if len(row) == 0 || row[0] == "" {
				continue
			}
			entry := map[string]string{}
			for i, v := range row {
				if i < len(rows[1]) {
					entry[rows[1][i]] = v
				}
			}
			result = append(result, entry)
		}
		s.Tables[name] = result
	}
	return nil
}

// InitializePayments 插入缺失原渠道商品，保留已有运营规则和订单快照。
// 卡的free沿原描述用作每日钻石；30日期限、福利档位和测试礼包为明示本地规则。
func (s *GameService) InitializePayments(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if len(s.Tables["Payment"]) == 0 || len(s.Tables["VipPerks"]) == 0 {
		return errors.New(constUtil.P3PaymentConfigError)
	}
	if err := d.AutoMigrate(&db.PaymentProduct{}, &db.PaymentOrder{}, &db.LocalPlayPaymentTime{}, &db.PaymentConfirmation{}, &db.MembershipClaim{}, &db.PaymentActivityRule{}, &db.PaymentActivityState{}, &db.PaymentGiftCode{}, &db.PaymentLuxuryOffer{}, &db.VIPShopProduct{}); err != nil {
		return err
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 预置关闭的本地原礼包入口，运营启用后沿原会员商店、实际付款资格和每日冻结报价购买。
		for _, spec := range []db.VIPShopProduct{
			{ID: constUtil.P3VIP7GrowthProduct, MinVIP: 7, Cost: 800, Reward: "item_041,1", Source: constUtil.P3VIP7GrowthSource, Enabled: false},
			{ID: constUtil.P3VIP8GrowthProduct, MinVIP: 8, Cost: 900, Reward: "item_042,1", Source: constUtil.P3VIP8GrowthSource, Enabled: false},
			{ID: constUtil.P3VIP9GrowthProduct, MinVIP: 9, Cost: 1000, Reward: "item_043,1", Source: constUtil.P3VIP9GrowthSource, Enabled: false},
			{ID: constUtil.P3VIP10GrowthProduct, MinVIP: 10, Cost: 1100, Reward: "item_044,1", Source: constUtil.P3VIP10GrowthSource, Enabled: false},
		} {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error; err != nil {
				return err
			}
		}
		if err := s.seedP3VIP6GrowthProduct(tx); err != nil {
			return err
		}
		for _, spec := range []db.VIPShopProduct{{ID: "p3vip_training", MinVIP: 1, Cost: 100, Reward: "item_007,20", Source: "本地会员目录；每日一份、100钻、原训练材料；待运营配置，默认关闭", Enabled: false}, {ID: "p3vip_red_card", MinVIP: 2, Cost: 300, Reward: "item_023,1", Source: "本地会员目录；每日一份、300钻、原红卡选择箱；待运营配置，默认关闭", Enabled: false}} {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error; err != nil {
				return err
			}
		}
		for _, row := range s.Tables["Payment"] {
			money, err := strconv.ParseFloat(row["money"], 64)
			if err != nil {
				return err
			}
			charge, err := strconv.ParseInt(row["charge"], 10, 64)
			if err != nil {
				return err
			}
			free, err := strconv.ParseInt(row["free"], 10, 64)
			if err != nil {
				return err
			}
			p := db.PaymentProduct{ID: row["paymentId"], Channels: row["channelIdList"], Shop: row["shopId"], Kind: row["paymentType"], Cents: int64(math.Round(money * 100)), Currency: row["moneyType"], Charge: charge, Free: free, Name: row["paymentName"], Description: row["paymentDesc"] + "；测试商品，不产生实际扣款", Source: "APK TablePayment；会员free为每日钻石，期限30日为本地兼容规则；已核实客户端AND0003本地映射gf_default，原Channels保留", Enabled: true}
			if err := validateP3Product(&p); err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
				return err
			}
		}
		// 自建红箱测试商品仅保留关闭的配置模板，等待运营确定奖励与价格。
		// 原箱子的选项、实例和背包处理仍沿 P2 使用接口。
		for _, p := range []db.PaymentProduct{
			{ID: "local-p3-red-card", Channels: "gf_default", Shop: "common", Kind: "once", Cents: 3000, Currency: "RMB", Charge: 300, Reward: "item_023,1", Name: "测试红卡礼包", Description: "仅隔离Demo测试支付；原红卡选择箱", Source: "本地P3测试商品；原item_023选择箱；已核实客户端AND0003本地映射gf_default", Enabled: false},
			{ID: "local-p3-red-equipment", Channels: "gf_default", Shop: "common", Kind: "once", Cents: 3000, Currency: "RMB", Charge: 300, Reward: "item_119,1", Name: "测试红装备礼包", Description: "仅隔离Demo测试支付；原红装备选择箱", Source: "本地P3测试商品；原item_119选择箱；已核实客户端AND0003本地映射gf_default", Enabled: false},
		} {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
				return err
			}
		}
		zone := time.FixedZone("UTC+8", 8*3600)
		start := time.Date(2026, 10, 11, 0, 0, 0, 0, zone).UnixMilli()
		end := time.Date(2026, 11, 11, 0, 0, 0, 0, zone).UnixMilli()
		rewardEnd := end + 7*24*3600*1000
		for _, r := range []db.PaymentActivityRule{
			{ID: "firstRecharge_local_p3", Kind: "first", Threshold: 1, Reward: s.table("GameConfigure", "StaticName", "FirstRechargeReward")["NameValue"], Name: "首充奖励"},
			{ID: "rechargeSingle_local_p3", Kind: "single", Threshold: 3000, Reward: "diamond,30", Name: "单笔测试充值30元"},
			{ID: "rechargeTotal_local_p3", Kind: "total", Threshold: 6000, Reward: "item_023,1", Name: "累计测试充值60元"},
			{ID: "continuedRecharge_local_p3", Kind: "continued", Threshold: 3, Reward: "item_119,1", Name: "连续三日测试充值"},
			{ID: "costTotal_local_p3", Kind: "consume", Threshold: 300, Reward: "diamond,30", Name: "累计消费300钻"},
		} {
			r.StartAt = start
			r.EndAt = end
			r.RewardEndAt = rewardEnd
			// 首充沿用原奖励，其他限时奖励交给运营配置后再启用。
			r.Enabled = r.Kind == "first"
			r.Source = "本地P3独立实例；确认订单人民币分计数，首充原GameConfigure奖励；其他档位本地测试；首次观察冻结门槛窗口奖励，UTC+8连续日；消费仅P1真实diamond_spent事件"
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error; err != nil {
				return err
			}
		}
		// 停止自动投放自建红箱礼包码；既有领取记录保留。
		return nil
	})
}

func validateP3Product(p *db.PaymentProduct) error {
	if p.ID == "" || p.Cents <= 0 || p.Charge < 0 || p.Free < 0 || p.Charge > math.MaxInt32 || p.Free > math.MaxInt32 || p.Currency != "RMB" || p.Source == "" || p.Shop != "common" || (p.Kind != "once" && p.Kind != "month" && p.Kind != "permanent") {
		return errors.New(constUtil.P3PaymentConfigError)
	}
	return nil
}

// p3Profile 串行化支付、会员和领奖；任何资源或事件失败回滚整笔订单事实。
func (s *GameService) p3Profile(ctx context.Context, id int64, fn func(*gorm.DB, *db.Profile, *pb.Player, *model.GameState) error) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err := lockProfilePeriods(tx); err != nil {
			return err
		}
		// p3Profile has an independent decode path. Persist only actual lazy penalty
		// changes with a neutral request, then let the original callback own its work.
		penaltyChanged, err := s.syncP12FieldPVPPenaltyForRead(tx, p, g, time.Now())
		if err != nil {
			return err
		}
		if penaltyChanged {
			recoveryCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, struct{}{})
			if err = saveProfile(tx.WithContext(recoveryCtx), &row, p, g); err != nil {
				return err
			}
		}
		if err = s.RefreshP5GuildPrayerPower(ctx, tx, p, g, p13UFNow(ctx)); err != nil {
			return err
		}
		return fn(tx, &row, p, g)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

func (s *GameService) generatePayment(ctx context.Context, id int64, req *pb.CSGenerateOrder, stamp int64, now time.Time) ([]proto.Message, error) {
	var order db.PaymentOrder
	raw, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	hash := p3Hash(raw)
	err = s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		if stamp <= 0 {
			return p3Error()
		}
		found := tx.Where("player_id = ? AND request_key = ?", id, stamp).First(&order).Error
		if found == nil {
			if order.RequestHash != hash {
				return p3Error()
			}
			return nil
		}
		if !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		var product db.PaymentProduct
		if err := tx.First(&product, "id = ?", req.GetPaymentId()).Error; err != nil {
			return err
		}
		if err := validateP3Product(&product); err != nil {
			return err
		}
		if !product.Enabled || req.GetShopId() != product.Shop || req.GetMoneyType() != product.Currency || math.IsNaN(req.GetMoney()) || math.IsInf(req.GetMoney(), 0) || math.Abs(req.GetMoney()*100-float64(product.Cents)) > 0.0001 {
			return p3Error()
		}
		channel := false
		for _, v := range strings.Split(product.Channels, ";") {
			channel = channel || v == p3PaymentChannel(req.GetChannelId())
		}
		if !channel {
			return p3Error()
		}
		if strings.HasPrefix(product.ID, "local-") && !strings.HasPrefix(row.Uid, "demo-") {
			return p3Error()
		}
		if product.Kind == "permanent" && p.GetPermanentCardValid() != 0 {
			return errors.New(constUtil.P3PaymentUnavailable)
		}
		if product.Kind == "month" && p.GetMonthCardEndTime() > LocalPlayAt(ctx, now).UnixMilli()+5*24*3600*1000 {
			return errors.New(constUtil.P3PaymentUnavailable)
		}
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			return err
		}
		if err := s.firstDoubleProduct(tx, id, &product); err != nil {
			return err
		}
		snapshot, err := json.Marshal(product)
		if err != nil {
			return err
		}
		order = db.PaymentOrder{ID: "p3-" + hex.EncodeToString(bytes), PlayerID: id, RequestKey: stamp, RequestHash: hash, ProductID: product.ID, Snapshot: string(snapshot), Status: constUtil.P3PaymentPending, CreatedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + constUtil.P3PaymentOrderLifetimeMS, Cents: product.Cents, Source: constUtil.P3PaymentSource}
		return tx.Create(&order).Error
	})
	if err != nil {
		return nil, err
	}
	// The committed order remains the retry identity. Only explicit local demo
	// startup can invoke the existing trusted test provider after this transaction.
	confirmed, err := s.autoConfirmDemoPaymentAt(ctx, &order, now)
	if err != nil {
		return nil, err
	}
	order = *confirmed
	return []proto.Message{&pb.SCGenerateOrder{OrderId: proto.String(order.ID), TransactionData: p3Transaction(&order)}}, nil
}

func p3Transaction(o *db.PaymentOrder) *pb.TransactionData {
	var p db.PaymentProduct
	_ = json.Unmarshal([]byte(o.Snapshot), &p)
	status := "PAY_STATUS_WAIT"
	if o.Status == constUtil.P3PaymentConfirmed {
		status = "PAY_STATUS_SUCCESS"
	}
	if o.Status == constUtil.P3PaymentFailed {
		status = "PAY_STATUS_FAIL"
	}
	return &pb.TransactionData{OrderId: proto.String(o.ID), PlayerId: proto.Int64(o.PlayerID), ServerId: proto.Int32(1), ChannelId: proto.String("local_demo_test"), PaymentId: proto.String(o.ProductID), PayStatus: proto.String(status), Price: proto.Float64(float64(o.Cents) / 100), MoneyType: proto.String(p.Currency), PayDiamond: proto.Int64(p.Charge), FreeDiamond: proto.Int64(max(0, o.Credited-p.Charge)), StartTime: proto.Int64(o.CreatedAt), EndTime: proto.Int64(o.ExpiresAt), PaymentName: proto.String(p.Name)}
}

// ConfirmTestPayment 仅本机管理adapter授权上下文可调用；游戏客户端不能构造授权。
// 金额/来源键/商品快照、到账、VIP、卡期限与确认事实在同一行锁事务提交。
func (s *GameService) ConfirmTestPayment(ctx context.Context, orderID, eventID string, cents int64, outcome string) (*db.PaymentOrder, error) {
	return s.confirmTestPaymentAt(ctx, orderID, eventID, cents, outcome, time.Now())
}
func (s *GameService) confirmTestPaymentAt(ctx context.Context, orderID, eventID string, cents int64, outcome string, now time.Time) (*db.PaymentOrder, error) {
	if ctx.Value(constUtil.P3TestPaymentContextKey) != true {
		return nil, errors.New(constUtil.P3PaymentRealDisabled)
	}
	if len(eventID) < 1 || len(eventID) > 128 || (outcome != constUtil.P3PaymentConfirmed && outcome != constUtil.P3PaymentFailed) {
		return nil, p3Error()
	}
	var initial db.PaymentOrder
	if err := db.GetIns().WithContext(ctx).First(&initial, "id = ?", orderID).Error; err != nil {
		return nil, err
	}
	var order db.PaymentOrder
	hash := p3Hash([]byte(fmt.Sprintf("%s|%d|%s", orderID, cents, outcome)))
	err := s.p3Profile(ctx, initial.PlayerID, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		if !walletAccountOwner(tx, row) || (ctx.Value(constUtil.LocalWalletPaymentContextKey) != true && !strings.HasPrefix(row.Uid, "demo-")) {
			return p3Error()
		}
		if ctx.Value(constUtil.LocalWalletAPIContextKey) == true && !localWalletSessionAllowed(ctx, row) {
			return ErrLocalWalletInvalid
		}
		if err := tx.First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if cents != order.Cents || order.Source != constUtil.P3PaymentSource {
			return p3Error()
		}
		var event db.PaymentConfirmation
		found := tx.First(&event, "event_id = ?", eventID).Error
		if found == nil {
			if event.OrderID != orderID || event.PayloadHash != hash {
				return p3Error()
			}
			return nil
		}
		if !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		if order.Status != constUtil.P3PaymentPending && order.Status != outcome {
			return p3Error()
		}
		if order.Status == constUtil.P3PaymentPending {
			if now.UnixMilli() < order.CreatedAt || now.UnixMilli() >= order.ExpiresAt {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			if outcome == constUtil.P3PaymentConfirmed {
				var product db.PaymentProduct
				if err := json.Unmarshal([]byte(order.Snapshot), &product); err != nil {
					return err
				}
				if err := validateP3Product(&product); err != nil {
					return err
				}
				if ctx.Value(constUtil.LocalWalletPaymentContextKey) == true {
					if err := s.LocalWalletDebitOrder(tx, row, &order, now); err != nil {
						return err
					}
				}
				credited := product.Charge
				if product.Kind == "once" {
					credited += product.Free
				}
				if product.FirstDouble && product.Kind == "once" {
					var count int64
					if err := tx.Model(&db.PaymentOrder{}).Where("player_id = ? AND product_id = ? AND status = ?", p.GetPlayerId(), product.ID, constUtil.P3PaymentConfirmed).Count(&count).Error; err != nil {
						return err
					}
					if count == 0 {
						credited += product.Charge
					}
				}
				if credited > math.MaxInt32 || p.GetChargeDiamond() > math.MaxInt64-product.Charge || p.GetChargeCount() == math.MaxInt32 {
					return p3Error()
				}
				reward := fmt.Sprintf("diamond,%d", credited)
				if product.Reward != "" {
					reward += ";" + product.Reward
				}
				if _, err := s.GrantFixedReward(p, g, reward); err != nil {
					return err
				}
				oldVIP := p.GetVip()
				p.ChargeDiamond = proto.Int64(p.GetChargeDiamond() + product.Charge)
				p.ChargeCount = proto.Int32(p.GetChargeCount() + 1)
				if err := s.applyP3VIP(p, oldVIP); err != nil {
					return err
				}
				if product.Kind == "month" {
					if p.GetMonthCardEndTime() > LocalPlayAt(ctx, now).UnixMilli()+5*24*3600*1000 {
						return errors.New(constUtil.P3PaymentUnavailable)
					}
					end := max(LocalPlayAt(ctx, now).UnixMilli(), p.GetMonthCardEndTime()) + constUtil.P3MonthCardDurationMS
					p.MonthCardEndTime = proto.Int64(end)
				}
				if product.Kind == "permanent" {
					if p.GetPermanentCardValid() != 0 {
						return errors.New(constUtil.P3PaymentUnavailable)
					}
					p.PermanentCardValid = proto.Int32(1)
				}
				order.Credited = credited
				order.ConfirmedAt = now.UnixMilli()
				if err := localPlayRecordPaymentTime(tx, &order, now); err != nil {
					return err
				}
				// The same transaction's activity reads need the accepted order fact.
				// Failure in an event or grant still rolls back this status and debit.
				order.Status = outcome
				if err := tx.Save(&order).Error; err != nil {
					return err
				}
				if err := s.RecordP1Event(tx, row.Id, "payment:"+order.ID, "payment_amount", cents, LocalPlayAt(ctx, now)); err != nil {
					return err
				}
				if err := s.RecordP1Event(tx, row.Id, "payment:"+order.ID, "diamond_purchase", product.Charge, LocalPlayAt(ctx, now)); err != nil {
					return err
				}
				if err := saveProfile(tx, row, p, g); err != nil {
					return err
				}
			}
			if order.Status != outcome {
				order.Status = outcome
				if err := tx.Save(&order).Error; err != nil {
					return err
				}
			}
			if outcome == constUtil.P3PaymentConfirmed {
				if err := s.refreshP3OpenServer(tx, p, g, LocalPlayAt(ctx, now)); err != nil {
					return err
				}
			}
		}
		event = db.PaymentConfirmation{EventID: eventID, OrderID: orderID, PayloadHash: hash, Outcome: outcome, At: now.UnixMilli(), Source: constUtil.P3PaymentSource}
		return tx.Create(&event).Error
	})
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// refreshP3OpenServer projects confirmed orders and evaluates original opening
// tasks in the payment transaction. Page reads use the same fact-only helper.
func (s *GameService) refreshP3OpenServer(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) error {
	if !gameSchemaHasTable(tx, &db.OpenServerActivityRule{}) {
		return nil
	}
	return s.refreshOpenServerProgressAt(tx, p, g, now)
}

// refreshP3OpenServerCounters rechecks actual paid orders within the accepted
// activity window; daily maximums never combine two small orders into one.
func (s *GameService) refreshP3OpenServerCounters(tx *gorm.DB, id int64, cfg *db.OpenServerActivityRule, now time.Time) error {
	if !gameSchemaHasTable(tx, &db.PaymentOrder{}) {
		return nil
	}
	paymentStart := cfg.StartAt
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return err
	}
	if cycle != nil {
		paymentStart = max(paymentStart, cycle.CreatedAt)
	}
	orders, err := localPlayActivityPaymentOrders(tx, id, cfg.ID, paymentStart, min(cfg.EndAt, now.UnixMilli()+1))
	if err != nil {
		return err
	}
	total, _, _, err := paymentTotalsFromOrders(orders)
	if err != nil || total/100 > math.MaxInt32 {
		if err != nil {
			return err
		}
		return p3Error()
	}
	// Rebuild every actual payment day, including orders confirmed before the
	// current server process or before the period-wide second-week policy.
	values := map[string]int32{constUtil.P3OpenServerTotalPaid: int32(total / 100)}
	for _, order := range orders {
		if order.Cents/100 > math.MaxInt32 {
			return p3Error()
		}
		key := constUtil.P3OpenServerSinglePaid + p3Day(time.UnixMilli(order.ConfirmedAt))
		values[key] = max(values[key], int32(order.Cents/100))
	}
	previous, err := openServerReadCounters(tx, id)
	if err != nil {
		return err
	}
	old := map[string]int32{}
	for _, row := range previous {
		old[row.CounterKey] = row.Value
		if strings.HasPrefix(row.CounterKey, constUtil.P3OpenServerSinglePaid) {
			if _, found := values[row.CounterKey]; !found {
				values[row.CounterKey] = 0
			}
		}
	}
	for key, value := range values {
		if old[key] == value {
			continue
		}
		counter := db.OpenServerActivityCounter{PlayerID: id, CounterKey: key, Value: value}
		if err = openServerSaveCounter(tx, &counter); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) applyP3VIP(p *pb.Player, old int32) error {
	next := old
	var before, after map[string]string
	for _, r := range s.Tables["VipPerks"] {
		level := int32(tableNumber(r, "vip"))
		if level == old {
			before = r
		}
		if tableNumber(r, "chargeDiamond") <= float64(p.GetChargeDiamond()) && level > next {
			next = level
		}
	}
	for _, r := range s.Tables["VipPerks"] {
		if int32(tableNumber(r, "vip")) == next {
			after = r
		}
	}
	if before == nil || after == nil {
		return errors.New(constUtil.P3PaymentConfigError)
	}
	warrior := int32(tableNumber(after, "numPerk3")-tableNumber(before, "numPerk3")) * constUtil.BagCapacityScale
	equipment := int32(tableNumber(after, "numPerk4")-tableNumber(before, "numPerk4")) * constUtil.BagCapacityScale
	if warrior < 0 || equipment < 0 || p.GetWarriorBagSize() > math.MaxInt32-warrior || p.GetEquipBagSize() > math.MaxInt32-equipment {
		return p3Error()
	}
	p.Vip = proto.Int32(next)
	p.WarriorBagSize = proto.Int32(p.GetWarriorBagSize() + warrior)
	p.EquipBagSize = proto.Int32(p.GetEquipBagSize() + equipment)
	return nil
}

// PaymentTotals 查询确认订单事实；测试金额与真实货币不会混为外部支付到账。
func PaymentTotals(tx *gorm.DB, id, start, end int64) (cents, charge, count int64, err error) {
	var orders []db.PaymentOrder
	orders, err = localPlayPaymentOrders(tx, id, start, end)
	if err != nil {
		return
	}
	return paymentTotalsFromOrders(orders)
}

func paymentTotalsFromOrders(orders []db.PaymentOrder) (cents, charge, count int64, err error) {
	for _, o := range orders {
		var p db.PaymentProduct
		if e := json.Unmarshal([]byte(o.Snapshot), &p); e != nil {
			err = e
			return
		}
		if o.Cents < 0 || cents > math.MaxInt64-o.Cents || p.Charge < 0 || charge > math.MaxInt64-p.Charge {
			err = p3Error()
			return
		}
		cents += o.Cents
		charge += p.Charge
		count++
	}
	return
}

// HandlePayment 供共享分发接入；通用活动只处理本域明确ID，其他请求交回旧分支。
func (s *GameService) HandlePayment(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSGetPaymentInfo:
		var products []db.PaymentProduct
		if err := db.GetIns().WithContext(ctx).Where("enabled = ? AND shop = ?", true, b.GetShopId()).Order("cents,id").Find(&products).Error; err != nil {
			return nil, true, err
		}
		r := &pb.SCGetPaymentInfo{}
		for _, p := range products {
			match := false
			for _, c := range strings.Split(p.Channels, ";") {
				match = match || c == p3PaymentChannel(b.GetChannelId())
			}
			if !match {
				continue
			}
			chargeType, giftRate, err := s.firstDoublePaymentInfo(ctx, id, p)
			if err != nil {
				return nil, true, err
			}
			r.PaymentInfoList = append(r.PaymentInfoList, &pb.PaymentInfo{PaymentId: proto.String(p.ID), ShopId: proto.String(p.Shop), ChargeType: proto.String(chargeType), PaymentType: proto.String(p.Kind), Money: proto.Float64(float64(p.Cents) / 100), MoneyType: proto.String(p.Currency), Charge: proto.Int64(p.Charge), Free: proto.Int64(p.Free), PaymentName: proto.String(p.Name), PaymentDesc: proto.String(p.Description), GiftRate: proto.Int32(giftRate)})
		}
		return []proto.Message{r}, true, nil
	case *pb.CSGenerateOrder:
		r, e := s.generatePayment(ctx, id, b, stamp, time.Now())
		return r, true, e
	case *pb.CSCheckOrder:
		var order db.PaymentOrder
		if err := db.GetIns().WithContext(ctx).Where("id = ? AND player_id = ? AND product_id = ?", b.GetOrderId(), id, b.GetPaymentId()).First(&order).Error; err != nil {
			return nil, true, err
		}
		p, _, err := s.resourceProfile(ctx, id)
		if err != nil {
			return nil, true, err
		}
		return []proto.Message{&pb.SCCheckOrder{TransactionData: p3Transaction(&order)}, &pb.SCPlayer{Player: p}}, true, nil
	case *pb.CSFailOrder:
		var order db.PaymentOrder
		err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
			if e := tx.Where("id = ? AND player_id = ? AND product_id = ?", b.GetOrderId(), id, b.GetPaymentId()).First(&order).Error; e != nil {
				return e
			}
			if order.Status == constUtil.P3PaymentConfirmed {
				return p3Error()
			}
			order.Status = constUtil.P3PaymentFailed
			return tx.Save(&order).Error
		})
		return []proto.Message{&pb.SCFailOrder{TransactionData: p3Transaction(&order)}}, true, err
	case *pb.CSIsCardClaimed, *pb.CSClaimCard, *pb.CSGetVipBoonInfo, *pb.CSGetVipDailyBoonReward, *pb.CSGetVipWeekBoonReward:
		r, e := s.membership(ctx, id, request, LocalPlayNow(ctx))
		return r, true, e
	case *pb.CSShopProductList:
		if b.GetShopType() == 2 {
			r, e := s.vipShop(ctx, id, request, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSShopBuy:
		if b.GetShopType() == 2 {
			r, e := s.vipShop(ctx, id, request, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSActivityInfo:
		if strings.HasSuffix(b.GetActivityId(), "_local_p3") {
			r, e := s.paymentActivity(ctx, id, request, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSActivityReward:
		if strings.HasSuffix(b.GetActivityId(), "_local_p3") {
			r, e := s.paymentActivity(ctx, id, request, LocalPlayNow(ctx))
			return r, true, e
		}
	case *pb.CSContinuedRechargeInfo, *pb.CSContinuedRechargeReward:
		activityID := ""
		switch continued := request.(type) {
		case *pb.CSContinuedRechargeInfo:
			activityID = continued.GetActivityId()
		case *pb.CSContinuedRechargeReward:
			activityID = continued.GetActivityId()
		}
		if strings.HasPrefix(activityID, constUtil.ReWelfareContinuedPrefix) {
			r, e := s.reContinuedWelfare(ctx, id, request, stamp, LocalPlayNow(ctx))
			return r, true, e
		}
		r, e := s.paymentActivity(ctx, id, request, LocalPlayNow(ctx))
		return r, true, e
	case *pb.CSConvertGiftCode, *pb.CSOpenLuxuryEquipment, *pb.CSLuxuryEquipmentInfo, *pb.CSLuxuryEquipmentReward:
		r, e := s.paymentOffer(ctx, id, request, LocalPlayNow(ctx))
		return r, true, e
	}
	return nil, false, nil
}
