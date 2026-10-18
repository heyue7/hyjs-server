package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// LoadP13GuildTables 读取原公会与公会战完整静态表，让报名快照固定实际使用的规则版本。
func (s *GameService) LoadP13GuildTables(root string) error {
	if s.Tables == nil {
		return errors.New("先加载基础原表")
	}
	for _, name := range []string{"UnionConfig", "UnionTech", "UnionLogo", "NPC", "UnionFightBaseCamp", "UnionFightBuff", "UnionFightCariot", "UnionFightFactory", "UnionFightGate", "UnionFightGuard", "UnionFightItem", "UnionFightNpc", "UnionFightRankReward", "UnionFightRankSpecialReward", "UnionFightResource", "UnionFightRevivePoint", "UnionFightTalk", "UnionFightTower", "UnionFightTransfer"} {
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
			return errors.New("原公会表结构无效")
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
	// 原指挥页从TableUnionFightTalk的content键读取TableText；将实际18段文案一并冻结到报名快照。
	file, err := os.Open(filepath.Join(root, "TableText.tsv"))
	if err != nil {
		return err
	}
	localized := map[string]string{}
	// 文本表含原文字面引号，按TSV分隔读取并保留正文，不套用CSV的引号语义。
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		row := strings.SplitN(scanner.Text(), "\t", 2)
		if len(row) == 2 {
			localized[row[0]] = strings.TrimSuffix(row[1], "\r")
		}
	}
	err = scanner.Err()
	file.Close()
	if err != nil {
		return err
	}
	for _, row := range s.Tables["UnionFightTalk"] {
		value := localized[row["content"]]
		if value == "" {
			return errors.New("原公会战指挥文案缺失")
		}
		row["localizedContent"] = value
	}
	return nil
}

// InitializeP13Guild 迁移独立表并保留管理/经济两个关闭门禁。
func (s *GameService) InitializeP13Guild(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13GuildPolicy{}, &db.P13Guild{}, &db.P13GuildMember{}, &db.P13GuildApply{}, &db.P13GuildReceipt{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13GuildPolicy{ID: 1, Source: constUtil.P13GuildSource}).Error; err != nil {
		return err
	}
	if err := s.InitializeP13GuildDonateBox(ctx); err != nil {
		return err
	}
	if err := s.InitializeP13GuildDonationBadge(ctx); err != nil {
		return err
	}
	return s.InitializeP13UnionFight(ctx)
}

// P13GuildScope 每次读取当前真实成员关系，离会/解散后立即失去公会频道资格。
func P13GuildScope(tx *gorm.DB, id int64) (string, int32, error) {
	if !tx.Migrator().HasTable(&db.P13GuildMember{}) {
		return "", 0, nil
	}
	var member db.P13GuildMember
	err := tx.First(&member, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	if member.GuildID == "" {
		return "", 0, nil
	}
	var guild db.P13Guild
	if err = tx.First(&guild, "id=?", member.GuildID).Error; err != nil {
		return "", 0, err
	}
	return member.GuildID, member.Role, nil
}

// P13GuildChannelEnabled 公会文字使用单独门禁，关闭不影响实际成员历史。
func P13GuildChannelEnabled(tx *gorm.DB) (bool, error) {
	if !tx.Migrator().HasTable(&db.P13GuildPolicy{}) {
		return false, nil
	}
	var cfg db.P13GuildPolicy
	if err := tx.First(&cfg, 1).Error; err != nil {
		return false, err
	}
	return cfg.ManagementEnabled && cfg.ChatEnabled, nil
}

// RecordP13GuildLogin 接收已认证真实玩家登录；每日活跃和积分只增加一次。
func (s *GameService) RecordP13GuildLogin(ctx context.Context, id int64) error {
	return s.p13GuildLoginAt(ctx, id, LocalPlayNow(ctx))
}
func (s *GameService) p13GuildLoginAt(ctx context.Context, id int64, now time.Time) error {
	// 旧独立域测试没有本域迁移；正式main初始化失败会在监听前返回错误。
	if !db.GetIns().WithContext(ctx).Migrator().HasTable(&db.P13GuildPolicy{}) {
		return nil
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		m, err := p13GuildMember(tx, id)
		if err != nil {
			return err
		}
		if m.GuildID == "" {
			return nil
		}
		m.LastLoginAt = now.UnixMilli()
		if policy.ManagementEnabled && policy.EconomyEnabled && m.LastLoginDay != p13GuildDay(now) {
			var guild db.P13Guild
			if err = tx.First(&guild, "id=?", m.GuildID).Error; err != nil {
				return err
			}
			active, err := s.p10Setting("UnionLoginActiveScore")
			if err != nil {
				return err
			}
			score, err := s.p10Setting("UnionLoginScore")
			if err != nil {
				return err
			}
			if int64(guild.ActiveScore)+int64(active) > math.MaxInt32 || int64(guild.Exp)+int64(score) > math.MaxInt32 {
				return errors.New("公会登录积分溢出")
			}
			guild.ActiveScore += active
			guild.Exp += score
			m.LastLoginDay = p13GuildDay(now)
			if err = tx.Save(&guild).Error; err != nil {
				return err
			}
		}
		return tx.Save(m).Error
	})
}

func p13GuildDay(now time.Time) string {
	return now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}
