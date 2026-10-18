package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math/big"
	"strings"
	"time"
)

// initializeWalletReasons 只首次插入28种试玩情境，保留运营后续调整。
func initializeWalletReasons(ctx context.Context) error {
	max := constUtil.LocalWalletMaxCreditCents
	rows := []db.LocalWalletReason{
		{ID: "change", Label: "零钱罐转入", MinCents: 1, MaxCents: 5000, Weight: 35, Enabled: true},
		{ID: "refund", Label: "购物退款到账", MinCents: 1, MaxCents: 100000, Weight: 25, Enabled: true},
		{ID: "redpacket", Label: "朋友的好运红包", MinCents: 100, MaxCents: 20000, Weight: 24, Enabled: true},
		{ID: "coffee", Label: "咖啡钱退回来了", MinCents: 500, MaxCents: 10000, Weight: 15, Enabled: true},
		{ID: "secondhand", Label: "闲置物品卖出", MinCents: 1000, MaxCents: 300000, Weight: 30, Enabled: true},
		{ID: "cashback", Label: "消费返现到账", MinCents: 1, MaxCents: 50000, Weight: 15, Enabled: true},
		{ID: "dinner", Label: "朋友归还聚餐垫付款", MinCents: 1000, MaxCents: 200000, Weight: 20, Enabled: true},
		{ID: "commute", Label: "通勤补贴到账", MinCents: 1000, MaxCents: 100000, Weight: 16, Enabled: true},
		{ID: "dailyjob", Label: "今日兼职报酬", MinCents: 5000, MaxCents: 200000, Weight: 30, Enabled: true},
		{ID: "commission", Label: "作品稿费到账", MinCents: 10000, MaxCents: 2000000, Weight: 30, Enabled: true},
		{ID: "salary", Label: "本月工资到账", MinCents: 100000, MaxCents: 5000000, Weight: 50, Enabled: true},
		{ID: "bonus", Label: "项目奖金到账", MinCents: 50000, MaxCents: 20000000, Weight: 35, Enabled: true},
		{ID: "annualbonus", Label: "年终奖到账，辛苦值得", MinCents: 300000, MaxCents: 100000000, Weight: 28, Enabled: true},
		{ID: "client", Label: "客户项目尾款到账", MinCents: 50000, MaxCents: 100000000, Weight: 30, Enabled: true},
		{ID: "store", Label: "小店营业款结算", MinCents: 10000, MaxCents: 50000000, Weight: 30, Enabled: true},
		{ID: "royalty", Label: "作品版权收益到账", MinCents: 100000, MaxCents: 500000000, Weight: 20, Enabled: true},
		{ID: "scholarship", Label: "奖学金到账", MinCents: 100000, MaxCents: 2000000, Weight: 14, Enabled: true},
		{ID: "birthday", Label: "生日祝福礼金", MinCents: 10000, MaxCents: 2000000, Weight: 15, Enabled: true},
		{ID: "family", Label: "家人的心意到账", MinCents: 10000, MaxCents: 10000000, Weight: 20, Enabled: true},
		{ID: "deposit", Label: "到期存款转入", MinCents: 100000, MaxCents: 1000000000, Weight: 25, Enabled: true},
		{ID: "dividend", Label: "本期分红到账", MinCents: 100000, MaxCents: 10000000000, Weight: 25, Enabled: true},
		{ID: "competition", Label: "比赛奖金到账", MinCents: 10000, MaxCents: 100000000, Weight: 18, Enabled: true},
		{ID: "scratch", Label: "刮刮乐中奖 {amount} 元", MinCents: 500, MaxCents: 10000000, Weight: 8, Enabled: true},
		{ID: "lottery", Label: "彩票中奖 {amount} 元，今天好运爆棚", MinCents: 10000000, MaxCents: 5000000000, Weight: 35, Enabled: true},
		{ID: "property", Label: "房产出售款到账", MinCents: 10000000, MaxCents: 10000000000, Weight: 25, Enabled: true},
		{ID: "business", Label: "创业股权转让款到账", MinCents: 100000000, MaxCents: 100000000000, Weight: 30, Enabled: true},
		{ID: "jackpot", Label: "超级联合奖池中奖 {amount} 元", MinCents: 1000000000, MaxCents: 1000000000000, Weight: 40, Enabled: true},
		{ID: "cosmic", Label: "宇宙级幸运大奖：奖金 {amount} 元", MinCents: 100000000000, MaxCents: max, Weight: 100, Enabled: true},
	}
	return db.GetIns().WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}
func walletReasonLabel(label string, cents int64) string {
	return strings.ReplaceAll(label, "{amount}", fmt.Sprintf("%d.%02d", cents/100, cents%100))
}

