package service

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

// InitializeOpenServerActivity 将APK原任务保存到SQL，仅补齐缺失记录并保留运营修改。
func (s *GameService) InitializeOpenServerActivity(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.OpenServerActivityRule{}, &db.OpenServerActivityTask{}, &db.OpenServerActivityState{}, &db.OpenServerActivityCounter{}, &db.OpenServerActivityReceipt{}, &db.OpeningCalendarCarry{}); err != nil {
		return err
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		zone := time.FixedZone("UTC+8", 8*3600)
		start := time.Date(2026, 10, 11, 0, 0, 0, 0, zone)
		cfg := db.OpenServerActivityRule{ID: constUtil.OpenServerActivityID, Enabled: true, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 0, 14).UnixMilli(), RewardEndAt: start.AddDate(0, 0, 15).UnixMilli(), Source: constUtil.OpenServerActivitySource}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
		if len(s.Tables["ActivityNewOpen7Day"]) != 249 {
			return errors.New("开服活动APK249任务表未完整加载")
		}
		for _, row := range s.Tables["ActivityNewOpen7Day"] {
			parse := func(key string, fallback int64) (int64, error) {
				if row[key] == "" {
					return fallback, nil
				}
				return strconv.ParseInt(row[key], 10, 32)
			}
			id, e := parse("id", 0)
			if e != nil {
				return e
			}
			title, e := parse("titleId", 0)
			if e != nil {
				return e
			}
			kind, e := parse("taskType", 0)
			if e != nil {
				return e
			}
			tap, e := parse("tap", 0)
			if e != nil {
				return e
			}
			day, e := parse("openDay", 0)
			if e != nil {
				return e
			}
			target, e := parse("num", 1)
			if e != nil {
				return e
			}
			price, e := parse("newPrice", 0)
			if e != nil {
				return e
			}
			task := db.OpenServerActivityTask{ID: int32(id), TitleID: int32(title), TaskType: int32(kind), Tap: int32(tap), OpenDay: int32(day), Target: int32(target), Visible: row["isVisable"] == "1", NewPrice: price, Reward: strings.TrimSuffix(strings.TrimSpace(row["reward"]), ";"), Source: constUtil.OpenServerActivitySource}
			if err := validateOpenServerTask(task); err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&task).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func loadOpenServerRule(tx *gorm.DB) (*db.OpenServerActivityRule, error) {
	if scoped, err := localPlayOpenServerRule(tx); err != nil || scoped != nil {
		return scoped, err
	}
	var cfg db.OpenServerActivityRule
	if err := tx.First(&cfg, "id = ?", constUtil.OpenServerActivityID).Error; err != nil {
		return nil, err
	}
	local := time.UnixMilli(cfg.StartAt).In(time.FixedZone("UTC+8", 8*3600))
	if local.Hour() != 0 || local.Minute() != 0 || local.Second() != 0 || local.Nanosecond() != 0 || cfg.EndAt <= cfg.StartAt || cfg.EndAt-cfg.StartAt > 366*24*3600000 || cfg.RewardEndAt < cfg.EndAt || cfg.RewardEndAt-cfg.EndAt > 90*24*3600000 {
		return nil, errors.New("开服活动SQL时间窗无效")
	}
	return &cfg, nil
}

func validateOpenServerTask(t db.OpenServerActivityTask) error {
	if t.ID <= 0 || t.TitleID <= 0 || t.OpenDay < 1 || t.OpenDay > 14 || t.Tap < 1 || t.Tap > 4 || t.Target < 1 || t.NewPrice < 0 || t.UsedStock < 0 || t.UsedStock > t.Target {
		return errors.New("开服任务SQL配置无效")
	}
	if openServerPurchase(t.TaskType) && t.NewPrice <= 0 {
		return errors.New("开服购买SQL价格无效")
	}
	if t.Reward == "" {
		return errors.New("开服任务奖励缺失")
	}
	return nil
}

// openServerClientCompatible 原协议不下发奖励/价格明细，领取前确保SQL与当前APK固定展示一致。
func (s *GameService) openServerClientCompatible(t db.OpenServerActivityTask) bool {
	row := s.table("ActivityNewOpen7Day", "id", strconv.FormatInt(int64(t.ID), 10))
	if row == nil {
		return false
	}
	price := int64(0)
	if row["newPrice"] != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(row["newPrice"]), 10, 64)
		if err != nil {
			return false
		}
		price = n
	}
	// 原表尾分号只是分隔符；语义资源、数量、顺序与价格均必须一致。
	normalize := func(value string) string { return strings.TrimRight(strings.TrimSpace(value), ";") }
	return t.NewPrice == price && normalize(t.Reward) == normalize(row["reward"])
}