func p13GuildMember(tx *gorm.DB, id int64) (*db.P13GuildMember, error) {
	var member db.P13GuildMember
	err := tx.First(&member, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &db.P13GuildMember{PlayerID: id}, nil
	}
	return &member, err
}
func (s *GameService) p13GuildCapacity(level int32) (int32, error) {
	cfg := s.table("UnionConfig", "Level", strconv.Itoa(int(level)))
	value, err := strconv.ParseInt(cfg["MaxMemberNum"], 10, 32)
	if err != nil || value < 1 {
		return 0, errors.New("原公会人数表缺失")
	}
	return int32(value), nil
}
func (s *GameService) p13GuildText(text, key string, empty bool) error {
	limit, err := s.p10Setting(key)
	if err != nil {
		return err
	}
	if empty && text == "" {
		return nil
	}
	budget := 0
	// 原UnionCreate/UnionDeclaration按UTF16字符<=256计1，其余计2；emoji计4。
	for _, unit := range utf16.Encode([]rune(text)) {
		if unit <= 256 {
			budget++
		} else {
			budget += 2
		}
	}
	if !utf8.ValidString(text) || budget > int(limit) || strings.TrimSpace(text) == "" {
		return errors.New("公会文本超限或无效")
	}
	for _, c := range text {
		if c < 32 && c != '\n' {
			return errors.New("公会文本包含控制字符")
		}
	}
	return nil
}
func (s *GameService) p13GuildEligibility(p *pb.Player, m *db.P13GuildMember, now time.Time) error {
	level, err := s.p10Setting("UnionOpenLevel")
	if err != nil {
		return err
	}
	if p.GetLevel() < level {
		return errors.New("角色未达到原公会开放等级")
	}
	if m.GuildID != "" {
		return errors.New("角色已有公会")
	}
	if m.QuitUntil > now.UnixMilli() {
		return errors.New("退出后原24小时冷却尚未结束")
	}
	return nil
}
func p13GuildNeedMember(tx *gorm.DB, id int64) (*db.P13GuildMember, *db.P13Guild, error) {
	m, err := p13GuildMember(tx, id)
	if err != nil {
		return nil, nil, err
	}
	if m.GuildID == "" {
		return nil, nil, errors.New("角色没有公会")
	}
	var g db.P13Guild
	if err = tx.First(&g, "id=?", m.GuildID).Error; err != nil {
		return nil, nil, err
	}
	return m, &g, nil
}
func p13GuildRole(m *db.P13GuildMember, need int32) error {
	if m.Role < need || m.Role > constUtil.P13GuildChairman {
		return errors.New("当前公会职位无此权限")
	}
	return nil
}

