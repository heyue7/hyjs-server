package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func adminInventoryError(message string) error {
	return &constUtil.AdminPlayerEditError{Message: message}
}

// adminInventoryCategory follows the same prefix dispatch as GrantFixedReward.
func (s *GameService) adminInventoryCategory(id string) string {
	switch {
	case strings.HasPrefix(id, "hero_"):
		row := s.table("Hero", "id", id)
		if row != nil && (row["heroClass"] == "0" || row["heroClass"] == "99") {
			return "warrior"
		}
	case strings.HasPrefix(id, "equip_"):
		if s.table("Equip", "id", id) != nil {
			return "equipment"
		}
	case strings.HasPrefix(id, "trea_"):
		if s.table("Treasure", "id", id) != nil {
			return "accessory"
		}
	case strings.HasPrefix(id, "item_"):
		if s.table("Item", "itemId", id) != nil {
			return "item"
		}
	case strings.HasPrefix(id, "frag_"):
		if s.table("Frag", "id", id) != nil {
			return "item"
		}
	case strings.HasPrefix(id, "MineItem_"):
		if row := s.table("MineSkill", "MineItemID", id); row != nil && row["Type"] != "mine" {
			return "gem"
		}
	case strings.HasPrefix(id, "FishItem_"):
		if row := s.table("FishSkill", "FishItemID", id); row != nil && row["Type"] != "dock" {
			return "cooking"
		}
	case strings.HasPrefix(id, "pet_"):
		if s.table("Pet", "id", id) != nil {
			return "pet"
		}
	}
	return ""
}

// AdminPlayerInventoryCatalogue filters the full indexed catalogue before pagination.
func (s *GameService) AdminPlayerInventoryCatalogue(category, q string, quality *int64, page, size int64) model.AdminPlayerInventoryCatalogueResponse {
	out := model.AdminPlayerInventoryCatalogueResponse{Items: []model.AdminItemDTO{}}
	for offset := int64(1); ; offset++ {
		batch := s.AdminCatalogueFiltered(model.AdminItemsRequest{Q: q, Quality: quality, Page: offset, PageSize: 500})
		for _, item := range batch.Items {
			c := s.adminInventoryCategory(item.ID)
			if c == "" || category != "" && category != c {
				continue
			}
			item.Category = c
			out.Total++
			if out.Total > (page-1)*size && int64(len(out.Items)) < size {
				out.Items = append(out.Items, item)
			}
		}
		if offset*500 >= batch.Total {
			break
		}
	}
	return out
}