// drawWalletReason 按实际金额选可用区间，再用密码学随机源按相对权重抽取。
func drawWalletReason(tx *gorm.DB, cents int64) (*db.LocalWalletReason, error) {
	var rows []db.LocalWalletReason
	if err := tx.Where("enabled=? AND min_cents<=? AND max_cents>=?", true, cents, cents).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	var total int64
	for _, r := range rows {
		if r.Weight > 0 && r.Weight <= 1000000 {
			total += r.Weight
		}
	}
	if total < 1 {
		return nil, errors.New("该金额暂无可用入账情境，请调整金额或在后台设置理由")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return nil, err
	}
	value := n.Int64()
	for _, r := range rows {
		if r.Weight < 1 || r.Weight > 1000000 {
			continue
		}
		if value < r.Weight {
			return &r, nil
		}
		value -= r.Weight
	}
	return nil, ErrLocalWalletInvalid
}
func walletSummary(tx *gorm.DB, id, start, end int64) (model.LocalWalletSummary, error) {
	summary := model.LocalWalletSummary{StartAt: start, EndAt: end}
	q := tx.Model(&db.LocalWalletEntry{}).Where("player_id=?", id)
	if start > 0 {
		q = q.Where("at>=?", start)
	}
	if end > 0 {
		q = q.Where("at<?", end)
	}
	var totals struct {
		Income  int64
		Expense int64
		Count   int64
	}
	err := q.Select("COALESCE(SUM(CASE WHEN delta_cents>0 THEN delta_cents ELSE 0 END),0) AS income, COALESCE(SUM(CASE WHEN delta_cents<0 THEN -delta_cents ELSE 0 END),0) AS expense, COUNT(*) AS count").Scan(&totals).Error
	summary.IncomeCents = totals.Income
	summary.ExpenseCents = totals.Expense
	summary.NetCents = totals.Income - totals.Expense
	summary.TransactionCount = totals.Count
	return summary, err
}
func walletLedger(tx *gorm.DB, id, start, end int64, kind string, page, size int64) (*model.LocalWalletView, error) {
	var balance db.LocalWalletBalance
	err := tx.First(&balance, "player_id=?", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	view := localWalletView(&balance, true)
	view.Page = page
	view.PageSize = size
	view.Summary, err = walletSummary(tx, id, start, end)
	if err != nil {
		return nil, err
	}
	q := tx.Model(&db.LocalWalletEntry{}).Where("player_id=?", id)
	if start > 0 {
		q = q.Where("at>=?", start)
	}
	if end > 0 {
		q = q.Where("at<?", end)
	}
	if kind == "credit" {
		q = q.Where("kind=?", "add")
	}
	if kind == "payment" {
		q = q.Where("kind=?", "debit")
	}
	if err = q.Count(&view.Total).Error; err != nil {
		return nil, err
	}
	var entries []db.LocalWalletEntry
	if err = q.Order("revision DESC, `key` DESC").Offset(int((page - 1) * size)).Limit(int(size)).Find(&entries).Error; err != nil {
		return nil, err
	}
	for _, e := range entries {
		k, label := "credit", e.Label
		if e.Kind == "debit" {
			k = "payment"
			if label == "" {
				label = "游戏充值"
			}
		} else if e.Kind == "add" {
			if label == "" {
				label = "余额入账"
			}
		} else {
			return nil, ErrLocalWalletInvalid
		}
		order := ""
		if e.OrderID != nil {
			order = *e.OrderID
		}
		view.Entries = append(view.Entries, model.LocalWalletEntryView{ID: e.Key, Kind: k, AmountCents: e.DeltaCents, BalanceCents: e.BalanceAfter, CreatedAt: e.At, Label: label, ReasonID: e.ReasonID, OrderID: order})
	}
	return view, nil
}

// LocalWalletLedger 仅读取当前认证角色的账单，时间使用真实账务时钟。
func (s *GameService) LocalWalletLedger(ctx context.Context, req model.LocalWalletLedgerRequest) (*model.LocalWalletView, error) {
	if !s.P3LocalWalletEnabled || ctx.Value(constUtil.LocalWalletAPIContextKey) != true {
		return nil, ErrLocalWalletDisabled
	}
	var view *model.LocalWalletView
	err := s.p3Profile(ctx, req.PlayerID, func(tx *gorm.DB, owner *db.Profile, _ *pb.Player, _ *model.GameState) error {
		if !walletAccountOwner(owner) || !localWalletSessionAllowed(ctx, owner) {
			return ErrLocalWalletInvalid
		}
		policy, err := localWalletPolicy(tx)
		if err != nil {
			return err
		}
		view, err = walletLedger(tx, req.PlayerID, req.StartAt, req.EndAt, req.Kind, req.Page, req.PageSize)
		if err == nil {
			view.Enabled = policy.Enabled
		}
		return err
	})
	return view, err
}

// AdminWalletReasons 返回供未来入账使用的实际运营权重和金额区间。
func AdminWalletReasons(ctx context.Context) ([]map[string]any, error) {
	var rows []db.LocalWalletReason
	if err := db.GetIns().WithContext(ctx).Order("min_cents,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{"id": r.ID, "label": r.Label, "minCents": r.MinCents, "maxCents": r.MaxCents, "weight": r.Weight, "enabled": r.Enabled, "version": adminVersion(r)})
	}
	return out, nil
}

// AdminSaveWalletReason 在事务内核对权限与版本，不重写已冻结入账文案。
func AdminSaveWalletReason(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminWalletReasonRequest) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.LocalWalletReason
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; err != nil {
			return err
		}
		if adminVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		row.Label = req.Label
		row.MinCents = req.MinCents
		row.MaxCents = req.MaxCents
		row.Weight = req.Weight
		row.Enabled = req.Enabled
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "wallet.reason.update", id, req.Label)
	})
}

// AdminWalletLedger 供授权运营人员按角色读取完整分页账单。
func AdminWalletLedger(ctx context.Context, p *model.AdminPrincipal, id, start, end int64, kind string, page, size int64) (*model.LocalWalletView, error) {
	var out *model.LocalWalletView
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, false); err != nil {
			return err
		}
		var owner db.Profile
		if err := tx.Select("id").First(&owner, id).Error; err != nil {
			return err
		}
		var err error
		out, err = walletLedger(tx, id, start, end, kind, page, size)
		return err
	})
	return out, err
}

// WalletMonthStart 保持账单自然月与客户端显示时区一致。
func WalletMonthStart(now time.Time) int64 {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc).UnixMilli()
}