// p13GuildMutate 先锁公共门禁，再锁角色；批准/转让/资源/原响应原子提交。
func (s *GameService) p13GuildMutate(ctx context.Context, id int64, request, response proto.Message, stamp int64, now time.Time, economy bool, action func(*gorm.DB, *db.Profile, *pb.Player, *model.GameState) error) ([]proto.Message, error) {
	if stamp <= 0 {
		return nil, errors.New("公会变更缺少原请求时间戳")
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	op := string(request.ProtoReflect().Descriptor().Name())
	var resultPlayer *pb.Player
	fresh := false
	var revokedIDs []int64
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		// Freeze due month identities before any post-cutoff membership or role mutation.
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		// 解散涉及全体成员位置，先按ID升序锁全员，避免与P10双角色业务逆序。
		if _, dissolve := request.(*pb.CSDissolveUnion); dissolve {
			member, err := p13GuildMember(tx, id)
			if err != nil {
				return err
			}
			var members []db.P13GuildMember
			if member.GuildID != "" {
				if err = tx.Where("guild_id=?", member.GuildID).Order("player_id").Find(&members).Error; err != nil {
					return err
				}
			}
			for _, m := range members {
				var locked db.Profile
				if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, m.PlayerID).Error; err != nil {
					return err
				}
				revokedIDs = append(revokedIDs, m.PlayerID)
			}
		}
		// 先锁较小目标角色，避免与好友等共享双角色行锁出现逆序。
		target := int64(0)
		switch b := request.(type) {
		case *pb.CSUnionAgreeApply:
			target = b.GetPlayerId()
		case *pb.CSUnionRefuseApply:
			target = b.GetPlayerId()
		case *pb.CSPromotionViceChairman:
			target = b.GetPlayerId()
		case *pb.CSDismissViceChairman:
			target = b.GetPlayerId()
		case *pb.CSRemoveUnionPlayer:
			target = b.GetPlayerId()
		case *pb.CSTransferChairman:
			target = b.GetPlayerId()
		}
		if target > 0 && target < id {
			var first db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&first, target).Error; err != nil {
				return err
			}
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err = s.RefreshP5GuildPrayerPower(ctx, tx, p, g, now); err != nil {
			return err
		}
		var receipt db.P13GuildReceipt
		err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, op, stamp).Error
		if err == nil {
			if !bytes.Equal(raw, receipt.Request) {
				return errors.New("公会请求时间戳冲突")
			}
			resultPlayer = proto.Clone(p).(*pb.Player)
			return proto.Unmarshal(receipt.Response, response)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !policy.ManagementEnabled {
			return errors.New(constUtil.P13GuildClosed)
		}
		if economy && !policy.EconomyEnabled {
			return errors.New(constUtil.P13GuildEconomyClosed)
		}
		if err = action(tx, &row, p, g); err != nil {
			return err
		}
		// Membership changes and the new prayer replace the current projection before saving.
		if err = s.RefreshP5GuildPrayerPower(ctx, tx, p, g, now); err != nil {
			return err
		}
		if _, err = s.P13GuildRoomNormalize(tx, id, p, g); err != nil {
			return err
		}
		// 最终保存当前内存快照前归一化战场位置，避免嵌套会员撤权保存的回城被旧g.Position覆盖。
		if _, err = s.P13UnionFightFieldNormalize(tx, id, p, g, now); err != nil {
			return err
		}
		switch b := request.(type) {
		case *pb.CSQuitUnion:
			revokedIDs = []int64{id}
		case *pb.CSRemoveUnionPlayer:
			revokedIDs = []int64{b.GetPlayerId()}
		}
		if err = saveProfile(tx, &row, p, g); err != nil {
			return err
		}
		resultPlayer = proto.Clone(p).(*pb.Player)
		fresh = true
		encoded, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: op, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error
	})
	if err != nil {
		return nil, err
	}
	if fresh && len(revokedIDs) > 0 {
		s.AfterP13GuildRoomRevoke(ctx, revokedIDs)
		s.AfterP13UnionFightFieldRevoke(ctx, revokedIDs, now)
	}
	if fresh {
		if _, opened := request.(*pb.CSUnionOpenBuilder); opened {
			s.AfterP13GuildBuildingChange(ctx, id)
		}
	}
	return []proto.Message{response, &pb.SCPlayer{Player: resultPlayer}}, nil
}
func (s *GameService) p13GuildPlayer(tx *gorm.DB, viewer, id int64, member *db.P13GuildMember, now time.Time) (*pb.UnionPlayerInfo, error) {
	info, err := s.p10Player(tx, id)
	if err != nil {
		return nil, err
	}
	friend, err := p10Friends(tx, viewer, id)
	if err != nil {
		return nil, err
	}
	role := int32(0)
	var contribution, coin, badge int32
	if member != nil {
		role = member.Role
		contribution = member.Contribution
		if member.DonateDay == p13GuildDay(now) {
			coin = int32(member.TodayCoin)
			badge = member.TodayBadge
		}
	}
	online := int64(0)
	// 原客户端0显示在线；缺离线时间日志时不伪造最后登录时间。
	if !s.p10ChannelOnline(tx, id) {
		if member != nil {
			online = member.LastLoginAt
		}
	}
	isFriend := int32(0)
	if friend {
		isFriend = 1
	}
	return &pb.UnionPlayerInfo{PlayerId: info.PlayerId, PlayerName: info.PlayerName, FightPower: proto.Int64(int64(info.GetFightPower())), Role: proto.Int32(role), HeroWarId: info.HeroWarId, IsFriend: proto.Int32(isFriend), LastLoginTime: proto.Int64(online), Level: info.Level, Contribution: proto.Int32(contribution), TodayCoin: proto.Int32(coin), TodayBadge: proto.Int32(badge)}, nil
}
func (s *GameService) p13GuildSimple(tx *gorm.DB, viewer int64, g db.P13Guild) (*pb.SimpleUnionInfo, error) {
	var members []db.P13GuildMember
	if err := tx.Where("guild_id=?", g.ID).Order("player_id").Find(&members).Error; err != nil {
		return nil, err
	}
	reply := &pb.SimpleUnionInfo{Level: proto.Int32(g.Level), UnionId: proto.String(g.ID), UnionName: proto.String(g.Name), Declaration: proto.String(g.Declaration), CurrentMemberNum: proto.Int32(int32(len(members))), UnionFlag: proto.Int32(g.Flag)}
	var total int64
	for _, m := range members {
		info, err := s.p10Player(tx, m.PlayerID)
		if err != nil {
			return nil, err
		}
		if int64(info.GetFightPower()) > math.MaxInt64-total {
			return nil, errors.New("公会战力溢出")
		}
		total += int64(info.GetFightPower())
		if m.Role == constUtil.P13GuildChairman {
			reply.ChairmanLevel = info.Level
			reply.ChairmanName = info.PlayerName
		}
	}
	reply.TotalFightPower = proto.Int64(total)
	var count int64
	if err := tx.Model(&db.P13GuildApply{}).Where("guild_id=? AND player_id=?", g.ID, viewer).Count(&count).Error; err != nil {
		return nil, err
	}
	reply.IsApplied = proto.Int32(int32(count))
	return reply, nil
}

