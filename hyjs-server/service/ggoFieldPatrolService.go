package service

import (
	"errors"
	"math"

	"hyjs-server/model"
)

// Every observer interpolates the same absolute server-time path. The path is
// traversed forwards and backwards; accepted fights freeze the elapsed time.
func ggoPatrolPosition(node *ggoMonsterNode, point *model.AdminFieldGGOPoint, now int64) (*model.AdminFieldGGOPosition, error) {
	if node == nil || point == nil || point.Position == nil {
		return nil, errors.New("沙漠小怪位置缺失")
	}
	path := point.Patrol
	if path == nil {
		return point.Position, nil
	}
	if len(path.Waypoints) < 2 || node.PatrolEpochMS <= 0 || path.Speed <= 0 {
		return nil, errors.New("沙漠小怪巡逻状态无效")
	}
	at := now
	if node.PatrolPausedAtMS > 0 {
		at = node.PatrolPausedAtMS
	}
	if at < node.PatrolEpochMS {
		at = node.PatrolEpochMS
	}
	segments := make([]float64, len(path.Waypoints)-1)
	var length float64
	for i := range segments {
		a, b := path.Waypoints[i], path.Waypoints[i+1]
		if a == nil || b == nil || a.X == nil || a.Y == nil || a.Z == nil || b.X == nil || b.Y == nil || b.Z == nil {
			return nil, errors.New("沙漠小怪巡逻路径无效")
		}
		dx, dy, dz := *b.X-*a.X, *b.Y-*a.Y, *b.Z-*a.Z
		segments[i] = math.Sqrt(dx*dx + dy*dy + dz*dz)
		if segments[i] < .001 || math.IsNaN(segments[i]) || math.IsInf(segments[i], 0) {
			return nil, errors.New("沙漠小怪巡逻路径无效")
		}
		length += segments[i]
	}
	d := math.Mod(float64(at-node.PatrolEpochMS)*path.Speed/1000, 2*length)
	if d > length {
		d = 2*length - d
	}
	for i, segment := range segments {
		if d <= segment || i == len(segments)-1 {
			a, b := path.Waypoints[i], path.Waypoints[i+1]
			ratio := math.Max(0, math.Min(1, d/segment))
			x, y, z := *a.X+(*b.X-*a.X)*ratio, *a.Y+(*b.Y-*a.Y)*ratio, *a.Z+(*b.Z-*a.Z)*ratio
			return &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}, nil
		}
		d -= segment
	}
	return nil, errors.New("沙漠小怪巡逻位置无效")
}

func ggoPatrolResume(node *ggoMonsterNode, now int64) error {
	if node == nil || node.PatrolPausedAtMS == 0 {
		return nil
	}
	if now < node.PatrolPausedAtMS {
		return errors.New("沙漠小怪巡逻时钟回退")
	}
	pause := now - node.PatrolPausedAtMS
	if node.PatrolEpochMS <= 0 || node.PatrolEpochMS > math.MaxInt64-pause {
		return errors.New("沙漠小怪巡逻计时溢出")
	}
	node.PatrolEpochMS += pause
	node.PatrolPausedAtMS = 0
	return nil
}
