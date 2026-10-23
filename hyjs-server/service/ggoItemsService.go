package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InstallGGOItems 安装随服务编译的八种GGO道具，启动不依赖外部工作目录。
func (s *GameService) InstallGGOItems() error {
	if s.Tables == nil {
		s.Tables = map[string][]map[string]string{}
	}
	rows := []map[string]string{
		{"itemId": "item_9701", "name": "残损芯片", "star": "1", "icon": "ggo_chip_white", "desc": "外壳受损，仍保存少量旧世数据。沙漠中收集后，可在安全区交给终端结算员兑换信用点。", "category": "11"},
		{"itemId": "item_9702", "name": "标准芯片", "star": "2", "icon": "ggo_chip_green", "desc": "保存完整的通用设备资料。沙漠中收集后，可在安全区交给终端结算员兑换信用点。", "category": "11"},
		{"itemId": "item_9703", "name": "军用芯片", "star": "3", "icon": "ggo_chip_blue", "desc": "储存旧世军用设备参数，具有较高回收价值。在安全区交给终端结算员可兑换信用点。", "category": "11"},
		{"itemId": "item_9704", "name": "机密芯片", "star": "4", "icon": "ggo_chip_purple", "desc": "储存受限技术档案的数据芯片。在安全区交给终端结算员可兑换信用点。", "category": "11"},
		{"itemId": "item_9705", "name": "核心芯片", "star": "5", "icon": "ggo_chip_orange", "desc": "保存大型旧世设备的核心控制资料。在安全区交给终端结算员可兑换信用点。", "category": "11"},
		{"itemId": "item_9706", "name": "原型芯片", "star": "6", "icon": "ggo_chip_red", "desc": "记录未量产旧世技术的稀有芯片。在安全区交给终端结算员可兑换信用点。", "category": "11"},
		{"itemId": "item_9710", "name": "信用点", "star": "5", "icon": "ggo_salvage_credit", "desc": "在安全区结算遗迹芯片后获得的货币，可在信用商店兑换物品。", "category": "11"},
		{"itemId": "item_9711", "name": "超频点数", "star": "6", "icon": "ggo_overclock_point", "desc": "获得后自动存入所属账号的蓝绿终端。每个道具增加1超频点数，不占用背包。", "category": "11"},
	}
	for _, next := range rows {
		next["intro"] = next["desc"]
		found := false
		for i, old := range s.Tables["Item"] {
			if old["itemId"] == next["itemId"] {
				s.Tables["Item"][i] = next
				found = true
				break
			}
		}
		if !found {
			s.Tables["Item"] = append(s.Tables["Item"], next)
		}
	}
	return nil
}

// ConvertGGOOverclockItem 在调用方角色行锁和存档事务内将道具原子存入账号终端。
// 没有超频道具时不访问数据库；事务失败时消费与流水由调用方一起回滚。
func ConvertGGOOverclockItem(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
	if game == nil {
		return nil
	}
	index := -1
	var amount int64
	for i, item := range game.Items {
		if item.GetId() != constUtil.GGOOverclockID {
			continue
		}
		if index >= 0 || item.GetNum() <= 0 {
			return errors.New("超频道具数量或重复条目无效")
		}
		index, amount = i, int64(item.GetNum())
	}
	if index < 0 {
		return nil
	}
	if row == nil || row.AccountID < 1 || amount > math.MaxInt64/constUtil.GGOOverclockUnit {
		return errors.New("超频道具账号归属或数量无效")
	}
	if _, err := AccountForProfile(tx, row); err != nil {
		return err
	}
	key := constUtil.GGOIncomeSource + ":" + uuid.NewString()
	if err := terminalCredit(tx, row.AccountID, AdminProfileServerID(*row), row.Id, key, constUtil.GGOIncomeSource, constUtil.GGOIncomeLabel, constUtil.GGOOverclockID, amount*constUtil.GGOOverclockUnit, time.Now()); err != nil {
		return err
	}
	game.Items = append(game.Items[:index:index], game.Items[index+1:]...)
	return nil
}

// InitializeGGO 只创建缺失目录和幂等回执，保留运营已编辑内容。
func (s *GameService) InitializeGGO(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.GGOShopReceipt{}); err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("kind=?", constUtil.GGOShopKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		// 只改旧测试条目的默认说明，保留运营自行填写的来源及商品参数。
		return tx.Model(&db.P4CommerceRule{}).Where("id=? AND kind=? AND source=?", constUtil.GGOSeedRuleID, constUtil.GGOShopKey, "GGO测试参数：1回收信用点兑换1超频道具，每日1次，可后台调整").Update("source", "GGO样式测试：1信用点兑换1超频道具，每日1次；正式周限购待配置").Error
	}
	p := p4BaseProduct(constUtil.GGOSeedProductID, constUtil.GGOOverclockID+",1", constUtil.GGOCurrencyID+",1", 1, 1)
	p.Name = "超频点数"
	p.DailyLimit = 1
	row := db.P4CommerceRule{ID: constUtil.GGOSeedRuleID, Kind: constUtil.GGOShopKey, ShopType: 970, Enabled: true, StartAt: 1, EndAt: 253402214400000, ClaimEndAt: 253402214400000, Rules: adminJSON(p4CommerceRules{LiveEditable: true, Products: []p4Product{p}}), Source: "GGO样式测试：1信用点兑换1超频道具，每日1次；正式周限购待配置"}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func ggoShopCostAllowed(key, currency string) bool {
	if currency == constUtil.GGOOverclockID {
		return false
	}
	if key == constUtil.GGOShopKey {
		return currency == constUtil.GGOCurrencyID
	}
	return true
}

