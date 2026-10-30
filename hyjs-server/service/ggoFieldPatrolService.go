package service

import (
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"sync"

	"hyjs-server/model"
)

//go:embed data/ggo_monster_patrol_routes.json
var ggoMonsterNavRouteData []byte

var (
	ggoMonsterNavRoutesOnce sync.Once
	ggoMonsterNavRoutes     map[string][]*model.AdminFieldGGOPosition
	ggoMonsterNavRoutesErr  error
)

// These routes are derived from the released Unity bundle and match only the
// recorded point positions. A changed administrator position never inherits
// a route validated for a different location.
func ggoMonsterNavRoute(point *model.AdminFieldGGOPoint) ([]*model.AdminFieldGGOPosition, error) {
	ggoMonsterNavRoutesOnce.Do(func() {
		var data struct {
			Schema string `json:"schema"`
			Routes []struct {
				ID        string                         `json:"id"`
				Waypoints []*model.AdminFieldGGOPosition `json:"waypoints"`
			} `json:"routes"`
		}
		if err := json.Unmarshal(ggoMonsterNavRouteData, &data); err != nil || data.Schema != "hyjs-desert-monster-nav-routes-v1" || len(data.Routes) != 20 {
			ggoMonsterNavRoutesErr = errors.New("沙漠小怪导航数据无效")
			return
		}
		routes := make(map[string][]*model.AdminFieldGGOPosition, len(data.Routes))
		for _, route := range data.Routes {
			if route.ID == "" || routes[route.ID] != nil || len(route.Waypoints) < 2 || len(route.Waypoints) > 32 {
				ggoMonsterNavRoutesErr = errors.New("沙漠小怪导航路径无效")
				return
			}
			var length float64
			for i, p := range route.Waypoints {
				if p == nil || p.X == nil || p.Y == nil || p.Z == nil || math.IsNaN(*p.X) || math.IsNaN(*p.Y) || math.IsNaN(*p.Z) || math.IsInf(*p.X, 0) || math.IsInf(*p.Y, 0) || math.IsInf(*p.Z, 0) {
					ggoMonsterNavRoutesErr = errors.New("沙漠小怪导航坐标无效")
					return
				}
				if i > 0 {
					prev := route.Waypoints[i-1]
					dx, dy, dz := *p.X-*prev.X, *p.Y-*prev.Y, *p.Z-*prev.Z
					d := math.Sqrt(dx*dx + dy*dy + dz*dz)
					if d < .001 {
						ggoMonsterNavRoutesErr = errors.New("沙漠小怪导航路径无效")
						return
					}
					length += d
				}
			}
			if math.Abs(length-1) > .0001 {
				ggoMonsterNavRoutesErr = errors.New("沙漠小怪导航路径长度无效")
				return
			}
			routes[route.ID] = route.Waypoints
		}
		ggoMonsterNavRoutes = routes
	})
	if ggoMonsterNavRoutesErr != nil {
		return nil, ggoMonsterNavRoutesErr
	}
	route := ggoMonsterNavRoutes[point.ID]
	if len(route) == 0 || point.Position.X == nil || point.Position.Y == nil || point.Position.Z == nil {
		return nil, nil
	}
	start := route[0]
	if math.Abs(*start.X-*point.Position.X) > .0001 || math.Abs(*start.Y-*point.Position.Y) > .0001 || math.Abs(*start.Z-*point.Position.Z) > .0001 {
		return nil, nil
	}
	return route, nil
}

// ggoPatrolPath uses a configured path or its verified fallback, capped at one
// metre. The snapshot sends the same effective route to every line observer.
func ggoPatrolPath(point *model.AdminFieldGGOPoint) (*model.AdminFieldGGOPatrol, error) {
	if point == nil || point.Position == nil {
		return nil, errors.New("沙漠小怪位置缺失")
	}
	var waypoints []*model.AdminFieldGGOPosition
	if point.Patrol != nil {
		waypoints = point.Patrol.Waypoints
	} else {
		var err error
		waypoints, err = ggoMonsterNavRoute(point)
		if err != nil || len(waypoints) == 0 {
			return nil, err
		}
	}
	if len(waypoints) < 2 || len(waypoints) > 32 {
		return nil, errors.New("沙漠小怪巡逻路径无效")
	}
	result := []*model.AdminFieldGGOPosition{waypoints[0]}
	remaining := 1.0
	for i := 1; i < len(waypoints) && remaining > .0000001; i++ {
		a, b := waypoints[i-1], waypoints[i]
		if a == nil || b == nil || a.X == nil || a.Y == nil || a.Z == nil || b.X == nil || b.Y == nil || b.Z == nil {
			return nil, errors.New("沙漠小怪巡逻路径无效")
		}
		dx, dy, dz := *b.X-*a.X, *b.Y-*a.Y, *b.Z-*a.Z
		length := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if length < .001 || math.IsNaN(length) || math.IsInf(length, 0) {
			return nil, errors.New("沙漠小怪巡逻路径无效")
		}
		if length <= remaining {
			result = append(result, b)
			remaining -= length
			continue
		}
		if remaining < .001 {
			break
		}
		ratio := remaining / length
		x, y, z := *a.X+dx*ratio, *a.Y+dy*ratio, *a.Z+dz*ratio
		result = append(result, &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z})
		remaining = 0
	}
	if len(result) < 2 {
		return nil, errors.New("沙漠小怪巡逻路径无效")
	}
	return &model.AdminFieldGGOPatrol{Waypoints: result, Speed: .8}, nil
}

// Every observer uses this absolute server clock: pause at each end, then walk
// the short segment. An accepted fight freezes this clock until its release.
func ggoPatrolPosition(node *ggoMonsterNode, point *model.AdminFieldGGOPoint, now int64) (*model.AdminFieldGGOPosition, error) {
	if node == nil || point == nil || point.Position == nil {
		return nil, errors.New("沙漠小怪位置缺失")
	}
	path, err := ggoPatrolPath(point)
	if err != nil {
		return nil, err
	}
	if path == nil {
		return point.Position, nil
	}
	if node.PatrolEpochMS <= 0 {
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
		dx, dy, dz := *b.X-*a.X, *b.Y-*a.Y, *b.Z-*a.Z
		segments[i] = math.Sqrt(dx*dx + dy*dy + dz*dz)
		length += segments[i]
	}
	travelMS := length / path.Speed * 1000
	const waitMS = 12000.0
	phase := math.Mod(float64(at-node.PatrolEpochMS), 2*(waitMS+travelMS))
	var ratio float64
	switch {
	case phase < waitMS:
		ratio = 0
	case phase < waitMS+travelMS:
		ratio = (phase - waitMS) / travelMS
	case phase < 2*waitMS+travelMS:
		ratio = 1
	default:
		ratio = 1 - (phase-2*waitMS-travelMS)/travelMS
	}
	distance := ratio * length
	for i, segment := range segments {
		if distance <= segment || i == len(segments)-1 {
			a, b := path.Waypoints[i], path.Waypoints[i+1]
			step := math.Max(0, math.Min(1, distance/segment))
			x, y, z := *a.X+(*b.X-*a.X)*step, *a.Y+(*b.Y-*a.Y)*step, *a.Z+(*b.Z-*a.Z)*step
			return &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}, nil
		}
		distance -= segment
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