func openServerPurchase(kind int32) bool {
	return kind == 7 || kind == 107 || kind == 108 || kind == 109
}
func openServerChoice(kind int32) bool { return kind == 28 || kind == 100 || kind == 107 }
func openServerDay(cfg *db.OpenServerActivityRule, now time.Time) int32 {
	// Unlocks follow realm natural days; participation expiry keeps the role's
	// own StartAt/EndAt, so joining late never loses configured duration.
	start := cfg.StartAt
	if cfg.UnlockStartAt > 0 {
		start = cfg.UnlockStartAt
	}
	if now.UnixMilli() < start {
		return 0
	}
	day := int32((now.UnixMilli()-start)/(24*3600000)) + 1
	if day > 14 {
		day = 14
	}
	return day
}
func openServerTaskDate(cfg *db.OpenServerActivityRule, day int32) string {
	return time.UnixMilli(cfg.StartAt).AddDate(0, 0, int(day)-1).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
}

// RecordOpenServerCounter 由角色行锁下的成功业务事务调用，查询接口从不增加计数。
func RecordOpenServerCounter(tx *gorm.DB, id int64, key string, delta int32) error {
	return recordOpenServerCounterAt(tx, id, key, delta, LocalPlayNow(tx.Statement.Context))
}
func recordOpenServerCounterAt(tx *gorm.DB, id int64, key string, delta int32, now time.Time) error {
	if delta < 1 {
		return errors.New("开服事件增量无效")
	}
	// 只接受已恢复业务的明确键，防止错把未恢复玩法标记为达成。
	valid := key == constUtil.OpenServerActivityCounterComposeTreasure || key == constUtil.OpenServerActivityCounterComposePurple || key == constUtil.OpenServerActivityCounterComposeOrange || key == constUtil.OpenServerActivityCounterBuyAttackToken
	for _, prefix := range []string{constUtil.OpenServerActivityCounterLogin, constUtil.OpenServerActivityCounterCopyWin, constUtil.OpenServerActivityCounterPredatory} {
		if strings.HasPrefix(key, prefix) {
			date := strings.TrimPrefix(key, prefix)
			parsed, e := time.Parse("2006-01-02", date)
			valid = e == nil && parsed.Format("2006-01-02") == date && date == now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
		}
	}
	if !valid {
		return errors.New("开服事件键无效")
	}
	cfg, e := loadPlayerOpenServerRule(tx, id)
	if e != nil {
		return e
	}
	if !cfg.Enabled || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
		return nil
	}
	if strings.HasPrefix(key, constUtil.OpenServerActivityCounterLogin) {
		if err := openingCalendarCarryLogin(tx, id, cfg, strings.TrimPrefix(key, constUtil.OpenServerActivityCounterLogin)); err != nil {
			return err
		}
	}
	counter, e := openServerReadCounter(tx, id, key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		counter = db.OpenServerActivityCounter{PlayerID: id, CounterKey: key}
	} else if e != nil {
		return e
	}
	if strings.HasPrefix(key, constUtil.OpenServerActivityCounterLogin) {
		if counter.Value < 0 || counter.Value > 1 {
			return errors.New("开服登录日期计数无效")
		}
		if counter.Value == 1 {
			return nil
		}
		delta = 1
	}
	if counter.Value < 0 || counter.Value > math.MaxInt32-delta {
		return errors.New("开服事件计数溢出")
	}
	counter.Value += delta
	return openServerSaveCounter(tx, &counter)
}

