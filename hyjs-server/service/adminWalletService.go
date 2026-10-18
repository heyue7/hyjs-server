package service

import (
	"context"
	"hyjs-server/db"
)

// AdminWallets 展示绑定角色的余额，未入账角色余额为0。
func AdminWallets(ctx context.Context, q string, page, size int64) (map[string]any, error) {
	players, err := AdminGamePlayers(ctx, q, page, size)
	if err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	ids := []int64{}
	for _, p := range players.Items {
		ids = append(ids, p.ID)
	}
	var balances []db.LocalWalletBalance
	if len(ids) > 0 {
		if err = db.GetIns().WithContext(ctx).Where("player_id IN ?", ids).Find(&balances).Error; err != nil {
			return nil, err
		}
	}
	byID := map[int64]db.LocalWalletBalance{}
	for _, b := range balances {
		byID[b.PlayerID] = b
	}
	for _, p := range players.Items {
		b := byID[p.ID]
		rows = append(rows, map[string]any{"playerId": p.ID, "name": p.Name, "balanceCents": b.BalanceCents, "revision": b.Revision})
	}
	return map[string]any{"items": rows, "total": players.Total}, nil
}