func adminInventoryVersion(row db.Profile, pets []*pb.Pet) string {
	h := sha256.New()
	h.Write([]byte(adminPlayerVersion(row)))
	for _, pet := range pets {
		raw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(pet)
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func adminInventoryBattleGuard(g *model.GameState) error {
	if g.Run != nil && !g.Run.Complete || g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
		return adminInventoryError("请先结束副本或野外战斗，再编辑背包")
	}
	if g.Position != nil {
		scene := g.Position.GetScene()
		if scene != "" && scene != "town" && scene != "town_main" && !strings.HasPrefix(scene, "field_") {
			return adminInventoryError("请先退出当前特殊场景，再编辑背包")
		}
	}
	return nil
}

// adminInventoryAttributes resolves only numeric fields explicitly listed here.
func (s *GameService) adminInventoryAttributes(p *pb.Player, m proto.Message) []model.AdminInventoryAttribute {
	out := []model.AdminInventoryAttribute{}
	add := func(key, label string, min, max int64) {
		v, ok := adminInventoryNumeric(m, key, false, 0)
		if ok {
			out = append(out, model.AdminInventoryAttribute{Key: key, Label: label, Value: v, Min: min, Max: max})
		}
	}
	maxTable := func(table, key, match, value, field string) int64 {
		var n int64
		for _, r := range s.Tables[table] {
			if key == "" || r[key] == match {
				n = max(n, int64(tableNumber(r, field)))
			}
		}
		return n
	}
	config := func(key string) int64 {
		return int64(tableNumber(s.table("GameConfigure", "StaticName", key), "NameValue"))
	}
	switch v := m.(type) {
	case *pb.Warrior:
		row := s.table("Hero", "id", v.GetId())
		quality := row["quality"]
		levelMax := min(int64(p.GetLevel()), maxTable("HeroExp", "quality", quality, "", "level"))
		if v.GetGuid() != p.GetHeroWarId() {
			add("level", "等级", 1, levelMax)
			exp, _ := s.warriorLevelExperience(quality, v.GetLevel())
			add("exp", "本级经验", 0, max(0, exp-1))
		}
		add("breached.level", "突破等级", 1, maxTable("HeroBreached", "", "", "", "id"))
		breach := s.table("HeroBreached", "id", strconv.Itoa(int(v.GetBreached().GetLevel())))
		add("breached.exp", "突破经验", 0, max(0, int64(tableNumber(breach, "Exp"))-1))
		limits, _ := s.trainingLimits(p, v)
		for i, key := range []string{"hp", "att", "pdef", "mdef"} {
			add("training."+key, []string{"训练生命", "训练攻击", "训练物防", "训练魔防"}[i], 0, int64(limits[i]))
		}
		effectiveLevel := v.GetLevel()
		if v.GetGuid() == p.GetHeroWarId() {
			effectiveLevel = p.GetLevel()
		}
		talentMax := int64(0)
		for _, r := range s.Tables["TalentAdvenced"] {
			if r["warId"] == v.GetId() && r["trunck"] == "1" && tableNumber(r, "levelLimit") <= float64(effectiveLevel) {
				talentMax = max(talentMax, int64(tableNumber(r, "talentLevel")))
			}
		}
		add("talent", "主干天赋", 0, talentMax)
		add("awake.stage", "觉醒阶段", 0, maxTable("HeroAwakenAdd", "heroId", v.GetId(), "", "stage"))
		fuseMax := maxTable("HeroAwakenFuseProp", "heroId", v.GetId(), "", "endLevel")
		if v.GetAwake().GetStage() < 2 {
			fuseMax = 0
		}
		add("awake.fuseLevel", "融合等级", 0, fuseMax)
		fuseExp := int64(tableNumber(s.fuseTable(v, v.GetAwake().GetFuseLevel()), "needExp"))
		if v.GetAwake().GetStage() < 2 {
			fuseExp = 0
		}
		add("awake.fuseExp", "融合经验", 0, max(0, fuseExp-1))
	case *pb.Equipment:
		maximum := config("maxEquipLevel")
		limit := int64(math.Ceil(float64(p.GetLevel()) * tableNumber(s.table("GameConfigure", "StaticName", "playerLevel_Equip_Value"), "NameValue")))
		if int64(p.GetLevel()) >= config("maxPlayerLevel") {
			limit = maximum
		}
		add("level", "强化等级", 1, min(maximum, limit))
		levels, _ := s.equipmentRefineLevels(v)
		add("refineLv", "精炼等级", 0, int64(len(levels)))
		var exp int64
		if int(v.GetRefineLv()) < len(levels) && v.GetRefineLv() >= 0 {
			exp = levels[v.GetRefineLv()] - 1
		}
		add("refineExp", "精炼经验", 0, max(0, exp))
		typ := s.table("Equip", "id", v.GetEquipId())["type"]
		add("star", "星级", 0, maxTable("EquipStar", "pos", typ, "", "starLevel"))
		add("starExp", "星级经验", 0, 0)
		add("forgeLevel", "锻造等级", 0, maxTable("EquipForgeProp", "equipId", v.GetEquipId(), "", "forgeLevel"))
	case *pb.Treasure:
		add("strengthLevel", "强化等级", 1, int64(tableNumber(s.table("Treasure", "id", v.GetTreasureId()), "maxStrengthen")))
		exp, _ := s.treasureLevelExp(v, v.GetStrengthLevel())
		add("currentExp", "强化经验", 0, max(0, exp-1))
		add("refineLevel", "精炼等级", 0, maxTable("TreasureRefineLevel", "treasureId", v.GetTreasureId(), "", "level"))
	case *pb.Pet:
		row := s.table("Pet", "id", v.GetId())
		material := s.p9Material(v)
		add("level", "等级", 1, min(int64(p.GetLevel()), int64(tableNumber(material, "maxPetLevel"))))
		exp, _ := s.p9LevelExp(v, v.GetLevel())
		add("exp", "本级经验", 0, max(0, exp-1))
		add("breached.level", "突破等级", 0, config("PetBreachedMaxLevel"))
		add("eat.level", "亲密等级", 0, maxTable("PetIntimacy", "quality", row["quality"], "", "intimacyLevel"))
		intimacy := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(v.GetEat().GetLevel())))
		intimacyExp := max(0, int64(tableNumber(intimacy, "intimacy"))-1)
		if int64(v.GetEat().GetLevel()) >= config("PetMaxIntimacyLevel") {
			intimacyExp = 0
		}
		add("eat.exp", "亲密经验", 0, intimacyExp)
		for _, equip := range v.GetPetEquipList() {
			for _, field := range []struct{ key, label, table string }{{"strengthLevel", "强化", "PetEquipStrength"}, {"advanceLevel", "进阶", "PetEquipAdvance"}, {"awakeLevel", "觉醒", "PetEquipAwake"}} {
				maximum := maxTable(field.table, "equipId", equip.GetEquipId(), "", "level")
				if field.key == "strengthLevel" {
					cap, _ := s.p9EquipStrengthCap(v, equip)
					maximum = int64(cap)
				}
				key := "equip." + equip.GetEquipId() + "." + field.key
				var val int32
				switch field.key {
				case "strengthLevel":
					val = equip.GetStrengthLevel()
				case "advanceLevel":
					val = equip.GetAdvanceLevel()
				case "awakeLevel":
					val = equip.GetAwakeLevel()
				}
				out = append(out, model.AdminInventoryAttribute{Key: key, Label: equip.GetEquipId() + " " + field.label, Value: int64(val), Min: 0, Max: maximum})
			}
		}
	}
	return out
}