// p13GuildQuery 返回真实成员与申请数据，空会入口仍可供原登录初始化使用。
func (s *GameService) p13GuildQuery(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		switch b := request.(type) {
		case *pb.CSUnionSearch:
			if b.GetPage() < 1 || b.GetPage() > 100000 {
				return errors.New("公会页码无效")
			}
			if b.GetName() != "" {
				if err := s.p13GuildText(b.GetName(), "UnionNameLengthLimit", false); err != nil {
					return err
				}
			}
			var guilds []db.P13Guild
			query := tx.Order("created_at,id").Limit(20).Offset(int(b.GetPage()-1) * 20)
			if b.GetName() != "" {
				query = query.Where("name=? OR id=?", b.GetName(), b.GetName())
			}
			if err := query.Find(&guilds).Error; err != nil {
				return err
			}
			reply := &pb.SCUnionSearch{}
			for _, g := range guilds {
				info, err := s.p13GuildSimple(tx, id, g)
				if err != nil {
					return err
				}
				reply.Unions = append(reply.Unions, info)
			}
			response = reply
			return nil
		case *pb.CSGetUnionInfo:
			member, err := p13GuildMember(tx, id)
			if err != nil {
				return err
			}
			reply := &pb.SCGetUnionInfo{HasUnion: proto.Int32(0)}
			response = reply
			if member.GuildID == "" {
				return nil
			}
			var guild db.P13Guild
			if err = tx.First(&guild, "id=?", member.GuildID).Error; err != nil {
				return err
			}
			info, err := s.p13GuildSimple(tx, id, guild)
			if err != nil {
				return err
			}
			reply.HasUnion = proto.Int32(1)
			reply.UnionInfo = info
			reply.Notice = proto.String(guild.Notice)
			reply.MyRole = proto.Int32(member.Role)
			reply.CoinNum = proto.Int64(guild.Coin)
			reply.BadgeNum = proto.Int32(guild.Badge)
			reply.CurrentExp = proto.Int32(guild.Exp)
			reply.ActiveScore = proto.Int32(guild.ActiveScore)
			reply.HeroStatueOpen = proto.Int32(guild.HeroStatueOpen)
			reply.GuardTreeOpen = proto.Int32(guild.GuardTreeOpen)
			if member.StaminaDay != p13GuildDay(now) {
				reply.CanGetStamina = proto.Int32(1)
			}
			if member.Role >= constUtil.P13GuildVice {
				var n int64
				if err = tx.Model(&db.P13GuildApply{}).Where("guild_id=?", guild.ID).Count(&n).Error; err != nil {
					return err
				}
				if n > 0 {
					reply.HasAppliers = proto.Int32(1)
				}
			}
			return nil
		case *pb.CSUnionHeroStatueInfo, *pb.CSUnionGuardTreeInfo, *pb.CSUnionLightChurchInfo:
			// 当前没有建设或祈祷事实；仅返回真实共享资产/个人贡献及未创建状态。
			_, guild, err := p13GuildNeedMember(tx, id)
			if err != nil {
				return err
			}
			switch b.(type) {
			case *pb.CSUnionHeroStatueInfo:
				response = &pb.SCUnionHeroStatueInfo{CoinNum: proto.Int64(guild.Coin), BadgeNum: proto.Int32(guild.Badge)}
			case *pb.CSUnionGuardTreeInfo:
				response = &pb.SCUnionGuardTreeInfo{CoinNum: proto.Int64(guild.Coin), BadgeNum: proto.Int32(guild.Badge)}
			case *pb.CSUnionLightChurchInfo:
				var profile db.Profile
				if err = tx.First(&profile, id).Error; err != nil {
					return err
				}
				player, _, err := decodeProfile(&profile)
				if err != nil {
					return err
				}
				// 原回调会直接读取addProp并重算阵容，零对象保证无未获授属性。
				response = &pb.SCUnionLightChurchInfo{PrayTimes: proto.Int32(0), Contribution: proto.Int32(player.GetContribution()), AddProp: &pb.BaseProp{}, InvalidTime: proto.Int64(0)}
			}
			return nil
		case *pb.CSUnionMembers, *pb.CSUnionApplyList:
			member, guild, err := p13GuildNeedMember(tx, id)
			if err != nil {
				return err
			}
			if _, ok := b.(*pb.CSUnionApplyList); ok {
				if err = p13GuildRole(member, constUtil.P13GuildVice); err != nil {
					return err
				}
				var applies []db.P13GuildApply
				if err = tx.Where("guild_id=?", guild.ID).Order("created_at,player_id").Find(&applies).Error; err != nil {
					return err
				}
				reply := &pb.SCUnionApplyList{}
				for _, a := range applies {
					info, err := s.p13GuildPlayer(tx, id, a.PlayerID, nil, now)
					if err != nil {
						return err
					}
					reply.List = append(reply.List, info)
				}
				response = reply
			} else {
				var members []db.P13GuildMember
				if err = tx.Where("guild_id=?", guild.ID).Order("role DESC,player_id").Find(&members).Error; err != nil {
					return err
				}
				reply := &pb.SCUnionMembers{}
				for _, m := range members {
					info, err := s.p13GuildPlayer(tx, id, m.PlayerID, &m, now)
					if err != nil {
						return err
					}
					reply.List = append(reply.List, info)
				}
				response = reply
			}
			return nil
		case *pb.CSUnionStore:
			m, g, err := p13GuildNeedMember(tx, id)
			if err != nil {
				return err
			}
			cap, err := s.p13GuildCapacity(g.Level)
			if err != nil {
				return err
			}
			var count int64
			if err = tx.Model(&db.P13GuildMember{}).Where("guild_id=?", g.ID).Count(&count).Error; err != nil {
				return err
			}
			reply := &pb.SCUnionStore{CoinNum: proto.Int64(g.Coin), BadgeNum: proto.Int32(g.Badge), CurrentScore: proto.Int32(g.DonateScore), CurrentPerson: proto.Int32(int32(count)), MaxPerson: proto.Int32(cap)}
			if m.DonateDay == p13GuildDay(now) {
				reply.DonateCoin = proto.Int64(m.TodayCoin)
				reply.DonateBadge = proto.Int32(m.TodayBadge)
			}
			boxes, dailyScore, available, err := s.ProjectP13GuildDonateBoxes(tx, m, now)
			if err != nil {
				return err
			}
			if available {
				reply.Boxes = boxes
				reply.CurrentScore = proto.Int32(dailyScore)
			}
			response = reply
			return nil
		default:
			return errors.New("公会查询入口未恢复")
		}
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

func (s *GameService) p13GuildCreate(ctx context.Context, id int64, b *pb.CSUnionCreate, stamp int64, now time.Time) ([]proto.Message, error) {
	response := &pb.SCUnionCreate{}
	return s.p13GuildMutate(ctx, id, b, response, stamp, now, false, func(tx *gorm.DB, row *db.Profile, p *pb.Player, state *model.GameState) error {
		if err := s.p13GuildText(b.GetName(), "UnionNameLengthLimit", false); err != nil {
			return err
		}
		m, err := p13GuildMember(tx, id)
		if err != nil {
			return err
		}
		if err = s.p13GuildEligibility(p, m, now); err != nil {
			return err
		}
		if _, err = s.p13GuildCapacity(1); err != nil {
			return err
		}
		cost, err := parsePoolCost(s.table("GameConfigure", "StaticName", "UnionCreateCost")["NameValue"])
		if err != nil {
			return err
		}
		if err = deductPoolCost(p, state, cost); err != nil {
			return err
		}
		guild := db.P13Guild{ID: uuid.NewString(), Name: b.GetName(), Level: 1, CreatedAt: now.UnixMilli()}
		if err = tx.Create(&guild).Error; err != nil {
			return err
		}
		m.GuildID = guild.ID
		m.Role = constUtil.P13GuildChairman
		m.JoinedAt = now.UnixMilli()
		m.QuitUntil = 0
		if err = tx.Save(m).Error; err != nil {
			return err
		}
		if err = tx.Where("player_id=?", id).Delete(&db.P13GuildApply{}).Error; err != nil {
			return err
		}
		response.Cost = []*pb.BaseResource{cost}
		return nil
	})
}
func (s *GameService) p13GuildApply(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var guildID string
	var response proto.Message
	cancel := false
	switch b := request.(type) {
	case *pb.CSUnionApply:
		guildID = b.GetUnionId()
		response = &pb.SCUnionApply{}
	case *pb.CSUnionUnApply:
		guildID = b.GetUnionId()
		response = &pb.SCUnionUnApply{}
		cancel = true
	}
	return s.p13GuildMutate(ctx, id, request, response, stamp, now, false, func(tx *gorm.DB, row *db.Profile, p *pb.Player, state *model.GameState) error {
		if guildID == "" || len(guildID) > 64 {
			return errors.New("公会编号无效")
		}
		if cancel {
			return tx.Where("player_id=? AND guild_id=?", id, guildID).Delete(&db.P13GuildApply{}).Error
		}
		m, err := p13GuildMember(tx, id)
		if err != nil {
			return err
		}
		if err = s.p13GuildEligibility(p, m, now); err != nil {
			return err
		}
		var guild db.P13Guild
		if err = tx.First(&guild, "id=?", guildID).Error; err != nil {
			return err
		}
		var applies int64
		if err = tx.Model(&db.P13GuildApply{}).Where("player_id=? AND guild_id=?", id, guildID).Count(&applies).Error; err != nil {
			return err
		}
		if applies > 0 {
			return nil
		}
		limit, err := s.p10Setting("UnionApplyMaxNum")
		if err != nil {
			return err
		}
		if err = tx.Model(&db.P13GuildApply{}).Where("player_id=?", id).Count(&applies).Error; err != nil {
			return err
		}
		if applies >= int64(limit) {
			return errors.New("原公会申请数已满")
		}
		return tx.Create(&db.P13GuildApply{PlayerID: id, GuildID: guildID, CreatedAt: now.UnixMilli()}).Error
	})
}

// p13GuildManage 由实际职位授权，目标角色始终绑定同一公会；会长不能退出或被踢。
func (s *GameService) p13GuildManage(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var target int64
	var response proto.Message
	switch b := request.(type) {
	case *pb.CSUnionAgreeApply:
		target = b.GetPlayerId()
		response = &pb.SCUnionAgreeApply{}
	case *pb.CSUnionRefuseApply:
		target = b.GetPlayerId()
		response = &pb.SCUnionRefuseApply{}
	case *pb.CSPromotionViceChairman:
		target = b.GetPlayerId()
		response = &pb.SCPromotionViceChairman{}
	case *pb.CSDismissViceChairman:
		target = b.GetPlayerId()
		response = &pb.SCDismissViceChairman{}
	case *pb.CSRemoveUnionPlayer:
		target = b.GetPlayerId()
		response = &pb.SCRemoveUnionPlayer{}
	case *pb.CSTransferChairman:
		target = b.GetPlayerId()
		response = &pb.SCTransferChairman{}
	case *pb.CSQuitUnion:
		response = &pb.SCQuitUnion{}
	case *pb.CSDissolveUnion:
		response = &pb.SCDissolveUnion{}
	case *pb.CSUnionNoticeChange:
		response = &pb.SCUnionNoticeChange{}
	case *pb.CSUnionDeclarationChange:
		response = &pb.SCUnionDeclarationChange{}
	case *pb.CSChangeUnionFlag:
		response = &pb.SCChangeUnionFlag{UnionFlag: b.UnionFlag}
	}
	return s.p13GuildMutate(ctx, id, request, response, stamp, now, false, func(tx *gorm.DB, row *db.Profile, p *pb.Player, state *model.GameState) error {
		actor, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		need := constUtil.P13GuildChairman
		switch request.(type) {
		case *pb.CSUnionAgreeApply, *pb.CSUnionRefuseApply, *pb.CSRemoveUnionPlayer, *pb.CSUnionNoticeChange:
			need = constUtil.P13GuildVice
		case *pb.CSQuitUnion:
			need = constUtil.P13GuildMember
		}
		if err = p13GuildRole(actor, need); err != nil {
			return err
		}
		if target != 0 {
			if target < 1 || target == id {
				return errors.New("公会目标角色无效")
			}
			var targetRow db.Profile
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&targetRow, target).Error; err != nil {
				return err
			}
			targetPlayer, _, err := decodeProfile(&targetRow)
			if err != nil {
				return err
			}
			targetMember, err := p13GuildMember(tx, target)
			if err != nil {
				return err
			}
			switch request.(type) {
			case *pb.CSUnionAgreeApply, *pb.CSUnionRefuseApply:
				var apply db.P13GuildApply
				if err = tx.First(&apply, "player_id=? AND guild_id=?", target, guild.ID).Error; err != nil {
					return err
				}
				if _, ok := request.(*pb.CSUnionAgreeApply); ok {
					if err = s.p13GuildEligibility(targetPlayer, targetMember, now); err != nil {
						return err
					}
					cap, err := s.p13GuildCapacity(guild.Level)
					if err != nil {
						return err
					}
					var count int64
					if err = tx.Model(&db.P13GuildMember{}).Where("guild_id=?", guild.ID).Count(&count).Error; err != nil {
						return err
					}
					if count >= int64(cap) {
						return errors.New("公会成员已满")
					}
					targetMember.GuildID = guild.ID
					targetMember.Role = constUtil.P13GuildMember
					targetMember.JoinedAt = now.UnixMilli()
					targetMember.QuitUntil = 0
					if err = tx.Save(targetMember).Error; err != nil {
						return err
					}
					return tx.Where("player_id=?", target).Delete(&db.P13GuildApply{}).Error
				}
				return tx.Delete(&apply).Error
			}
			if targetMember.GuildID != guild.ID {
				return errors.New("目标不属于当前公会")
			}
			switch request.(type) {
			case *pb.CSPromotionViceChairman:
				if targetMember.Role != constUtil.P13GuildMember {
					return errors.New("目标已任职")
				}
				limit, err := s.p10Setting("UnionViceNum")
				if err != nil {
					return err
				}
				var count int64
				if err = tx.Model(&db.P13GuildMember{}).Where("guild_id=? AND role=?", guild.ID, constUtil.P13GuildVice).Count(&count).Error; err != nil {
					return err
				}
				if count >= int64(limit) {
					return errors.New("原副会长名额已满")
				}
				targetMember.Role = constUtil.P13GuildVice
			case *pb.CSDismissViceChairman:
				if targetMember.Role != constUtil.P13GuildVice {
					return errors.New("目标不是副会长")
				}
				targetMember.Role = constUtil.P13GuildMember
			case *pb.CSTransferChairman:
				actor.Role = constUtil.P13GuildMember
				if err = tx.Save(actor).Error; err != nil {
					return err
				}
				targetMember.Role = constUtil.P13GuildChairman
			case *pb.CSRemoveUnionPlayer:
				if targetMember.Role >= actor.Role {
					return errors.New("不能移除同级或上级")
				}
				return s.p13GuildLeave(tx, targetMember, now)
			}
			return tx.Save(targetMember).Error
		}
		switch b := request.(type) {
		case *pb.CSQuitUnion:
			if actor.Role == constUtil.P13GuildChairman {
				return errors.New("会长需先转让或解散")
			}
			return s.p13GuildLeave(tx, actor, now)
		case *pb.CSDissolveUnion:
			var members []db.P13GuildMember
			if err = tx.Where("guild_id=?", guild.ID).Find(&members).Error; err != nil {
				return err
			}
			for i := range members {
				if err = s.p13GuildLeave(tx, &members[i], now); err != nil {
					return err
				}
			}
			if err = tx.Where("guild_id=?", guild.ID).Delete(&db.P13GuildApply{}).Error; err != nil {
				return err
			}
			return tx.Delete(guild).Error
		case *pb.CSUnionNoticeChange:
			if err = s.p13GuildText(b.GetContent(), "UnionNoticeLengthLimit", true); err != nil {
				return err
			}
			guild.Notice = b.GetContent()
		case *pb.CSUnionDeclarationChange:
			if err = s.p13GuildText(b.GetContent(), "UnionDeclarationLengthLimit", true); err != nil {
				return err
			}
			guild.Declaration = b.GetContent()
		case *pb.CSChangeUnionFlag:
			if len(s.table("UnionLogo", "id", strconv.Itoa(int(b.GetUnionFlag())))) == 0 {
				return errors.New("原公会旗帜不存在")
			}
			guild.Flag = b.GetUnionFlag()
		default:
			return errors.New("公会目标编号缺失")
		}
		return tx.Save(guild).Error
	})
}
func (s *GameService) p13GuildLeave(tx *gorm.DB, m *db.P13GuildMember, now time.Time) error {
	return s.WithP5GuildMembershipRevocation(tx.Statement.Context, tx, m.PlayerID, now, func() error {
		hours, err := s.p10Setting("UnionQuitInterval")
		if err != nil {
			return err
		}
		m.GuildID = ""
		m.Role = constUtil.P13GuildMember
		m.QuitUntil = now.Add(time.Duration(hours) * time.Hour).UnixMilli()
		if err = tx.Save(m).Error; err != nil {
			return err
		}
		if err = s.P13GuildRoomRevoke(tx, m.PlayerID); err != nil {
			return err
		}
		if err = s.P13UnionFightFieldRevoke(tx, m.PlayerID, now); err != nil {
			return err
		}
		return tx.Where("player_id=?", m.PlayerID).Delete(&db.P13GuildApply{}).Error
	})
}

