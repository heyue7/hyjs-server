using System;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using ProtoBuf.Message;
using AssetLoad;
using Dict.Blo;
using Dict.DataModel;
using Room;
using UI;
using UnityEngine;

namespace HYJS.DungeonContent
{
    // Mapping source: RE OfflineBootstrap SHA b26c6b453e5a534bcdfa7da5d302f9266c9ade88e813129d6ecd321ebe59179b.
    // Only pinned main-story and awake visual aliases are admitted. No offline game logic is used.
    public static class DungeonContent
    {
        private static readonly HashSet<string> Whitelist = new HashSet<string>(new string[] {
            "map_arena_001",
            "map_cave_001",
            "map_caverun_001",
            "map_city_001",
            "map_desert_001",
            "map_desertrun_002",
            "map_forest_002",
            "map_forest_003",
            "map_forestfield_001",
            "map_forestrun_001",
            "map_forestrun_002",
            "map_grass_001",
            "map_grass_002",
            "map_grasslandrun_001",
            "map_grassrun_002",
            "map_guild_001",
            "map_maze_001",
            "map_maze_003",
            "map_mazerun_001",
            "map_mazerun_002",
            "map_nightforest_001",
            "map_nightforestrun_001",
            "map_nightforestfield_001",
            "map_portal_001",
            "map_portal_002",
            "map_portal_003",
            "map_portal_004",
            "map_portal_005",
            "map_select_001",
            "map_snowfield_001",
            "map_snowforest_001",
            "map_snowforestrun_001",
            "map_startscenes_001",
            "map_stonerelics_002",
            "map_stonerelicsrun_002",
            "map_street_001",
            "map_streetrun_001"
        }, StringComparer.Ordinal);
        private static readonly Dictionary<string, string> Aliases = new Dictionary<string, string>(StringComparer.Ordinal) {
            { "map_forest_001", "map_forest_002" },
            { "map_forestrun_003", "map_forestrun_002" },
            { "map_stonerelics_001", "map_stonerelics_002" },
            { "map_stonerelicsrun_001", "map_stonerelicsrun_002" },
            { "map_cave_004", "map_cave_001" },
            { "map_caverun_002", "map_caverun_001" },
            { "map_desertrun_001", "map_desertrun_002" },
            { "map_wasteland_001", "map_nightforest_001" },
            { "map_wastelandrun_001", "map_nightforestrun_001" },
            { "map_forestrun_004", "map_forestrun_002" },
            { "map_grasslandrun_003", "map_grassrun_002" },
            { "map_stonerelicsrun_003", "map_stonerelicsrun_002" },
            { "map_wastelandrun_002", "map_nightforestrun_001" },
            { "map_grassrun_001", "map_grassrun_002" },
            { "map_mazerun_004", "map_mazerun_001" },
            { "map_grassrun_004", "map_grassrun_002" },
            { "map_valley_001", "map_forest_002" },
            { "map_valleyrun_001", "map_forestrun_002" },
            { "map_bridge_001", "map_forest_002" },
            { "map_mazerun_003", "map_mazerun_001" },
            { "map_valleyrun_002", "map_forestrun_002" },
            // RE Awake Register: resource-only aliases, original battle/monster identities stay intact.
            { "map_ice_001", "map_snowforest_001" },
            { "map_volcano_001", "map_cave_001" },
            { "map_volcanofield_001", "map_forestfield_001" },
            { "map_desertfield_001", "map_desertrun_002" }
        };
        private sealed class Replacement
        {
            public string Original;
            public string Scene;
        }
        private static readonly Dictionary<string, Replacement> Replaced = new Dictionary<string, Replacement>(StringComparer.Ordinal);
        private static readonly Dictionary<string, Vector3> Positions = new Dictionary<string, Vector3>(StringComparer.Ordinal);
        private static string activeMission;
        private static RoomCopy activeRoom;
        private static Vector3 spawn;
        private static bool prepared;