func adminInventoryNumeric(m proto.Message, key string, set bool, value int64) (int64, bool) {
	if pet, ok := m.(*pb.Pet); ok && strings.HasPrefix(key, "equip.") {
		parts := strings.Split(key, ".")
		if len(parts) != 3 {
			return 0, false
		}
		for _, equip := range pet.GetPetEquipList() {
			if equip.GetEquipId() == parts[1] {
				return adminInventoryNumeric(equip, parts[2], set, value)
			}
		}
		return 0, false
	}
	current := m.ProtoReflect()
	parts := strings.Split(key, ".")
	for i, part := range parts {
		f := current.Descriptor().Fields().ByName(protoreflect.Name(part))
		if f == nil || f.IsList() || f.IsMap() {
			return 0, false
		}
		if i < len(parts)-1 {
			if f.Kind() != protoreflect.MessageKind {
				return 0, false
			}
			if set {
				current = current.Mutable(f).Message()
			} else {
				current = current.Get(f).Message()
			}
			continue
		}
		switch f.Kind() {
		case protoreflect.Int32Kind, protoreflect.Int64Kind:
			if set {
				if f.Kind() == protoreflect.Int32Kind {
					current.Set(f, protoreflect.ValueOfInt32(int32(value)))
				} else {
					current.Set(f, protoreflect.ValueOfInt64(value))
				}
			}
			return current.Get(f).Int(), true
		}
	}
	return 0, false
}