// RecordOpenServerLogin 只在真实角色登录成功事务中记录UTC+8日期，重连不增加。
func RecordOpenServerLogin(tx *gorm.DB, id int64) error {
	now := LocalPlayNow(tx.Statement.Context)
	return recordOpenServerCounterAt(tx, id, constUtil.OpenServerActivityCounterLogin+now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02"), 1, now)
}

// RefreshOpenServerProgress 由存档事务调用，活动结束后停止评估当前状态。
func (s *GameService) RefreshOpenServerProgress(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
	return s.refreshOpenServerProgressAt(tx, player, g, LocalPlayNow(tx.Statement.Context))
}
func (s *GameService) refreshOpenServerProgressAt(tx *gorm.DB, player *pb.Player, g *model.GameState, now time.Time) error {
	cfg, e := loadPlayerOpenServerRule(tx, player.GetPlayerId())
	if e != nil {
		return e
	}
	if !cfg.Enabled || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
		return nil
	}
	// Recheck committed payment facts when opening the page as well as paying;
	// orders confirmed before this policy must satisfy the same original tasks.
	if e := s.refreshP3OpenServerCounters(tx, player.GetPlayerId(), cfg, now); e != nil {
		return e
	}
	tasks, e := openServerReadTasks(tx, openServerDay(cfg, now))
	if e != nil {
		return e
	}
	counters, e := openServerReadCounters(tx, player.GetPlayerId())
	if e != nil {
		return e
	}
	counters, e = openingCalendarCarryCounters(tx, player.GetPlayerId(), cfg, counters)
	if e != nil {
		return e
	}
	values := map[string]int32{}
	for _, c := range counters {
		if c.Value < 0 {
			return errors.New("开服计数存档无效")
		}
		values[c.CounterKey] = c.Value
	}
	states, e := openServerReadStates(tx, player.GetPlayerId())
	if e != nil {
		return e
	}
	saved := map[int32]db.OpenServerActivityState{}
	for _, state := range states {
		saved[state.TaskID] = state
	}
	var bestArenaRank int64
	arenaRankRead := false
	// 原第六天任务按历史最高星数达标，重置挑战不会清除已取得的星数纪录。
	// 只在当前已开放且未达成的试炼任务存在时读一次；不创建轮次或逐档回放历史战斗。
	for _, task := range tasks {
		if task.Visible && task.TaskType == 24 && !saved[task.ID].Achieved {
			if !gameSchemaHasTable(tx, &db.P7RushState{}) {
				break
			}
			var rush db.P7RushState
			err := tx.Select("total_stars", "record_stars").Where("player_id=?", player.GetPlayerId()).First(&rush).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if rush.TotalStars < 0 || rush.RecordStars < 0 {
				return errors.New("试炼星数存档无效")
			}
			values[constUtil.OpenServerActivityCounterRushStars] = max(rush.TotalStars, rush.RecordStars)
			break
		}
	}
	for _, t := range tasks {
		if e := validateOpenServerTask(t); e != nil {
			return e
		}
		if !t.Visible || openServerPurchase(t.TaskType) {
			continue
		}
		state := saved[t.ID]
		if state.Claims < 0 || state.Progress < 0 {
			return errors.New("开服任务存档无效")
		}
		if state.Claims > 0 || state.Achieved && t.TaskType != 2 && t.TaskType != 101 {
			continue
		}
		current, e := s.openServerCurrent(player, g, cfg, t, values)
		if t.TaskType == 8 {
			if !arenaRankRead {
				bestArenaRank, _, e = P11ArenaVerifiedBestWonRank(tx, player.GetPlayerId())
				arenaRankRead = true
			}
			if bestArenaRank > math.MaxInt32 {
				return errors.New("竞技场历史排名超出活动范围")
			}
			current = int32(bestArenaRank)
		}
		if e != nil {
			return e
		}
		if current < 0 {
			return errors.New("开服任务进度无效")
		}
		achieved := current >= t.Target
		if t.TaskType == 8 {
			achieved = current > 0 && current <= t.Target
		}
		if state.Progress != current || state.Achieved != achieved {
			state.PlayerID = player.GetPlayerId()
			state.TaskID = t.ID
			state.Progress = current
			state.Achieved = achieved
			if e := openServerSaveState(tx, &state); e != nil {
				return e
			}
		}
	}
	return nil
}