        public static string Scene(string original)
        {
            if (String.IsNullOrEmpty(original)) return original;
            var assets = AssetBundleManager.Instance;
            if (assets == null || assets.IsAssetExist("map/" + original)) return original;
            string replacement;
            if (!Aliases.TryGetValue(original, out replacement)) return original;
            return Whitelist.Contains(replacement) && assets.IsAssetExist("map/" + replacement) ? replacement : original;
        }

        private static bool KnownMission(string id)
        {
            int number;
            return Int32.TryParse(id, out number) && number >= 1 && number <= 161 && number.ToString() == id;
        }

        private static bool ReadyScene(string scene)
        {
            string resolved = Scene(scene);
            var assets = AssetBundleManager.Instance;
            return !String.IsNullOrEmpty(resolved) && Whitelist.Contains(resolved) && assets != null && assets.IsAssetExist("map/" + resolved);
        }

        private static RoomChallenge awakeRoom;
        private static string awakeChapter;
        private static bool awakeReplaced;
        private static bool awakePrepared;
        private static Vector3 awakeSpawn;
        private static SCEnterAwakeCopy awakeEntry;
        private static readonly Dictionary<string, string> AwakeReplacedChapters = new Dictionary<string, string>(StringComparer.Ordinal);
        private static readonly BindingFlags AwakeFields = BindingFlags.Instance | BindingFlags.NonPublic;
        private static object AwakeField(RoomChallenge room, string name)
        {
            return typeof(RoomChallenge).GetField(name, AwakeFields).GetValue(room);
        }
        private static bool AwakeKnown(string chapter)
        {
            return chapter == "1" || chapter == "2" || chapter == "3" || chapter == "4" || chapter == "5";
        }
        private static Vector3 AwakeVector(string text)
        {
            string[] parts = text.Split(',');
            if (parts.Length != 3) throw new InvalidOperationException("Awake substitute coordinate invalid");
            return new Vector3(Single.Parse(parts[0], CultureInfo.InvariantCulture), Single.Parse(parts[1], CultureInfo.InvariantCulture), Single.Parse(parts[2], CultureInfo.InvariantCulture));
        }
        private static string AwakeText(Vector3 point)
        {
            return point.x.ToString("R", CultureInfo.InvariantCulture) + "," + point.y.ToString("R", CultureInfo.InvariantCulture) + "," + point.z.ToString("R", CultureInfo.InvariantCulture);
        }
        // The drop-source route opens the chapter catalogue, before any chapter is selected.
        public static bool AwakeEntryReady()
        {
            foreach (string id in new string[] { "1", "2", "3", "4", "5" })
            {
                var chapter = DictAwakeCopyBlo.GetOne(id);
                if (chapter != null && ReadyScene(chapter.sceneId) && ReadyScene(chapter.BattleMap)) return true;
            }
            return false;
        }
        public static void AwakeAdapt(RoomChallenge room)
        {
            string id = StaticData.challengeChapterId;
            if (room == null || !AwakeKnown(id)) return;
            var chapter = DictAwakeCopyBlo.GetOne(id);
            if (chapter == null) return;
            var entry = AwakeField(room, "enterMsg") as SCEnterAwakeCopy;
            bool resume = awakeRoom == room && awakeChapter == id && awakeEntry == entry;
            if (!resume) { awakePrepared = false; awakeReplaced = false; }
            string scene = Scene(chapter.sceneId);
            if (scene != chapter.sceneId) { AwakeReplacedChapters[id] = scene; chapter.sceneId = scene; }
            string remembered;
            awakeReplaced = AwakeReplacedChapters.TryGetValue(id, out remembered) && remembered == chapter.sceneId;
            awakeEntry = entry;
            chapter.BattleMap = Scene(chapter.BattleMap);
            awakeRoom = room;
            awakeChapter = id;
        }
        private static bool AwakeCurrent(RoomChallenge room)
        {
            return (int)RoomManager.curSceneType == 10 && room != null && room == awakeRoom && awakeChapter == StaticData.challengeChapterId && awakeReplaced;
        }
        public static void AwakePrepare(RoomChallenge room)
        {
            if (!AwakeCurrent(room) || awakePrepared || (bool)AwakeField(room, "sceneLoaded")) return;
            var chapter = DictAwakeCopyBlo.GetOne(awakeChapter);
            var entry = AwakeField(room, "enterMsg") as SCEnterAwakeCopy;
            if (chapter == null || entry == null) throw new InvalidOperationException("Awake substitute lacks server entry");
            int n = entry.monsters.Count;
            var original = new Vector3[n];
            var radii = new float[n];
            var groups = new int[n];
            for (int i = 0; i < n; i++)
            {
                original[i] = AwakeVector(entry.monsters[i].pos);
                var outside = DictAwakeMonsterOutSideBlo.GetOne(entry.monsters[i].monId);
                if (outside == null) throw new InvalidOperationException("Awake substitute missing outside row");
                radii[i] = (float)(outside.ViewR + outside.wanderR);
                groups[i] = i;
            }
            // Preserve the original neighbouring encounter groups without changing any AI radius.
            for (int i = 0; i < n; i++) for (int j = 0; j < i; j++)
            {
                Vector3 delta = original[i] - original[j]; delta.y = 0;
                if (delta.magnitude <= radii[i] + radii[j])
                {
                    int a = groups[i], b = groups[j];
                    for (int k = 0; k < n; k++) if (groups[k] == a) groups[k] = b;
                }
            }
            var groupIDs = new List<int>();
            for (int i = 0; i < n; i++) if (!groupIDs.Contains(groups[i])) groupIDs.Add(groups[i]);
            var anchors = Layout(chapter.sceneId, 1 + groupIDs.Count + entry.boxes.Count);
            var proposed = new Vector3[n];
            var centers = new Vector3[groupIDs.Count];
            var path = new NavMeshPath();
            for (int group = 0; group < groupIDs.Count; group++)
            {
                Vector3 center = Vector3.zero; int members = 0;
                for (int i = 0; i < n; i++) if (groups[i] == groupIDs[group]) { center += original[i]; members++; }
                center /= members;
                centers[group] = center;
                for (int i = 0; i < n; i++) if (groups[i] == groupIDs[group])
                {
                    Vector3 offset = original[i] - center; offset.y = 0;
                    NavMeshHit hit;
                    if (!NavMesh.SamplePosition(anchors[group + 1] + offset, out hit, 1.0f, -1)) throw new InvalidOperationException("Awake substitute encounter group does not fit NavMesh");
                    proposed[i] = hit.position;
                    if (!NavMesh.CalculatePath(anchors[0], hit.position, -1, path) || path.status != NavMeshPathStatus.PathComplete) throw new InvalidOperationException("Awake substitute encounter unreachable");
                    Vector3 distance = hit.position - anchors[0]; distance.y = 0;
                    if (distance.magnitude <= radii[i]) throw new InvalidOperationException("Awake substitute spawn enters encounter radius");
                }
            }
            for (int i = 0; i < n; i++) for (int j = 0; j < i; j++) if (groups[i] != groups[j])
            {
                Vector3 delta = proposed[i] - proposed[j]; delta.y = 0;
                if (delta.magnitude <= radii[i] + radii[j]) throw new InvalidOperationException("Awake substitute independent encounter groups overlap");
            }
            for (int i = 0; i < n; i++) for (int j = 0; j < i; j++) if (groups[i] == groups[j])
            {
                Vector3 before = original[i] - original[j]; before.y = 0;
                Vector3 after = proposed[i] - proposed[j]; after.y = 0;
                if (before.magnitude <= radii[i] + radii[j] && after.magnitude > radii[i] + radii[j]) throw new InvalidOperationException("Awake substitute encounter group split");
            }
            var proposedBoxes = new Vector3[entry.boxes.Count];
            for (int box = 0; box < entry.boxes.Count; box++)
            {
                Vector3 originalBox = AwakeVector(entry.boxes[box].pos);
                int guardGroup = -1;
                for (int i = 0; i < n; i++)
                {
                    Vector3 delta = originalBox - original[i]; delta.y = 0;
                    if (delta.magnitude <= radii[i]) { guardGroup = groupIDs.IndexOf(groups[i]); break; }
                }
                Vector3 candidate = anchors[1 + groupIDs.Count + box];
                if (guardGroup >= 0) { Vector3 offset = originalBox - centers[guardGroup]; offset.y = 0; candidate = anchors[1 + guardGroup] + offset; }
                NavMeshHit hit;
                if (!NavMesh.SamplePosition(candidate, out hit, 1.0f, -1) || !NavMesh.CalculatePath(anchors[0], hit.position, -1, path) || path.status != NavMeshPathStatus.PathComplete) throw new InvalidOperationException("Awake substitute box unreachable");
                proposedBoxes[box] = hit.position;
                for (int i = 0; i < n; i++)
                {
                    Vector3 before = originalBox - original[i]; before.y = 0;
                    Vector3 after = hit.position - proposed[i]; after.y = 0;
                    if ((before.magnitude <= radii[i]) != (after.magnitude <= radii[i])) throw new InvalidOperationException("Awake substitute box encounter protection changed");
                }
            }
            // Commit visual coordinates only after validating every server-provided object.
            awakeSpawn = anchors[0]; chapter.bornPos = AwakeText(awakeSpawn);
            for (int i = 0; i < n; i++) entry.monsters[i].pos = AwakeText(proposed[i]);
            for (int i = 0; i < entry.boxes.Count; i++) entry.boxes[i].pos = AwakeText(proposedBoxes[i]);
            awakePrepared = true;
        }
        public static void AwakeAlignPlayer(RoomChallenge room)
        {
            if (!AwakeCurrent(room) || !awakePrepared || room.charactorManager == null) return;
            var player = room.charactorManager.GetSelfCharactor();
            if (player == null) return;
            player.transform.position = awakeSpawn;
            var agent = player.GetComponent<NavMeshAgent>();
            if (agent != null) agent.Warp(awakeSpawn);
            player.StopTo(awakeSpawn);
        }

