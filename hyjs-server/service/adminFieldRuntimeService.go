package service

import (
	"context"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
)

// fieldRuntimeForRequest isolates per-realm map overlays from the immutable base combat catalogue.
func (s *GameService) fieldRuntimeForRequest(ctx context.Context, r *model.GamePacket) (*GameService, error) {
	name := model.OpcodeNames[r.Opcode]
	if name == "" && r.Body != nil {
		name = string(r.Body.ProtoReflect().Descriptor().Name())
	}
	// Original 30000..30099 contains scene, mine/fish, PVE/PK and heartbeat consumers.
	relevant := (r.Opcode >= 30000 && r.Opcode < 30100) || strings.Contains(name, "WorldBoss") || strings.Contains(name, "Side") || strings.Contains(name, "Field") || name == "CSGatherCost" || name == "CSGuideMine" || name == "CSGuideFish" || name == "CSPingTcp" || name == "CSTransferScene"

	if !relevant {
		return s, nil
	}
	tables, err := s.AdminFieldRuntimeTables(ctx, s.OriginalServerID())
	if err != nil {
		return nil, err
	}
	return s.withRuntimeTables(tables), nil
}

// withRuntimeTables 只复制运行期只读字段，socket及缓存所有权继续由基础服务持有。
func (s *GameService) withRuntimeTables(tables map[string][]map[string]string) *GameService {
	clone := &GameService{RuntimeBase: s.p10ClientOwner(), ServerID: s.ServerID, Host: s.Host, HTTPPort: s.HTTPPort, TCPPort: s.TCPPort, Tables: tables, AdminCatalogueData: s.AdminCatalogueData, ServerClockReady: s.ServerClockReady, P1Ready: s.P1Ready, P4Ready: s.P4Ready, P5Ready: s.P5Ready, P8GatherReady: s.P8GatherReady, P8LifeReady: s.P8LifeReady, P9PetsReady: s.P9PetsReady, P9IntimacyReady: s.P9IntimacyReady, P10VoiceReady: s.P10VoiceReady, P12WorldBossBattleReady: s.P12WorldBossBattleReady, P12FieldPVPReady: s.P12FieldPVPReady, P12WantedReady: s.P12WantedReady, P3AutoConfirmDemoPayments: s.P3AutoConfirmDemoPayments, P3LocalWalletEnabled: s.P3LocalWalletEnabled, LocalPlayToolsEnabled: s.LocalPlayToolsEnabled}
	clone.ServerClockState.Store(s.ServerClockState.Load())
	return clone
}

// bindFieldRuntime refreshes the context's service pointer after applying a trusted realm overlay.
func bindFieldRuntime(ctx context.Context, s *GameService) context.Context {
	return context.WithValue(ctx, constUtil.GameServiceContextKey, s)
}

// guildRuntimeForRequest 将已发布全局规则接到真实消费者，场景心跳无需重读公会配置。
func (s *GameService) guildRuntimeForRequest(ctx context.Context, r *model.GamePacket) (*GameService, error) {
	if r == nil || r.Body == nil {
		return s, nil
	}
	name := string(r.Body.ProtoReflect().Descriptor().Name())
	if !strings.Contains(name, "Union") && !strings.Contains(name, "Guild") && !strings.Contains(name, "Pray") && !strings.Contains(name, "Fight") && !strings.Contains(name, "Copy") && !strings.Contains(name, "Formation") && !strings.Contains(name, "Warrior") && !strings.Contains(name, "Login") && !strings.Contains(name, "Player") && !strings.Contains(name, "Shop") {
		return s, nil
	}
	tables, err := AdminGuildRuntimeTables(ctx, s.Tables)
	if err != nil {
		return nil, err
	}
	return s.withRuntimeTables(tables), nil
}