// openServerCurrent 从真实存档或事务事件读取条件，未恢复玩法一律返回未达成。
func (s *GameService) openServerCurrent(player *pb.Player, g *model.GameState, cfg *db.OpenServerActivityRule, t db.OpenServerActivityTask, values map[string]int32) (int32, error) {
	date := openServerTaskDate(cfg, t.OpenDay)
	value := func(prefix string, sum bool) int32 {
		if (t.OpenDay < 8 || t.OpenDay > 14) && t.TaskType != 28 {
			return values[prefix+date]
		}
		// Second-week tasks and the day-seven character choice accept actual
		// events throughout this period. Other first-week dates stay intact.
		var current int64
		for key, n := range values {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			at, err := time.ParseInLocation("2006-01-02", strings.TrimPrefix(key, prefix), time.FixedZone("UTC+8", 8*3600))
			if err != nil || at.AddDate(0, 0, 1).UnixMilli() <= cfg.StartAt || at.UnixMilli() >= cfg.EndAt {
				continue
			}
			if sum {
				current = min(int64(math.MaxInt32), current+int64(n))
			} else {
				current = max(current, int64(n))
			}
		}
		return int32(current)
	}
	switch t.TaskType {
	case 2:
		return values[constUtil.P3OpenServerTotalPaid], nil
	case 101:
		return value(constUtil.P3OpenServerSinglePaid, false), nil
	case 1, 28, 100:
		return value(constUtil.OpenServerActivityCounterLogin, false), nil
	case 4:
		return player.GetLevel(), nil
	case 3:
		// 仅所有原关卡都已有正星结算的章节计为通关，当前章节游标无法证明完成。
		var maxChapter int32
		for _, chapter := range s.Tables["CopyChapter"] {
			total, passed := 0, 0
			for _, mission := range s.Tables["CopyMission"] {
				if mission["ChapterId"] != chapter["ChapterId"] {
					continue
				}
				total++
				for _, done := range g.Copy.GetPassedCopyInfo() {
					if done.GetChapterId() == chapter["ChapterId"] && done.GetMissionId() == mission["MissionId"] && done.GetStar() > 0 {
						passed++
						break
					}
				}
			}
			n, e := strconv.ParseInt(chapter["ChapterSequence"], 10, 32)
			if e == nil && total > 0 && total == passed && int32(n) > maxChapter {
				maxChapter = int32(n)
			}
		}
		return maxChapter, nil
	case 9:
		return values[constUtil.OpenServerActivityCounterComposeTreasure], nil
	case 10:
		return values[constUtil.OpenServerActivityCounterComposePurple], nil
	case 11:
		return values[constUtil.OpenServerActivityCounterComposeOrange], nil
	case 18:
		return values[constUtil.OpenServerActivityCounterBuyAttackToken], nil
	case 102:
		return value(constUtil.OpenServerActivityCounterCopyWin, true), nil
	case 106:
		return value(constUtil.OpenServerActivityCounterPredatory, true), nil
	case 24:
		return values[constUtil.OpenServerActivityCounterRushStars], nil
	case 27:
		return s.formationFightPower(player, g)
	case 5, 6, 14, 15, 16, 17, 25, 26:
		return s.openServerFormationCurrent(g, t.TaskType)
	default:
		return 0, nil
	}
}

func (s *GameService) openServerFormationCurrent(g *model.GameState, kind int32) (int32, error) {
	var slots []*pb.FormationItem
	for _, slot := range g.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() == 0 {
				slots = append(slots, slot)
				break
			}
		}
	}
	if len(slots) == 0 || ((kind == 5 || kind == 6) && len(slots) < 5) {
		return 0, nil
	}
	min, max := int32(math.MaxInt32), int32(0)
	for _, slot := range slots {
		positions := map[int32]bool{}
		if kind == 16 || kind == 17 {
			found := false
			for _, thing := range slot.GetThings() {
				if thing.GetPos() != 0 {
					continue
				}
				for _, w := range g.Warriors {
					if w.GetGuid() == thing.GetGuid() && w.GetId() == thing.GetId() {
						n := w.GetBreached().GetLevel()
						if n < min {
							min = n
						}
						if n > max {
							max = n
						}
						found = true
					}
				}
			}
			if !found {
				return 0, errors.New("开服任务上阵角色实例缺失")
			}
			continue
		}
		for _, thing := range slot.GetThings() {
			pos := thing.GetPos()
			if kind == 25 || kind == 26 {
				if pos != 5 && pos != 6 {
					continue
				}
				for _, trea := range g.Treasures {
					if trea.GetGuid() == thing.GetGuid() && trea.GetTreasureId() == thing.GetId() {
						n := trea.GetRefineLevel()
						if n < min {
							min = n
						}
						if n > max {
							max = n
						}
						positions[pos] = true
						break
					}
				}
			} else {
				if pos < 1 || pos > 4 {
					continue
				}
				for _, equip := range g.Equipment {
					if equip.GetGuid() == thing.GetGuid() && equip.GetEquipId() == thing.GetId() {
						n := equip.GetRefineLv()
						if kind == 5 {
							n = int32(tableNumber(s.table("Equip", "id", equip.GetEquipId()), "quality"))
						}
						if kind == 6 {
							n = equip.GetLevel()
						}
						if n < min {
							min = n
						}
						if n > max {
							max = n
						}
						positions[pos] = true
						break
					}
				}
			}
		}
		if kind == 5 || kind == 6 || kind == 14 {
			if len(positions) != 4 {
				return 0, nil
			}
		}
		if kind == 25 && len(positions) != 2 {
			return 0, nil
		}
	}
	if kind == 15 || kind == 17 || kind == 26 {
		return max, nil
	}
	if min == math.MaxInt32 {
		return 0, nil
	}
	return min, nil
}