        public static bool MainEntryReady()
        {
            var page = PageManager.Instance.currentPage as UICopy;
            if (page == null || !KnownMission(page.currentSelectionLevelId)) return false;
            var mission = DictCopyMissionBlo.GetTableCopyMission(page.currentSelectionLevelId);
            if (mission == null || !ReadyScene(mission.SceneName)) return false;
            var tasks = DictCopyTaskBlo.GetTasksByLevelId(mission.MissionId);
            if (tasks == null || tasks.Count == 0) return false;
            foreach (var task in tasks) if (task == null || !ReadyScene(task.SceneName)) return false;
            return true;
        }

        public static void AdaptMission(RoomCopy room)
        {
            string id = StaticData.currFightLevelId;
            bool resume = room != null && room == activeRoom && room.missionLoaded && activeMission == id && prepared;
            if (!resume)
            {
                activeMission = null;
                activeRoom = null;
                prepared = false;
                Positions.Clear();
            }
            if (room == null || !KnownMission(id)) return;
            var mission = DictCopyMissionBlo.GetTableCopyMission(id);
            if (mission == null) return;
            string mapped = Scene(mission.SceneName);
            Replacement record;
            if (mapped != mission.SceneName)
            {
                record = new Replacement();
                record.Original = mission.SceneName;
                record.Scene = mapped;
                Replaced[id] = record;
                mission.SceneName = mapped;
            }
            else if (!Replaced.TryGetValue(id, out record) || record.Scene != mission.SceneName)
            {
                activeMission = null;
                activeRoom = null;
                prepared = false;
                Positions.Clear();
                return;
            }
            activeMission = id;
            activeRoom = room;
            var tasks = DictCopyTaskBlo.GetTasksByLevelId(id);
            if (tasks == null) return;
            var byId = new Dictionary<string, TableCopyTask>(StringComparer.Ordinal);
            foreach (var task in tasks)
            {
                task.SceneName = Scene(task.SceneName);
                byId[task.TaskId] = task;
            }
            if (StaticData.jobs != null)
                foreach (var job in StaticData.jobs)
                {
                    TableCopyTask task;
                    if (job != null && byId.TryGetValue(job.taskId, out task))
                    {
                        job.sceneName = task.SceneName;
                        Vector3 point;
                        if (resume && Positions.TryGetValue(job.taskId, out point)) { job.bornX = point.x; job.bornZ = point.z; }
                    }
                }
            Debug.Log("HYJS content substitute mission=" + id + " original=" + record.Original + " display=" + record.Scene);
        }

