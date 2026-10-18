package service

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// 原报名配置决定同场实际计算输入；热表和当前引擎升级不能替换旧报名版本。
func (s *GameService) p13UFBattleAuditFrozenEngine(tx *gorm.DB, match *db.P13UnionFightMatch, applied map[string][]string) (*GameService, error) {
	if match.EngineVersion != constUtil.P6EngineVersion || match.ClientVersion != constUtil.P6OriginalClientHash {
		return nil, errors.New("原报名引擎/客户端版本与当前P6不一致，不能新算本场战争审计")
	}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, err
	}
	source := fmt.Sprintf("%s;match=%s;matchTablesSHA=%s;engine=%s;client=%s", constUtil.P13UnionFightBattleFrozenSource, match.ID, match.ConfigurationHash, match.EngineVersion, match.ClientVersion)
	for guid := range applied {
		applied[guid] = append(applied[guid], source)
	}
	return original, nil
}