// openServerActivity 保持原57017/19/21消息形状，角色与全服限量库存同事务提交。
func (s *GameService) openServerActivity(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, error) {
	return s.openServerActivityAt(ctx, id, request, stamp, LocalPlayNow(ctx))
}
func (s *GameService) openServerActivityAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var result proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; e != nil {
			return e
		}
		player, g, e := decodeProfileForCalendarUpdate(tx, &row)
		if e != nil {
			return e
		}
		if e = s.PrepareP5GuildPrayerProperties(ctx, tx, player, g, p13UFNow(ctx)); e != nil {
			return e
		}
		var wire []byte
		if req, ok := request.(*pb.CSGetOpenServerReward); ok {
			if stamp < 1 || req.GetItemId() < 1 {
				return errors.New("开服领取请求无效")
			}
			wire, e = proto.Marshal(req)
			if e != nil {
				return e
			}
			receipt, e := openServerReadReceipt(tx, id, stamp)
			if e == nil {
				if !bytes.Equal(receipt.Request, wire) {
					return errors.New("开服领取时间戳冲突")
				}
				reply := &pb.SCGetOpenServerReward{}
				if e := proto.Unmarshal(receipt.Response, reply); e != nil {
					return e
				}
				if e := s.ValidateCopyAwardReceiptCurrent(tx, id, g, &pb.SCCopyFightEnd{Gain: reply.GetGet()}); e != nil {
					return e
				}
				result = reply
				return nil
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		cfg, e := loadPlayerOpenServerRule(tx, id)
		if e != nil {
			return e
		}
		if !cfg.Enabled || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.RewardEndAt {
			return errors.New("开服活动未开放或领取已截止")
		}
		if e := s.refreshOpenServerProgressAt(tx, player, g, now); e != nil {
			return e
		}
		tasks, e := openServerReadTasks(tx, 0)
		if e != nil {
			return e
		}
		states, e := openServerReadStates(tx, id)
		if e != nil {
			return e
		}
		saved := map[int32]db.OpenServerActivityState{}
		for _, state := range states {
			saved[state.TaskID] = state
		}
		currentDay := openServerDay(cfg, now)
		switch req := request.(type) {
		case *pb.CSOpenServerActivity:
			if req.GetDayType() != 7 && req.GetDayType() != 14 {
				return errors.New("开服活动dayType必须7或14")
			}
			var days []int32
			seen := map[int32]bool{}
			for _, task := range tasks {
				if (req.GetDayType() == 7 && task.OpenDay > 7) || (req.GetDayType() == 14 && task.OpenDay < 8) {
					continue
				}
				item, e := s.openServerItem(cfg, task, saved[task.ID], currentDay, now)
				if e != nil {
					return e
				}
				if item.GetState() == 1 && !seen[task.OpenDay] {
					days = append(days, task.OpenDay)
					seen[task.OpenDay] = true
				}
			}
			result = &pb.SCOpenServerActivity{ActivityEndTime: proto.Int64(cfg.EndAt), GetRewardEndTime: proto.Int64(cfg.RewardEndAt), CurrentDay: proto.Int32(min(currentDay, req.GetDayType())), Days: days}
		case *pb.CSNewOpenSevenDayByTitle:
			if req.GetDay() < 1 || req.GetDay() > currentDay || req.GetTap() < 1 || req.GetTap() > 4 {
				return errors.New("开服活动日期或页签尚未开放")
			}
			var list []*pb.OpenServer7DayItem
			var taps []int32
			seen := map[int32]bool{}
			for _, task := range tasks {
				if task.OpenDay != req.GetDay() {
					continue
				}
				item, e := s.openServerItem(cfg, task, saved[task.ID], currentDay, now)
				if e != nil {
					return e
				}
				if task.Tap == req.GetTap() {
					list = append(list, item)
				}
				if item.GetState() == 1 && !seen[task.Tap] {
					taps = append(taps, task.Tap)
					seen[task.Tap] = true
				}
			}
			result = &pb.SCNewOpenSevenDayByTitle{ItemList: list, RedTap: taps}
		case *pb.CSGetOpenServerReward:
			count, e := model.GamePurchaseCount(req)
			if e != nil {
				return e
			}
			task, e := openServerReadTask(tx, req.GetItemId())
			if e != nil {
				return e
			}
			state := saved[task.ID]
			if !s.openServerClientCompatible(task) {
				return errors.New("开服奖励或价格与APK固定展示不一致，请同步客户端原表配置")
			}
			item, e := s.openServerItem(cfg, task, state, currentDay, now)
			if e != nil {
				return e
			}
			if item.GetState() != 1 {
				return errors.New("开服任务未达成、已领取或库存不足")
			}
			if count > 1 && task.TaskType != 107 && task.TaskType != 108 {
				return errors.New("该开服奖励每次限购一份")
			}
			if (task.TaskType == 107 || task.TaskType == 108) && count > task.Target-state.Claims {
				return errors.New("开服剩余购买次数不足")
			}
			content := task.Reward
			if openServerChoice(task.TaskType) {
				selected := ""
				for _, part := range strings.Split(content, ";") {
					pair := strings.Split(part, ",")
					if len(pair) == 2 && pair[0] == req.GetHeroId() {
						selected = part
						break
					}
				}
				if selected == "" {
					return errors.New("开服选奖励ID无效")
				}
				content = selected
			} else if req.GetHeroId() != "" {
				return errors.New("当前开服任务不接受选奖励ID")
			}
			totalPrice := task.NewPrice * int64(count)
			if openServerPurchase(task.TaskType) {
				if task.NewPrice < 0 || task.NewPrice > math.MaxInt32/int64(count) || player.GetDiamond() < totalPrice {
					return errors.New("开服抢购钻石不足或SQL价格超限")
				}
				player.Diamond = proto.Int64(player.GetDiamond() - totalPrice)
				// 实际总价在本次购买事务记账，后续返奖/保存失败与回执一起回滚。
				if totalPrice > 0 {
					source := "opening:purchase:" + strconv.FormatInt(id, 10) + ":" + strconv.FormatInt(stamp, 10) + ":" + strconv.Itoa(int(task.ID))
					if e = s.RecordP1Cost(tx, id, source, []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(int32(totalPrice))}}, now); e != nil {
						return e
					}
				}
			}
			if count > 1 {
				content, e = p4ScaleRewards(content, count)
				if e != nil {
					return e
				}
			}
			gain, e := s.openServerGain(player, g, content)
			if e != nil {
				return e
			}
			if openServerPurchase(task.TaskType) {
				gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String("diamond"), Num: proto.Int32(-int32(totalPrice))})
			}
			state.PlayerID = id
			state.TaskID = task.ID
			state.Claims += count
			if e := openServerSaveState(tx, &state); e != nil {
				return e
			}
			if task.TaskType == 7 || task.TaskType == 109 {
				task.UsedStock++
				if e := openServerSaveStock(tx, task, id); e != nil {
					return e
				}
			}
			if e := saveProfile(tx, &row, player, g); e != nil {
				return e
			}
			next, e := s.openServerItem(cfg, task, state, currentDay, now)
			if e != nil {
				return e
			}
			result = &pb.SCGetOpenServerReward{Get: gain, NewItemState: proto.Int32(next.GetState())}
			response, e := proto.Marshal(result)
			if e != nil {
				return e
			}
			return openServerCreateReceipt(tx, &db.OpenServerActivityReceipt{PlayerID: id, Timestamp: stamp, Request: wire, Response: response})
		default:
			return errors.New("开服活动请求类型无效")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