        private static bool Current()
        {
            return (int)RoomManager.curSceneType == 3 && activeMission != null && activeMission == StaticData.currFightLevelId && Replaced.ContainsKey(activeMission);
        }

        private static List<Vector3> Layout(string scene, int count)
        {
            var mesh = NavMesh.CalculateTriangulation();
            if (mesh.vertices == null || mesh.vertices.Length == 0) throw new InvalidOperationException("Dungeon substitute NavMesh absent: " + scene);
            var samples = new List<Vector3>();
            var keys = new HashSet<string>(StringComparer.Ordinal);
            for (int triangle = 0; triangle + 2 < mesh.indices.Length; triangle += 3)
            {
                Vector3 a = mesh.vertices[mesh.indices[triangle]];
                Vector3 b = mesh.vertices[mesh.indices[triangle + 1]];
                Vector3 c = mesh.vertices[mesh.indices[triangle + 2]];
                for (int u = 1; u < 8; u++)
                    for (int v = 1; v < 8 - u; v++)
                    {
                        NavMeshHit hit;
                        Vector3 candidate = a + (b - a) * (u / 8.0f) + (c - a) * (v / 8.0f);
                        if (!NavMesh.SamplePosition(candidate, out hit, 1.0f, -1)) continue;
                        string key = Mathf.Round(hit.position.x * 2.0f).ToString() + ":" + Mathf.Round(hit.position.z * 2.0f).ToString();
                        if (keys.Add(key)) samples.Add(hit.position);
                    }
            }
            samples.Sort(delegate(Vector3 a, Vector3 b) { int z = a.z.CompareTo(b.z); return z != 0 ? z : a.x.CompareTo(b.x); });
            var connected = new List<Vector3>();
            int anchors = Math.Min(12, samples.Count);
            var testedOrigins = new List<Vector3>();
            var testedReachable = new List<List<Vector3>>();
            var path = new NavMeshPath();
            for (int anchor = 0; anchor < anchors; anchor++)
            {
                int index = anchors == 1 ? 0 : anchor * (samples.Count - 1) / (anchors - 1);
                List<Vector3> reachable = null;
                // One-way OffMeshLinks do not establish equal reachable sets.
                // Reuse only after checking both directions on the actual loaded mesh.
                for (int previous = 0; previous < testedOrigins.Count; previous++)
                {
                    bool forward = NavMesh.CalculatePath(testedOrigins[previous], samples[index], -1, path) && path.status == NavMeshPathStatus.PathComplete;
                    bool backward = forward && NavMesh.CalculatePath(samples[index], testedOrigins[previous], -1, path) && path.status == NavMeshPathStatus.PathComplete;
                    if (backward) { reachable = testedReachable[previous]; break; }
                }
                if (reachable == null)
                {
                    reachable = new List<Vector3>();
                    foreach (Vector3 candidate in samples)
                        if (NavMesh.CalculatePath(samples[index], candidate, -1, path) && path.status == NavMeshPathStatus.PathComplete) reachable.Add(candidate);
                    testedOrigins.Add(samples[index]);
                    testedReachable.Add(reachable);
                }
                if (reachable.Count > connected.Count) connected = reachable;
            }
            if (connected.Count < count) throw new InvalidOperationException("Dungeon substitute connected points insufficient: " + scene);
            var chosen = new List<Vector3>();
            chosen.Add(connected[0]);
            connected.RemoveAt(0);
            while (chosen.Count < count)
            {
                int best = -1;
                float distance = -1.0f;
                for (int i = 0; i < connected.Count; i++)
                {
                    float nearest = Single.MaxValue;
                    foreach (Vector3 previous in chosen) nearest = Math.Min(nearest, (connected[i] - previous).sqrMagnitude);
                    if (nearest > distance) { best = i; distance = nearest; }
                }
                if (best < 0 || distance < 4.0f) throw new InvalidOperationException("Dungeon substitute encounter spacing insufficient: " + scene);
                chosen.Add(connected[best]);
                connected.RemoveAt(best);
            }
            return chosen;
        }

