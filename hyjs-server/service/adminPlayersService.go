package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminPlayerVersion(row db.Profile) string {
	return adminDigest(string(adminJSON(struct {
		Player, Game *string
		Session      string
	}{row.Player, row.Game, hex.EncodeToString(row.Session)})))
}
func adminPlayerProjection(row db.Profile, details bool) (model.AdminGamePlayerDTO, error) {
	p, g, err := decodeProfile(&row)
	if err != nil {
		return model.AdminGamePlayerDTO{}, err
	}
	out := model.AdminGamePlayerDTO{ID: row.Id, ServerID: AdminProfileServerID(row), Name: p.GetName(), Level: int64(p.GetLevel()), Vip: int64(p.GetVip()), Chapter: g.Copy.GetCurrentChapterId(), Mission: g.Copy.GetCurrentMissionId(), Scene: g.Position.GetScene(), Version: adminPlayerVersion(row), Balances: map[string]int64{"coin": p.GetCoin(), "diamond": int64(p.GetDiamond()), "energy": int64(p.GetEnergy()), "stamina": int64(p.GetStamina()), "light": int64(p.GetLight()), "point": int64(p.GetPoint()), "popularity": int64(p.GetPopularity()), "score": int64(p.GetScore())}, Counts: map[string]int64{"characters": int64(len(g.Warriors)), "equipment": int64(len(g.Equipment)), "accessories": int64(len(g.Treasures)), "items": int64(len(g.Items)), "gems": int64(len(g.Gems)), "cooking": int64(len(g.Cooking))}}
	if row.CreatedAt != nil {
		out.CreatedAt = *row.CreatedAt
	}
	if details {
		out.Inventory = []model.AdminPlayerInventoryItem{}
		for _, w := range g.Warriors {
			out.Inventory = append(out.Inventory, model.AdminPlayerInventoryItem{ResourceID: w.GetId(), Amount: 1, Guid: w.GetGuid(), Level: int64(w.GetLevel())})
		}
		for _, e := range g.Equipment {
			out.Inventory = append(out.Inventory, model.AdminPlayerInventoryItem{ResourceID: e.GetEquipId(), Amount: 1, Guid: e.GetGuid(), Level: int64(e.GetLevel())})
		}
		for _, e := range g.Treasures {
			out.Inventory = append(out.Inventory, model.AdminPlayerInventoryItem{ResourceID: e.GetTreasureId(), Amount: 1, Guid: e.GetGuid(), Level: int64(e.GetStrengthLevel())})
		}
		for _, group := range []any{g.Items, g.Gems, g.Cooking} {
			encoded, _ := json.Marshal(group)
			var items []struct {
				ID  string `json:"id"`
				Num int64  `json:"num"`
			}
			_ = json.Unmarshal(encoded, &items)
			for _, item := range items {
				out.Inventory = append(out.Inventory, model.AdminPlayerInventoryItem{ResourceID: item.ID, Amount: item.Num})
			}
		}
		if len(out.Inventory) > 2000 {
			out.Inventory = out.Inventory[:2000]
		}
	}
	return out, nil
}

// AdminGamePlayers 分页查询昵称及ID，只选择业务存档和版本所需列。
func AdminGamePlayers(ctx context.Context, q string, page, size int64) (*model.AdminGamePlayersResponse, error) {
	query := db.GetIns().WithContext(ctx).Model(&db.Profile{}).Where("player IS NOT NULL AND game IS NOT NULL")
	if serverID := AdminSelectedServerID(ctx); serverID > 0 {
		query = query.Where("server_id=?", serverID)
	}
	if q != "" {
		expression := "json_extract(player, '$.name')"
		if query.Dialector.Name() == "mysql" {
			expression = "JSON_UNQUOTE(JSON_EXTRACT(player, '$.name'))"
		}
		id, _ := strconv.ParseInt(q, 10, 64)
		query = query.Where("(id=? OR "+expression+" LIKE ?)", id, "%"+q+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []db.Profile
	if err := query.Select("id", "server_id", "player", "game", "session", "created_at").Order("id DESC").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &model.AdminGamePlayersResponse{Items: []model.AdminGamePlayerDTO{}, Total: total}
	for _, row := range rows {
		dto, err := adminPlayerProjection(row, false)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, dto)
	}
	return out, nil
}

// AdminGamePlayer 读取一名游戏角色的有限运营详情。
func AdminGamePlayer(ctx context.Context, id int64) (*model.AdminGamePlayerDTO, error) {
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).Select("id", "server_id", "player", "game", "session", "created_at").First(&row, id).Error; err != nil {
		return nil, err
	}
	if err := AdminCheckProfileScope(ctx, row); err != nil {
		return nil, err
	}
	dto, err := adminPlayerProjection(row, true)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

// AdminRenameGamePlayer 保留原付费改名计数和旧回执，用独立行政覆盖防止旧请求复活。
func AdminRenameGamePlayer(ctx context.Context, p *model.AdminPrincipal, id int64, req model.AdminGamePlayerNameRequest) (*model.AdminGamePlayerDTO, error) {
	if !playerRenameNameValid(req.Name) || strings.TrimSpace(req.Reason) == "" {
		return nil, constUtil.ErrAdminInput
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
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
		player, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		state, err := readPlayerRenameState(player, game)
		if err != nil {
			return err
		}
		oldName := player.GetName()
		if oldName == req.Name {
			return constUtil.ErrAdminInput
		}
		player.Name = proto.String(req.Name)
		if state.Count > 0 {
			state.AdminName = req.Name
			encoded, err := json.Marshal(state)
			if err != nil {
				return err
			}
			p5SetKV(game, constUtil.PlayerRenameStateKey, string(encoded))
		}
		playerJSON, err := json.Marshal(player)
		if err != nil {
			return err
		}
		gameJSON, err := json.Marshal(game)
		if err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{"player": string(playerJSON), "game": string(gameJSON)}).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "player.rename", strconv.FormatInt(id, 10), oldName+" -> "+req.Name+"; "+req.Reason)
	})
	if err != nil {
		return nil, err
	}
	return AdminGamePlayer(ctx, id)
}

// AdminKickGamePlayer 轮换原16字节会话，不改战斗、资源、密码或玩家进度。
func AdminKickGamePlayer(ctx context.Context, p *model.AdminPrincipal, id int64, req model.AdminGamePlayerKickRequest) (*model.AdminGamePlayerDTO, error) {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
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
		session := make([]byte, 16)
		if _, err := rand.Read(session); err != nil {
			return err
		}
		if err := tx.Model(&row).Update("session", session).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "player.kick", strconv.FormatInt(id, 10), req.Reason)
	})
	if err != nil {
		return nil, err
	}
	return AdminGamePlayer(ctx, id)
}