func (s *GameService) openServerItem(cfg *db.OpenServerActivityRule, t db.OpenServerActivityTask, state db.OpenServerActivityState, currentDay int32, now time.Time) (*pb.OpenServer7DayItem, error) {
	if e := validateOpenServerTask(t); e != nil {
		return nil, e
	}
	if state.Claims < 0 || state.Progress < 0 {
		return nil, errors.New("开服任务存档无效")
	}
	status, current := int32(0), state.Progress
	limit := int32(1)
	remaining := current
	if t.TaskType == 107 || t.TaskType == 108 {
		limit = t.Target
		// 原 HalfPriceBg 用原表总量减 currentNum 计算剩余；此字段承载已购数。
		current = state.Claims
		remaining = t.Target - state.Claims
	}
	if t.TaskType == 7 || t.TaskType == 109 {
		current = t.Target - t.UsedStock
		remaining = current
	}
	if state.Claims > limit {
		return nil, errors.New("开服已领取数量超过SQL上限")
	}
	supported, e := s.openServerRewardSupported(t.Reward)
	if e != nil {
		return nil, e
	}
	if t.Visible && t.OpenDay <= currentDay {
		if state.Claims >= limit {
			status = 2
		} else if !supported || !s.openServerClientCompatible(t) {
			// SQL漂移阻止新领取，已提交领取的显示状态继续保持。
			status = 0
		} else if openServerPurchase(t.TaskType) {
			if now.UnixMilli() < cfg.EndAt && remaining > 0 {
				status = 1
			}
		} else if state.Achieved {
			status = 1
		}
	}
	visible := int32(0)
	if t.Visible {
		visible = 1
	}
	return &pb.OpenServer7DayItem{ItemId: proto.Int32(t.ID), CurrentNum: proto.Int32(current), State: proto.Int32(status), IsVisable: proto.Int32(visible)}, nil
}

