package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
	"time"
)

type p4ExchangeBinding struct {
	Domain     string `json:"domain,omitempty"`
	Index      int32  `json:"index"`
	ChildID    string `json:"childId"`
	ChildIndex int32  `json:"childIndex"`
	ProductID  string `json:"productId"`
}

func p4ExchangeKey(now time.Time) string {
	return constUtil.P4ResourceExchangePrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
}

// p4ExchangeDirectory holds stable outer indices while every child owns its original quota.
func (s *GameService) p4ExchangeDirectory(tx *gorm.DB, key string, now time.Time) (*db.P4ResourceExchangeDirectory, []p4ExchangeBinding, error) {
	expected, err := localPlayExchangeKey(tx, now)
	if err != nil {
		return nil, nil, err
	}
	if key != expected {
		return nil, nil, errors.New("资源兑换实例未开放")
	}
	private := localPlayPrivateCycleID(key) != ""
	if private {
		allowed, err := localPlayActivityAccess(tx, key, false)
		if err != nil {
			return nil, nil, err
		}
		if !allowed {
			return nil, nil, errors.New("资源兑换实例不属于当前角色")
		}
	}
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	row := &db.P4ResourceExchangeDirectory{ID: key, Enabled: true, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), Source: constUtil.P4ResourceExchangeSource, Bindings: []byte{}, InputHash: ""}
	// Create the directory lock before child locks so concurrent first queries share one mapping.
	if !private {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(row, "id=?", key).Error; err != nil {
		return nil, nil, err
	}
	if !row.Enabled || now.UnixMilli() < row.StartAt || now.UnixMilli() >= row.EndAt {
		return nil, nil, errors.New("资源兑换实例未开放")
	}
	var bindings []p4ExchangeBinding
	if len(row.Bindings) == 0 {
		if private {
			return nil, nil, errors.New("私期资源兑换冻结目录缺失")
		}
		var children []db.P4CommerceRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("shop_type=0 AND kind=? AND start_at<? AND end_at>?", "exchange", row.EndAt, row.StartAt).Order("id").Find(&children).Error; err != nil {
			return nil, nil, err
		}
		for _, live := range children {
			allowed, err := localPlayActivityVisible(tx, live.ID)
			if err != nil {
				return nil, nil, err
			}
			if !allowed {
				continue
			}
			// New fireworks always append after old mappings, including the first publication.
			if strings.HasPrefix(live.ID, constUtil.P5GrowthResourceSupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P4LuckySupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P5WorldSeedSupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P5AwakeMaterialSupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P5AwakeElementSupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P5GloryFragmentSupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P9PetFoodSupplyPrefix) {
				continue
			}
			if strings.HasPrefix(live.ID, constUtil.P15RedOSSupplyPrefix) {
				allowed, err := p15RedOSSupplyAllowed(tx)
				if err != nil {
					return nil, nil, err
				}
				if !allowed {
					continue
				}
			}
			if ((strings.HasPrefix(live.ID, constUtil.P8IngredientSupplyPrefix) || (strings.HasPrefix(live.ID, constUtil.P15RedOSSupplyPrefix) || strings.HasPrefix(live.ID, constUtil.P5CrystalSupplyPrefix))) && !live.Enabled) || isP15SeasonalChild(live.ID) || !strings.HasPrefix(live.ID, "limitExchange_") {
				continue
			}
			cfg, rules, err := s.p4CommerceSnapshot(live)
			if err != nil {
				return nil, nil, err
			}
			if len(live.Frozen) == 0 {
				raw, _ := json.Marshal(cfg)
				if err = tx.Model(&live).Update("frozen", raw).Error; err != nil {
					return nil, nil, err
				}
			}
			for _, product := range rules.Products {
				bindings = append(bindings, p4ExchangeBinding{Index: int32(len(bindings) + 1), ChildID: cfg.ID, ChildIndex: product.Index, ProductID: product.ID})
			}
		}
		// Red P2 retains its own mutable operator catalog, period, daily state and stock.
		if tx.Migrator().HasTable(&db.RedP2Exchange{}) {
			var red []db.RedP2Exchange
			if err := tx.Order("id").Find(&red).Error; err != nil {
				return nil, nil, err
			}
			for _, product := range red {
				if product.ID < 1 {
					return nil, nil, errors.New("红色兑换商品索引无效")
				}
				bindings = append(bindings, p4ExchangeBinding{Domain: "red-p2", Index: int32(len(bindings) + 1), ChildID: constUtil.P2RedExchangeActivity, ChildIndex: product.ID, ProductID: strconv.Itoa(int(product.ID))})
			}
		}
		if len(bindings) == 0 || len(bindings) > 1000 {
			return nil, nil, errors.New("资源兑换目录为空或超限")
		}
		row.Bindings, _ = json.Marshal(bindings)
		hash := sha256.Sum256(row.Bindings)
		row.InputHash = hex.EncodeToString(hash[:])
		if err := tx.Save(row).Error; err != nil {
			return nil, nil, err
		}
	} else {
		hash := sha256.Sum256(row.Bindings)
		if row.InputHash != hex.EncodeToString(hash[:]) || json.Unmarshal(row.Bindings, &bindings) != nil {
			return nil, nil, errors.New("资源兑换映射损坏")
		}
	}
	// 私期完整目录已在工厂冻结，后续查询不追加全局商品或重建映射。
	if !private {
		if e := s.p5AppendGoldEggBinding(tx, row, &bindings, now); e != nil {
			return nil, nil, e
		}
		if e := s.p8AppendIngredientBindings(tx, row, &bindings, now); e != nil {
			return nil, nil, e
		}
		if e := s.p15AppendRedOSBindings(tx, row, &bindings, now); e != nil {
			return nil, nil, e
		}
		if e := s.p5AppendCrystalBindings(tx, row, &bindings, now); e != nil {
			return nil, nil, e
		}
		if e := s.p4AppendLuckySupplyBinding(tx, row, &bindings, now); e != nil {
			return nil, nil, e
		}
		if err := s.p5AppendWorldSeedBinding(tx, row, &bindings, now); err != nil {
			return nil, nil, err
		}
		if err := s.p5AppendAwakeMaterialBinding(tx, row, &bindings, now); err != nil {
			return nil, nil, err
		}
		if err := s.p5AppendGrowthResourceBindings(tx, row, &bindings, now); err != nil {
			return nil, nil, err
		}
		if err := s.p5AppendAwakeElementBinding(tx, row, &bindings, now); err != nil {
			return nil, nil, err
		}
		if err := s.p5AppendGloryFragmentBinding(tx, row, &bindings, now); err != nil {
			return nil, nil, err
		}
		if err := s.p9AppendPetFoodBinding(tx, row, &bindings, now); err != nil {
			return nil, nil, err
		}
	}
	seen := map[int32]bool{}
	for _, b := range bindings {
		if b.Index < 1 || seen[b.Index] || b.ChildIndex < 1 || b.ChildID == key || b.ProductID == "" {
			return nil, nil, errors.New("资源兑换映射无效")
		}
		seen[b.Index] = true
	}
	return row, bindings, nil
}

