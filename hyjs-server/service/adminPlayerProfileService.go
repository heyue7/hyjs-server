package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// AdminGamePlayerEditOptions 从已加载原表生成表单选项，身份与归属沿当前角色校验。
func (s *GameService) AdminGamePlayerEditOptions(ctx context.Context, id int64) (*model.AdminGamePlayerEditOptionsResponse, error) {
	current, err := AdminGamePlayer(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &model.AdminGamePlayerEditOptionsResponse{
		Chapters:      []model.AdminPlayerChapterOption{},
		Scenes:        []model.AdminPlayerEditOption{{Value: "town_main", Label: "主城"}},
		BalanceLimits: map[string]int64{"coin": 9007199254740991, "diamond": 9007199254740991, "coinpet": 9007199254740991, "energy": math.MaxInt32, "stamina": math.MaxInt32, "light": math.MaxInt32, "point": math.MaxInt32, "popularity": math.MaxInt32, "score": math.MaxInt32, "contribution": math.MaxInt32},
	}
	for _, row := range s.Tables["UserExp"] {
		out.LevelMax = max(out.LevelMax, int64(tableNumber(row, "level")))
	}
	for _, row := range s.Tables["VipPerks"] {
		out.VIPMax = max(out.VIPMax, int64(tableNumber(row, "vip")))
	}
	for _, chapter := range s.Tables["CopyChapter"] {
		entry := model.AdminPlayerChapterOption{Value: chapter["ChapterId"], Label: "第" + chapter["ChapterId"] + "章", Missions: []model.AdminPlayerEditOption{}}
		for _, mission := range s.Tables["CopyMission"] {
			if mission["ChapterId"] == chapter["ChapterId"] {
				entry.Missions = append(entry.Missions, model.AdminPlayerEditOption{Value: mission["MissionId"], Label: "关卡 " + mission["MissionId"]})
			}
		}
		out.Chapters = append(out.Chapters, entry)
	}
	for i, name := range []string{"森林野外", "雪原野外", "沙漠野外", "夜森林野外", "火山野外", "峡谷野外"} {
		key := fmt.Sprintf("field_%03d", i+1)
		if s.fieldSceneRow(key) != nil && s.P8FieldSceneAvailable(key) {
			out.Scenes = append(out.Scenes, model.AdminPlayerEditOption{Value: key, Label: name})
		}
	}
	found := false
	for _, scene := range out.Scenes {
		found = found || scene.Value == current.Scene
	}
	if !found && current.Scene != "" {
		out.Scenes = append(out.Scenes, model.AdminPlayerEditOption{Value: current.Scene, Label: "当前场景 · " + current.Scene})
	}
	return out, nil
}

// adminSetCopyProgress 调整至指定可挑战关卡，保留奖励领取标记，不触发原结算或奖励。
func (s *GameService) adminSetCopyProgress(g *model.GameState, chapter, mission string) {
	s.refreshCopyProgress(g)
	beforeTarget := true
	for _, row := range s.Tables["CopyMission"] {
		if row["MissionId"] == mission {
			beforeTarget = false
		}
		for _, info := range g.Copy.GetPassedCopyInfo() {
			if info.GetChapterId() == row["ChapterId"] && info.GetMissionId() == row["MissionId"] {
				if beforeTarget {
					info.Star = proto.Int32(max(info.GetStar(), 1))
				} else {
					info.Star = proto.Int32(0)
				}
			}
		}
		// 已有剧情完成事实保持；只补齐新进度在原入口需要的剧情资格。
		if beforeTarget || row["MissionId"] == mission {
			if key := row["OpenId"]; key != "" && !slices.Contains(g.Quests, key) {
				g.Quests = append(g.Quests, key)
			}
		}
	}
	s.refreshCopyProgress(g)
	g.Copy.CurrentChapterId, g.Copy.CurrentMissionId = proto.String(chapter), proto.String(mission)
}

// adminPlayerChangeScene 使用已恢复的入口坐标与原野外状态切换，不覆盖进行中的玩法。
func (s *GameService) adminPlayerChangeScene(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, scene string) error {
	if (g.Run != nil && !g.Run.Complete) || (g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled) {
		return &constUtil.AdminPlayerEditError{Message: "请先结束当前副本或野外战斗，再修改场景"}
	}
	current := g.Position.GetScene()
	if current != "town" && current != "town_main" && s.fieldSceneRow(current) == nil {
		return &constUtil.AdminPlayerEditError{Message: "请先退出当前特殊场景，再修改场景"}
	}
	if s.P12FieldPVPReady {
		if err := CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
			return &constUtil.AdminPlayerEditError{Message: "请先完成当前野外状态恢复，再修改场景"}
		}
	}
	if s.P8GatherReady {
		if err := s.P8CancelGather(tx, id, g); err != nil {
			return err
		}
	}
	point := &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}
	if scene != "town" && scene != "town_main" {
		row := s.fieldSceneRow(scene)
		if row == nil || !s.P8FieldSceneAvailable(scene) || int64(p.GetLevel()) < int64(tableNumber(row, "levelLimit")) {
			return &constUtil.AdminPlayerEditError{Message: "该场景尚未开放或角色等级不足"}
		}
		if err := s.fieldSelectScene(g, scene); err != nil {
			return err
		}
		var err error
		point, err = fieldPoint(row["Resurrection"])
		if err != nil {
			return err
		}
		g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(scene), Current: point}
		if err := s.refreshField(id, g); err != nil {
			return err
		}
	}
	g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(scene), Current: point}
	g.PositionLoading = true
	return nil
}