// p13GuildEconomy 采用原最小捐献单位/上限/贡献转换；公会和玩家资产同事务变化。
func (s *GameService) p13GuildEconomy(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	switch request.(type) {
	case *pb.CSUnionDonate:
		response = &pb.SCUnionDonate{}
	case *pb.CSUnionUpgrade:
		response = &pb.SCUnionUpgrade{}
	case *pb.CSGetUnionStamina:
		response = &pb.SCGetUnionStamina{}
	}
	return s.p13GuildMutate(ctx, id, request, response, stamp, now, true, func(tx *gorm.DB, row *db.Profile, p *pb.Player, state *model.GameState) error {
		m, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		day := p13GuildDay(now)
		switch b := request.(type) {
		case *pb.CSGetUnionStamina:
			if m.StaminaDay == day {
				return errors.New("今日公会体力已领取")
			}
			amount, err := s.p10Setting("UnionGetStaminaDaily")
			if err != nil || amount < 1 {
				return errors.New("原公会体力配置无效")
			}
			gain, err := s.GrantFixedReward(p, state, fmt.Sprintf("stamina,%d", amount))
			if err != nil {
				return err
			}
			response.(*pb.SCGetUnionStamina).Get = gain
			m.StaminaDay = day
		case *pb.CSUnionUpgrade:
			if err = p13GuildRole(m, constUtil.P13GuildChairman); err != nil {
				return err
			}
			if _, err = s.p13GuildCapacity(guild.Level + 1); err != nil {
				return errors.New("已到原公会最高等级")
			}
			cfg := s.table("UnionConfig", "Level", strconv.Itoa(int(guild.Level)))
			coin, e1 := strconv.ParseInt(cfg["UnionCoin"], 10, 64)
			badge, e2 := strconv.ParseInt(cfg["UnionBadge"], 10, 32)
			exp, e3 := strconv.ParseInt(cfg["UnionExpScore"], 10, 32)
			if e1 != nil || e2 != nil || e3 != nil || coin < 0 || badge < 0 || exp < 0 {
				return errors.New("原公会升级成本无效")
			}
			if guild.Coin < coin || guild.Badge < int32(badge) || guild.Exp < int32(exp) {
				return errors.New("公会升级资源不足")
			}
			guild.Coin -= coin
			guild.Badge -= int32(badge)
			guild.Exp -= int32(exp)
			guild.Level++
		case *pb.CSUnionDonate:
			if m.DonateDay != day {
				m.DonateDay = day
				m.TodayCoin = 0
				m.TodayBadge = 0
			}
			var amount, limit, contribution, donate, score int32
			var currency string
			switch b.GetDonateType() {
			case "coin":
				amount, err = s.p10Setting("UnionMinDailyDonateCoin")
				if err != nil {
					return err
				}
				limit, err = s.p10Setting("UnionMaxDailyDonateCoin")
				if err != nil {
					return err
				}
				contribution, err = s.p10Setting("UnionDonateCoinContributVal")
				if err != nil {
					return err
				}
				donate, err = s.p10Setting("UnionDonateCoinDonateVal")
				if err != nil {
					return err
				}
				if m.TodayCoin == 0 {
					score, err = s.p10Setting("UnionFirstContributeCoinScore")
					if err != nil {
						return err
					}
				}
				if m.TodayCoin+int64(amount) > int64(limit) || guild.Coin > math.MaxInt64-int64(amount) {
					return errors.New("金币捐献达到原上限")
				}
				currency = "coin"
			case "badge":
				amount, err = s.p10Setting("UnionMinDailyDonateBadge")
				if err != nil {
					return err
				}
				limit, err = s.p10Setting("UnionMaxDailyDonateBadge")
				if err != nil {
					return err
				}
				contribution, err = s.p10Setting("UnionDonateBadgeContributVal")
				if err != nil {
					return err
				}
				donate, err = s.p10Setting("UnionDonateBadgeDonateVal")
				if err != nil {
					return err
				}
				score, err = s.p10Setting("UnionUseBadgeScore")
				if err != nil {
					return err
				}
				if int64(m.TodayBadge)+int64(amount) > int64(limit) || int64(guild.Badge)+int64(amount) > math.MaxInt32 {
					return errors.New("徽章捐献达到原上限")
				}
				currency = "item_089"
			default:
				return errors.New("原捐献种类无效")
			}
			if amount < 1 || contribution < 1 || donate < 1 || int64(p.GetContribution())+int64(contribution) > math.MaxInt32 || int64(m.Contribution)+int64(contribution) > math.MaxInt32 || int64(guild.DonateScore)+int64(donate) > math.MaxInt32 || int64(guild.Exp)+int64(score) > math.MaxInt32 {
				return errors.New("公会捐献配置或数值无效")
			}
			if err = deductPoolCost(p, state, &pb.BaseResource{Id: proto.String(currency), Num: proto.Int32(amount)}); err != nil {
				return err
			}
			if b.GetDonateType() == "coin" {
				guild.Coin += int64(amount)
				m.TodayCoin += int64(amount)
			} else {
				guild.Badge += amount
				m.TodayBadge += amount
			}
			p.Contribution = proto.Int32(p.GetContribution() + contribution)
			m.Contribution += contribution
			guild.DonateScore += donate
			guild.Exp += score
			// 真捐扣费和当日宝箱来源在同一公会/角色事务内提交。
			if err = s.RecordP13GuildDonateBox(tx, m, b, stamp, amount, donate, now); err != nil {
				return err
			}
			if err = s.RecordP13GuildDonationBadge(tx, m, b, stamp, amount, now); err != nil {
				return err
			}
			reply := response.(*pb.SCUnionDonate)
			reply.ContributionAdd = proto.Int32(contribution)
			reply.DonateScoreAdd = proto.Int32(donate)
			reply.Score = proto.Int32(score)
		}
		if err = tx.Save(m).Error; err != nil {
			return err
		}
		return tx.Save(guild).Error
	})
}

