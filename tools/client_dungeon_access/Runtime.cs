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
        private static RoomCopy encounterRoom;
        private static string encounterMission;

        // The native boss story can leave the room. Keep that encounter behind all minions.
        public static bool AllowTask(string taskId)
        {
            if ((int)RoomManager.curSceneType != 3 || encounterRoom == null ||
                encounterMission != StaticData.currFightLevelId || !KnownMission(encounterMission)) return true;
            var tasks = DictCopyTaskBlo.GetTasksByLevelId(encounterMission);
            TableCopyTask target = null;
            foreach (var task in tasks) if (task.TaskId == taskId) { target = task; break; }
            if (target == null || target.IsBoss != 1) return true;
            // On reconnect jobs contains only the server's remaining encounters.
            if (StaticData.jobs != null && StaticData.jobs.Count > 0)
            {
                foreach (var job in StaticData.jobs)
                    if (job.taskId != taskId && !encounterRoom.deadMonsters.Contains(job.taskId)) return false;
            }
            else foreach (var task in tasks)
                if (task.TaskId != taskId && !encounterRoom.deadMonsters.Contains(task.TaskId)) return false;
            return true;
        }

        // Called after each native load, including battle return with missionLoaded=true.
        public static void AdvanceBoss(RoomCopy room)
        {
            if (room == null || room != encounterRoom || !room.missionLoaded || room.charactorManager == null ||
                (int)RoomManager.curSceneType != 3 || !KnownMission(encounterMission) || encounterMission != StaticData.currFightLevelId) return;
            if (StaticData.jobs != null && StaticData.jobs.Count > 0)
            {
                foreach (var job in StaticData.jobs)
                    if (job.isBoss == 1 && !room.deadMonsters.Contains(job.taskId) && AllowTask(job.taskId) &&
                        room.charactorManager.GetCharactor(job.taskId) == null) room.charactorManager.CreateMonster(job);
            }
            else foreach (var task in DictCopyTaskBlo.GetTasksByLevelId(encounterMission))
                if (task.IsBoss == 1 && !room.deadMonsters.Contains(task.TaskId) && AllowTask(task.TaskId) &&
                    room.charactorManager.GetCharactor(task.TaskId) == null) room.charactorManager.CreateMonster(task);
        }

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
            Awake85Transition(room, id, entry);
            if (Awake85NightForest.Applies(awake85State, room, id, entry, (int)RoomManager.curSceneType))
            {
                if (chapter.sceneId != "map_nightforestfield_001")
                    throw new InvalidOperationException("Awake85 night scene changed");
                Awake85EnsureNightForest();
            }
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
            if (Awake85Current(room))
            {
                if (!awakePrepared && !(bool)AwakeField(room, "sceneLoaded")) Awake85Prepare(room);
                return;
            }
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
            if ((!AwakeCurrent(room) && !Awake85Current(room)) || !awakePrepared || room.charactorManager == null) return;
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
            encounterRoom = room;
            encounterMission = KnownMission(id) ? id : null;
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
            if (room == null || room.missionLoaded || room != encounterRoom || !KnownMission(encounterMission)) return;
            if (!Current()) { PrepareOriginalBoxes(); return; }
            var mission = DictCopyMissionBlo.GetTableCopyMission(activeMission);
            var tasks = DictCopyTaskBlo.GetTasksByLevelId(activeMission);
            int boxCount = StaticData.boxs == null ? 0 : StaticData.boxs.Count;
            var points = Layout(mission.SceneName, tasks.Count + boxCount + 1);
            Vector3 origin = points[0];
            var ordered = new List<Vector3>(points.GetRange(1, points.Count - 1));
            var distances = new Dictionary<Vector3, float>();
            foreach (Vector3 point in ordered) distances[point] = PathDistance(origin, point);
            ordered.Sort(delegate(Vector3 a, Vector3 b) { return distances[a].CompareTo(distances[b]); });
            // Boxes are reachable before the final story. The boss occupies the farthest encounter.
            var orderedTasks = new List<TableCopyTask>(tasks);
            orderedTasks.Sort(delegate(TableCopyTask a, TableCopyTask b) {
                int boss = a.IsBoss.CompareTo(b.IsBoss);
                return boss != 0 ? boss : Int32.Parse(a.TaskId).CompareTo(Int32.Parse(b.TaskId));
            });
            // Validate all encounters before applying spatial fields; IDs, AI radii and combat values stay native.
            for (int i = 0; i < tasks.Count; i++)
            {
                var path = new NavMeshPath();
                if (!NavMesh.CalculatePath(origin, ordered[i + boxCount], -1, path) || path.status != NavMeshPathStatus.PathComplete)
                    throw new InvalidOperationException("Dungeon substitute encounter unreachable: " + orderedTasks[i].TaskId);
            }
            spawn = origin;
            mission.BornX = spawn.x;
            mission.BornZ = spawn.z;
            for (int i = 0; i < tasks.Count; i++)
            {
                Vector3 point = ordered[i + boxCount];
                orderedTasks[i].BornX = point.x;
                orderedTasks[i].BornZ = point.z;
                Positions[orderedTasks[i].TaskId] = point;
            }
            for (int i = 0; i < boxCount; i++) SetBoxPoint(StaticData.boxs[i], ordered[i]);
            if (StaticData.jobs != null)
                foreach (var job in StaticData.jobs)
                {
                    Vector3 point;
                    if (job != null && Positions.TryGetValue(job.taskId, out point)) { job.bornX = point.x; job.bornZ = point.z; }
                }
            prepared = true;
        }

        private static float PathDistance(Vector3 origin, Vector3 point)
        {
            var path = new NavMeshPath();
            if (!NavMesh.CalculatePath(origin, point, -1, path) || path.status != NavMeshPathStatus.PathComplete)
                throw new InvalidOperationException("Dungeon point unreachable");
            float length = 0;
            for (int i = 1; i < path.corners.Length; i++) length += Vector3.Distance(path.corners[i - 1], path.corners[i]);
            return length;
        }
        private static void SetBoxPoint(CopyBoxInfo box, Vector3 point)
        {
            box.x = point.x; box.y = point.y; box.z = point.z;
        }
        private static void PrepareOriginalBoxes()
        {
            if (StaticData.boxs == null || StaticData.boxs.Count == 0) return;
            var mission = DictCopyMissionBlo.GetTableCopyMission(encounterMission);
            if (mission == null) return;
            NavMeshHit spawnHit;
            if (!NavMesh.SamplePosition(new Vector3((float)mission.BornX, 0, (float)mission.BornZ), out spawnHit, 10f, -1))
                throw new InvalidOperationException("Dungeon box lacks reachable spawn");
            var mesh = NavMesh.CalculateTriangulation();
            var proposed = new List<Vector3>();
            foreach (var box in StaticData.boxs)
            {
                Vector3 original = new Vector3((float)box.x, (float)box.y, (float)box.z);
                NavMeshHit hit;
                var path = new NavMeshPath();
                bool found = NavMesh.SamplePosition(original, out hit, 2f, -1) &&
                    NavMesh.CalculatePath(spawnHit.position, hit.position, -1, path) && path.status == NavMeshPathStatus.PathComplete;
                Vector3 best = found ? hit.position : Vector3.zero;
                float nearest = found ? (best - original).sqrMagnitude : Single.MaxValue;
                // Repair an off-mesh/unreachable original chest using the nearest reachable triangle interior.
                for (int i = 0; !found && i + 2 < mesh.indices.Length; i += 3)
                {
                    Vector3 center = (mesh.vertices[mesh.indices[i]] + mesh.vertices[mesh.indices[i + 1]] + mesh.vertices[mesh.indices[i + 2]]) / 3f;
                    if ((center - original).sqrMagnitude >= nearest || !NavMesh.SamplePosition(center, out hit, 0.5f, -1) ||
                        !NavMesh.CalculatePath(spawnHit.position, hit.position, -1, path) || path.status != NavMeshPathStatus.PathComplete) continue;
                    best = hit.position; nearest = (best - original).sqrMagnitude;
                }
                if (nearest == Single.MaxValue) throw new InvalidOperationException("Dungeon chest unreachable: " + box.boxId);
                proposed.Add(best);
            }
            for (int i = 0; i < proposed.Count; i++) SetBoxPoint(StaticData.boxs[i], proposed[i]);
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

        // v85 layout is bound to a server-generated run and never changes an
        // unmarked entry. The original helper's legacy awakening path remains.
        private static Awake85State awake85State;

        private static bool Awake85ExpectedNightCell(AssetCell cell)
        {
            return cell != null && Awake85NightForest.Matches(cell.path, cell.hashCode, cell.size);
        }

        private static void Awake85EnsureNightForest()
        {
            var manager = AssetBundleManager.Instance;
            if (manager == null) throw new InvalidOperationException("Awake85 asset manager unavailable");
            var fullField = typeof(AssetBundleManager).GetField("assetFileTool", AwakeFields);
            var smallField = typeof(AssetBundleManager).GetField("smallAssetFileTool", AwakeFields);
            var full = fullField == null ? null : fullField.GetValue(manager) as AssetFileTool;
            var small = smallField == null ? null : smallField.GetValue(manager) as AssetFileTool;
            if (full == null || small == null || full.assetCells == null || small.assetCells == null)
                throw new InvalidOperationException("Awake85 asset manifests unavailable");
            var parsed = AssetFileTool.LoadFromString(Awake85NightForest.ManifestText);
            if (parsed == null) throw new InvalidOperationException("Awake85 night manifest parse failed");
            Awake85NightForest.Register<AssetCell>(full.assetCells, small.assetCells, parsed.assetCells,
                new Func<AssetCell, bool>(Awake85ExpectedNightCell));
        }

        private static void Awake85RestoreBorn(object owner, string value)
        {
            var chapter = owner as TableAwakeCopyChapter;
            if (chapter == null) throw new InvalidOperationException("Awake85 chapter restore invalid");
            chapter.bornPos = value;
        }

        private static void Awake85Transition(RoomChallenge room, string chapter, SCEnterAwakeCopy entry)
        {
            if (awake85State != null)
            {
                if (awake85State.Same(room, chapter, entry)) return;
                awake85State.Leave(Awake85RestoreBorn);
            }
            if (entry == null) return;
            string[] boxes = new string[entry.boxes.Count];
            string[] monsters = new string[entry.monsters.Count];
            for (int i = 0; i < boxes.Length; i++) boxes[i] = entry.boxes[i] == null ? null : entry.boxes[i].guid;
            for (int i = 0; i < monsters.Length; i++) monsters[i] = entry.monsters[i] == null ? null : entry.monsters[i].guid;
            string run = Awake85Contract.Identify(chapter, boxes, monsters);
            if (run == null) return;
            if (awake85State == null) awake85State = new Awake85State();
            awake85State.Bind(room, chapter, entry, run);
        }

        private static bool Awake85Current(RoomChallenge room)
        {
            return (int)RoomManager.curSceneType == 10 && room != null && room == awakeRoom &&
                awake85State != null && awake85State.Active(room, StaticData.challengeChapterId,
                    AwakeField(room, "enterMsg"), (int)RoomManager.curSceneType);
        }

        private static void Awake85Prepare(RoomChallenge room)
        {
            var chapter = DictAwakeCopyBlo.GetOne(awakeChapter);
            var entry = AwakeField(room, "enterMsg") as SCEnterAwakeCopy;
            if (chapter == null || entry == null || awake85State == null)
                throw new InvalidOperationException("Awake85 server entry unavailable");
            string run = awake85State.Run;
            string[] boxGuids = new string[entry.boxes.Count];
            string[] monsterGuids = new string[entry.monsters.Count];
            for (int i = 0; i < boxGuids.Length; i++) boxGuids[i] = entry.boxes[i] == null ? null : entry.boxes[i].guid;
            for (int i = 0; i < monsterGuids.Length; i++) monsterGuids[i] = entry.monsters[i] == null ? null : entry.monsters[i].guid;
            if (Awake85Contract.Identify(awakeChapter, boxGuids, monsterGuids) != run)
                throw new InvalidOperationException("Awake85 entry source changed");

            int cornerMask = 0;
            int[] boxIndex = new int[4];
            for (int i = 0; i < entry.boxes.Count; i++)
            {
                Vector3 original = AwakeVector(entry.boxes[i].pos);
                if (Math.Abs(original.x) != 14.0f || original.y != 0.0f || Math.Abs(original.z) != 14.0f)
                    throw new InvalidOperationException("Awake85 box template invalid");
                int corner = (original.x > 0 ? 2 : 0) + (original.z > 0 ? 1 : 0);
                if ((cornerMask & (1 << corner)) != 0) throw new InvalidOperationException("Awake85 box corner duplicated");
                cornerMask |= 1 << corner;
                boxIndex[Awake85Contract.BoxSlot(entry.boxes[i].guid, run)] = i;
            }
            if (cornerMask != 15) throw new InvalidOperationException("Awake85 box corners incomplete");
            if (entry.monsters.Count == 1)
            {
                var monster = entry.monsters[0];
                Vector3 original = AwakeVector(monster.pos);
                if (original.x != 0.0f || original.y != 0.0f || original.z != 0.0f)
                    throw new InvalidOperationException("Awake85 monster template invalid");
                var outside = DictAwakeMonsterOutSideBlo.GetOne(monster.monId);
                if (outside == null || outside.ViewR < 0 || outside.wanderR < 0 ||
                    Double.IsNaN(outside.ViewR) || Double.IsInfinity(outside.ViewR) ||
                    Double.IsNaN(outside.wanderR) || Double.IsInfinity(outside.wanderR) ||
                    outside.ViewR + outside.wanderR > 9.0)
                    throw new InvalidOperationException("Awake85 monster radius invalid");
            }

            List<Vector3> candidates = null;
            int[] slots = null;
            for (int count = 6; count <= 12 && slots == null; count += 2)
            {
                candidates = Layout(chapter.sceneId, count);
                var x = new float[candidates.Count];
                var z = new float[candidates.Count];
                var paths = new bool[candidates.Count, candidates.Count];
                var path = new NavMeshPath();
                for (int i = 0; i < candidates.Count; i++)
                {
                    x[i] = candidates[i].x; z[i] = candidates[i].z;
                    if (Single.IsNaN(candidates[i].y) || Single.IsInfinity(candidates[i].y))
                        throw new InvalidOperationException("Awake85 candidate height invalid");
                    for (int j = 0; j < candidates.Count; j++)
                        paths[i, j] = i == j || (NavMesh.CalculatePath(candidates[i], candidates[j], -1, path) &&
                            path.status == NavMeshPathStatus.PathComplete);
                }
                slots = Awake85Contract.Select(x, z, paths);
            }
            if (slots == null || candidates == null) throw new InvalidOperationException("Awake85 safe slots unavailable");
            string[] positions = new string[6];
            for (int i = 0; i < positions.Length; i++) positions[i] = AwakeText(candidates[slots[i]]);
            // All checks finish before mutating the shared chapter or server entry.
            awake85State.CaptureBorn(chapter, chapter.bornPos);
            awakeSpawn = candidates[slots[0]];
            chapter.bornPos = positions[0];
            if (entry.monsters.Count == 1) entry.monsters[0].pos = positions[1];
            for (int slot = 0; slot < boxIndex.Length; slot++) entry.boxes[boxIndex[slot]].pos = positions[slot + 2];
            awakePrepared = true;
        }
    }
}