func (s *GameService) adminInventoryRows(tx *gorm.DB, row db.Profile, p *pb.Player, g *model.GameState) ([]model.AdminInventoryRow, []*pb.Pet, error) {
	pets := []*pb.Pet{}
	if s.P9PetsReady {
		var err error
		pets, err = P9PetList(tx, row.Id)
		if err != nil {
			return nil, nil, err
		}
	}
	held, holdErr := p5WarriorMaterialHolds(tx, row.Id)
	if holdErr != nil {
		return nil, nil, holdErr
	}
	entries := []model.AdminInventoryRow{}
	catalogue := map[string]model.AdminItemDTO{}
	for _, item := range s.AdminCatalogueFiltered(model.AdminItemsRequest{Page: 1, PageSize: math.MaxInt32}).Items {
		catalogue[item.ID] = item
	}
	activityErr := s.adminInventoryActivityGuard(tx, row.Id, g)
	add := func(category, id, guid string, amount int64, m proto.Message) {
		item := catalogue[id]
		entry := model.AdminInventoryRow{Key: guid, Category: category, ResourceID: id, GUID: guid, Amount: amount, Name: item.Name, Quality: item.Quality, QualityLabel: item.QualityLabel, IconURL: item.IconURL, Attributes: []model.AdminInventoryAttribute{}, Editable: true, Removable: true}
		if entry.Name == "" {
			entry.Name = id
		}
		if guid == "" {
			entry.Key = id
		} else {
			entry.Attributes = s.adminInventoryAttributes(p, m)
			if n, ok := adminInventoryNumeric(m, "level", false, 0); ok {
				entry.Level = n
			}
			if category == "accessory" {
				entry.Level = int64(m.(*pb.Treasure).GetStrengthLevel())
			}
			if guid == p.GetHeroWarId() {
				entry.Level = int64(p.GetLevel())
				entry.Removable = false
				entry.BlockedReason = "主角不可移除；主角等级通过玩家资料修改"
			} else if err := P5GuardInstance(p, g, guid); err != nil {
				entry.Removable = false
				entry.BlockedReason = err.Error()
			}
			if category == "pet" && p9Guarded(g, guid) {
				entry.Removable = false
				entry.BlockedReason = "守护中的精灵不可移除"
			}
			if equip, ok := m.(*pb.Equipment); ok {
				for _, gem := range equip.GetGems() {
					if gem != "" {
						entry.Removable = false
						entry.BlockedReason = "请先卸下已镶嵌宝石"
					}
				}
			}
		}
		if category == "warrior" && s.table("Hero", "id", id)["heroClass"] == "99" {
			entry.Editable = false
			entry.BlockedReason = "经验卡沿原规则不可培养，可添加或移除"
		}
		if held[guid] {
			entry.Editable = false
			entry.Removable = false
			entry.BlockedReason = "远征派遣中的人物不可修改或移除"
		}
		if err := activityErr; err != nil {
			entry.Editable = false
			entry.Removable = false
			entry.BlockedReason = err.Error()
		}
		entries = append(entries, entry)
	}
	for _, v := range g.Warriors {
		add("warrior", v.GetId(), v.GetGuid(), 1, v)
	}
	for _, v := range g.Equipment {
		add("equipment", v.GetEquipId(), v.GetGuid(), 1, v)
	}
	for _, v := range g.Treasures {
		add("accessory", v.GetTreasureId(), v.GetGuid(), 1, v)
	}
	for _, set := range []struct {
		category string
		items    []*pb.SimpleItem
	}{{"item", g.Items}, {"gem", g.Gems}, {"cooking", g.Cooking}} {
		for _, v := range set.items {
			add(set.category, v.GetId(), "", int64(v.GetNum()), nil)
		}
	}
	for _, v := range pets {
		add("pet", v.GetId(), v.GetGuid(), 1, v)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Key < entries[j].Key
	})
	return entries, pets, nil
}