// p4ExchangeProjection projects the same underlying inventories and limits into one original page.
func (s *GameService) p4ExchangeProjection(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, key string, now time.Time) (*pb.SCActivityShopInfo, error) {
	if e := s.seedP4Commerce(tx, now); e != nil {
		return nil, e
	}
	directory, bindings, err := s.p4ExchangeDirectory(tx, key, now)
	if err != nil {
		return nil, err
	}
	reply := &pb.SCActivityShopInfo{ActivityId: proto.String(key), StartTime: proto.Int64(directory.StartAt), EndTime: proto.Int64(directory.EndAt), Param: proto.String("1.00")}
	children := map[string]map[int32]*pb.ActivityShopItem{}
	for _, binding := range bindings {
		projection, known := children[binding.ChildID]
		if !known && binding.Domain == "red-p2" {
			projection = map[int32]*pb.ActivityShopItem{}
			children[binding.ChildID] = projection
			var period db.GameplayPool
			if e := tx.First(&period, "id=?", constUtil.P2RedWheelPool).Error; e != nil {
				return nil, e
			}
			if period.Enabled && now.UnixMilli() >= period.StartAt && now.UnixMilli() < period.EndAt && p.GetLevel() >= 20 {
				items, e := s.redExchangeItems(tx, p, g)
				if e != nil {
					return nil, e
				}
				for _, item := range items {
					projection[item.GetIndex()] = item
				}
			}
			known = true
		}
		if !known {
			var live db.P4CommerceRule
			if err = tx.First(&live, "id=?", binding.ChildID).Error; err != nil {
				return nil, err
			}
			cfg, _, e := s.p4CommerceSnapshot(live)
			if e != nil {
				return nil, e
			}
			projection = map[int32]*pb.ActivityShopItem{}
			children[binding.ChildID] = projection
			if strings.HasPrefix(binding.ChildID, constUtil.P15RedOSSupplyPrefix) {
				allowed, e := p15RedOSSupplyAllowed(tx)
				if e != nil {
					return nil, e
				}
				if !allowed {
					continue
				}
			}
			if strings.HasPrefix(binding.ChildID, constUtil.P4LuckySupplyPrefix) {
				ready, e := p4LuckySupplyReady(tx)
				if e != nil {
					return nil, e
				}
				if !ready {
					continue
				}
			}
			if strings.HasPrefix(binding.ChildID, constUtil.P5GoldEggSupplyPrefix) {
				var use db.P5GoldEggRule
				if e = tx.First(&use, "id=?", constUtil.P5GoldEggRuleID).Error; e != nil {
					return nil, e
				}
				if !use.Enabled {
					continue
				}
			}
			if !live.Enabled || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
				continue
			}
			cfg, rules, row, state, e := s.loadP4Commerce(tx, id, binding.ChildID, 0, now)
			if e != nil {
				return nil, e
			}
			_, items, e := s.p4Projection(tx, p, cfg, rules, state)
			if e != nil {
				return nil, e
			}
			if e = saveP4Commerce(tx, row, state); e != nil {
				return nil, e
			}
			for _, item := range items {
				projection[item.GetIndex()] = item
			}
		}
		if item := projection[binding.ChildIndex]; item != nil {
			cloned := proto.Clone(item).(*pb.ActivityShopItem)
			cloned.Index = proto.Int32(binding.Index)
			reply.ShopItemList = append(reply.ShopItemList, cloned)
		}
	}
	return reply, nil
}
func (s *GameService) p4ExchangeResolve(tx *gorm.DB, key string, index int32, now time.Time) (p4ExchangeBinding, error) {
	_, bindings, err := s.p4ExchangeDirectory(tx, key, now)
	if err != nil {
		return p4ExchangeBinding{}, err
	}
	for _, binding := range bindings {
		if binding.Index == index {
			if binding.Domain == "red-p2" {
				if binding.ProductID != strconv.Itoa(int(binding.ChildIndex)) || binding.ChildID != constUtil.P2RedExchangeActivity {
					return binding, errors.New("红色兑换映射无效")
				}
				return binding, nil
			}
			var live db.P4CommerceRule
			if err = tx.First(&live, "id=?", binding.ChildID).Error; err != nil {
				return binding, err
			}
			_, rules, e := s.p4CommerceSnapshot(live)
			if e != nil {
				return binding, e
			}
			for _, product := range rules.Products {
				if product.Index == binding.ChildIndex && product.ID == binding.ProductID {
					return binding, nil
				}
			}
			return binding, errors.New("资源兑换商品映射不匹配冻结目录")
		}
	}
	return p4ExchangeBinding{}, errors.New("当前目录无该商品")
}

