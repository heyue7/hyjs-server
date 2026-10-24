package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type dailyCopyDrop struct {
	ID      string
	Options [][2]int64
}

// InitializeDailyCopy 验证 APK 三张原表，并保留已有 SQL 本地策略。
func (s *GameService) InitializeDailyCopy(ctx context.Context) error {
	if len(s.Tables["DailyCopy"]) == 0 || len(s.Tables["DailyCopyChapter"]) == 0 || len(s.Tables["DailyCopyEnemy"]) == 0 {
		return errors.New(constUtil.DailyCopyConfigError)
	}
	seen := map[string]bool{}
	for _, row := range s.Tables["DailyCopy"] {
		if row["Type"] == "" || seen[row["Type"]] {
			return errors.New(constUtil.DailyCopyConfigError)
		}
		seen[row["Type"]] = true
		if _, _, _, err := dailyCopyTypeRule(row, time.Now()); err != nil {
			return err
		}
	}
	seen = map[string]bool{}
	for _, row := range s.Tables["DailyCopyChapter"] {
		if row["ChapterId"] == "" || seen[row["ChapterId"]] || s.table("DailyCopy", "Type", row["Type"]) == nil {
			return errors.New(constUtil.DailyCopyConfigError)
		}
		seen[row["ChapterId"]] = true
		if _, err := dailyCopyPositive(row["LevelLimit"]); err != nil {
			return err
		}
		if _, err := dailyCopyPositive(row["Recommend"]); err != nil {
			return err
		}
		if _, err := s.dailyCopyDrops(row); err != nil {
			return err
		}
		if _, err := s.dailyCopyMonster(row["ChapterId"]); err != nil {
			return err
		}
	}
	for _, row := range s.Tables["DailyCopyEnemy"] {
		if !seen[row["ChapterId"]] {
			return errors.New(constUtil.DailyCopyConfigError)
		}
	}
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.DailyCopyPolicy{}, &db.DailyCopyState{}, &db.DailyCopyFight{}, &db.DailyCopyStartReceipt{}); err != nil {
		return err
	}
	rule := &db.DailyCopyPolicy{ID: constUtil.DailyCopyPolicyID, Enabled: true, RoomID: constUtil.DailyCopyDefaultRoom, MaxRound: constUtil.DailyCopyDefaultRounds, WinStars: constUtil.DailyCopyDefaultStars, Source: constUtil.DailyCopyPolicySource}
	return d.Clauses(clause.OnConflict{DoNothing: true}).Create(rule).Error
}

func dailyCopyPositive(raw string) (int32, error) {
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 1 {
		return 0, errors.New(constUtil.DailyCopyConfigError)
	}
	return int32(n), nil
}

// dailyCopyTypeRule 将原表 1=周一至7=周日映射到 UTC+8 业务日。
func dailyCopyTypeRule(row map[string]string, now time.Time) (bool, int32, int32, error) {
	if row == nil {
		return false, 0, 0, errors.New(constUtil.DailyCopyConfigError)
	}
	stamina, err := dailyCopyPositive(row["Stamina"])
	if err != nil {
		return false, 0, 0, err
	}
	limit, err := dailyCopyPositive(row["Count"])
	if err != nil {
		return false, 0, 0, err
	}
	weekday := int(now.In(time.FixedZone("UTC+8", 8*3600)).Weekday())
	if weekday == 0 {
		weekday = 7
	}
	open := false
	seen := map[int]bool{}
	for _, raw := range strings.Split(row["OpenDay"], ",") {
		day, err := strconv.Atoi(raw)
		if err != nil || day < 1 || day > 7 || seen[day] {
			return false, 0, 0, errors.New(constUtil.DailyCopyConfigError)
		}
		seen[day] = true
		open = open || day == weekday
	}
	return open, stamina, limit, nil
}

func dailyCopyDay(now time.Time) string {
	return now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
}