// AdminPlayerInventory reads native stored data without normal gameplay normalization.
func (s *GameService) AdminPlayerInventory(ctx context.Context, id int64, category, q string, quality *int64, page, size int64) (*model.AdminPlayerInventoryResponse, error) {
	var row db.Profile
	tx := db.GetIns().WithContext(ctx)
	if err := tx.First(&row, id).Error; err != nil {
		return nil, err
	}
	if err := AdminCheckProfileScope(ctx, row); err != nil {
		return nil, err
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	rows, pets, err := s.adminInventoryRows(tx, row, p, g)
	if err != nil {
		return nil, err
	}
	out := &model.AdminPlayerInventoryResponse{Items: []model.AdminInventoryRow{}, Categories: []model.AdminInventoryCategory{}, Version: adminInventoryVersion(row, pets)}
	for i, c := range []string{"warrior", "equipment", "accessory", "item", "gem", "cooking", "pet"} {
		entry := model.AdminInventoryCategory{Value: c, Label: []string{"人物", "装备", "饰品", "道具", "宝石/矿石", "料理/食材", "精灵"}[i]}
		for _, v := range rows {
			if v.Category == c {
				entry.Count++
			}
		}
		out.Categories = append(out.Categories, entry)
	}
	for _, v := range rows {
		if category != "" && category != v.Category || quality != nil && *quality != v.Quality || q != "" && !strings.Contains(strings.ToLower(v.Name+" "+v.ResourceID+" "+v.GUID), strings.ToLower(q)) {
			continue
		}
		out.Total++
		if out.Total > (page-1)*size && int64(len(out.Items)) < size {
			out.Items = append(out.Items, v)
		}
	}
	return out, nil
}

// AdminUpdatePlayerInventory commits the native edit and audit under the profile lock.
func (s *GameService) AdminUpdatePlayerInventory(ctx context.Context, actor *model.AdminPrincipal, id int64, req model.AdminPlayerInventoryRequest) (*model.AdminPlayerInventoryMutationResponse, error) {
	out := &model.AdminPlayerInventoryMutationResponse{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.adminApplyPlayerInventory(tx, actor, id, req, out)
	})
	return out, err
}