// p13GuildOpenBuilder 从真实公会资源支付原建设费用，建设状态与回执同事务提交。
func (s *GameService) p13GuildOpenBuilder(ctx context.Context, id int64, request *pb.CSUnionOpenBuilder, stamp int64, now time.Time) ([]proto.Message, error) {
	return s.p13GuildMutate(ctx, id, request, &pb.SCUnionOpenBuilder{}, stamp, now, true, func(tx *gorm.DB, row *db.Profile, p *pb.Player, state *model.GameState) error {
		member, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		// 原客户端myRole非零才发建设确认；0成员不能代副会长或会长消费共享资产。
		if err = p13GuildRole(member, constUtil.P13GuildVice); err != nil {
			return err
		}
		var (
			opened            *int32
			coinKey, badgeKey string
		)
		switch request.GetType() {
		case constUtil.P13GuildHeroStatue:
			opened = &guild.HeroStatueOpen
			coinKey, badgeKey = "UnionOpenHeroStatueCoinCost", "UnionOpenHeroStatueBadgeCost"
		case constUtil.P13GuildGuardTree:
			opened = &guild.GuardTreeOpen
			coinKey, badgeKey = "UnionOpenGuardTreeCoinCost", "UnionOpenGuardTreeBadgeCost"
		default:
			return errors.New("原公会建设类型无效")
		}
		if *opened != 0 {
			return errors.New("公会建筑已经创建")
		}
		coin, err := s.p10Setting(coinKey)
		if err != nil {
			return err
		}
		badge, err := s.p10Setting(badgeKey)
		if err != nil {
			return err
		}
		if coin <= 0 || badge <= 0 {
			return errors.New("原公会建设成本配置无效")
		}
		if guild.Coin < int64(coin) || guild.Badge < badge {
			return errors.New("公会建设金币或徽章不足")
		}
		guild.Coin -= int64(coin)
		guild.Badge -= badge
		*opened = 1
		return tx.Save(guild).Error
	})
}