func (s *GameService) openServerRewardSupported(content string) (bool, error) {
	supported := true
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return false, errors.New("开服奖励SQL格式无效")
		}
		n, e := strconv.ParseInt(pair[1], 10, 32)
		if e != nil || n < 1 {
			return false, errors.New("开服奖励SQL数量无效")
		}
		key := pair[0]
		if (strings.HasPrefix(key, "hero_") || strings.HasPrefix(key, "equip_") || strings.HasPrefix(key, "trea_")) && n > constUtil.OpenServerActivityMaximumInstances {
			return false, errors.New("开服奖励实例数量超过本地服安全上限100")
		}
		switch key {
		case "coin", "diamond", "light", "point", "score", "popularity", "energy", "stamina", "contribution":
			continue
		}
		switch {
		case strings.HasPrefix(key, "item_"):
			supported = supported && s.table("Item", "itemId", key) != nil
		case strings.HasPrefix(key, "frag_"):
			supported = supported && s.table("Frag", "id", key) != nil
		case strings.HasPrefix(key, "equip_"):
			supported = supported && s.table("Equip", "id", key) != nil
		case strings.HasPrefix(key, "trea_"):
			supported = supported && s.table("Treasure", "id", key) != nil
		case strings.HasPrefix(key, "hero_"):
			supported = supported && s.table("Hero", "id", key) != nil
		case strings.HasPrefix(key, "MineItem_"):
			supported = supported && s.table("MineSkill", "MineItemID", key) != nil
		case strings.HasPrefix(key, "FishItem_"):
			supported = supported && s.table("FishSkill", "FishItemID", key) != nil
		default:
			supported = false
		}
	}
	return supported, nil
}