func (s *GameService) ggoCatalogue(tx *gorm.DB, player *pb.Player, game *model.GameState) (*model.GGOShopCatalogueResponse, error) {
	tx = tx.Session(&gorm.Session{})
	managed, raw, err := s.adminReadShop(tx, constUtil.GGOShopKey)
	if err != nil {
		return nil, err
	}
	out := &model.GGOShopCatalogueResponse{Name: constUtil.GGOShopName, CurrencyID: constUtil.GGOCurrencyID, CurrencyName: "信用点", CurrencyIcon: "ggo_salvage_credit", Balance: p4ResourceBalance(player, game, constUtil.GGOCurrencyID), Version: managed.Version, Enabled: managed.Settings.Enabled, Items: []model.GGOShopItem{}}
	if raw.Period == nil {
		out.Enabled = false
		return out, nil
	}
	now := LocalPlayNow(tx.Statement.Context)
	if err := s.requireShopAvailability(tx, constUtil.GGOShopKey, now); err != nil {
		if errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
			out.Enabled = false
		} else {
			return nil, err
		}
	}
	var counts []struct {
		ProductID string
		Bought    int64
		Today     int64
	}
	day := now.Format("2006-01-02")
	if err := tx.Model(&db.ShopState{}).Select("product_id, SUM(bought) AS bought, SUM(CASE WHEN day=? THEN bought ELSE 0 END) AS today", day).Where("player_id=? AND product_id LIKE ?", player.GetPlayerId(), constUtil.GGOShopStatePrefix+"%").Group("product_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	var stocks []db.P4CommerceStock
	if err := tx.Where("rule_id=?", raw.Period.ID).Find(&stocks).Error; err != nil {
		return nil, err
	}
	var rules p4CommerceRules
	if json.Unmarshal(raw.Period.Rules, &rules) != nil {
		return nil, constUtil.ErrAdminConfiguration
	}
	if len(rules.Products) > 0 {
		if _, err := s.validateP4Commerce(*raw.Period); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(rules.Products, func(i, j int) bool {
		if rules.Products[i].Priority == rules.Products[j].Priority {
			return rules.Products[i].Index < rules.Products[j].Index
		}
		return rules.Products[i].Priority < rules.Products[j].Priority
	})
	for _, p := range rules.Products {
		if len(p.ID)+len(constUtil.GGOShopStatePrefix) > 64 {
			return nil, constUtil.ErrAdminConfiguration
		}
		if !p.isEnabled() {
			continue
		}
		reward := strings.Split(p.Rewards, ",")
		cost, err := s.p4ParseCost(p.Cost)
		if err != nil || !ggoShopCostAllowed(constUtil.GGOShopKey, cost.GetId()) || len(reward) != 2 || strings.Contains(p.Rewards, ";") {
			return nil, constUtil.ErrAdminConfiguration
		}
		n, err := p1PositiveAmount(reward[1])
		if err != nil {
			return nil, err
		}
		item := s.table("Item", "itemId", reward[0])
		if item == nil {
			return nil, constUtil.ErrAdminConfiguration
		}
		remaining := int64(p.DailyLimit)
		if p.Stock >= 0 {
			remaining = min(remaining, int64(p.Stock))
		}
		total := int64(0)
		for _, c := range counts {
			if c.ProductID == constUtil.GGOShopStatePrefix+p.ID {
				total += c.Bought
				remaining -= c.Today
			}
		}
		if p.Stock >= 0 {
			for _, stock := range stocks {
				if stock.ProductID == p.ID {
					remaining = min(remaining, int64(p.Stock)-int64(stock.Used))
				}
			}
		}
		if p.TotalLimit > 0 {
			remaining = min(remaining, int64(p.TotalLimit)-total)
		}
		if player.GetLevel() < p.MinLevel || player.GetVip() < p.MinVIP {
			remaining = 0
		}
		quality, _ := strconv.ParseInt(item["star"], 10, 64)
		name := p.Name
		if name == "" {
			name = item["name"]
		}
		out.Items = append(out.Items, model.GGOShopItem{ID: p.ID, Name: name, ResourceID: reward[0], Amount: n, Price: max(1, (int64(cost.GetNum())*int64(p.DiscountBps)+9999)/10000), Remaining: max(0, remaining), Description: item["desc"], Icon: item["icon"], Quality: quality})
	}
	return out, nil
}

// GGOShopCatalogue 读取当前运营目录和持久化余额。
func (s *GameService) GGOShopCatalogue(ctx context.Context, playerID int64, session []byte) (*model.GGOShopCatalogueResponse, error) {
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	authorized, err := AuthorizeAccount(ctx, playerID, session)
	if err != nil {
		return nil, err
	}
	if AdminProfileServerID(*authorized) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, authorized)
	if err != nil {
		return nil, err
	}
	var out *model.GGOShopCatalogueResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&row, playerID).Error; err != nil {
			return err
		}
		if AdminProfileServerID(row) != s.OriginalServerID() || !bytes.Equal(row.Session, session) {
			return errors.New("本地会话无效，请重新登录")
		}
		player, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		out, err = s.ggoCatalogue(tx, player, game)
		return err
	})
	return out, err
}