// P13GuildBuildingState 映射持久化建设事实，重连、重新进入和原刷新共用同一来源。
func (s *GameService) P13GuildBuildingState(tx *gorm.DB, id int64) (*pb.SCUnionBuilding, error) {
	_, guild, err := p13GuildNeedMember(tx, id)
	if err != nil {
		return nil, err
	}
	return &pb.SCUnionBuilding{HeroStatueOpen: proto.Int32(guild.HeroStatueOpen), GuardTreeOpen: proto.Int32(guild.GuardTreeOpen)}, nil
}

// AfterP13GuildBuildingChange 在事务提交后广播给当前已认证本会领地成员，失败推送可由原刷新恢复。
func (s *GameService) AfterP13GuildBuildingChange(ctx context.Context, id int64) {
	tx := db.GetIns().WithContext(ctx)
	if !tx.Migrator().HasTable(&db.P10ScenePresence{}) {
		return
	}
	member, _, err := p13GuildNeedMember(tx, id)
	if err != nil {
		return
	}
	state, err := s.P13GuildBuildingState(tx, id)
	if err != nil {
		return
	}
	s.p10SceneBroadcast(ctx, constUtil.P13GuildRoomScopePrefix+member.GuildID, 1, 0, state)
}

// DispatchP13Guild 接入原CS/SC消息；未恢复的经济规则明确返回错误。
func (s *GameService) DispatchP13Guild(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	if messages, handled, err := s.DispatchP13UnionFight(ctx, id, request, stamp); handled {
		return messages, true, err
	}
	if messages, handled, err := s.DispatchP13GuildRoom(ctx, id, request); handled {
		return messages, true, err
	}
	now := LocalPlayNow(ctx)
	var messages []proto.Message
	var err error
	switch b := request.(type) {
	case *pb.CSGetUnionInfo, *pb.CSUnionSearch, *pb.CSUnionMembers, *pb.CSUnionApplyList, *pb.CSUnionStore, *pb.CSUnionHeroStatueInfo, *pb.CSUnionGuardTreeInfo, *pb.CSUnionLightChurchInfo:
		messages, err = s.p13GuildQuery(ctx, id, b, now)
	case *pb.CSUnionCreate:
		messages, err = s.p13GuildCreate(ctx, id, b, stamp, now)
	case *pb.CSUnionApply, *pb.CSUnionUnApply:
		messages, err = s.p13GuildApply(ctx, id, b, stamp, now)
	case *pb.CSUnionAgreeApply, *pb.CSUnionRefuseApply, *pb.CSPromotionViceChairman, *pb.CSDismissViceChairman, *pb.CSRemoveUnionPlayer, *pb.CSTransferChairman, *pb.CSQuitUnion, *pb.CSDissolveUnion, *pb.CSUnionNoticeChange, *pb.CSUnionDeclarationChange, *pb.CSChangeUnionFlag:
		messages, err = s.p13GuildManage(ctx, id, b, stamp, now)
	case *pb.CSUnionDonate, *pb.CSUnionUpgrade, *pb.CSGetUnionStamina:
		messages, err = s.p13GuildEconomy(ctx, id, b, stamp, now)
	case *pb.CSUnionOpenDonateBox:
		messages, err = s.p13GuildOpenDonateBox(ctx, id, b, stamp, now)
	case *pb.CSUnionOpenBuilder:
		messages, err = s.p13GuildOpenBuilder(ctx, id, b, stamp, now)
	case *pb.CSUnionHeroStatueUpgrade, *pb.CSUnionGuardTreeUpgrade:
		return nil, true, errors.New(constUtil.P13GuildBuildingUnverified)
	case *pb.CSUnionPray:
		return nil, true, errors.New(constUtil.P13GuildPrayerUnverified)
	default:
		return nil, false, nil
	}
	return messages, true, err
}
