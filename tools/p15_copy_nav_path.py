"""Offline original Copy NavMesh corridor and bounded joystick suggestions.

Run with the existing .analysis-venv/bin/python (UnityPy/Pillow). No device,
database, network, server request, position write or battle operation exists here.
Supports the verified original Unity NavMesh tile layout version16 only.
"""
import argparse
import hashlib
import heapq
import json
import math
from pathlib import Path
import struct
import zipfile

import UnityPy
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
EPS = 0.0001


def digest(path):
    with Path(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def rows(name):
    path = ROOT / 'analysis/tables' / ('Table' + name + '.tsv')
    lines = path.read_text().splitlines()
    fields = lines[1].split('\t')
    return [dict(zip(fields, line.split('\t'))) for line in lines[5:]
            if line.split('\t')[0].strip()]


def decode_scene(apk, scene):
    """Validate original file-backed tile bounds before interpreting polygons."""
    entry = 'assets/android/map/' + scene
    with zipfile.ZipFile(apk) as archive:
        raw = archive.read(entry)
    environment = UnityPy.load(raw)
    candidates = [obj for obj in environment.objects if obj.type.name == 'NavMeshData']
    if len(candidates) != 1:
        raise ValueError('Expected exactly one original NavMeshData in ' + entry)
    tree = candidates[0].read_typetree()
    polygons = []
    for tile_id, tile in enumerate(tree['m_NavMeshTiles']):
        data = bytes(tile['m_MeshData'])
        if len(data) < 72:
            raise ValueError('Original tile header is truncated')
        header = struct.unpack_from('<18I', data)
        count, vertex_count = header[5:7]
        if (header[:2] != (0x444e4156, 16) or not 0 < count < 65536
                or not 3 <= vertex_count < 65536
                or 72 + 12 * vertex_count + 32 * count > len(data)):
            raise ValueError('Unsupported original tile layout; no guessed decode')
        vertices = [struct.unpack_from('<3f', data, 72 + 12 * i)
                    for i in range(vertex_count)]
        if not all(math.isfinite(value) for point in vertices for value in point):
            raise ValueError('Non-finite original NavMesh vertex')
        for index in range(count):
            poly = struct.unpack_from('<12H2I', data, 72 + 12 * vertex_count + 32 * index)
            n = poly[13]
            if not 3 <= n <= 6 or any(v >= vertex_count for v in poly[:n]):
                raise ValueError('Original polygon vertex bounds are invalid')
            polygons.append({'tile': tile_id, 'poly': index,
                             'vertices': [vertices[v] for v in poly[:n]],
                             'neighbors': list(poly[6:6 + n]), 'flags': poly[12]})
    obstacles = [obj.read_typetree() for obj in environment.objects
                 if obj.type.name == 'NavMeshObstacle']
    return polygons, {'apk': str(Path(apk).resolve()), 'apkSHA256': digest(apk),
                      'entry': entry, 'bundleSHA256': hashlib.sha256(raw).hexdigest(),
                      'navMeshPathID': candidates[0].path_id, 'navVersion': 16,
                      'parameters': tree['m_NavMeshParams'], 'tileCount': len(tree['m_NavMeshTiles']),
                      'polygonCount': len(polygons), 'serializedNavMeshObstacleCount': len(obstacles),
                      'layout': '72byte header/float3 vertices/6uint16 vertices+6uint16 neighbors+uint32 flags+uint32 count'}


def center(poly):
    points = poly['vertices']
    return tuple(sum(p[axis] for p in points) / len(points) for axis in range(3))


def cross(a, b):
    return a[0] * b[1] - a[1] * b[0]


def xz(point):
    return point[0], point[2]


def distance(a, b):
    return math.hypot(a[0] - b[0], a[2] - b[2])


def clip_segment(start, end, poly):
    """Exact X/Z half-plane clipping against a convex original polygon."""
    points = [xz(p) for p in poly['vertices']]
    area = sum(cross(points[i], points[(i + 1) % len(points)]) for i in range(len(points)))
    if abs(area) < EPS:
        raise ValueError('Degenerate original polygon')
    winding = 1 if area > 0 else -1
    origin = xz(start)
    direction = (end[0] - start[0], end[2] - start[2])
    lo, hi = 0.0, 1.0
    for i, a in enumerate(points):
        b = points[(i + 1) % len(points)]
        edge = (b[0] - a[0], b[1] - a[1])
        base = winding * cross(edge, (origin[0] - a[0], origin[1] - a[1]))
        slope = winding * cross(edge, direction)
        if abs(slope) < EPS:
            if base < -EPS:
                return None
        elif slope > 0:
            lo = max(lo, (-EPS - base) / slope)
        else:
            hi = min(hi, (-EPS - base) / slope)
        if lo > hi + EPS:
            return None
    return max(0, lo), min(1, hi)


def segment_covered(start, end, polygons):
    intervals = sorted(interval for poly in polygons
                       if (interval := clip_segment(start, end, poly)) is not None)
    covered = 0.0
    for lo, hi in intervals:
        if lo > covered + EPS:
            return False
        covered = max(covered, hi)
    return covered >= 1 - EPS


def locate(point, polygons):
    return [i for i, poly in enumerate(polygons) if poly['flags']
            and clip_segment(point, point, poly) is not None]


def overlap(a, b, c, d, climb):
    """Find a real shared edge/overlapping opposite tile portal, never a corner."""
    dx, dz = b[0] - a[0], b[2] - a[2]
    length = math.hypot(dx, dz)
    if length < EPS:
        return None
    if (abs(cross((dx, dz), (c[0] - a[0], c[2] - a[2]))) / length > EPS
            or abs(cross((dx, dz), (d[0] - a[0], d[2] - a[2]))) / length > EPS):
        return None
    tc = ((c[0] - a[0]) * dx + (c[2] - a[2]) * dz) / length ** 2
    td = ((d[0] - a[0]) * dx + (d[2] - a[2]) * dz) / length ** 2
    low, high = max(0, min(tc, td)), min(1, max(tc, td))
    if (high - low) * length <= EPS or abs(td - tc) < EPS:
        return None
    ends = []
    for t in (low, high):
        q = (t - tc) / (td - tc)
        first_y = a[1] + t * (b[1] - a[1])
        second_y = c[1] + q * (d[1] - c[1])
        if abs(first_y - second_y) > climb + EPS:
            return None
        ends.append((a[0] + dx * t, (first_y + second_y) / 2, a[2] + dz * t))
    return ends


def adjacency(polygons, climb, clearance):
    graph = {i: {} for i in range(len(polygons))}
    by_identity = {(p['tile'], p['poly']): i for i, p in enumerate(polygons)}
    edges = []
    for i, poly in enumerate(polygons):
        if not poly['flags']:
            continue
        points = poly['vertices']
        for j, flag in enumerate(poly['neighbors']):
            if flag:
                edges.append((i, points[j], points[(j + 1) % len(points)], flag))
    for offset, (i, a, b, flag) in enumerate(edges):
        for j, c, d, other_flag in edges[offset + 1:]:
            if i == j:
                continue
            if flag < 0x8000:
                if by_identity.get((polygons[i]['tile'], flag - 1)) != j:
                    continue
                if other_flag != polygons[i]['poly'] + 1:
                    continue
            else:
                # Original external edge directions must be cardinal and opposite.
                side, other_side = flag & 0x7fff, other_flag & 0x7fff
                if (other_flag < 0x8000 or polygons[i]['tile'] == polygons[j]['tile']
                        or side not in (0, 2, 4, 6) or other_side != (side + 4) % 8):
                    continue
            portal = overlap(a, b, c, d, climb)
            if portal is None or distance(*portal) < 2 * clearance + EPS:
                continue
            graph[i][j] = graph[j][i] = portal
    return graph


def route(start, end, polygons, graph):
    starts, goals = locate(start, polygons), locate(end, polygons)
    if not starts or not goals:
        raise ValueError('Start/target is outside original walkable polygons; no implicit snap or teleport')
    costs = {i: distance(start, center(polygons[i])) for i in starts}
    queue = [(cost, i) for i, cost in costs.items()]
    heapq.heapify(queue)
    previous = {}
    while queue:
        cost, i = heapq.heappop(queue)
        if cost > costs[i] + EPS:
            continue
        for j in graph[i]:
            candidate = cost + distance(center(polygons[i]), center(polygons[j]))
            if candidate < costs.get(j, math.inf):
                costs[j], previous[j] = candidate, i
                heapq.heappush(queue, (candidate, j))
    reached = [i for i in goals if i in costs]
    if not reached:
        raise ValueError('No original declared polygon route with the selected portal clearance')
    current = min(reached, key=lambda i: costs[i] + distance(center(polygons[i]), end))
    path = [current]
    while current in previous:
        current = previous[current]
        path.append(current)
    path.reverse()
    portals = [graph[i][j] for i, j in zip(path, path[1:])]
    points = [tuple(start)] + [tuple((a[k] + b[k]) / 2 for k in range(3))
                               for a, b in portals] + [tuple(end)]
    corridor = [polygons[i] for i in path]
    # Every unsmoothed segment joins portals of one convex polygon.
    if not all(segment_covered(a, b, corridor) for a, b in zip(points, points[1:])):
        raise ValueError('Declared route failed exact original polygon coverage')
    heights = [v[1] for poly in corridor for v in poly['vertices']]
    flat = max(heights) - min(heights) < 0.02
    if flat:
        # Only shortcut a single flat original surface; never flatten stacked/sloped paths.
        simplified, i = [points[0]], 0
        while i < len(points) - 1:
            j = len(points) - 1
            while j > i + 1 and not segment_covered(points[i], points[j], corridor):
                j -= 1
            simplified.append(points[j])
            i = j
    else:
        simplified = points
    return {'polygonPath': path, 'portalMidpoints': points[1:-1],
            'turningWaypoints': simplified, 'flatSurfaceShortcut': flat,
            'portalWidths': [distance(*p) for p in portals],
            'pathLength': sum(distance(a, b) for a, b in zip(simplified, simplified[1:])),
            'straightChordCovered': segment_covered(start, end, corridor)}


def joystick_steps(points, camera, base, radius, maximum_step, speed, cap):
    steps = []
    angle = math.radians(camera)
    for a, b in zip(points, points[1:]):
        length = distance(a, b)
        if length < EPS:
            continue
        count = math.ceil(length / maximum_step)
        for i in range(1, count + 1):
            world_dx, world_dz = (b[0] - a[0]) / count, (b[2] - a[2]) / count
            # Inverse of original Euler(camera)*(joystickX,0,joystickY); screen Y points down.
            dx = math.cos(angle) * world_dx - math.sin(angle) * world_dz
            dy = -(math.sin(angle) * world_dx + math.cos(angle) * world_dz)
            magnitude = math.hypot(dx, dy)
            destination = [a[k] + (b[k] - a[k]) * i / count for k in range(3)]
            step_length = length / count
            steps.append({'worldWaypoint': destination, 'plannedMaximumWorldStep': step_length,
                          'screenDirection': [dx / magnitude, dy / magnitude],
                          'joystickFrom': list(base),
                          'joystickTo': [round(base[0] + radius * dx / magnitude),
                                         round(base[1] + radius * dy / magnitude)],
                          'durationMs': max(1, min(cap, round(step_length / speed * 1000))) if speed else min(cap, 350),
                          'durationBasis': 'operator measured speed, bounded; observe actual position again' if speed else 'calibration cap only; not predicted arrival',
                          'afterStep': 'Observe actual UI position/encounter; replan from observed world position. Stop when battle/dialogue starts.'})
    return steps


def draw_route(polygons, routes, targets, output):
    vertices = [v for poly in polygons for v in poly['vertices']]
    minx, maxx = min(v[0] for v in vertices), max(v[0] for v in vertices)
    minz, maxz = min(v[2] for v in vertices), max(v[2] for v in vertices)
    scale = min(18, 1300 / max(maxx - minx, maxz - minz))
    project = lambda p: (round((p[0] - minx) * scale + 45), round((maxz - p[2]) * scale + 45))
    image = Image.new('RGB', (round((maxx - minx) * scale + 100),
                              round((maxz - minz) * scale + 100)), '#141c27')
    draw = ImageDraw.Draw(image)
    for poly in polygons:
        draw.polygon([project(v) for v in poly['vertices']], fill='#314d5d', outline='#688a8c')
    for index, plan in enumerate(routes):
        points = [project(v) for v in plan['turningWaypoints']]
        draw.line(points, fill=['#ffd166', '#06d6a0', '#ef476f', '#a4baff'][index % 4], width=3)
        for point in points:
            draw.ellipse((point[0] - 2, point[1] - 2, point[0] + 2, point[1] + 2), fill='white')
    for target in targets:
        x, y = project(target['position'])
        draw.text((x + 5, y - 8), target['name'], fill='white')
    for z in range(math.floor(minz / 5) * 5, math.ceil(maxz / 5) * 5 + 1, 5):
        draw.text((3, project((0, 0, z))[1]), str(z), fill='white')
    for x in range(math.floor(minx / 5) * 5, math.ceil(maxx / 5) * 5 + 1, 5):
        draw.text((project((x, 0, 0))[0], image.height - 20), str(x), fill='white')
    image.save(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', type=Path, default=ROOT / 'apks/original/10056262.apk')
    parser.add_argument('--mission', required=True)
    parser.add_argument('--start', type=float, nargs=2, metavar=('X', 'Z'),
                        help='Observed current world X/Z; omitted uses original mission spawn')
    parser.add_argument('--start-evidence', type=Path, help='Optional actual coordinate panel capture; never captured by this tool')
    parser.add_argument('--next-task', help='First unfinished original task; no completion is inferred')
    parser.add_argument('--joystick-base', type=int, nargs=2, default=[440, 1000])
    parser.add_argument('--viewport', type=int, nargs=2, default=[2800, 1260])
    parser.add_argument('--stick-radius', type=int, default=180)
    parser.add_argument('--max-world-step', type=float, default=2.5)
    parser.add_argument('--measured-speed', type=float, help='Observed metres/sec, never guessed from duration alone')
    parser.add_argument('--duration-cap-ms', type=int, default=700)
    parser.add_argument('--portal-clearance', type=float, default=0.1,
                        help='Minimum portal half-width; baked walkable radius remains the wall clearance')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    for value in [*(args.start or []), args.max_world_step, args.portal_clearance,
                  *([args.measured_speed] if args.measured_speed is not None else [])]:
        if not math.isfinite(value):
            parser.error('Coordinates and numeric limits must be finite')
    if (not 0 < args.max_world_step <= 5 or not 0 <= args.portal_clearance <= 1
            or not 50 <= args.duration_cap_ms <= 1500 or not 1 <= args.stick_radius <= 300
            or (args.measured_speed is not None and not 0 < args.measured_speed <= 30)):
        parser.error('Use bounded step/clearance/duration/radius and actual measured positive speed')
    width, height = args.viewport
    bx, by = args.joystick_base
    if not (0 <= bx - args.stick_radius and bx + args.stick_radius < width
            and 0 <= by - args.stick_radius and by + args.stick_radius < height):
        parser.error('Joystick base plus radius must fit the observed viewport')
    output = args.output.resolve()
    picture = output.with_suffix('.png')
    if not output.is_relative_to(ROOT / 'analysis') or output.exists() or picture.exists():
        parser.error('Use a new immutable output under project analysis')
    mission = next((row for row in rows('CopyMission') if row['MissionId'] == args.mission), None)
    if mission is None:
        parser.error('Original mission does not exist')
    tasks = [row for row in rows('CopyTask') if row['MissionId'] == args.mission]
    if args.next_task:
        indices = [i for i, row in enumerate(tasks) if row['TaskId'] == args.next_task]
        if len(indices) != 1:
            parser.error('Next task must belong to the original mission')
        tasks = tasks[indices[0]:]
    scene = mission['SceneName']
    if not scene.startswith('map_') or not all(c.isalnum() or c == '_' for c in scene):
        parser.error('Unsupported original scene entry')
    polygons, source = decode_scene(args.apk.resolve(), scene)
    graph = adjacency(polygons, float(source['parameters']['walkableClimb']), args.portal_clearance)
    start = args.start or [float(mission['BornX']), float(mission['BornZ'])]
    point = (start[0], 0, start[1])
    located = locate(point, polygons)
    if not located:
        raise ValueError('Observed start is outside baked walkable polygons; inspect its actual coordinates')
    point = (point[0], center(polygons[located[0]])[1], point[2])
    targets = [{'name': 'start', 'position': point}]
    plans = []
    camera = float(mission['CameraAngle'])
    for task in tasks:
        target = (float(task['BornX']), 0, float(task['BornZ']))
        containing = locate(target, polygons)
        if not containing:
            raise ValueError('Original task birth point has no walkable polygon: ' + task['TaskId'])
        target = (target[0], center(polygons[containing[0]])[1], target[2])
        plan = route(point, target, polygons, graph)
        plan.update(taskId=task['TaskId'], targetOriginalBornPosition=target,
                    targetIsBoss=task['IsBoss'] == '1', targetMonster=task['HeroId'],
                    battleScene=task['SceneName'],
                    joystickSuggestions=joystick_steps(plan['turningWaypoints'], camera,
                        args.joystick_base, args.stick_radius, args.max_world_step,
                        args.measured_speed, args.duration_cap_ms))
        plans.append(plan)
        targets.append({'name': task['TaskId'], 'position': target})
        point = target
    files = ['CopyMission', 'CopyTask', 'StoryQuest', 'Story', 'CopyAirWall']
    result = {'status': 'OFFLINE_ORIGINAL_NAV_PATH_PHONE_EXECUTION_PENDING', 'source': source,
              'toolSHA256': digest(__file__),
              'tableSHA256': {name: digest(ROOT / 'analysis/tables' / ('Table' + name + '.tsv')) for name in files},
              'mission': mission, 'cameraAngle': camera,
              'startBasis': 'operator provided observed X/Z; no automatic device read' if args.start else 'original mission spawn only',
              'startEvidence': {'path': str(args.start_evidence.resolve()), 'sha256': digest(args.start_evidence)} if args.start_evidence else None,
              'portalClearance': args.portal_clearance, 'polygons': polygons,
              'directedLinks': sum(len(v) for v in graph.values()), 'routes': plans,
              'originalStoryQuests': [row for row in rows('StoryQuest') if row['missionId'] == args.mission],
              'originalAirWalls': [row for row in rows('CopyAirWall') if row['MissionId'] == args.mission],
              'visualization': str(picture),
              'limits': ['Offline declared NavMesh polygons/portals only; actual dynamic colliders and actors still require UI observation.',
                         'Only the first leg starts from the supplied observed point; later legs assume original birth centers, not actual post-battle positions. Replan after every actual fight.',
                         'Suggestions never issue ADB/HTTP/SQL or move a player. Duration caps do not prove distance or arrival.',
                         'Stop navigation on dialogue/encounter; two ineffective attempts require position/corridor diagnosis.']}
    output.parent.mkdir(parents=True, exist_ok=True)
    draw_route(polygons, plans, targets, picture)
    with output.open('x') as destination:
        destination.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': result['status'], 'output': str(output), 'polygons': len(polygons),
                      'links': result['directedLinks'],
                      'tasks': [p['taskId'] for p in plans], 'visualization': str(picture)}))


if __name__ == '__main__':
    main()