// P4ResourceExchangeCoversRed allows the root list to hide the legacy Type23 only
// when the open aggregate contains every enabled P2 product and original eligibility holds.
func (s *GameService) P4ResourceExchangeCoversRed(ctx context.Context, id int64, now time.Time) (bool, error) {
	if !s.P4Ready {
		return false, nil
	}
	covered := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		key, err := localPlayExchangeKey(tx, now)
		if err != nil {
			return err
		}
		var directory db.P4ResourceExchangeDirectory
		e := tx.First(&directory, "id=?", key).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil && (!directory.Enabled || now.UnixMilli() < directory.StartAt || now.UnixMilli() >= directory.EndAt) {
			return nil
		}
		if !tx.Migrator().HasTable(&db.RedP2Exchange{}) {
			return nil
		}
		var period db.GameplayPool
		if e = tx.First(&period, "id=?", constUtil.P2RedWheelPool).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			}
			return e
		}
		if !period.Enabled || now.UnixMilli() < period.StartAt || now.UnixMilli() >= period.EndAt {
			return nil
		}
		var profile db.Profile
		if e = tx.First(&profile, id).Error; e != nil {
			return e
		}
		p, _, e := decodeProfile(&profile)
		if e != nil {
			return e
		}
		if p.GetLevel() < 20 {
			return nil
		}
		_, bindings, e := s.p4ExchangeDirectory(tx, key, now)
		if e != nil {
			return e
		}
		var products []db.RedP2Exchange
		if e = tx.Where("enabled=?", true).Find(&products).Error; e != nil {
			return e
		}
		if len(products) == 0 {
			return nil
		}
		mapped := map[int32]bool{}
		for _, binding := range bindings {
			if binding.Domain == "red-p2" && binding.ChildID == constUtil.P2RedExchangeActivity {
				mapped[binding.ChildIndex] = true
			}
		}
		for _, product := range products {
			if !mapped[product.ID] {
				return nil
			}
		}
		covered = true
		return nil
	})
	return covered, err
}

func (s *GameService) p4ExchangeRedOpen(tx *gorm.DB, now time.Time) (bool, error) {
	if !tx.Migrator().HasTable(&db.RedP2Exchange{}) {
		return false, nil
	}
	var count int64
	if e := tx.Model(&db.RedP2Exchange{}).Where("enabled=?", true).Count(&count).Error; e != nil {
		return false, e
	}
	if count == 0 {
		return false, nil
	}
	var period db.GameplayPool
	e := tx.First(&period, "id=?", constUtil.P2RedWheelPool).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	return period.Enabled && now.UnixMilli() >= period.StartAt && now.UnixMilli() < period.EndAt, nil
}