// adminApplyPlayerInventory lets batch additions share one transaction and the existing edit guards.
func (s *GameService) adminApplyPlayerInventory(tx *gorm.DB, actor *model.AdminPrincipal, id int64, req model.AdminPlayerInventoryRequest, out *model.AdminPlayerInventoryMutationResponse) error {
	ctx := tx.Statement.Context
	if err := ServerGameClockGuardTransaction(tx); err != nil {
		return err
	}
	if err := adminOperationUser(tx, actor, true); err != nil {
		return err
	}
	var row db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
		return err
	}
	if err := AdminCheckProfileScope(ctx, row); err != nil {
		return err
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		return err
	}
	rows, pets, err := s.adminInventoryRows(tx, row, p, g)
	if err != nil {
		return err
	}
	action := "player.inventory." + req.Operation
	target := strconv.FormatInt(id, 10) + ":" + req.RequestID
	hashReq := req
	hashReq.Version = ""
	bodyHash := p3Hash(adminJSON(hashReq))
	var audit db.AdminAudit
	lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("target=? AND action LIKE ?", target, "player.inventory.%").First(&audit).Error
	if lookup == nil {
		var receipt struct {
			BodyHash string `json:"bodyHash"`
		}
		if json.Unmarshal([]byte(audit.Detail), &receipt) != nil || audit.Actor != actor.Username || receipt.BodyHash != bodyHash || audit.Action != action {
			return adminInventoryError("请求编号已经用于其他操作")
		}
		out.Version = adminInventoryVersion(row, pets)
		return nil
	}
	if !errors.Is(lookup, gorm.ErrRecordNotFound) {
		return lookup
	}
	if adminInventoryVersion(row, pets) != req.Version {
		return constUtil.ErrAdminConfigConflict
	}
	if err := s.adminInventoryActivityGuard(tx, id, g); err != nil {
		return err
	}
	beforeP := proto.Clone(p).(*pb.Player)
	beforeG := &model.GameState{}
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, beforeG); err != nil {
		return err
	}
	var selected *model.AdminInventoryRow
	for i := range rows {
		if rows[i].Category == req.Category && rows[i].Key == req.Key {
			if selected != nil {
				return adminInventoryError("背包身份重复，请先修复存档")
			}
			selected = &rows[i]
		}
	}
	if req.Operation == "add" {
		if s.adminInventoryCategory(req.ResourceID) != req.Category {
			return adminInventoryError("该资源不属于可添加的背包分类")
		}
		if req.Category == "pet" {
			if !s.P9PetsReady {
				return adminInventoryError("精灵功能尚未就绪")
			}
			if _, err = s.P9GrantPet(tx, p, req.ResourceID, int32(*req.Amount)); err != nil {
				return adminInventoryError(err.Error())
			}
		} else {
			if _, err = s.GrantFixedReward(p, g, fmt.Sprintf("%s,%d", req.ResourceID, *req.Amount)); err != nil {
				return adminInventoryError(err.Error())
			}
		}
	} else {
		if selected == nil {
			return adminInventoryError("背包资源不存在或身份不匹配")
		}
		if req.Operation == "remove" && !selected.Removable {
			return adminInventoryError(selected.BlockedReason)
		}
		if req.Operation == "update" && !selected.Editable {
			return adminInventoryError(selected.BlockedReason)
		}
		if selected.GUID == "" {
			if len(req.Attributes) != 0 {
				return adminInventoryError("堆叠道具仅支持数量修改")
			}
			var list *[]*pb.SimpleItem
			switch req.Category {
			case "item":
				list = &g.Items
			case "gem":
				list = &g.Gems
			case "cooking":
				list = &g.Cooking
			default:
				return adminInventoryError("该分类不支持堆叠修改")
			}
			next := []*pb.SimpleItem{}
			for _, v := range *list {
				if v.GetId() != req.Key {
					next = append(next, v)
				} else if req.Operation == "update" {
					v.Num = proto.Int32(int32(*req.Amount))
					next = append(next, v)
				}
			}
			*list = next
		} else {
			if req.Amount != nil {
				return adminInventoryError("实例数量不可修改")
			}
			var current proto.Message
			if req.Category == "pet" {
				current, err = p9OwnedPet(tx, id, req.Key)
			} else {
				current, err = p5OwnedInstance(g, req.Key)
			}
			if err != nil {
				return err
			}
			if req.Operation == "remove" {
				switch req.Category {
				case "warrior":
					for i, v := range g.Warriors {
						if v.GetGuid() == req.Key {
							g.Warriors = append(g.Warriors[:i], g.Warriors[i+1:]...)
							break
						}
					}
				case "equipment":
					for i, v := range g.Equipment {
						if v.GetGuid() == req.Key {
							g.Equipment = append(g.Equipment[:i], g.Equipment[i+1:]...)
							break
						}
					}
				case "accessory":
					for i, v := range g.Treasures {
						if v.GetGuid() == req.Key {
							g.Treasures = append(g.Treasures[:i], g.Treasures[i+1:]...)
							break
						}
					}
				case "pet":
					pet := current.(*pb.Pet)
					data, e := proto.Marshal(pet)
					if e != nil {
						return e
					}
					if e = tx.Create(&db.P9PetRetired{GUID: req.Key, PlayerID: id, PetID: pet.GetId(), Data: data, Operation: "admin.inventory.remove", Timestamp: time.Now().UnixMilli(), Source: "后台移除，无卖出返还；投入历史保留"}).Error; e != nil {
						return e
					}
					if e = tx.Delete(&db.P9PetInstance{}, "player_id=? AND guid=?", id, req.Key).Error; e != nil {
						return e
					}
					p5SetKV(g, "p9.pet:"+req.Key, "")
				}
			} else {
				before := proto.Clone(current)
				if err = s.adminInventoryApplyAttributes(tx, p, current, req.Attributes); err != nil {
					return err
				}
				if req.Category == "pet" {
					if err = p9SavePet(tx, id, g, current.(*pb.Pet)); err != nil {
						return err
					}
				} else if p5KV(g, "p5.invest:"+req.Key) != "" {
					state, e := p5InvestmentState(beforeG, req.Key, before)
					if e != nil {
						return adminInventoryError("实例投入历史与当前状态不一致，请先核查")
					}
					state.Expected, e = p5InvestmentFingerprint(current)
					if e != nil {
						return e
					}
					data, e := json.Marshal(state)
					if e != nil {
						return e
					}
					p5SetKV(g, "p5.invest:"+req.Key, string(data))
				}
			}
		}
	}
	if s.P5Ready {
		power, e := s.formationFightPower(p, g)
		if e != nil {
			return adminInventoryError(e.Error())
		}
		p.FightPower = proto.Int32(power)
	}
	p5SetKV(g, "admin.inventory.revision", uuid.NewString())
	// Admin edits deliberately keep their delta-only saver. Convert immediate
	// terminal items here so add/update/batch share its existing atomic receipt.
	if err = ConvertGGOOverclockItem(tx, &row, p, g); err != nil {
		return err
	}
	if err = adminSavePlayerChanges(tx, &row, beforeP, beforeG, p, g); err != nil {
		return err
	}
	detail := struct {
		BodyHash string                            `json:"bodyHash"`
		Request  model.AdminPlayerInventoryRequest `json:"request"`
		Before   *model.AdminInventoryRow          `json:"before,omitempty"`
	}{bodyHash, req, selected}
	detail.Request.Version = ""
	if err = adminAudit(tx, actor, action, target, string(adminJSON(detail))); err != nil {
		return err
	}
	if s.P9PetsReady {
		pets, err = P9PetList(tx, id)
		if err != nil {
			return err
		}
	}
	out.Version = adminInventoryVersion(row, pets)
	return nil
}

