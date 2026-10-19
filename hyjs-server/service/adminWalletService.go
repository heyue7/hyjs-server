package service

import (
	"context"
	"hyjs-server/db"
)

// AdminWallets keeps the legacy role-filtered endpoint while projecting the actual account-owned balance.
func AdminWallets(ctx context.Context, q string, page, size int64) (map[string]any, error) {
	players, err := AdminGamePlayers(ctx, q, page, size)
	if err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	for _, player := range players.Items {
		balance, err := walletReadBalance(db.GetIns().WithContext(ctx), player.ID, false)
		if err != nil {
			return nil, err
		}
		rows = append(rows, map[string]any{"playerId": player.ID, "accountId": player.AccountID, "name": player.Name, "balanceCents": balance.BalanceCents, "revision": balance.Revision})
	}
	return map[string]any{"items": rows, "total": players.Total}, nil
}