// openServerGain 统一固定奖励解析，装备与饰品共享容量，经验材料保留Hero实例。
func (s *GameService) openServerGain(player *pb.Player, g *model.GameState, content string) (*pb.ResultObject, error) {
	supported, e := s.openServerRewardSupported(content)
	if e != nil {
		return nil, e
	}
	if !supported {
		return nil, errors.New("开服奖励资源尚未恢复")
	}
	gain := &pb.ResultObject{}
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		key := pair[0]
		n64, _ := strconv.ParseInt(pair[1], 10, 32)
		n := int32(n64)
		switch {
		case strings.HasPrefix(key, "MineItem_"):
			// 原 AddSimpleItem 按前缀更新宝石库存；奖励与开箱共用真实堆叠校验。
			if err := s.gemInventoryDelta(player, g, map[string]int64{key: n64}); err != nil {
				return nil, err
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(n)})
		case strings.HasPrefix(key, "FishItem_"):
			// 食材和成品沿原 FishSkill ID 投影到料理库存，保留原制作/使用资格。
			if err := s.cookingInventoryDelta(player, g, map[string]int64{key: n64}); err != nil {
				return nil, err
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(n)})
		case key == "coin" || key == "diamond" || strings.HasPrefix(key, "item_") || strings.HasPrefix(key, "equip_"):
			reward, e := s.chapterBoxGain(player, g, part)
			if e != nil {
				return nil, e
			}
			gain.Simps = append(gain.Simps, reward.Simps...)
			gain.Equips = append(gain.Equips, reward.Equips...)
		case strings.HasPrefix(key, "trea_") || strings.HasPrefix(key, "frag_trea_"):
			if e := s.treasureSourceGain(player, g, gain, key, n); e != nil {
				return nil, e
			}
		case strings.HasPrefix(key, "hero_"):
			if n > constUtil.OpenServerActivityMaximumInstances || int64(len(g.Warriors))+int64(n) > int64(player.GetWarriorBagSize()) {
				return nil, errors.New("开服角色或经验材料背包已满")
			}
			for i := int32(0); i < n; i++ {
				w := &pb.Warrior{Id: proto.String(key), Guid: proto.String(uuid.NewString()), Level: proto.Int32(1), Exp: proto.Int64(0), Talent: proto.Int32(0), Training: &pb.Training{}, TempTraining: &pb.Training{}, Breached: &pb.Breached{Level: proto.Int32(1)}, Awake: &pb.Awake{}}
				g.Warriors = append(g.Warriors, w)
				gain.Wars = append(gain.Wars, proto.Clone(w).(*pb.Warrior))
			}
		case strings.HasPrefix(key, "frag_"):
			var owned *pb.SimpleItem
			for _, item := range g.Items {
				if item.GetId() == key {
					if owned != nil {
						return nil, errors.New("开服碎片存档重复")
					}
					owned = item
				}
			}
			if owned == nil {
				owned = &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(0)}
				g.Items = append(g.Items, owned)
			}
			if owned.GetNum() < 0 || int64(owned.GetNum())+n64 > math.MaxInt32 {
				return nil, errors.New("开服碎片奖励溢出")
			}
			owned.Num = proto.Int32(owned.GetNum() + n)
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(n)})
		default:
			var balance *int32
			switch key {
			case "contribution":
				balance = player.Contribution
			case "light":
				balance = player.Light
			case "point":
				balance = player.Point
			case "score":
				balance = player.Score
			case "popularity":
				balance = player.Popularity
			case "energy":
				balance = player.Energy
			case "stamina":
				balance = player.Stamina
			}
			value := int32(0)
			if balance != nil {
				value = *balance
			}
			if value < 0 || int64(value)+n64 > math.MaxInt32 {
				return nil, errors.New("开服基础资源奖励溢出")
			}
			value += n
			switch key {
			case "contribution":
				player.Contribution = proto.Int32(value)
			case "light":
				player.Light = proto.Int32(value)
			case "point":
				player.Point = proto.Int32(value)
			case "score":
				player.Score = proto.Int32(value)
			case "popularity":
				player.Popularity = proto.Int32(value)
			case "energy":
				player.Energy = proto.Int32(value)
			case "stamina":
				player.Stamina = proto.Int32(value)
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(n)})
		}
	}
	return gain, nil
}

// openServerOpenStates 仅恢复已有真实固定日历依据的两个原开服活动入口。
func (s *GameService) openServerOpenStates(ctx context.Context, id int64) ([]*pb.OpenState, error) {
	return s.openServerOpenStatesAt(ctx, id, LocalPlayNow(ctx))
}
func (s *GameService) openServerOpenStatesAt(ctx context.Context, id int64, now time.Time) ([]*pb.OpenState, error) {
	d := db.GetIns().WithContext(ctx)
	var profile db.Profile
	if err := d.First(&profile, id).Error; err != nil {
		return nil, err
	}
	if _, _, err := decodeProfileForUpdate(&profile); err != nil {
		return nil, err
	}
	cfg, err := loadPlayerOpenServerRule(d, id)
	if err != nil {
		return nil, err
	}
	seven, fourteen := int32(0), int32(0)
	if cfg.Enabled && now.UnixMilli() >= cfg.StartAt && now.UnixMilli() < cfg.RewardEndAt {
		// Late joiners retain the first seven-day entrance as well as every
		// later unlocked tier for their full individual participation window.
		day := openServerDay(cfg, now)
		if day > 0 {
			seven = 1
		}
		if day >= 8 {
			fourteen = 1
		}
	}
	states := []*pb.OpenState{{Type: proto.String(constUtil.OpenServerActivitySevenDayType), IsOpen: proto.Int32(seven)}, {Type: proto.String(constUtil.OpenServerActivityFourteenDayType), IsOpen: proto.Int32(fourteen)}}
	// 原主城依据benefitGiftActivity显隐按钮；只展示当前本人实际可买的冻结商品。
	if s.P4Ready {
		benefit, err := s.BenefitGiftOpenState(ctx, id, now)
		if err != nil {
			return nil, err
		}
		states = append(states, benefit)
	}
	king, err := s.kingRecruitOpenState(ctx, id, now)
	if err != nil {
		return nil, err
	}
	states = append(states, king)
	return states, nil
}
