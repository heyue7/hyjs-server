package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (s *GameService) InitializeP5(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.P5MutationReceipt{}, &db.P5Collection{}, &db.P5Achievement{}, &db.P5AchievementClaim{}, &db.P5GoldEggRule{}, &db.CopyResetRule{}, &db.CopyResetDaily{}, &db.CopyBoxRule{}, &db.CopyBoxClaim{}); err != nil {
		return err
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 原成就奖励未恢复时不再自动补设投放，保留配置表和历史领取。
		if e := s.seedP5GoldEgg(tx); e != nil {
			return e
		}
		if e := s.initializeCopyReset(tx); e != nil {
			return e
		}
		return s.initializeCopyBoxes(tx)
	})
}

func (s *GameService) LoadP5Tables(root string) error {
	for _, name := range []string{"Yuan", "Title", "MainHeroQuality", "CopyBox"} {
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
			return p5Error()
		}
		var entries []map[string]string
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
			entries = append(entries, entry)
		}
		s.Tables[name] = entries
	}
	return nil
}

func p5KV(g *model.GameState, key string) string {
	for _, v := range g.KV {
		if v.GetKey() == key {
			return v.GetValue()
		}
	}
	return ""
}
func p5SetKV(g *model.GameState, key, value string) {
	for _, v := range g.KV {
		if v.GetKey() == key {
			v.Value = proto.String(value)
			return
		}
	}
	g.KV = append(g.KV, &pb.KeyValueEntry{Key: proto.String(key), Value: proto.String(value)})
}
func p5Error() error { return errors.New("养成请求条件不满足或原配置缺失") }
func p5Warrior(g *model.GameState, guid string) (*pb.Warrior, error) {
	var result *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetGuid() == guid {
			if result != nil {
				return nil, p5Error()
			}
			result = w
		}
	}
	if result == nil {
		return nil, p5Error()
	}
	return result, nil
}

// p5Mutation binds the successful response to the exact request inside the player lock.
func (s *GameService) p5Mutation(ctx context.Context, id int64, op string, stamp int64, request, response proto.Message, action func(*gorm.DB, *pb.Player, *model.GameState) error) ([]proto.Message, error) {
	if stamp <= 0 {
		return nil, p5Error()
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	var player *pb.Player
	err = s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		var receipt db.P5MutationReceipt
		err := tx.First(&receipt, "player_id=? AND operation=? AND timestamp=?", id, op, stamp).Error
		if err == nil {
			if !bytes.Equal(raw, receipt.Request) {
				return p5Error()
			}
			// Original LightUp increments the local seed level on every successful
			// callback. Only replay the current node, never an earlier committed step.
			if seed, ok := request.(*pb.CSLightenWorldSeed); ok && p.GetCurrentWorldSeedId() != seed.GetId()+1 {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			if err := proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			if err := s.validateP5GrowthReceipt(g, response); err != nil {
				return err
			}
			if err := validateP5FormationReceipt(p, g, response); err != nil {
				return err
			}
			if err := validateP5HelperReceipt(g, request, response); err != nil {
				return err
			}
			if err := validateP5SkillOrderReceipt(g, request, response); err != nil {
				return err
			}
			if err := s.validateP5EquipmentFormationOffReceipt(g, request, response); err != nil {
				return err
			}
			if err := s.validateP5TreasureFormationOffReceipt(g, request, response); err != nil {
				return err
			}
			if err := s.validateP5TacticalReceipt(g, request, response, stamp); err != nil {
				return err
			}
			player = proto.Clone(p).(*pb.Player)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		switch op {
		case "warrior-reset", "warrior-decompose", "equipment-reset", "equipment-decompose", "treasure-reset":
			gate := int32(tableNumber(s.table("GameConfigure", "StaticName", "RecycleRoomLevel"), "NameValue"))
			if gate <= 0 || p.GetLevel() < gate {
				return p5Error()
			}
		}
		if err = s.ObserveP5Collection(tx, id, g); err != nil {
			return err
		}
		if err = action(tx, p, g); err != nil {
			return err
		}
		if err = s.ObserveP5Collection(tx, id, g); err != nil {
			return err
		}
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return err
		}
		p.FightPower = proto.Int32(power)
		if err = saveProfile(tx, row, p, g); err != nil {
			return err
		}
		player = proto.Clone(p).(*pb.Player)
		data, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Create(&db.P5MutationReceipt{PlayerID: id, Operation: op, Timestamp: stamp, Request: raw, Response: data}).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: player}}, nil
}

