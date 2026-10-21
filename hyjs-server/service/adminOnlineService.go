package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AdminOnlinePlayers 按当前TCP连接快照核验会话和账号，统计口径不依赖城镇场景。
func (s *GameService) AdminOnlinePlayers(ctx context.Context, principal *model.AdminPrincipal, q string, page, size int64) (*model.AdminOnlinePlayersResponse, error) {
	d := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(d, principal, false); err != nil {
		return nil, err
	}
	out := &model.AdminOnlinePlayersResponse{Servers: []model.AdminOnlineServer{}, Items: []model.AdminOnlinePlayer{}, ObservedAt: time.Now().UnixMilli()}
	sessions := map[int64][]byte{}
	owner := s.p10ClientOwner()
	// 拷贝当前连接会话，后续查询与分页使用同一份快照。
	p10ClientSinks.Range(func(key, value any) bool {
		k := key.(p10ClientKey)
		if k.service == owner {
			sessions[k.playerID] = append([]byte(nil), value.(*p10ClientSink).session...)
		}
		return true
	})
	var servers []db.AdminGameServer
	if err := d.Select("id", "name").Order("id").Find(&servers).Error; err != nil {
		return nil, err
	}
	indexes := map[int64]int{}
	for _, row := range servers {
		indexes[row.ID] = len(out.Servers)
		out.Servers = append(out.Servers, model.AdminOnlineServer{ServerID: row.ID, Name: row.Name, Connected: row.ID == s.OriginalServerID()})
	}
	ids := make([]int64, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var online []model.AdminOnlinePlayer
	// 分块批量读取有限字段，账号校验同样批量执行，避免逐角色查询。
	for start := 0; start < len(ids); start += 400 {
		end := start + 400
		if end > len(ids) {
			end = len(ids)
		}
		var rows []db.Profile
		if err := d.Select("id", "server_id", "account_id", "account_auth_revision", "player", "session").Where("id IN ?", ids[start:end]).Order("id").Find(&rows).Error; err != nil {
			return nil, err
		}
		accountIDs := []int64{}
		for _, row := range rows {
			if row.AccountID > 0 {
				accountIDs = append(accountIDs, row.AccountID)
			}
		}
		accounts := map[int64]db.GameAccount{}
		if len(accountIDs) > 0 {
			var values []db.GameAccount
			if err := db.AccountsTable(d, "game_accounts").Select("id", "enabled", "auth_revision").Where("id IN ?", accountIDs).Find(&values).Error; err != nil {
				return nil, err
			}
			for _, account := range values {
				accounts[account.ID] = account
			}
		}
		for _, row := range rows {
			serverID := AdminProfileServerID(row)
			if serverID != s.OriginalServerID() || row.Player == nil || len(sessions[row.Id]) != 16 || len(row.Session) != 16 || subtle.ConstantTimeCompare(sessions[row.Id], row.Session) != 1 {
				continue
			}
			if row.AccountID > 0 {
				account, ok := accounts[row.AccountID]
				if !ok || !account.Enabled || account.AuthRevision != row.AccountAuthRevision {
					continue
				}
			}
			var player pb.Player
			if err := json.Unmarshal([]byte(*row.Player), &player); err != nil {
				return nil, err
			}
			if index, ok := indexes[serverID]; ok {
				out.Servers[index].Online++
			}
			if selected := AdminSelectedServerID(ctx); selected > 0 && selected != serverID {
				continue
			}
			if q != "" && strconv.FormatInt(row.Id, 10) != q && !strings.Contains(player.GetName(), q) {
				continue
			}
			online = append(online, model.AdminOnlinePlayer{ID: row.Id, ServerID: serverID, Name: player.GetName(), Level: int64(player.GetLevel()), VIP: int64(player.GetVip())})
		}
	}
	out.Total = int64(len(online))
	start := (page - 1) * size
	if start < out.Total {
		end := start + size
		if end > out.Total {
			end = out.Total
		}
		out.Items = append(out.Items, online[start:end]...)
	}
	return out, nil
}