// AdminUpdateGamePlayerProfile 在同一版本事务保存资料、资源和进度；所有统计由存档计算。
func (s *GameService) AdminUpdateGamePlayerProfile(ctx context.Context, actor *model.AdminPrincipal, id int64, req model.AdminGamePlayerProfileRequest) (*model.AdminGamePlayerDTO, error) {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if adminPlayerVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		beforeP := proto.Clone(p).(*pb.Player)
		beforeG := &model.GameState{}
		gameJSON, err := json.Marshal(g)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(gameJSON, beforeG); err != nil {
			return err
		}
		progressChanged := req.Chapter != g.Copy.GetCurrentChapterId() || req.Mission != g.Copy.GetCurrentMissionId()
		if progressChanged && g.Run != nil && !g.Run.Complete {
			return &constUtil.AdminPlayerEditError{Message: "请先退出当前副本，再修改主线进度"}
		}
		if p.GetName() != req.Name {
			state, err := readPlayerRenameState(p, g)
			if err != nil {
				return err
			}
			if state.Count > 0 {
				state.AdminName = req.Name
				data, err := json.Marshal(state)
				if err != nil {
					return err
				}
				p5SetKV(g, constUtil.PlayerRenameStateKey, string(data))
			}
		}
		p.Name, p.Level, p.Exp, p.Vip = proto.String(req.Name), proto.Int32(int32(req.Level)), proto.Int64(req.Exp), proto.Int32(int32(req.VIP))
		p.Coin, p.Diamond, p.Coinpet = proto.Int64(req.Balances["coin"]), proto.Int64(req.Balances["diamond"]), proto.Int64(req.Balances["coinpet"])
		p.Energy, p.Stamina = proto.Int32(int32(req.Balances["energy"])), proto.Int32(int32(req.Balances["stamina"]))
		p.Light, p.Point = proto.Int32(int32(req.Balances["light"])), proto.Int32(int32(req.Balances["point"]))
		p.Popularity, p.Score, p.Contribution = proto.Int32(int32(req.Balances["popularity"])), proto.Int32(int32(req.Balances["score"])), proto.Int32(int32(req.Balances["contribution"]))
		// 行政设置以提交时为恢复周期起点，避免旧游标立即覆盖指定余额。
		if p.GetEnergy() != beforeP.GetEnergy() {
			p.EnergyLastRefreshTime = proto.Int64(time.Now().UnixMilli())
		}
		if p.GetStamina() != beforeP.GetStamina() {
			p.StaminaLastRefreshTime = proto.Int64(time.Now().UnixMilli())
		}
		if progressChanged {
			s.adminSetCopyProgress(g, req.Chapter, req.Mission)
		}
		if req.Scene != g.Position.GetScene() {
			if err := s.adminPlayerChangeScene(tx, id, p, g, req.Scene); err != nil {
				return err
			}
		}
		previousCreatedAt := row.CreatedAt
		row.CreatedAt = &req.CreatedAt
		if req.Level != int64(beforeP.GetLevel()) && s.P5Ready {
			power, err := s.formationFightPower(p, g)
			if err != nil {
				return err
			}
			p.FightPower = proto.Int32(power)
		}
		if err := adminSavePlayerChanges(tx, &row, beforeP, beforeG, p, g); err != nil {
			return err
		}
		// 审计仅保存运营表单前后值，不复制密码、会话或完整背包。
		before := model.AdminGamePlayerProfileRequest{Name: beforeP.GetName(), Level: int64(beforeP.GetLevel()), Exp: beforeP.GetExp(), VIP: int64(beforeP.GetVip()), Chapter: beforeG.Copy.GetCurrentChapterId(), Mission: beforeG.Copy.GetCurrentMissionId(), Scene: beforeG.Position.GetScene(), Balances: map[string]int64{"coin": beforeP.GetCoin(), "diamond": beforeP.GetDiamond(), "coinpet": beforeP.GetCoinpet(), "energy": int64(beforeP.GetEnergy()), "stamina": int64(beforeP.GetStamina()), "light": int64(beforeP.GetLight()), "point": int64(beforeP.GetPoint()), "popularity": int64(beforeP.GetPopularity()), "score": int64(beforeP.GetScore()), "contribution": int64(beforeP.GetContribution())}}
		if previousCreatedAt != nil {
			before.CreatedAt = *previousCreatedAt
		}
		req.Version = ""
		detail := struct {
			ServerID      int64 `json:"serverId"`
			Before, After model.AdminGamePlayerProfileRequest
		}{AdminProfileServerID(row), before, req}
		return adminAudit(tx, actor, "player.profile", strconv.FormatInt(id, 10), string(adminJSON(detail)))
	})
	if err != nil {
		return nil, err
	}
	return AdminGamePlayer(ctx, id)
}