// DispatchP5 handles only the new domain; existing P2 boxes/fragments remain their owner's path.
func (s *GameService) DispatchP5(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var messages []proto.Message
	var err error
	switch b := request.(type) {
	case *pb.CSCopyBox:
		messages, err = s.copyBoxOpen(ctx, id, b, stamp)
	case *pb.CSCopyGetResetInfo:
		messages, err = s.copyGetResetInfo(ctx, id, b)
	case *pb.CSCopyResetChallengeCount:
		messages, err = s.copyResetChallenge(ctx, id, b, stamp)
	case *pb.CSHelperOn:
		messages, err = s.p5HelperOn(ctx, id, b, stamp)
	case *pb.CSHelperOff:
		messages, err = s.p5HelperOff(ctx, id, b, stamp)
	case *pb.CSSetSkillOrder:
		messages, err = s.p5SkillOrder(ctx, id, b, stamp)
	case *pb.CSLightenWorldSeed:
		messages, err = s.lightenWorldSeed(ctx, id, b, stamp)
	case *pb.CSItemUse:
		if b.GetCostItem().GetId() == "item_360" {
			messages, err = s.p5GoldEggUse(ctx, id, b, stamp)
			break
		}
		if redChoiceBox(b.GetCostItem().GetId()) {
			return nil, false, nil
		}
		if row := s.table("Item", "itemId", b.GetCostItem().GetId()); row != nil && row["category"] == "1" {
			messages, err = s.p5BasicItemUse(ctx, id, b, stamp)
			break
		}
		switch b.GetCostItem().GetId() {
		case "item_022", "item_024", "item_025", "item_115", "item_116", "item_117":
			return nil, false, nil
		}
		row := s.table("Item", "itemId", b.GetCostItem().GetId())
		if row == nil || (row["category"] != "2" && row["category"] != "5") {
			return nil, false, nil
		}
		messages, err = s.p5BoxUse(ctx, id, b, stamp)
	case *pb.CSWarriorSell:
		messages, err = s.p5WarriorRecycle(ctx, id, b.GetGuid(), false, stamp, b)
	case *pb.CSWarriorDecompose:
		messages, err = s.p5WarriorRecycle(ctx, id, b.GetGuid(), true, stamp, b)
	case *pb.CSWarriorReset:
		messages, err = s.p5WarriorReset(ctx, id, b, stamp)
	case *pb.CSEquipmentSell:
		messages, err = s.p5EquipmentSell(ctx, id, b, stamp)
	case *pb.CSEquipmentFragSell:
		messages, err = s.p5EquipmentFragSell(ctx, id, b, stamp)
	case *pb.CSWarriorFragSell:
		messages, err = s.p5WarriorFragSell(ctx, id, b, stamp)
	case *pb.CSEquipDecompose:
		messages, err = s.p5EquipmentDecompose(ctx, id, b, stamp)
	case *pb.CSTreasureSell:
		messages, err = s.p5TreasureSell(ctx, id, b, stamp)
	case *pb.CSEquipReBuild:
		messages, err = s.p5EquipmentReset(ctx, id, b, stamp)
	case *pb.CStreasureRebuild:
		messages, err = s.p5TreasureReset(ctx, id, b, stamp)
	case *pb.CSEquipStrengthenOneKey:
		messages, err = s.p5StrengthenOne(ctx, id, b, stamp)
	case *pb.CSEquipStrengthenMoreOneKey:
		messages, err = s.p5StrengthenMore(ctx, id, b, stamp)
	case *pb.CSWarriorHandbook:
		messages, err = s.p5Handbook(ctx, id)
	case *pb.CSAchievementList:
		messages, err = s.p5Achievements(ctx, id)
	case *pb.CSGetAchievementReward:
		messages, err = s.p5AchievementReward(ctx, id, b, stamp)
	case *pb.CSTitleList:
		messages, err = s.p5Titles(ctx, id)
	case *pb.CSChangeTitle:
		messages, err = s.p5ChangeTitle(ctx, id, b, stamp)
	default:
		return nil, false, nil
	}
	return messages, true, err
}

// P5GuardInstance protects original formation, assistance and explicit lock/dispatch state.
func P5GuardInstance(p *pb.Player, g *model.GameState, guid string) error {
	if guid == "" || guid == p.GetHeroWarId() {
		return p5Error()
	}
	for _, slot := range g.Formation {
		for _, v := range slot.Things {
			if v.GetGuid() == guid {
				return errors.New("上阵或穿戴实例不可回收")
			}
		}
	}
	for pos := 1; pos <= 6; pos++ {
		if p5KV(g, fmt.Sprintf("p5.helper:%d", pos)) == guid {
			return errors.New("助阵伙伴不可回收")
		}
	}
	if p5KV(g, "p5.lock:"+guid) == "1" || p5KV(g, "p5.dispatch:"+guid) != "" {
		return errors.New("锁定或派遣实例不可回收")
	}
	return nil
}

func p5ListValid(keys []string) error {
	if len(keys) < 1 || len(keys) > 100 {
		return p5Error()
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" || len(key) > 128 || seen[key] {
			return p5Error()
		}
		seen[key] = true
	}
	return nil
}
func p5ResourceSpec(resources []*pb.BaseResource) string {
	parts := []string{}
	for _, r := range resources {
		parts = append(parts, r.GetId()+","+strconv.FormatInt(int64(r.GetNum()), 10))
	}
	return strings.Join(parts, ";")
}