func loadDailyCopyPolicy(tx *gorm.DB) (*db.DailyCopyPolicy, error) {
	var policy db.DailyCopyPolicy
	if err := tx.First(&policy, "id = ?", constUtil.DailyCopyPolicyID).Error; err != nil {
		return nil, err
	}
	if policy.Source == "" || policy.RoomID != constUtil.DailyCopyDefaultRoom || policy.MaxRound < 1 || policy.MaxRound > constUtil.DailyCopyDefaultRounds || policy.WinStars < 1 || policy.WinStars > 5 {
		return nil, errors.New(constUtil.DailyCopyConfigError)
	}
	return &policy, nil
}

func loadDailyCopyState(tx *gorm.DB, id int64, kind, day string, limit int32) (*db.DailyCopyState, error) {
	row := &db.DailyCopyState{PlayerID: id, Type: kind, Day: day, Limit: limit}
	err := tx.Where("player_id = ? AND day = ? AND type = ?", id, day, kind).First(row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if row.Limit < 1 || row.Used < 0 || row.Used > row.Limit {
		return nil, errors.New(constUtil.DailyCopyConfigError)
	}
	return row, nil
}

// dailyCopyDrops 恢复每项资源的数量及原始权重；无 Drop1 时使用固定 Drop。
func (s *GameService) dailyCopyDrops(chapter map[string]string) ([]dailyCopyDrop, error) {
	var drops []dailyCopyDrop
	seen := map[string]bool{}
	weighted := chapter["Drop1"] != ""
	separator := ";"
	if weighted {
		separator = "|"
	}
	value := chapter["Drop"]
	if weighted {
		value = chapter["Drop1"]
	}
	for _, part := range strings.Split(value, separator) {
		var drop dailyCopyDrop
		if weighted {
			pair := strings.Split(part, ":")
			if len(pair) != 2 {
				return nil, errors.New(constUtil.DailyCopyConfigError)
			}
			drop.ID = pair[0]
			for _, option := range strings.Split(pair[1], ";") {
				entry := strings.Split(option, ",")
				if len(entry) != 2 {
					return nil, errors.New(constUtil.DailyCopyConfigError)
				}
				n, err := strconv.ParseInt(entry[0], 10, 32)
				w, weightErr := strconv.ParseInt(entry[1], 10, 32)
				if err != nil || weightErr != nil || n < 0 || w < 1 {
					return nil, errors.New(constUtil.DailyCopyConfigError)
				}
				drop.Options = append(drop.Options, [2]int64{n, w})
			}
		} else {
			pair := strings.Split(part, ",")
			if len(pair) != 2 {
				return nil, errors.New(constUtil.DailyCopyConfigError)
			}
			drop.ID = pair[0]
			n, err := dailyCopyPositive(pair[1])
			if err != nil {
				return nil, err
			}
			drop.Options = [][2]int64{{int64(n), 1000}}
		}
		if seen[drop.ID] || !s.dailyCopyResourceSupported(drop.ID) {
			return nil, errors.New(constUtil.DailyCopyConfigError)
		}
		seen[drop.ID] = true
		var total int64
		for _, option := range drop.Options {
			total += option[1]
		}
		if total != 1000 {
			return nil, errors.New(constUtil.DailyCopyConfigError)
		}
		drops = append(drops, drop)
	}
	return drops, nil
}

func (s *GameService) dailyCopyResourceSupported(key string) bool {
	switch {
	case key == "coin", key == "diamond":
		return true
	case strings.HasPrefix(key, "item_"):
		return s.table("Item", "itemId", key) != nil
	case strings.HasPrefix(key, "hero_exp_"):
		return s.table("Hero", "id", key)["heroClass"] == "99"
	case strings.HasPrefix(key, "trea_"):
		return s.table("Treasure", "id", key) != nil
	case strings.HasPrefix(key, "frag_trea_"):
		return s.table("Frag", "id", key) != nil
	}
	return false
}

func dailyCopyDraw(drops []dailyCopyDrop) (string, error) {
	var rewards []string
	for _, drop := range drops {
		pick, err := rand.Int(rand.Reader, big.NewInt(1000))
		if err != nil {
			return "", err
		}
		n := pick.Int64()
		for _, option := range drop.Options {
			if n < option[1] {
				if option[0] > 0 {
					rewards = append(rewards, fmt.Sprintf("%s,%d", drop.ID, option[0]))
				}
				break
			}
			n -= option[1]
		}
	}
	return strings.Join(rewards, ";"), nil
}

// dailyCopyMonster 使用原章节怪物属性、技能、天赋和位置构建客户端可运行的阵容。
func (s *GameService) dailyCopyMonster(chapter string) (*pb.BattleMonster, error) {
	monster := &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(chapter), GroupIndex: proto.Int32(1), Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	seen := map[int32]bool{}
	guids := map[string]bool{}
	for _, row := range s.Tables["DailyCopyEnemy"] {
		if row["ChapterId"] != chapter {
			continue
		}
		pose, err := dailyCopyPositive(row["Location"])
		if err != nil || pose > 15 || seen[pose] || row["CopyEnemyId"] == "" || guids[row["CopyEnemyId"]] || s.table("Hero", "id", row["HeroId"]) == nil {
			return nil, errors.New(constUtil.DailyCopyConfigError)
		}
		seen[pose] = true
		guids[row["CopyEnemyId"]] = true
		actor, err := s.actor(row, "daily-monster-"+row["CopyEnemyId"], pose, true)
		if err != nil {
			return nil, err
		}
		if actor.GetProp().GetHp() <= 0 || actor.GetProp().GetAtt() <= 0 || math.IsNaN(actor.GetProp().GetHp()) || math.IsInf(actor.GetProp().GetHp(), 0) {
			return nil, errors.New(constUtil.DailyCopyConfigError)
		}
		monster.Formation = append(monster.Formation, actor)
	}
	if len(monster.Formation) == 0 {
		return nil, errors.New(constUtil.DailyCopyConfigError)
	}
	return monster, nil
}

func (s *GameService) dailyCopy(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, error) {
	return s.dailyCopyAt(ctx, id, request, stamp, time.Now())
}

// dailyCopyAt 用角色行锁串行化体力、次数和战斗回执，所有写入同一事务提交。
func (s *GameService) dailyCopyAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	var updated *pb.Player
	var currentState *pb.SCDailyCopyState
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if err = s.RefreshP5GuildPrayerPower(ctx, tx, player, game, p13UFNow(ctx)); err != nil {
			return err
		}
		switch b := request.(type) {
		case *pb.CSDailyCopyState:
			response, err = s.dailyCopyInfo(tx, player, now)
		case *pb.CSDailyCopyFightStart:
			var changed bool
			response, changed, err = s.dailyCopyStart(tx, player, game, b, stamp, now)
			updated = player
			if err == nil && changed {
				err = saveProfile(tx, &profile, player, game)
			}
		case *pb.CSDailyCopyFightEnd:
			var changed bool
			response, changed, err = s.dailyCopyEnd(tx, player, game, b, now)
			updated = player
			if err == nil && changed {
				err = saveProfile(tx, &profile, player, game)
			}
			// 冻结结算回包保持不变，追加当前业务日的次数和开放状态。
			if err == nil {
				currentState, err = s.dailyCopyInfo(tx, player, now)
			}
		default:
			return errors.New(constUtil.DailyCopyBattleError)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if end, ok := response.(*pb.SCDailyCopyFightEnd); ok {
		// Start已经扣费且SCPlayer提供权威余额；原结果页稍后会Delete(cost)。
		// 只投影尚待客户端应用的成本，保留冻结回执的真实体力成本供重放校验。
		projected := proto.Clone(end).(*pb.SCDailyCopyFightEnd)
		costs := projected.Cost
		projected.Cost = nil
		for _, cost := range costs {
			if cost.GetId() != "stamina" {
				projected.Cost = append(projected.Cost, cost)
			}
		}
		response = projected
	}
	messages := []proto.Message{response}
	if updated != nil {
		messages = append(messages, &pb.SCPlayer{Player: updated})
	}
	if currentState != nil {
		messages = append(messages, currentState)
	}
	return messages, nil
}

func (s *GameService) dailyCopyInfo(tx *gorm.DB, player *pb.Player, now time.Time) (*pb.SCDailyCopyState, error) {
	// 星期开放与剩余次数跟随个人游玩日，金融及战斗时钟保持真实时间。
	now = LocalPlayAt(tx.Statement.Context, now)
	policy, err := loadDailyCopyPolicy(tx)
	if err != nil {
		return nil, err
	}
	var active db.DailyCopyFight
	activeErr := tx.Where("player_id = ? AND day = ? AND settled = ?", player.GetPlayerId(), dailyCopyDay(now), false).First(&active).Error
	if activeErr != nil && !errors.Is(activeErr, gorm.ErrRecordNotFound) {
		return nil, activeErr
	}
	result := &pb.SCDailyCopyState{}
	for _, rule := range s.Tables["DailyCopy"] {
		open, _, limit, err := dailyCopyTypeRule(rule, now)
		if err != nil {
			return nil, err
		}
		state, err := loadDailyCopyState(tx, player.GetPlayerId(), rule["Type"], dailyCopyDay(now), limit)
		if err != nil {
			return nil, err
		}
		item := &pb.DailyCopyStateType{Type: proto.String(rule["Type"]), IsOpen: proto.Int32(0), LastTime: proto.Int32(state.Limit - state.Used)}
		if policy.Enabled && open {
			item.IsOpen = proto.Int32(1)
		}
		// 原按钮要求lastTime>0；已扣费未结算的挑战投影为一次续接资格，数据库次数保持已使用。
		if activeErr == nil {
			item.IsOpen = proto.Int32(0)
			if active.Type == rule["Type"] {
				item.IsOpen = proto.Int32(1)
				item.LastTime = proto.Int32(max(item.GetLastTime(), 1))
			}
		}
		for _, chapter := range s.Tables["DailyCopyChapter"] {
			if chapter["Type"] != rule["Type"] {
				continue
			}
			level, err := dailyCopyPositive(chapter["LevelLimit"])
			if err != nil {
				return nil, err
			}
			recommend, err := dailyCopyPositive(chapter["Recommend"])
			if err != nil {
				return nil, err
			}
			entry := &pb.DailyCopyState{ChapterId: proto.String(chapter["ChapterId"]), LevelLimit: proto.Int32(level), Recommend: proto.Int32(recommend), Drop: proto.String(chapter["Drop"]), IsOpen: proto.Int32(0)}
			if item.GetIsOpen() == 1 && player.GetLevel() >= level {
				entry.IsOpen = proto.Int32(1)
			}
			if activeErr == nil {
				entry.IsOpen = proto.Int32(0)
				if active.ChapterID == chapter["ChapterId"] {
					entry.IsOpen = proto.Int32(1)
				}
			}
			item.DailyCopyState = append(item.DailyCopyState, entry)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func decodeDailyCopyStart(row *db.DailyCopyFight) (*pb.SCDailyCopyFightStart, error) {
	start := &pb.SCDailyCopyFightStart{}
	if err := proto.Unmarshal(row.Start, start); err != nil {
		return nil, err
	}
	if start.GetFightId() != row.FightID || start.GetBattlePlayer().GetPlayerId() != row.PlayerID || start.GetMaxRoundNum() < 1 || row.Stamina < 1 || row.WinStars < 1 || row.WinStars > 5 || len(start.GetMonster().GetFormation()) == 0 || len(start.GetBattlePlayer().GetFormation()) == 0 {
		return nil, errors.New(constUtil.DailyCopyConfigError)
	}
	return start, nil
}

func (s *GameService) dailyCopyStart(tx *gorm.DB, player *pb.Player, game *model.GameState, request *pb.CSDailyCopyFightStart, stamp int64, now time.Time) (*pb.SCDailyCopyFightStart, bool, error) {
	playNow := LocalPlayAt(tx.Statement.Context, now)
	if stamp <= 0 || request.GetChapterId() == "" {
		return nil, false, errors.New(constUtil.DailyCopyReplayError)
	}
	var receipt db.DailyCopyStartReceipt
	err := tx.Where("player_id = ? AND stamp = ?", player.GetPlayerId(), stamp).First(&receipt).Error
	if err == nil {
		if receipt.ChapterID != request.GetChapterId() {
			return nil, false, errors.New(constUtil.DailyCopyReplayError)
		}
		var previous db.DailyCopyFight
		if err := tx.First(&previous, "fight_id = ? AND player_id = ?", receipt.FightID, player.GetPlayerId()).Error; err != nil {
			return nil, false, err
		}
		start, err := decodeDailyCopyStart(&previous)
		return start, false, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	err = tx.Where("player_id = ?", player.GetPlayerId()).Order("stamp DESC").First(&receipt).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if err == nil && stamp < receipt.Stamp {
		return nil, false, errors.New(constUtil.DailyCopyReplayError)
	}
	// 同日重新进入未结算章节复用原快照，避免重复扣体力和次数。
	var active db.DailyCopyFight
	err = tx.Where("player_id = ? AND day = ? AND settled = ?", player.GetPlayerId(), dailyCopyDay(playNow), false).First(&active).Error
	if err == nil {
		if active.ChapterID != request.GetChapterId() {
			return nil, false, errors.New(constUtil.DailyCopyActiveError)
		}
		start, err := decodeDailyCopyStart(&active)
		if err == nil {
			err = tx.Create(&db.DailyCopyStartReceipt{PlayerID: player.GetPlayerId(), Stamp: stamp, ChapterID: request.GetChapterId(), FightID: active.FightID}).Error
		}
		return start, false, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if s.P12WantedReady {
		if err := P12WantedRequireIdle(tx, player.GetPlayerId(), now); err != nil {
			return nil, false, err
		}
	}
	if s.P12FieldPVPReady {
		if err := P12FieldPVPRequireIdle(tx, player.GetPlayerId(), now); err != nil {
			return nil, false, err
		}
	}
	// 既有回执和恢复当前资源战斗已在前面处理；新场不能与其他已开始战斗并行。
	if game.Run != nil && game.Run.Fight != nil && !game.Run.Fight.Settled || game.Field != nil && game.Field.Fight != nil && !game.Field.Fight.Settled {
		return nil, false, errors.New(constUtil.DailyCopyActiveError)
	}
	policy, err := loadDailyCopyPolicy(tx)
	if err != nil {
		return nil, false, err
	}
	chapter := s.table("DailyCopyChapter", "ChapterId", request.GetChapterId())
	if chapter == nil || !policy.Enabled {
		return nil, false, errors.New(constUtil.DailyCopyUnavailable)
	}
	open, stamina, limit, err := dailyCopyTypeRule(s.table("DailyCopy", "Type", chapter["Type"]), playNow)
	if err != nil {
		return nil, false, err
	}
	level, err := dailyCopyPositive(chapter["LevelLimit"])
	if err != nil {
		return nil, false, err
	}
	state, err := loadDailyCopyState(tx, player.GetPlayerId(), chapter["Type"], dailyCopyDay(playNow), limit)
	if err != nil {
		return nil, false, err
	}
	if !open || player.GetLevel() < level || state.Used >= state.Limit {
		return nil, false, errors.New(constUtil.DailyCopyUnavailable)
	}
	if player.GetStamina() < stamina {
		return nil, false, errors.New(constUtil.DailyCopyStaminaError)
	}
	party, err := s.battlePlayer(player, game)
	if err != nil || len(party.GetFormation()) == 0 {
		return nil, false, errors.New(constUtil.DailyCopyBattleError)
	}
	monster, err := s.dailyCopyMonster(chapter["ChapterId"])
	if err != nil {
		return nil, false, err
	}
	drops, err := s.dailyCopyDrops(chapter)
	if err != nil {
		return nil, false, err
	}
	reward, err := dailyCopyDraw(drops)
	if err != nil {
		return nil, false, err
	}
	seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		return nil, false, err
	}
	start := &pb.SCDailyCopyFightStart{RoomId: proto.String(policy.RoomID), RandSeed: proto.Int64(seed.Int64()), BattlePlayer: party, Monster: monster, MaxRoundNum: proto.Int32(policy.MaxRound), FightId: proto.String(uuid.NewString())}
	row := &db.DailyCopyFight{FightID: start.GetFightId(), PlayerID: player.GetPlayerId(), StartStamp: stamp, Day: state.Day, Type: state.Type, ChapterID: chapter["ChapterId"], Stamina: stamina, WinStars: policy.WinStars, Reward: reward}
	row.Start, err = proto.Marshal(start)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Create(row).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Create(&db.DailyCopyStartReceipt{PlayerID: player.GetPlayerId(), Stamp: stamp, ChapterID: request.GetChapterId(), FightID: row.FightID}).Error; err != nil {
		return nil, false, err
	}
	state.Used++
	if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(state).Error; err != nil {
		return nil, false, err
	}
	player.Stamina = proto.Int32(player.GetStamina() - stamina)
	return start, true, nil
}

// validateDailyCopyEnd 只做可证实的会话、回合、HP边界与阵容引用校验，不声称权威模拟。
func validateDailyCopyEnd(start *pb.SCDailyCopyFightStart, request *pb.CSDailyCopyFightEnd) error {
	op := request.GetOptRecord()
	// 原protobuf-net对isWin=0和死亡方HP=0不编码字段；Getter默认零与原协议一致。
	if request.GetIsWin() < 0 || request.GetIsWin() > 1 || op == nil || op.GetFinalRoundNum() < 1 || op.GetFinalRoundNum() > start.GetMaxRoundNum() {
		return errors.New(constUtil.DailyCopyBattleError)
	}
	actors := [2]map[string]string{{}, {}}
	var hp [2]float64
	for side, lineup := range [][]*pb.BattleHero{start.GetBattlePlayer().GetFormation(), start.GetMonster().GetFormation()} {
		for _, hero := range lineup {
			if hero == nil || hero.GetGuid() == "" || actors[side][hero.GetGuid()] != "" || hero.GetProp().GetHp() <= 0 {
				return errors.New(constUtil.DailyCopyConfigError)
			}
			// Original FActor.uid prefixes the frozen BattleHero.guid with camp.
			// Historical raw GUID receipts retain their existing accepted identity.
			actors[side][hero.GetGuid()] = hero.GetGuid()
			actors[side][strconv.Itoa(side)+"_"+hero.GetGuid()] = hero.GetGuid()
			hp[side] += hero.GetProp().GetHp()
		}
	}
	for side, left := range []float64{op.GetAttackerTotalLeftHp(), op.GetDefenderTotalLeftHp()} {
		if math.IsNaN(left) || math.IsInf(left, 0) || left < 0 || left > hp[side] || math.IsNaN(hp[side]) || math.IsInf(hp[side], 0) {
			return errors.New(constUtil.DailyCopyBattleError)
		}
	}
	if request.GetIsWin() == 1 && (op.GetAttackerTotalLeftHp() <= 0 || op.GetDefenderTotalLeftHp() != 0) || request.GetIsWin() == 0 && op.GetAttackerTotalLeftHp() > 0 && op.GetFinalRoundNum() != start.GetMaxRoundNum() {
		return errors.New(constUtil.DailyCopyBattleError)
	}
	for _, skill := range op.GetSkillOpts() {
		if skill == nil || skill.GetCamp() < 0 || skill.GetCamp() > 1 || actors[skill.GetCamp()][skill.GetActorUid()] == "" || skill.GetRoundNum() < 0 || skill.GetRoundNum() > op.GetFinalRoundNum() || math.IsNaN(skill.GetRoundTime()) || math.IsInf(skill.GetRoundTime(), 0) || skill.GetRoundTime() < 0 {
			return errors.New(constUtil.DailyCopyBattleError)
		}
	}
	for side, trans := range []*pb.BattleTransOperate{op.GetAttackerTransOpt(), op.GetDefenderTransOpt()} {
		if trans == nil {
			continue
		}
		if trans.GetRoundNum() < 0 || trans.GetRoundNum() > op.GetFinalRoundNum() || len(trans.GetActorUids()) != len(trans.GetFpids()) {
			return errors.New(constUtil.DailyCopyBattleError)
		}
		seen := map[string]bool{}
		for _, guid := range trans.GetActorUids() {
			canonical := actors[side][guid]
			if canonical == "" || seen[canonical] {
				return errors.New(constUtil.DailyCopyBattleError)
			}
			seen[canonical] = true
		}
	}
	return nil
}

func (s *GameService) dailyCopyGain(player *pb.Player, game *model.GameState, reward string) (*pb.ResultObject, error) {
	gain := &pb.ResultObject{}
	if reward == "" {
		return gain, nil
	}
	for _, part := range strings.Split(reward, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 || !s.dailyCopyResourceSupported(pair[0]) {
			return nil, errors.New(constUtil.DailyCopyConfigError)
		}
		num, err := dailyCopyPositive(pair[1])
		if err != nil {
			return nil, err
		}
		key := pair[0]
		switch {
		case strings.HasPrefix(key, "hero_exp_"):
			if num > constUtil.TreasureBoxMaximumBatch || int64(len(game.Warriors))+int64(num) > int64(player.GetWarriorBagSize()) {
				return nil, errors.New("伙伴背包已满，请先整理经验材料")
			}
			for n := int32(0); n < num; n++ {
				warrior := &pb.Warrior{Id: proto.String(key), Guid: proto.String(uuid.NewString()), Level: proto.Int32(1), Exp: proto.Int64(0), Talent: proto.Int32(0), Training: &pb.Training{}, TempTraining: &pb.Training{}, Breached: &pb.Breached{Level: proto.Int32(1)}, Awake: &pb.Awake{}}
				game.Warriors = append(game.Warriors, warrior)
				gain.Wars = append(gain.Wars, proto.Clone(warrior).(*pb.Warrior))
			}
		case strings.HasPrefix(key, "trea_"), strings.HasPrefix(key, "frag_trea_"):
			if err := s.treasureSourceGain(player, game, gain, key, num); err != nil {
				return nil, err
			}
		default:
			current, err := s.chapterBoxGain(player, game, part)
			if err != nil {
				return nil, err
			}
			gain.Simps = append(gain.Simps, current.GetSimps()...)
		}
	}
	return gain, nil
}

func (s *GameService) dailyCopyEnd(tx *gorm.DB, player *pb.Player, game *model.GameState, request *pb.CSDailyCopyFightEnd, now time.Time) (*pb.SCDailyCopyFightEnd, bool, error) {
	var fight db.DailyCopyFight
	err := tx.Where("fight_id = ? AND player_id = ?", request.GetFightId(), player.GetPlayerId()).First(&fight).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, errors.New(constUtil.DailyCopyBattleError)
	}
	if err != nil {
		return nil, false, err
	}
	if request.GetChapterId() != fight.ChapterID {
		return nil, false, errors.New(constUtil.DailyCopyBattleError)
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, false, err
	}
	if fight.Settled {
		if !bytes.Equal(raw, fight.EndRequest) || len(fight.Response) == 0 {
			return nil, false, errors.New(constUtil.DailyCopyReplayError)
		}
		result := &pb.SCDailyCopyFightEnd{}
		if err := proto.Unmarshal(fight.Response, result); err != nil {
			return nil, false, err
		}
		if err := s.validateP5GrowthReceipt(game, result); err != nil {
			return nil, false, err
		}
		return result, false, nil
	}
	start, err := decodeDailyCopyStart(&fight)
	if err != nil {
		return nil, false, err
	}
	if err := validateDailyCopyEnd(start, request); err != nil {
		return nil, false, err
	}
	var state db.DailyCopyState
	if err := tx.Where("player_id = ? AND day = ? AND type = ?", fight.PlayerID, fight.Day, fight.Type).First(&state).Error; err != nil {
		return nil, false, err
	}
	if state.Used < 1 || state.Limit < state.Used {
		return nil, false, errors.New(constUtil.DailyCopyConfigError)
	}
	result := &pb.SCDailyCopyFightEnd{IsWin: proto.Int32(request.GetIsWin()), Get: &pb.ResultObject{}, Cost: []*pb.BaseResource{{Id: proto.String("stamina"), Num: proto.Int32(fight.Stamina)}}, LastTime: proto.Int32(state.Limit - state.Used), StarNum: proto.Int32(0)}
	if request.GetIsWin() == 1 {
		gain, err := s.dailyCopyGain(player, game, fight.Reward)
		if err != nil {
			return nil, false, err
		}
		result.Get = gain
		result.StarNum = proto.Int32(fight.WinStars)
	}
	response, err := proto.Marshal(result)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Model(&fight).Updates(map[string]any{"settled": true, "end_request": raw, "response": response}).Error; err != nil {
		return nil, false, err
	}
	return result, true, nil
}
