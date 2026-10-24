package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
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

type p13GuildTechDirectory struct {
	InitialLevel *int64   `json:"initialLevel"`
	HeroStatue   []string `json:"heroStatue"`
	GuardTree    []string `json:"guardTree"`
}
type p13GuildTechOriginalRow struct {
	Level int64  `json:"level"`
	Type  string `json:"type"`
	Prop  string `json:"prop"`
	Coin  int64  `json:"coin"`
	Badge int32  `json:"badge"`
}

func p13GuildTechType(kind string) bool {
	switch kind {
	case "peratt", "perhp", "perpdef", "permdef", "cri", "hit", "anticri", "dodge":
		return true
	}
	return false
}
func p13GuildTechDirectoryRead(raw, hash string) (*p13GuildTechDirectory, error) {
	var directory p13GuildTechDirectory
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if p6Digest([]byte(raw)) != hash || decoder.Decode(&directory) != nil || directory.InitialLevel == nil || *directory.InitialLevel != 0 || len(directory.HeroStatue) == 0 || len(directory.GuardTree) == 0 {
		return nil, errors.New("公会科技目录配置无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("公会科技目录配置无效")
	}
	seen := map[string]bool{}
	for _, group := range [][]string{directory.HeroStatue, directory.GuardTree} {
		for _, kind := range group {
			if !p13GuildTechType(kind) || seen[kind] {
				return nil, errors.New("公会科技目录必须明确分配原八种属性")
			}
			seen[kind] = true
		}
	}
	if len(seen) != constUtil.P13GuildTechTypeCount {
		return nil, errors.New("公会科技目录必须明确分配原八种属性")
	}
	return &directory, nil
}
func p13GuildTechBuilding(directory *p13GuildTechDirectory, kind string) string {
	for _, item := range directory.HeroStatue {
		if item == kind {
			return constUtil.P13GuildTechHeroStatue
		}
	}
	for _, item := range directory.GuardTree {
		if item == kind {
			return constUtil.P13GuildTechGuardTree
		}
	}
	return ""
}
func (s *GameService) p13GuildTechOriginalRows() ([]p13GuildTechOriginalRow, string, error) {
	rows := make([]p13GuildTechOriginalRow, 0, constUtil.P13GuildTechOriginalRowCount)
	for _, item := range s.Tables["UnionTech"] {
		level, err := strconv.ParseInt(item["techLevel"], 10, 64)
		if err != nil {
			return nil, "", err
		}
		coin, badge := int64(0), int64(0)
		if item["UnionCoin"] != "" {
			coin, err = strconv.ParseInt(item["UnionCoin"], 10, 64)
			if err != nil {
				return nil, "", err
			}
		}
		if item["UnionBadge"] != "" {
			badge, err = strconv.ParseInt(item["UnionBadge"], 10, 32)
			if err != nil {
				return nil, "", err
			}
		}
		rows = append(rows, p13GuildTechOriginalRow{Level: level, Type: item["type"], Prop: item["prop"], Coin: coin, Badge: int32(badge)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Type != rows[j].Type {
			return rows[i].Type < rows[j].Type
		}
		return rows[i].Level < rows[j].Level
	})
	raw, err := json.Marshal(rows)
	if err != nil {
		return nil, "", err
	}
	if len(rows) != constUtil.P13GuildTechOriginalRowCount || (p6Digest(raw) != constUtil.P13GuildTechOriginalRowsHash && len(s.Tables["AdminGuildSettings"]) == 0) {
		return nil, "", errors.New("公会科技原始属性和消耗配置不兼容")
	}
	return rows, string(raw), nil
}
func p13GuildTechOfferHash(row *db.P13GuildTechOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13GuildTechUpgradeHash(row *db.P13GuildTechUpgrade) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13GuildTechOfferRead(offer *db.P13GuildTechOffer) (*p13GuildTechDirectory, []p13GuildTechOriginalRow, error) {
	if offer.GuildID == "" || offer.Version == "" || offer.PublishedAt <= 0 || offer.CapturedAt < offer.PublishedAt || (offer.Source != constUtil.P13GuildTechSource && offer.Source != constUtil.AdminGuildTechSource) || offer.SourceHash != p13GuildTechOfferHash(offer) || offer.OriginalClient != constUtil.P6OriginalClientHash || (offer.Source == constUtil.P13GuildTechSource && offer.OriginalRowsHash != constUtil.P13GuildTechOriginalRowsHash) || p6Digest([]byte(offer.OriginalRowsJSON)) != offer.OriginalRowsHash {
		return nil, nil, errors.New("公会科技实际来源无效")
	}
	directory, err := p13GuildTechDirectoryRead(offer.DirectoryJSON, offer.DirectoryHash)
	if err != nil {
		return nil, nil, err
	}
	var rows []p13GuildTechOriginalRow
	if json.Unmarshal([]byte(offer.OriginalRowsJSON), &rows) != nil || len(rows) != constUtil.P13GuildTechOriginalRowCount {
		return nil, nil, errors.New("公会科技原始记录不完整")
	}
	return directory, rows, nil
}
func p13GuildTechRowAt(rows []p13GuildTechOriginalRow, kind string, level int64) (p13GuildTechOriginalRow, error) {
	for _, row := range rows {
		if row.Type == kind && row.Level == level {
			return row, nil
		}
	}
	return p13GuildTechOriginalRow{}, errors.New("公会科技原等级配置不存在")
}
func p13GuildTechRowHash(row p13GuildTechOriginalRow) string {
	raw, _ := json.Marshal(row)
	return p6Digest(raw)
}

// p13GuildTechLevels validates contiguous actual upgrades. A current mutable
// operator policy never changes the original costs/props already frozen here.
func p13GuildTechLevels(tx *gorm.DB, offer *db.P13GuildTechOffer) (map[string]int64, []db.P13GuildTechUpgrade, error) {
	directory, original, err := p13GuildTechOfferRead(offer)
	if err != nil {
		return nil, nil, err
	}
	var rows []db.P13GuildTechUpgrade
	if err = tx.Where("guild_id=?", offer.GuildID).Order("tech_type,level").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	levels := map[string]int64{}
	previousTimes := map[string]int64{}
	for _, row := range rows {
		expected, err := p13GuildTechRowAt(original, row.Type, row.Level)
		if err != nil {
			return nil, nil, err
		}
		if row.Level < 1 || row.Level > constUtil.P13GuildTechMaxLevel || row.Level != levels[row.Type]+1 || row.GuildID != offer.GuildID || row.Building != p13GuildTechBuilding(directory, row.Type) || row.PlayerID <= 0 || row.Stamp <= 0 || row.AppliedAt < offer.CapturedAt || row.AppliedAt < previousTimes[row.Type] || row.OfferHash != offer.SourceHash || row.OriginalRowHash != p13GuildTechRowHash(expected) || row.Prop != expected.Prop || row.Coin != expected.Coin || row.Badge != expected.Badge || row.SourceHash != p13GuildTechUpgradeHash(&row) {
			return nil, nil, errors.New("公会科技实际升级与原消耗不符")
		}
		levels[row.Type] = row.Level
		previousTimes[row.Type] = row.AppliedAt
	}
	return levels, rows, nil
}
func p13GuildTechAddProp(prop *pb.BaseProp, kind string, value float64) {
	switch kind {
	case "peratt":
		prop.Peratt = proto.Float64(prop.GetPeratt() + value)
	case "perhp":
		prop.Perhp = proto.Float64(prop.GetPerhp() + value)
	case "perpdef":
		prop.Perpdef = proto.Float64(prop.GetPerpdef() + value)
	case "permdef":
		prop.Permdef = proto.Float64(prop.GetPermdef() + value)
	case "cri":
		prop.Cri = proto.Float64(prop.GetCri() + value)
	case "hit":
		prop.Hit = proto.Float64(prop.GetHit() + value)
	case "anticri":
		prop.AntiCri = proto.Float64(prop.GetAntiCri() + value)
	case "dodge":
		prop.Dodge = proto.Float64(prop.GetDodge() + value)
	}
}

// InitializeP13GuildTech creates only a disabled local directory. All original
// eighty level rows are verified before any request can spend guild funds.
func (s *GameService) InitializeP13GuildTech(ctx context.Context) error {
	if _, _, err := s.p13GuildTechOriginalRows(); err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13GuildTechPolicy{}, &db.P13GuildTechOffer{}, &db.P13GuildTechUpgrade{}); err != nil {
		return err
	}
	// Fixed source schema is ready only after the complete migration succeeds.
	s.p10ClientOwner().guildTechSchemaReady.Store(true)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13GuildTechPolicy{ID: 1, DirectoryJSON: "{}", DirectoryHash: p6Digest([]byte("{}")), Source: constUtil.P13GuildTechSource}).Error
}

// P13GuildTechProperties reads permanent guild assets for the current actual
// member. It never writes, migrates, locks guild/profile rows, or grants a level.
func (s *GameService) P13GuildTechProperties(ctx context.Context, tx *gorm.DB, id int64, now time.Time) (*pb.BaseProp, string, error) {
	tx = tx.WithContext(ctx)
	owner := s.p10ClientOwner()
	if !owner.guildCoreSchemaReady.Load() || !owner.guildTechSchemaReady.Load() {
		for _, table := range []any{&db.P13GuildTechOffer{}, &db.P13GuildTechUpgrade{}, &db.P13GuildMember{}, &db.P13Guild{}} {
			if !gameSchemaHasTable(tx, table) {
				return &pb.BaseProp{}, "", nil
			}
		}
	}
	member, err := p13GuildMember(tx, id)
	if err != nil {
		return nil, "", err
	}
	if member.GuildID == "" {
		return &pb.BaseProp{}, "", nil
	}
	var guild db.P13Guild
	err = tx.First(&guild, "id=?", member.GuildID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.BaseProp{}, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var offer db.P13GuildTechOffer
	err = tx.First(&offer, "guild_id=?", member.GuildID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.BaseProp{}, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	levels, steps, err := p13GuildTechLevels(tx, &offer)
	if err != nil {
		return nil, "", err
	}
	if len(steps) == 0 {
		return nil, "", errors.New("公会科技缺少实际升级记录")
	}
	if offer.CapturedAt > now.UnixMilli() {
		return nil, "", errors.New("公会科技升级时刻晚于当前快照")
	}
	directory, original, err := p13GuildTechOfferRead(&offer)
	if err != nil {
		return nil, "", err
	}
	prop := &pb.BaseProp{}
	digests := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.AppliedAt > now.UnixMilli() {
			return nil, "", errors.New("公会科技升级时刻晚于当前快照")
		}
		digests = append(digests, step.SourceHash)
	}
	for kind, level := range levels {
		building := p13GuildTechBuilding(directory, kind)
		if building == constUtil.P13GuildTechHeroStatue && guild.HeroStatueOpen != 1 || building == constUtil.P13GuildTechGuardTree && guild.GuardTreeOpen != 1 {
			return nil, "", errors.New("公会科技所属建筑状态无效")
		}
		row, err := p13GuildTechRowAt(original, kind, level)
		if err != nil {
			return nil, "", err
		}
		parts := strings.Split(row.Prop, ",")
		if len(parts) != 2 || parts[0] != kind {
			return nil, "", errors.New("公会科技原属性无效")
		}
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, "", err
		}
		p13GuildTechAddProp(prop, kind, value)
	}
	raw, _ := json.Marshal(struct {
		GuildID string   `json:"guildId"`
		Offer   string   `json:"offer"`
		Steps   []string `json:"steps"`
	}{member.GuildID, offer.SourceHash, digests})
	return prop, p6Digest(raw), nil
}

func (s *GameService) p13GuildTechQuery(ctx context.Context, id int64, building string, now time.Time) ([]proto.Message, error) {
	var items []*pb.UnionPropAddItem
	var coin int64
	var badge int32
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.ManagementEnabled {
			return errors.New(constUtil.P13GuildClosed)
		}
		_, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		coin, badge = guild.Coin, guild.Badge
		if building == constUtil.P13GuildTechHeroStatue && guild.HeroStatueOpen != 1 || building == constUtil.P13GuildTechGuardTree && guild.GuardTreeOpen != 1 {
			return nil
		}
		var offer db.P13GuildTechOffer
		var directory *p13GuildTechDirectory
		levels := map[string]int64{}
		var original []p13GuildTechOriginalRow
		err = tx.First(&offer, "guild_id=?", guild.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var source db.P13GuildTechPolicy
			if err = tx.First(&source, 1).Error; err != nil {
				return err
			}
			if !source.Enabled {
				return nil
			}
			if source.Source != constUtil.P13GuildTechSource || source.Version == "" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() {
				return errors.New("公会科技目录尚未发布")
			}
			directory, err = p13GuildTechDirectoryRead(source.DirectoryJSON, source.DirectoryHash)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			directory, original, err = p13GuildTechOfferRead(&offer)
			if err != nil {
				return err
			}
			var steps []db.P13GuildTechUpgrade
			levels, steps, err = p13GuildTechLevels(tx, &offer)
			if err != nil {
				return err
			}
			for _, step := range steps {
				if step.AppliedAt > now.UnixMilli() {
					return errors.New("公会科技升级时刻晚于当前查询")
				}
			}
		}
		kinds := directory.HeroStatue
		if building == constUtil.P13GuildTechGuardTree {
			kinds = directory.GuardTree
		}
		for _, kind := range kinds {
			level := levels[kind]
			prop := kind + ",0"
			if level > 0 {
				row, err := p13GuildTechRowAt(original, kind, level)
				if err != nil {
					return err
				}
				prop = row.Prop
			}
			items = append(items, &pb.UnionPropAddItem{Type: proto.String(kind), Level: proto.Int32(int32(level)), Prop: proto.String(prop)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if building == constUtil.P13GuildTechHeroStatue {
		return []proto.Message{&pb.SCUnionHeroStatueInfo{UnionPropAddAtt: items, CoinNum: proto.Int64(coin), BadgeNum: proto.Int32(badge)}}, nil
	}
	return []proto.Message{&pb.SCUnionGuardTreeInfo{UnionPropAddDef: items, CoinNum: proto.Int64(coin), BadgeNum: proto.Int32(badge)}}, nil
}

func (s *GameService) p13GuildTechUpgrade(ctx context.Context, id int64, request proto.Message, kind, building string, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply proto.Message = &pb.SCUnionHeroStatueUpgrade{}
	if building == constUtil.P13GuildTechGuardTree {
		reply = &pb.SCUnionGuardTreeUpgrade{}
	}
	fresh := false
	guildID := ""
	messages, err := s.p13GuildMutate(ctx, id, request, reply, stamp, now, true, func(tx *gorm.DB, profile *db.Profile, player *pb.Player, game *model.GameState) error {
		member, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		if member.Role != constUtil.P13GuildChairman && member.Role != constUtil.P13GuildVice {
			return errors.New("公会职务不允许升级科技")
		}
		if building == constUtil.P13GuildTechHeroStatue && guild.HeroStatueOpen != 1 || building == constUtil.P13GuildTechGuardTree && guild.GuardTreeOpen != 1 {
			return errors.New("请先建设公会建筑")
		}
		var policy db.P13GuildTechPolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled {
			return errors.New(constUtil.P13GuildTechClosed)
		}
		var offer db.P13GuildTechOffer
		err = tx.First(&offer, "guild_id=?", guild.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if policy.Source != constUtil.P13GuildTechSource || policy.PublishedAt <= 0 || policy.PublishedAt > now.UnixMilli() || strings.TrimSpace(policy.Version) == "" || len(policy.Version) > 128 {
				return errors.New("公会科技目录尚未发布")
			}
			if _, err = p13GuildTechDirectoryRead(policy.DirectoryJSON, policy.DirectoryHash); err != nil {
				return err
			}
			_, originalJSON, err := s.p13GuildTechOriginalRows()
			if err != nil {
				return err
			}
			offer = db.P13GuildTechOffer{GuildID: guild.ID, Version: policy.Version, DirectoryJSON: policy.DirectoryJSON, DirectoryHash: policy.DirectoryHash, OriginalRowsJSON: originalJSON, OriginalRowsHash: p6Digest([]byte(originalJSON)), OriginalClient: constUtil.P6OriginalClientHash, PublishedAt: policy.PublishedAt, CapturedAt: now.UnixMilli(), Source: constUtil.P13GuildTechSource}
			if offer.OriginalRowsHash != constUtil.P13GuildTechOriginalRowsHash {
				offer.Source = constUtil.AdminGuildTechSource
			}
			offer.SourceHash = p13GuildTechOfferHash(&offer)
			if err = tx.Create(&offer).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		directory, original, err := p13GuildTechOfferRead(&offer)
		if err != nil {
			return err
		}
		if p13GuildTechBuilding(directory, kind) != building {
			return errors.New("科技不属于当前建筑目录")
		}
		levels, steps, err := p13GuildTechLevels(tx, &offer)
		if err != nil {
			return err
		}
		for _, step := range steps {
			if step.AppliedAt > now.UnixMilli() {
				return errors.New("科技升级早于既有实际记录")
			}
		}
		level := levels[kind]
		if level >= constUtil.P13GuildTechMaxLevel {
			return errors.New("公会科技已达到原最高等级")
		}
		row, err := p13GuildTechRowAt(original, kind, level+1)
		if err != nil {
			return err
		}
		if guild.Coin < row.Coin || guild.Badge < row.Badge {
			return errors.New("公会金币或徽章不足")
		}
		guild.Coin -= row.Coin
		guild.Badge -= row.Badge
		if err = tx.Save(guild).Error; err != nil {
			return err
		}
		step := db.P13GuildTechUpgrade{GuildID: guild.ID, Type: kind, Level: level + 1, Building: building, PlayerID: id, Stamp: stamp, AppliedAt: now.UnixMilli(), Coin: row.Coin, Badge: row.Badge, Prop: row.Prop, OfferHash: offer.SourceHash, OriginalRowHash: p13GuildTechRowHash(row)}
		step.SourceHash = p13GuildTechUpgradeHash(&step)
		if err = tx.Create(&step).Error; err != nil {
			return err
		}
		player.NeedRefreshUnionProp = proto.Int32(1)
		fresh = true
		guildID = guild.ID
		return nil
	})
	if err == nil && fresh {
		s.afterP13GuildTechUpgrade(ctx, guildID)
	}
	return messages, err
}

// Current online members receive the original refresh flag after commit. Their
// subsequent original57133 query rechecks current membership and current assets.
func (s *GameService) afterP13GuildTechUpgrade(ctx context.Context, guild string) {
	tx := db.GetIns().WithContext(ctx)
	var members []db.P13GuildMember
	if tx.Where("guild_id=?", guild).Find(&members).Error != nil {
		return
	}
	for _, member := range members {
		var profile db.Profile
		if tx.First(&profile, member.PlayerID).Error != nil {
			continue
		}
		player, _, err := decodeProfile(&profile)
		if err != nil {
			continue
		}
		player.NeedRefreshUnionProp = proto.Int32(1)
		s.publishP10Private(ctx, member.PlayerID, &pb.SCPlayer{Player: player})
	}
}

// DispatchP13GuildTech preserves original HTTP query and empty-upgrade responses.
func (s *GameService) DispatchP13GuildTech(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	now := p13UFNow(ctx)
	switch body := request.(type) {
	case *pb.CSUnionHeroStatueInfo:
		messages, err := s.p13GuildTechQuery(ctx, id, constUtil.P13GuildTechHeroStatue, now)
		return messages, true, err
	case *pb.CSUnionGuardTreeInfo:
		messages, err := s.p13GuildTechQuery(ctx, id, constUtil.P13GuildTechGuardTree, now)
		return messages, true, err
	case *pb.CSUnionHeroStatueUpgrade:
		messages, err := s.p13GuildTechUpgrade(ctx, id, body, body.GetType(), constUtil.P13GuildTechHeroStatue, stamp, now)
		return messages, true, err
	case *pb.CSUnionGuardTreeUpgrade:
		messages, err := s.p13GuildTechUpgrade(ctx, id, body, body.GetType(), constUtil.P13GuildTechGuardTree, stamp, now)
		return messages, true, err
	default:
		return nil, false, nil
	}
}
