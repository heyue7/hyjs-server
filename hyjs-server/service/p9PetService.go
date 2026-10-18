package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) LoadP9Tables(root string) error {
	for _, name := range []string{"Pet", "PetTalent", "PetTalentSkill", "PetExp", "PetIntimacy", "PetGet", "PetEquip", "PetEquipStrength", "PetEquipAdvance", "PetEquipAwake", "PetTalentMaterial"} {
		file, err := os.Open(filepath.Join(root, "Table"+name+".tsv"))
		if err != nil {
			return err
		}
		reader := csv.NewReader(file)
		reader.Comma, reader.FieldsPerRecord = '\t', -1
		rows, err := reader.ReadAll()
		file.Close()
		if err != nil {
			return err
		}
		if len(rows) < 5 {
			return p5Error()
		}
		entries := []map[string]string{}
		for _, values := range rows[5:] {
			if len(values) == 0 || values[0] == "" {
				continue
			}
			entry := map[string]string{}
			for i, value := range values {
				if i < len(rows[1]) {
					entry[rows[1][i]] = value
				}
			}
			entries = append(entries, entry)
		}
		s.Tables[name] = entries
	}
	return nil
}

func (s *GameService) InitializeP9Pets(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P9PetInstance{}, &db.P9PetState{}, &db.P9PetReceipt{}); err != nil {
		return err
	}
	return s.initializeP9Recruit(tx)
}

func (s *GameService) p9Open(p *pb.Player) error {
	gate := tableNumber(s.table("GameConfigure", "StaticName", "PetUnlockLevel"), "NameValue")
	if gate < 1 || float64(p.GetLevel()) < gate {
		return errors.New("守护精灵尚未达到原开放等级")
	}
	return nil
}