func (s *GameService) adminInventoryApplyAttributes(tx *gorm.DB, p *pb.Player, current proto.Message, attrs map[string]int64) error {
	// Apply to the transaction-local instance, then resolve dependent ranges from the resulting level.
	allowed := map[string]bool{}
	for _, a := range s.adminInventoryAttributes(p, current) {
		allowed[a.Key] = true
	}
	for key, value := range attrs {
		if !allowed[key] || value < 0 || value > math.MaxInt32 {
			return adminInventoryError("未知属性或属性数值超出范围：" + key)
		}
		if _, ok := adminInventoryNumeric(current, key, true, value); !ok {
			return adminInventoryError("无法修改该属性：" + key)
		}
	}
	if pet, ok := current.(*pb.Pet); ok {
		row := s.table("Pet", "id", pet.GetId())
		intimacy := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(pet.GetEat().GetLevel())))
		unlocked := int(tableNumber(intimacy, "additionalTalent"))
		if unlocked > len(pet.GetRandPropList()) {
			if !s.P9IntimacyReady {
				return adminInventoryError("亲密成长规则尚未就绪")
			}
			if err := s.p9AllocateIntimacy(tx, p.GetPlayerId(), pet, unlocked); err != nil {
				return err
			}
		}
	}
	for _, a := range s.adminInventoryAttributes(p, current) {
		if a.Value < a.Min || a.Value > a.Max {
			return adminInventoryError(a.Label + "超出原表或当前等级允许的范围")
		}
	}
	switch v := current.(type) {
	case *pb.Equipment:
		if err := equipmentBasicState(v); err != nil {
			return adminInventoryError(err.Error())
		}
		if err := s.equipmentAdvancedProperties(v, map[string]float64{}); err != nil {
			return adminInventoryError(err.Error())
		}
	case *pb.Warrior:
		if v.GetBreached().GetLevel() > 1 {
			if err := s.growthLevel(p, v, "HeroBreachedLevel"); err != nil {
				return adminInventoryError(err.Error())
			}
		}
		if growthValues(v.GetTraining()) != [4]int32{} {
			if err := s.growthLevel(p, v, "HeroTrainLevel"); err != nil {
				return adminInventoryError(err.Error())
			}
		}
		if trainingPending(v.GetTempTraining()) {
			return adminInventoryError("请先确认或放弃待保存的训练结果")
		}
		effectiveLevel := v.GetLevel()
		if v.GetGuid() == p.GetHeroWarId() {
			effectiveLevel = p.GetLevel()
		}
		nodes := map[string]map[string]string{}
		active := map[string]bool{}
		for _, node := range s.Tables["TalentAdvenced"] {
			if node["warId"] == v.GetId() {
				nodes[node["talentLevel"]] = node
				if node["trunck"] == "1" && int64(tableNumber(node, "talentLevel")) <= int64(v.GetTalent()) {
					active[node["talentLevel"]] = true
				}
			}
		}
		for _, branch := range v.GetTalentBranch() {
			active[branch] = true
		}
		for key := range active {
			node := nodes[key]
			if node == nil || tableNumber(node, "levelLimit") > float64(effectiveLevel) {
				return adminInventoryError("天赋与人物等级不匹配")
			}
			for _, parent := range []string{node["parent"], node["parent2"]} {
				if parent != "" && !active[parent] {
					return adminInventoryError("修改将破坏已持有天赋的前置关系")
				}
			}
		}
		if v.GetAwake().GetStage() > 0 {
			if err := s.awakeEligible(p, v); err != nil {
				return adminInventoryError(err.Error())
			}
		}
		if v.GetAwake().GetStage() == 2 {
			skill := s.table("Hero", "id", v.GetId())["AwakeSkillId"]
			if skill == "" || s.table("Skill", "SkillId", skill) == nil {
				return adminInventoryError("原觉醒技能配置缺失")
			}
			v.AwakenSkill = proto.String(skill + ",2")
		} else {
			v.AwakenSkill = nil
		}
		talents := []string{}
		if err := s.AddWarriorAwakeProperties(v, map[string]float64{}, map[string]float64{}, &talents); err != nil {
			return adminInventoryError(err.Error())
		}
		limits, e := s.trainingLimits(p, v)
		if e != nil {
			return adminInventoryError(e.Error())
		}
		for i, value := range growthValues(v.GetTraining()) {
			if value < 0 || value > limits[i] {
				return adminInventoryError("训练属性超出当前等级上限")
			}
		}
	case *pb.Pet:
		material := s.p9Material(v)
		if v.GetBreached().GetLevel() > 0 {
			prior := proto.Clone(v).(*pb.Pet)
			prior.Breached.Level = proto.Int32(v.GetBreached().GetLevel() - 1)
			step := s.p9Material(prior)
			if step == nil || float64(v.GetLevel()) < tableNumber(step, "petLevel") {
				return adminInventoryError("精灵突破与当前等级不匹配")
			}
		}
		if material == nil || v.GetLevel() > p.GetLevel() || float64(v.GetLevel()) > tableNumber(material, "maxPetLevel") {
			return adminInventoryError("精灵等级与突破阶段不匹配")
		}
		for _, equip := range v.GetPetEquipList() {
			cap, err := s.p9EquipStrengthCap(v, equip)
			if err != nil || equip.GetStrengthLevel() > cap {
				return adminInventoryError("精灵装备强化超过等级或进阶上限")
			}
			if equip.GetAdvanceLevel() > 0 {
				step := s.p9TablePair("PetEquipAdvance", "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(equip.GetAdvanceLevel())))
				if step == nil || float64(equip.GetStrengthLevel()) < tableNumber(step, "needStrengthLevel") {
					return adminInventoryError("精灵装备进阶与强化等级不匹配")
				}
			}
		}
		if _, _, err := s.p9PetProperties(v); err != nil {
			return adminInventoryError(err.Error())
		}
	case *pb.Treasure:
		if err := s.treasureProperties(v, map[string]float64{}, map[string]float64{}); err != nil {
			return adminInventoryError(err.Error())
		}
	}
	return nil
}

