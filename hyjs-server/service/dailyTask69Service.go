package service

import "hyjs-server/constUtil"

func dailyTask69DefaultReward(kind int32) string {
	switch kind {
	case constUtil.DailyTaskRecruit, constUtil.DailyTaskEquipRefine:
		return "diamond,40"
	case constUtil.DailyTaskTreasureStrengthen, constUtil.DailyTaskCombine:
		return "diamond,30"
	default:
		return "diamond,20"
	}
}