        public static void Prepare(RoomCopy room)
        {
            if (!Current() || room == null || room.missionLoaded) return;
            var mission = DictCopyMissionBlo.GetTableCopyMission(activeMission);
            var tasks = DictCopyTaskBlo.GetTasksByLevelId(activeMission);
            var points = Layout(mission.SceneName, tasks.Count + 1);
            Vector3 origin = points[0];
            // Validate all encounters before applying spatial fields; IDs, AI radii and combat values stay native.
            for (int i = 0; i < tasks.Count; i++)
            {
                var path = new NavMeshPath();
                if (!NavMesh.CalculatePath(origin, points[i + 1], -1, path) || path.status != NavMeshPathStatus.PathComplete)
                    throw new InvalidOperationException("Dungeon substitute encounter unreachable: " + tasks[i].TaskId);
            }
            spawn = origin;
            mission.BornX = spawn.x;
            mission.BornZ = spawn.z;
            for (int i = 0; i < tasks.Count; i++)
            {
                Vector3 point = points[i + 1];
                tasks[i].BornX = point.x;
                tasks[i].BornZ = point.z;
                Positions[tasks[i].TaskId] = point;
            }
            if (StaticData.jobs != null)
                foreach (var job in StaticData.jobs)
                {
                    Vector3 point;
                    if (job != null && Positions.TryGetValue(job.taskId, out point)) { job.bornX = point.x; job.bornZ = point.z; }
                }
            prepared = true;
        }