// adminInventoryActivityGuard keeps already frozen combat snapshots immutable.
func (s *GameService) adminInventoryActivityGuard(tx *gorm.DB, id int64, g *model.GameState) error {
	if err := adminInventoryBattleGuard(g); err != nil {
		return err
	}
	if err := p7AwakeRequireIdle(tx, id); err != nil {
		return adminInventoryError(err.Error())
	}
	if s.P12WantedReady {
		if err := P12WantedRequireIdle(tx, id, time.Now()); err != nil {
			return adminInventoryError(err.Error())
		}
	}
	if s.P12FieldPVPReady {
		if err := P12FieldPVPRequireIdle(tx, id, time.Now()); err != nil {
			return adminInventoryError(err.Error())
		}
	}
	for _, check := range []struct {
		table any
		where string
	}{{&db.DailyCopyFight{}, "player_id=? AND settled=false"}, {&db.P7RushFight{}, "player_id=? AND status IN ('captured','computed') AND diagnostic=false"}, {&db.P12WorldBossBattleReceipt{}, "player_id=? AND committed=false AND abandoned_at=0"}} {
		if gameSchemaHasTable(tx, check.table) {
			var count int64
			if err := tx.Model(check.table).Where(check.where, id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return adminInventoryError("存在尚未结束的活动或首领战斗，请先完成或退出")
			}
		}
	}
	return nil
}
