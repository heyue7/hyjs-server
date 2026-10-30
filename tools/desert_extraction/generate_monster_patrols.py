"""Derive bounded monster paths from the exact released Unity 5.3 NavMesh.

This tool reads a saved configuration and a bundle only. It does not connect
to a server or operate a device. Its output contains no other configuration.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import struct

import UnityPy

BUNDLE_SHA = "2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1"
CONFIG_SHA = "c85d8cf3fe62c106ce437c21a1e2fd87e187704dc9c5951aa24dffdc55f38a44"
# Unity emits single-precision projected positions; M12 differs from its baked
# edge by less than one millionth of a world unit after serialization.
EPS = 1e-5


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def cross(a, b):
    return a[0] * b[1] - a[1] * b[0]


def clip(start, end, vertices):
    # Convex X/Z clipping; the original NavMesh polygons and detail triangles
    # are the only authority for covered space.
    xy = [(v[0], v[2]) for v in vertices]
    area = sum(cross(xy[i], xy[(i + 1) % len(xy)]) for i in range(len(xy)))
    if abs(area) < EPS:
        return None
    sign = 1 if area > 0 else -1
    origin = (start[0], start[2])
    delta = (end[0] - start[0], end[2] - start[2])
    lo, hi = 0.0, 1.0
    for i, a in enumerate(xy):
        b = xy[(i + 1) % len(xy)]
        edge = (b[0] - a[0], b[1] - a[1])
        base = sign * cross(edge, (origin[0] - a[0], origin[1] - a[1]))
        slope = sign * cross(edge, delta)
        if abs(slope) < EPS:
            if base < -EPS:
                return None
        elif slope > 0:
            lo = max(lo, (-EPS - base) / slope)
        else:
            hi = min(hi, (-EPS - base) / slope)
        if lo > hi + EPS:
            return None
    return max(0.0, lo), min(1.0, hi)


def height(triangle, x, z):
    a, b, c = triangle
    den = (b[2] - c[2]) * (a[0] - c[0]) + (c[0] - b[0]) * (a[2] - c[2])
    if abs(den) < EPS:
        return None
    u = ((b[2] - c[2]) * (x - c[0]) + (c[0] - b[0]) * (z - c[2])) / den
    v = ((c[2] - a[2]) * (x - c[0]) + (a[0] - c[0]) * (z - c[2])) / den
    return u * a[1] + v * b[1] + (1 - u - v) * c[1]


def decode(bundle):
    env = UnityPy.load(str(bundle))
    objs = [o for o in env.objects if o.type.name == "NavMeshData"]
    if len(objs) != 1:
        raise ValueError("Expected one NavMeshData")
    tree = objs[0].read_typetree()
    polygons = []
    for tile_id, tile in enumerate(tree["m_NavMeshTiles"]):
        data = bytes(tile["m_MeshData"])
        if len(data) < 72:
            raise ValueError("Truncated tile")
        header = struct.unpack_from("<18I", data)
        count, vertex_count, detail_count, detail_vertices, detail_triangles, bv_count = header[5:11]
        if header[:2] != (0x444E4156, 16) or detail_count != count or not 0 < count < 65536 or not 3 <= vertex_count < 65536:
            raise ValueError("Unsupported NavMesh tile")
        poly_at = 72 + 12 * vertex_count
        detail_at = poly_at + 32 * count
        detail_vertices_at = detail_at + 12 * detail_count
        triangles_at = detail_vertices_at + 12 * detail_vertices
        if triangles_at + 8 * detail_triangles + 16 * bv_count != len(data):
            raise ValueError("Unexpected tile detail layout")
        vertices = [struct.unpack_from("<3f", data, 72 + 12 * i) for i in range(vertex_count)]
        extra = [struct.unpack_from("<3f", data, detail_vertices_at + 12 * i) for i in range(detail_vertices)]
        if not all(math.isfinite(value) for p in vertices + extra for value in p):
            raise ValueError("Non-finite tile vertex")
        for i in range(count):
            poly = struct.unpack_from("<12H2I", data, poly_at + 32 * i)
            n = poly[13]
            if not 3 <= n <= 6 or any(v >= vertex_count for v in poly[:n]):
                raise ValueError("Invalid polygon")
            outline = [vertices[v] for v in poly[:n]]
            vertex_base, tri_base, vertex_n, tri_n = struct.unpack_from("<IIHH", data, detail_at + 12 * i)
            if vertex_base + vertex_n > detail_vertices or tri_base + tri_n > detail_triangles:
                raise ValueError("Invalid detail range")
            points = outline + extra[vertex_base:vertex_base + vertex_n]
            triangles = []
            for j in range(tri_base, tri_base + tri_n):
                indices = struct.unpack_from("<4H", data, triangles_at + j * 8)[:3]
                if any(k >= len(points) for k in indices):
                    raise ValueError("Invalid detail triangle")
                triangles.append([points[k] for k in indices])
            polygons.append(dict(tile=tile_id, poly=i, flags=poly[12], vertices=outline,
                                 neighbors=list(poly[6:6 + n]), triangles=triangles))
    return polygons, tree["m_NavMeshParams"], objs[0].path_id


def profile(start, end, polygon):
    if (cover := clip(start, end, polygon["vertices"])) is None or cover[0] > 1e-7 or cover[1] < 1 - 1e-7:
        return None
    intervals = []
    breaks = [0.0, 1.0]
    for triangle in polygon["triangles"]:
        interval = clip(start, end, triangle)
        if interval is not None and interval[1] - interval[0] > 1e-7:
            intervals.append((interval[0], interval[1], triangle))
            breaks.extend(interval)
    breaks.sort()
    distinct = []
    for value in breaks:
        if not distinct or value - distinct[-1] > 1e-4:
            distinct.append(value)
        elif distinct[-1] > 0 and value < 1:
            # Adjacent triangle clips overlap at the floating-point tolerance.
            distinct[-1] = (distinct[-1] + value) / 2
    if not distinct or distinct[0] > 1e-6 or distinct[-1] < 1 - 1e-6:
        return None
    points = []
    for lo, hi in zip(distinct, distinct[1:]):
        mid = (lo + hi) / 2
        covering = [tri for left, right, tri in intervals if left - 1e-6 <= mid <= right + 1e-6]
        if not covering:
            return None
        triangle = covering[0]
        segment = []
        for t in (lo, hi):
            x = start[0] + (end[0] - start[0]) * t
            z = start[2] + (end[2] - start[2]) * t
            y = height(triangle, x, z)
            if y is None:
                return None
            segment.append((x, y, z))
        if points and abs(points[-1][1] - segment[0][1]) > .002:
            return None
        if not points:
            points.append(segment[0])
        points.append(segment[1])
    if len(points) < 2 or abs(points[0][1] - start[1]) > .002:
        return None
    points[0] = start
    return points


def distance(a, b):
    return math.sqrt(sum((x - y) ** 2 for x, y in zip(a, b)))


def boundary_distance(point, polygon):
    result = math.inf
    vertices = polygon["vertices"]
    for i, flag in enumerate(polygon["neighbors"]):
        if flag:
            continue
        a, b = vertices[i], vertices[(i + 1) % len(vertices)]
        dx, dz = b[0] - a[0], b[2] - a[2]
        length = dx * dx + dz * dz
        if length == 0:
            continue
        ratio = max(0, min(1, ((point[0] - a[0]) * dx + (point[2] - a[2]) * dz) / length))
        result = min(result, math.hypot(point[0] - a[0] - ratio * dx, point[2] - a[2] - ratio * dz))
    return result if math.isfinite(result) else 1000.0


def choose(start, polygons):
    candidates = []
    for poly in polygons:
        if not poly["flags"] or clip(start, start, poly["vertices"]) is None:
            continue
        for i in range(128):
            angle = 2 * math.pi * i / 128
            end = (start[0] + math.cos(angle), start[1], start[2] + math.sin(angle))
            path = profile(start, end, poly)
            if path is None:
                continue
            remaining = 1.0
            result = [start]
            for a, b in zip(path, path[1:]):
                length = distance(a, b)
                if length < 1e-6:
                    continue
                if remaining <= length:
                    fraction = remaining / length
                    result.append(tuple(a[j] + fraction * (b[j] - a[j]) for j in range(3)))
                    remaining = 0
                    break
                result.append(b)
                remaining -= length
            if remaining > 1e-6 or len(result) > 32 or any(distance(a, b) < .001 for a, b in zip(result, result[1:])):
                continue
            clearance = min(boundary_distance(p, poly) for p in result[1:])
            candidates.append((clearance, -len(result), -i, poly, result))
    if not candidates:
        raise ValueError("No safe one-metre route on the containing polygon: " + str(start))
    _, _, _, poly, route = max(candidates, key=lambda c: c[:3])
    return poly, route


def verify_path(points, polygon):
    # Check every resulting segment against the baked polygon and independently
    # compare sampled heights with the serialized detail triangles.
    for a, b in zip(points, points[1:]):
        covered = clip(a, b, polygon["vertices"])
        if covered is None or covered[0] > 1e-6 or covered[1] < 1 - 1e-6:
            raise ValueError("Result leaves its NavMesh polygon")
        for t in (0, .25, .5, .75, 1):
            sample = tuple(a[j] + t * (b[j] - a[j]) for j in range(3))
            heights = [height(tri, sample[0], sample[2]) for tri in polygon["triangles"]
                       if clip(sample, sample, tri) is not None]
            heights = [h for h in heights if h is not None]
            if not heights or min(abs(h - sample[1]) for h in heights) > .002:
                raise ValueError("Result differs from the NavMesh detail height")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--config", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists() or digest(args.bundle) != BUNDLE_SHA or digest(args.config) != CONFIG_SHA:
        raise ValueError("Fresh output and exact source hashes required")
    cfg = json.loads(args.config.read_bytes())
    monsters = [p for p in cfg["ggo"]["runtime"]["points"] if p["kind"] == "monster" and not p.get("patrol")]
    if {p["id"] for p in monsters} != {"M%02d" % i for i in range(5, 25)}:
        raise ValueError("Unexpected unpatrolled point inventory")
    polygons, params, path_id = decode(args.bundle)
    routes = []
    for row in monsters:
        p = row["position"]
        start = (p["x"], p["y"], p["z"])
        polygon, points = choose(start, polygons)
        verify_path(points, polygon)
        length = sum(distance(a, b) for a, b in zip(points, points[1:]))
        if abs(length - 1) > 1e-6:
            raise ValueError("Route length mismatch: " + row["id"])
        routes.append(dict(id=row["id"], tile=polygon["tile"], polygon=polygon["poly"],
                           waypoints=[dict(x=x, y=y, z=z) for x, y, z in points]))
    output = dict(schema="hyjs-desert-monster-nav-routes-v1", bundleSHA256=BUNDLE_SHA,
                  configurationSHA256=CONFIG_SHA, navMeshPathID=path_id,
                  navTileVersion=16, navTiles=28, polygonCount=len(polygons),
                  bakedWalkableRadius=params["walkableRadius"], routes=routes)
    args.output.write_text(json.dumps(output, ensure_ascii=False, indent=2) + "\n")
    print("Derived %d one-metre routes from %d NavMesh polygons" % (len(routes), len(polygons)))


if __name__ == "__main__":
    main()