        public static void AlignPlayer(RoomCopy room)
        {
            if (!Current() || !prepared || room == null || room.charactorManager == null) return;
            var player = room.charactorManager.GetSelfCharactor();
            if (player == null) return;
            player.transform.position = spawn;
            var agent = player.GetComponent<NavMeshAgent>();
            if (agent != null) agent.Warp(spawn);
            player.StopTo(spawn);
        }

        public static void AlignMonster(RCAIMonster ai)
        {
            if (!Current() || !prepared || ai == null || ai.owner == null) return;
            Vector3 point;
            if (!Positions.TryGetValue(ai.taskId, out point)) return;
            ai.bornPosition = point;
            ai.owner.transform.position = point;
            var agent = ai.owner.GetComponent<NavMeshAgent>();
            if (agent != null) agent.Warp(point);
        }

        public static Vector3 ProjectTarget(Vector3 original)
        {
            if (!Current() || !prepared) return original;
            NavMeshHit hit;
            if (NavMesh.SamplePosition(original, out hit, 0.05f, -1)) return original;
            return NavMesh.SamplePosition(original, out hit, 100.0f, -1) ? hit.position : original;
        }

        public static bool SkipArea(string area)
        {
            return Current() && prepared && area == "mission_" + activeMission;
        }
    }
}