// GGOShopBuy 锁角色后核对UUID回执、目录版本、限购和余额，原子扣费发奖存档。
func (s *GameService) GGOShopBuy(ctx context.Context, req model.GGOShopBuyRequest, session []byte) (*model.GGOShopBuyResponse, error) {
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	authorized, err := AuthorizeAccount(ctx, req.PlayerID, session)
	if err != nil {
		return nil, err
	}
	if AdminProfileServerID(*authorized) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, authorized)
	if err != nil {
		return nil, err
	}
	var out model.GGOShopBuyResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, req.PlayerID).Error; err != nil {
			return err
		}
		if AdminProfileServerID(row) != s.OriginalServerID() || !bytes.Equal(row.Session, session) {
			return errors.New("本地会话无效，请重新登录")
		}

		var prior db.GGOShopReceipt
		err := tx.First(&prior, "player_id=? AND request_id=?", req.PlayerID, req.RequestID).Error
		if err == nil {
			if prior.ProductID != req.ProductID || prior.Version != req.Version {
				return errors.New("购买请求编号已用于其他商品或版本")
			}
			return json.Unmarshal(prior.Response, &out)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		catalogue, err := s.ggoCatalogue(tx.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "UPDATE"}), player, game)
		if err != nil {
			return err
		}
		if catalogue.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if !catalogue.Enabled {
			return constUtil.ErrP4CommerceNotOpen
		}
		var offer *model.GGOShopItem
		for i := range catalogue.Items {
			if catalogue.Items[i].ID == req.ProductID {
				offer = &catalogue.Items[i]
				break
			}
		}
		if offer == nil || offer.Remaining < 1 {
			return errors.New("商品已下架或达到限购")
		}
		cost, err := s.p4ParseCost(fmt.Sprintf("%s,%d", constUtil.GGOCurrencyID, offer.Price))
		if err != nil {
			return err
		}
		now := LocalPlayNow(ctx)
		if err = s.p4Deduct(tx, row.Id, player, game, cost, "ggo-shop:"+req.RequestID, now); err != nil {
			return err
		}
		if _, err = s.GrantFixedReward(player, game, fmt.Sprintf("%s,%d", offer.ResourceID, offer.Amount)); err != nil {
			return err
		}
		// Shared save hook converts the reward before serializing the inventory.
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		state := db.ShopState{PlayerID: row.Id, ProductID: constUtil.GGOShopStatePrefix + req.ProductID, Day: now.Format("2006-01-02")}
		if err = tx.FirstOrCreate(&state).Error; err != nil {
			return err
		}
		if state.Bought == math.MaxInt32 {
			return errors.New("购买次数溢出")
		}
		state.Bought++
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		var cfg db.P4CommerceRule
		if err = tx.Select("id").Where("kind=? AND id NOT LIKE ?", constUtil.GGOShopKey, "shop_schedule_%").Order("start_at DESC,id DESC").First(&cfg).Error; err != nil {
			return err
		}
		stock := db.P4CommerceStock{RuleID: cfg.ID, ProductID: req.ProductID}
		if err = tx.FirstOrCreate(&stock).Error; err != nil {
			return err
		}
		if stock.Used == math.MaxInt32 {
			return errors.New("商品库存计数溢出")
		}
		stock.Used++
		if err = tx.Save(&stock).Error; err != nil {
			return err
		}
		after, err := s.ggoCatalogue(tx, player, game)
		if err != nil {
			return err
		}
		out = model.GGOShopBuyResponse{Catalogue: after, RewardID: offer.ResourceID, RewardAmount: offer.Amount, RequestID: req.RequestID}
		if offer.ResourceID == constUtil.GGOOverclockID {
			out.TerminalPointsAdded = offer.Amount
		}
		payload, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Create(&db.GGOShopReceipt{PlayerID: row.Id, RequestID: req.RequestID, ProductID: req.ProductID, Version: req.Version, Response: payload, CreatedAt: now.UnixMilli()}).Error
	})
	return &out, err
}