func (s *GameService) p9Mutation(ctx context.Context, id int64, operation string, stamp int64, request, response proto.Message, apply func(*gorm.DB, *pb.Player, *model.GameState) error) ([]proto.Message, error) {
	if stamp <= 0 {
		return nil, p5Error()
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		raw = []byte{}
	}
	err = s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		var receipt db.P9PetReceipt
		err := tx.First(&receipt, "player_id=? AND operation=? AND timestamp=?", id, operation, stamp).Error
		if err == nil {
			if !bytes.Equal(raw, receipt.Request) {
				return p5Error()
			}
			if err := proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			return s.p9ReceiptCurrent(tx, id, p, g, request, response)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err = s.p9Open(p); err != nil {
			return err
		}
		if err = apply(tx, p, g); err != nil {
			return err
		}
		if err = saveProfile(tx, row, p, g); err != nil {
			return err
		}
		data, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		if data == nil {
			data = []byte{}
		}
		return tx.Create(&db.P9PetReceipt{PlayerID: id, Operation: operation, Timestamp: stamp, Request: raw, Response: data}).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// p9ReceiptCurrent 只重放仍与当前实例相符的投影，旧培养/重置回包不能覆盖新状态。
// 原回执和投入事实始终保留；失效请求只要求客户端刷新，不重复执行成长或退款。
func (s *GameService) p9ReceiptCurrent(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, request, response proto.Message) error {
	checkPet := func(pet *pb.Pet) error {
		if pet == nil {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
		current, err := p9OwnedPet(tx, id, pet.GetGuid())
		if err != nil || !proto.Equal(current, pet) {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
		return nil
	}
	if result, ok := response.(interface{ GetPet() *pb.Pet }); ok {
		if err := checkPet(result.GetPet()); err != nil {
			return err
		}
	}
	if result, ok := response.(interface{ GetPetEquip() *pb.PetEquip }); ok {
		equip := result.GetPetEquip()
		owner, err := p9OwnedPet(tx, id, equip.GetPetGuid())
		if err != nil {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
		current, err := s.p9OwnedEquip(owner, equip.GetEquipId())
		if err != nil || !proto.Equal(current, equip) {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
	}
	if result, ok := response.(interface{ GetPetEquip() []*pb.PetEquip }); ok {
		for _, equip := range result.GetPetEquip() {
			owner, err := p9OwnedPet(tx, id, equip.GetPetGuid())
			if err != nil {
				return errors.New(constUtil.P9PetReceiptExpired)
			}
			current, err := s.p9OwnedEquip(owner, equip.GetEquipId())
			if err != nil || !proto.Equal(current, equip) {
				return errors.New(constUtil.P9PetReceiptExpired)
			}
		}
	}
	if result, ok := response.(interface{ GetGet() *pb.ResultObject }); ok {
		for _, pet := range result.GetGet().GetPets() {
			if err := checkPet(pet); err != nil {
				return err
			}
		}
	}
	if result, ok := response.(*pb.SCFormationOn); ok {
		var current *pb.FormationItem
		for _, slot := range g.Formation {
			if slot.GetFormatPos() == result.GetFormationItem().GetFormatPos() {
				current = slot
			}
		}
		if current == nil || !proto.Equal(current, result.GetFormationItem()) || result.GetFightpower() != p.GetFightPower() {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
	}
	if result, ok := response.(*pb.SCFormationOff); ok {
		original, ok := request.(*pb.CSFormationOff)
		if !ok {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
		// PetPage's original callback clears position 7 at the captured camp,
		// regardless of GUID. Equal total power cannot prove that camp is empty.
		var current *pb.FormationItem
		for _, slot := range g.Formation {
			if slot.GetFormatPos() != original.GetFormatPos() {
				continue
			}
			if current != nil {
				return errors.New(constUtil.P9PetReceiptExpired)
			}
			current = slot
		}
		if current == nil {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
		for _, thing := range current.GetThings() {
			if thing.GetPos() == 7 {
				return errors.New(constUtil.P9PetReceiptExpired)
			}
		}
		if _, err := p9OwnedPet(tx, id, original.GetGuid()); err != nil || p9Guarded(g, original.GetGuid()) || result.GetFightpower() != p.GetFightPower() {
			return errors.New(constUtil.P9PetReceiptExpired)
		}
	}
	return nil
}

func p9State(tx *gorm.DB, id int64) (*db.P9PetState, error) {
	var state db.P9PetState
	err := tx.First(&state, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &db.P9PetState{PlayerID: id, Source: constUtil.P9PetSource}, nil
	}
	if err != nil {
		return nil, err
	}
	if state.Source == "" || state.NextFree < 0 || state.FreeRemaining < 0 || state.MustTimes < 0 {
		return nil, p5Error()
	}
	return &state, nil
}

// P9GrantPet creates complete native instances under the caller's player row lock.
// Initial random intimacy attributes are explicitly absent until their original
// allocation rule is recovered; the original defaultTalent remains effective.
func (s *GameService) P9GrantPet(tx *gorm.DB, p *pb.Player, key string, count int32) ([]*pb.Pet, error) {
	row := s.table("Pet", "id", key)
	if row == nil || row["defaultTalent"] == "" || count < 1 || count > 100 {
		return nil, p5Error()
	}
	baseFound := false
	for _, talent := range s.Tables["PetTalent"] {
		baseFound = baseFound || talent["petTalentId"] == row["defaultTalent"] && talent["talentLevel"] == "0"
	}
	if !baseFound {
		return nil, errors.New("原精灵基础天赋缺失")
	}
	var owned int64
	if err := tx.Model(&db.P9PetInstance{}).Where("player_id=?", p.GetPlayerId()).Count(&owned).Error; err != nil {
		return nil, err
	}
	if p.GetPetBagSize() < 1 || owned+int64(count) > int64(p.GetPetBagSize()) {
		return nil, errors.New("精灵背包容量不足")
	}
	pets := []*pb.Pet{}
	for n := int32(0); n < count; n++ {
		pet := &pb.Pet{Id: proto.String(key), Guid: proto.String(uuid.NewString()), Level: proto.Int32(1), Exp: proto.Int64(0), Breached: &pb.PetBreached{Level: proto.Int32(0)}, Eat: &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0), HdTimeList: []int32{0, 0, 0}}, InitProp: proto.String(row["defaultTalent"])}
		for _, talent := range s.Tables["PetTalentSkill"] {
			if talent["petId"] != key {
				continue
			}
			level, err := strconv.ParseInt(talent["talentLevel"], 10, 32)
			if err != nil || level < 1 || talent["talent"] == "" || talent["odd"] != "100" {
				return nil, errors.New("精灵随机技能配置尚未确认")
			}
			pet.TalentList = append(pet.TalentList, &pb.PetTalentSkill{Talent: proto.String(talent["talent"]), TalentLevel: proto.Int32(int32(level))})
		}
		if row["quality"] == "23" {
			seen := map[string]bool{}
			for _, equip := range s.Tables["PetEquip"] {
				if equip["petId"] != key {
					continue
				}
				if seen[equip["position"]] {
					return nil, p5Error()
				}
				seen[equip["position"]] = true
				pet.PetEquipList = append(pet.PetEquipList, &pb.PetEquip{PetGuid: pet.Guid, EquipId: proto.String(equip["id"]), StrengthLevel: proto.Int32(0), AdvanceLevel: proto.Int32(0), AwakeLevel: proto.Int32(0)})
			}
			if len(pet.PetEquipList) != 4 {
				return nil, errors.New("金色精灵原装备四槽缺失")
			}
		}
		data, err := proto.Marshal(pet)
		if err != nil {
			return nil, err
		}
		if err = tx.Create(&db.P9PetInstance{GUID: pet.GetGuid(), PlayerID: p.GetPlayerId(), PetID: key, Data: data, CreatedAt: time.Now().UnixMilli(), Source: constUtil.P9PetSource}).Error; err != nil {
			return nil, err
		}
		pets = append(pets, pet)
	}
	return pets, nil
}

func (s *GameService) p9PetFirst(ctx context.Context, id, stamp int64, b *pb.CSPetFirstRecruit) ([]proto.Message, error) {
	out := &pb.SCPetFirstRecruit{Get: &pb.ResultObject{}}
	return s.p9Mutation(ctx, id, constUtil.P9PetFirstOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		state, err := p9State(tx, id)
		if err != nil {
			return err
		}
		if state.FirstGranted {
			return errors.New("首次精灵招募已经领取")
		}
		parts := strings.Split(s.table("GameConfigure", "StaticName", "PetFirstRecruitGet")["NameValue"], ",")
		if len(parts) != 2 || parts[1] != "1" {
			return p5Error()
		}
		out.Get.Pets, err = s.P9GrantPet(tx, p, parts[0], 1)
		if err != nil {
			return err
		}
		state.FirstGranted = true
		return tx.Save(state).Error
	})
}

func P9PetList(tx *gorm.DB, id int64) ([]*pb.Pet, error) {
	var rows []db.P9PetInstance
	if err := tx.Where("player_id=?", id).Order("created_at,guid").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []*pb.Pet{}
	for _, row := range rows {
		pet := &pb.Pet{}
		if err := proto.Unmarshal(row.Data, pet); err != nil {
			return nil, err
		}
		if pet.GetGuid() != row.GUID || pet.GetId() != row.PetID || row.Source == "" || pet.GetEat() == nil || pet.GetBreached() == nil {
			return nil, p5Error()
		}
		out = append(out, pet)
	}
	return out, nil
}

func (s *GameService) p9PetBox(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, bool, error) {
	options := []map[string]string{}
	cost := b.GetCostItem()
	for _, row := range s.Tables["ItemUse"] {
		if row["itemId"] != cost.GetId() {
			continue
		}
		if !strings.HasPrefix(row["result"], "pet_") {
			return nil, false, nil
		}
		options = append(options, row)
	}
	if len(options) == 0 {
		return nil, false, nil
	}
	out := &pb.SCItemUse{Get: &pb.ResultObject{}}
	frames, err := s.p9Mutation(ctx, id, constUtil.P9PetBoxOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if cost.GetNum() < 1 || cost.GetNum() > 100 || cost.GetGuid() != "" || b.GetGetItemIndex() != 0 || s.table("Item", "itemId", cost.GetId())["category"] != "2" {
			return p5Error()
		}
		weights := []string{}
		for i, row := range options {
			w, err := strconv.ParseInt(row["odds"], 10, 32)
			if err != nil || w < 1 {
				return p5Error()
			}
			weights = append(weights, strconv.Itoa(i)+","+strconv.FormatInt(w, 10))
		}
		if err := deductPoolCost(p, g, cost); err != nil {
			return err
		}
		out.Cost = proto.Clone(cost).(*pb.BaseResource)
		for n := int32(0); n < cost.GetNum(); n++ {
			chosen, err := fieldWeighted(strings.Join(weights, ";"))
			if err != nil {
				return err
			}
			selected, err := strconv.Atoi(chosen)
			if err != nil {
				return err
			}
			parts := strings.Split(options[selected]["result"], ",")
			if len(parts) != 2 || parts[1] != "1" {
				return p5Error()
			}
			pets, err := s.P9GrantPet(tx, p, parts[0], 1)
			if err != nil {
				return err
			}
			out.Get.Pets = append(out.Get.Pets, pets...)
		}
		return nil
	})
	return frames, true, err
}

func (s *GameService) DispatchP9Pets(ctx context.Context, id int64, b proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var out []proto.Message
	var err error
	switch request := b.(type) {
	case *pb.CSPetFirstRecruit:
		out, err = s.p9PetFirst(ctx, id, stamp, request)
	case *pb.CSPetList:
		pets, e := P9PetList(db.GetIns().WithContext(ctx), id)
		err = e
		out = []proto.Message{&pb.SCPetList{PetList: pets}}
	case *pb.CSItemUse:
		return s.p9PetBox(ctx, id, request, stamp)
	case *pb.CSPetRecruitGet:
		out, err = s.P9PetRecruitInfo(ctx, id)
	case *pb.CSPetRecruit:
		out, err = s.p9PetRecruit(ctx, id, request, stamp)
	default:
		return nil, false, nil
	}
	return out, true, err
}
