// Diagnostic caller only: all battle arithmetic, tables, RNG and AI stay in
// the APK's original assembly. The private input folder contains frozen wire
// files; this runner sends no game requests and grants no rewards.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Reflection;
using System.Text;
using Battle.Engine;
using Battle.Engine.LogicModel;
using ProtoBuf.Message;
using UnityEngine;

public static class NativeBatchRunner
{
    private static bool running;

    public static void ObservePVEUIPress(int camp, string actorUid, int skillIndex)
    {
        LocalPVEUICompatibility.ObservePress(camp, actorUid, skillIndex);
    }

    public static void ObservePVEUIBegin(object skill)
    {
        LocalPVEUICompatibility.ObserveBegin(skill);
    }

    public static void AfterPVEUIUpdate(object manager)
    {
        LocalPVEUICompatibility.AfterUpdate(manager);
    }

    public static void ApplyActivityInformation(object panel, object packet, bool success)
    {
        LocalActivityInformation.Apply(panel, packet, success);
    }

    public static void ApplyExpeditionPreview(object panel)
    {
        LocalExpeditionPresentation.Apply(panel);
    }

    public static bool TryStart(object candidate)
    {
        PlaybackKit kit = candidate as PlaybackKit;
        if (kit == null || kit.serverId == null || kit.serverId.value != "60006") return false;
        if (running) return true;
        string selector = kit.input.value;
        if (selector != "golden" && selector != "highest" && selector != "pve-auto" && selector != "pve-compat") return true;
        string folder = Path.Combine(Application.persistentDataPath, "native-batch/" + selector);
        if (!Directory.Exists(folder)) return true;
        running = true;
        kit.StartCoroutine(Run(folder));
        return true;
    }

    // Snapshot the real reply and original character position around callbacks.
    // Reflection reads lifecycle fields without changing Unity or server state.
    public static void LogRoomPosition(object net, object response, int phase)
    {
        try
        {
            object room = Field(net, "roomBase");
            object manager = Field(room, "charactorManager");
            MethodInfo getter = manager == null ? null : manager.GetType().GetMethod("GetSelfCharactor");
            object self = getter == null ? null : getter.Invoke(manager, new object[0]);
            Component character = self as Component;
            string position = character == null ? "null" : Vector(character.transform.position);
            object move = Field(self, "move");
            object agent = Field(move, "_agent");
            string nav = "null";
            if (agent != null)
            {
                PropertyInfo onMesh = agent.GetType().GetProperty("isOnNavMesh");
                PropertyInfo enabled = agent.GetType().GetProperty("enabled");
                nav = "{\"position\":" + Json(agent.GetType().GetProperty("nextPosition").GetValue(agent, null))
                    + ",\"onMesh\":" + Json(onMesh == null ? null : onMesh.GetValue(agent, null))
                    + ",\"enabled\":" + Json(enabled.GetValue(agent, null)) + "}";
            }
            SCCurrentPosition current = response as SCCurrentPosition;
            SCLoginScene login = response as SCLoginScene;
            MovePlayer player = current != null ? current.self : (login != null ? login.self : null);
            string reply = player == null || player.position == null || player.position.current == null ? "null"
                : "{\"scene\":" + Json(player.position.scene) + ",\"x\":" + Number(player.position.current.x)
                    + ",\"y\":" + Number(player.position.current.y) + ",\"z\":" + Number(player.position.current.z) + "}";
            string line = "{\"phase\":" + phase + ",\"net\":" + Json(net.GetType().FullName)
                + ",\"initState\":" + Json(Field(net, "initState")) + ",\"reply\":" + reply
                + ",\"characterPosition\":" + position + ",\"nav\":" + nav + "}\n";
            File.AppendAllText(Path.Combine(Application.persistentDataPath, "native-room-position.jsonl"), line);
        }
        catch (Exception e)
        {
            File.AppendAllText(Path.Combine(Application.persistentDataPath, "native-room-position.jsonl"),
                "{\"diagnosticError\":" + Json(e.ToString()) + "}\n");
        }
    }

    private static object Field(object target, string name)
    {
        if (target == null) return null;
        for (Type t = target.GetType(); t != null; t = t.BaseType)
        {
            FieldInfo field = t.GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly);
            if (field != null) return field.GetValue(target);
        }
        return null;
    }

    private static string Vector(Vector3 point)
    {
        return "{\"x\":" + Number(point.x) + ",\"y\":" + Number(point.y) + ",\"z\":" + Number(point.z) + "}";
    }

    private static IEnumerator Run(string folder)
    {
        string status = Path.Combine(folder, "status.txt");
        string[] paths = Directory.GetFiles(folder, "*.pb");
        Array.Sort(paths, StringComparer.Ordinal);
        try
        {
            for (int i = 0; i < paths.Length; i++)
            {
                string name = Path.GetFileNameWithoutExtension(paths[i]);
                File.WriteAllText(status, "running " + i + "/" + paths.Length + " " + name);
                string error = null;
                try { RunCase(paths[i], Path.GetFileName(folder) == "pve-auto", Path.GetFileName(folder) == "pve-compat"); }
                catch (Exception e) { error = e.ToString(); }
                if (error != null)
                {
                    File.WriteAllText(status, "failed " + name + "\n" + error);
                    yield break;
                }
                // Keep the Unity event loop alive between bounded original runs.
                yield return null;
            }
            File.WriteAllText(status, "complete " + paths.Length);
        }
        finally { running = false; }
    }

    private static void RunCase(string path, bool forceAutomatic, bool localCompatibility)
    {
        SCFightPlayback wire;
        using (FileStream input = File.OpenRead(path)) wire = ProtoBuf.Serializer.Deserialize<SCFightPlayback>(input);
        string manualDriverPath = Path.ChangeExtension(path, ".rush-auto-start.txt");
        string manualPolicyPath = Path.ChangeExtension(path, ".rush-auto-policy.txt");
        bool scheduledManual = false;
        string manualPolicySHA = null;
        if (File.Exists(manualPolicyPath))
        {
            byte[] policyBytes = File.ReadAllBytes(manualPolicyPath);
            string policy = Encoding.UTF8.GetString(policyBytes);
            if (policy != "prepublished-diagnostic-schedule-v1" || !File.Exists(manualDriverPath)
                || wire.optRecord != null || forceAutomatic || localCompatibility
                || (wire.battleType != 9 && wire.battleType != 10)
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("explicit manual schedule requires exact typed highest mode9/10 and no playback record");
            double delay = Double.Parse(File.ReadAllText(manualDriverPath), CultureInfo.InvariantCulture);
            if (Double.IsNaN(delay) || Double.IsInfinity(delay) || delay < 0 || delay > 110)
                throw new InvalidOperationException("explicit manual schedule delay is invalid");
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                manualPolicySHA = BitConverter.ToString(hash.ComputeHash(policyBytes)).Replace("-", "").ToLowerInvariant();
            scheduledManual = true;
        }
        SCRushFightStart rush = null;
        string typedSHA = null;
        string typedPath = Path.ChangeExtension(path, ".rush-start.bin");
        if (File.Exists(typedPath))
        {
            if (localCompatibility || (wire.battleType != 9 && wire.battleType != 10)
                || (forceAutomatic ? wire.optRecord != null : wire.optRecord == null && !scheduledManual))
                throw new InvalidOperationException("typed Rush requires explicit automatic or exact recorded mode9/10");
            byte[] typedBytes = File.ReadAllBytes(typedPath);
            using (MemoryStream input = new MemoryStream(typedBytes)) rush = ProtoBuf.Serializer.Deserialize<SCRushFightStart>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        SCPvpFightResult arena = null;
        string arenaPath = Path.ChangeExtension(path, ".arena-result.bin");
        if (File.Exists(arenaPath))
        {
            if (rush != null || forceAutomatic || localCompatibility || wire.battleType != 2
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("typed arena callback requires original automatic highest mode2 only");
            byte[] typedBytes = File.ReadAllBytes(arenaPath);
            using (MemoryStream input = new MemoryStream(typedBytes)) arena = ProtoBuf.Serializer.Deserialize<SCPvpFightResult>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        SCLimitPvpFightResult limit = null;
        string limitPath = Path.ChangeExtension(path, ".limit-result.bin");
        if (File.Exists(limitPath))
        {
            if (rush != null || arena != null || forceAutomatic || localCompatibility || wire.battleType != 20
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("typed Limit callback requires independent highest mode20 only");
            byte[] typedBytes = File.ReadAllBytes(limitPath);
            using (MemoryStream input = new MemoryStream(typedBytes)) limit = ProtoBuf.Serializer.Deserialize<SCLimitPvpFightResult>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        SCGrab grab = null;
        string grabPath = Path.ChangeExtension(path, ".grab-start.bin");
        if (File.Exists(grabPath))
        {
            if (rush != null || arena != null || limit != null || forceAutomatic || localCompatibility || wire.battleType != 19
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("typed Grab requires independent automatic highest mode19");
            byte[] typedBytes = File.ReadAllBytes(grabPath);
            using (MemoryStream input = new MemoryStream(typedBytes)) grab = ProtoBuf.Serializer.Deserialize<SCGrab>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        SCFightTreasureStartResult plunder = null;
        string targetFrag = null;
        string targetSHA = null;
        string plunderPath = Path.ChangeExtension(path, ".plunder-start.bin");
        if (File.Exists(plunderPath))
        {
            if (rush != null || arena != null || forceAutomatic || localCompatibility || wire.battleType != 3
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("typed plunder requires original automatic highest mode3");
            byte[] typedBytes = File.ReadAllBytes(plunderPath);
            byte[] targetBytes = File.ReadAllBytes(Path.ChangeExtension(path, ".plunder-target.txt"));
            using (MemoryStream input = new MemoryStream(typedBytes)) plunder = ProtoBuf.Serializer.Deserialize<SCFightTreasureStartResult>(input);
            targetFrag = Encoding.UTF8.GetString(targetBytes);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
            {
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
                targetSHA = BitConverter.ToString(hash.ComputeHash(targetBytes)).Replace("-", "").ToLowerInvariant();
            }
        }
        KFReplayData kf = null;
        long kfRealm = 0;
        long kfPID = 0;
        string kfDriverSHA = null;
        bool kfProjectionOnly = false;
        bool kfSameRealm = false;
        string kfPath = Path.ChangeExtension(path, ".kf-replay.bin");
        if (File.Exists(kfPath))
        {
            if (rush != null || arena != null || limit != null || plunder != null || forceAutomatic || localCompatibility || wire.battleType != 17
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("KF realm typed source requires independent local highest17 scope");
            byte[] typedBytes = File.ReadAllBytes(kfPath);
            byte[] driverBytes = File.ReadAllBytes(Path.ChangeExtension(path, ".kf-viewer.txt"));
            string[] viewer = Encoding.UTF8.GetString(driverBytes).Split('|');
            if (viewer.Length != 3 || (viewer[0] != "local-kf-replay-joint-realm-player-viewer-v1" && viewer[0] != "original-kf-numeric-pid-wrapper-only-v1" && viewer[0] != "local-kf-replay-same-realm-distinct-player-viewer-v1"))
                throw new InvalidOperationException("KF realm viewer rule missing");
            kfProjectionOnly = viewer[0] == "original-kf-numeric-pid-wrapper-only-v1";
            kfSameRealm = viewer[0] == "local-kf-replay-same-realm-distinct-player-viewer-v1";
            kfRealm = Int64.Parse(viewer[1], CultureInfo.InvariantCulture);
            if (kfProjectionOnly && kfRealm != 0) throw new InvalidOperationException("original numeric wrapper probe requires Zone0 context");
            kfPID = Int64.Parse(viewer[2], CultureInfo.InvariantCulture);
            using (MemoryStream input = new MemoryStream(typedBytes)) kf = ProtoBuf.Serializer.Deserialize<KFReplayData>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
            {
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
                kfDriverSHA = BitConverter.ToString(hash.ComputeHash(driverBytes)).Replace("-", "").ToLowerInvariant();
            }
        }
        SCWorldBossFight worldboss = null;
        string worldbossPath = Path.ChangeExtension(path, ".worldboss-start.bin");
        if (File.Exists(worldbossPath))
        {
            if (rush != null || arena != null || limit != null || grab != null || plunder != null || kf != null
                || forceAutomatic || localCompatibility || wire.battleType != 18
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("WorldBoss requires independently typed automatic highest18");
            byte[] typedBytes = File.ReadAllBytes(worldbossPath);
            using (MemoryStream input = new MemoryStream(typedBytes)) worldboss = ProtoBuf.Serializer.Deserialize<SCWorldBossFight>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        SCRebelFightStart wanted = null;
        string wantedPath = Path.ChangeExtension(path, ".wanted-start.bin");
        if (File.Exists(wantedPath))
        {
            if (rush != null || arena != null || limit != null || grab != null || plunder != null || kf != null || worldboss != null
                || forceAutomatic || localCompatibility || wire.battleType != 8
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("Wanted8 requires independent highest typed source");
            byte[] typedBytes = File.ReadAllBytes(wantedPath);
            using (MemoryStream input = new MemoryStream(typedBytes)) wanted = ProtoBuf.Serializer.Deserialize<SCRebelFightStart>(input);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        SCAwakeCopyFightBegin awake = null;
        string awakePath = Path.ChangeExtension(path, ".awake-start.bin");
        SidePvpFightData fieldPVP = null;
        string fieldPVPPath = Path.ChangeExtension(path, ".field-pvp.bin");
        if (File.Exists(awakePath) || File.Exists(fieldPVPPath))
        {
            if (rush != null || arena != null || limit != null || grab != null || plunder != null || kf != null || worldboss != null || wanted != null
                || forceAutomatic || localCompatibility || File.Exists(awakePath) && File.Exists(fieldPVPPath)
                || Path.GetFileName(Path.GetDirectoryName(path)) != "highest")
                throw new InvalidOperationException("Awake16/Field5 requires exactly one independent highest typed source");
            bool isAwake = File.Exists(awakePath);
            if (wire.battleType != (isAwake ? 16 : 5)) throw new InvalidOperationException("Independent typed mode differs");
            byte[] typedBytes = File.ReadAllBytes(isAwake ? awakePath : fieldPVPPath);
            using (MemoryStream input = new MemoryStream(typedBytes))
            {
                if (isAwake) awake = ProtoBuf.Serializer.Deserialize<SCAwakeCopyFightBegin>(input);
                else fieldPVP = ProtoBuf.Serializer.Deserialize<SidePvpFightData>(input);
            }
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                typedSHA = BitConverter.ToString(hash.ComputeHash(typedBytes)).Replace("-", "").ToLowerInvariant();
        }
        double manualAutoStart = -1.0;
        string manualDriverSHA = null;
        if (File.Exists(manualDriverPath))
        {
            if (rush == null || forceAutomatic || localCompatibility || (wire.optRecord == null && !scheduledManual))
                throw new InvalidOperationException("manual-to-auto requires exact typed source and observed record or explicit SHA-pinned schedule");
            byte[] driverBytes = File.ReadAllBytes(manualDriverPath);
            manualAutoStart = Double.Parse(Encoding.UTF8.GetString(driverBytes), CultureInfo.InvariantCulture);
            using (System.Security.Cryptography.SHA256 hash = System.Security.Cryptography.SHA256.Create())
                manualDriverSHA = BitConverter.ToString(hash.ComputeHash(driverBytes)).Replace("-", "").ToLowerInvariant();
        }
        string lastState = null;
        EngineData latestData = null;
        using (StreamWriter output = new StreamWriter(Path.ChangeExtension(path, ".native.jsonl"), false, new UTF8Encoding(false)))
        {
            string scope = wanted != null ? "original-wanted-automatic-ai" : awake != null ? "original-awake16-automatic-ai" : fieldPVP != null ? "original-field-pvp5-field002-automatic-ai" : worldboss != null ? "original-worldboss-automatic-ai" : kf != null ? (kfProjectionOnly ? "original-kf-numeric-pid-wrapper-only-v1" : kfSameRealm ? "local-kf-replay-same-realm-distinct-player-viewer-v1" : "local-kf-replay-joint-realm-player-viewer-v1") : manualAutoStart >= 0 ? "original-pve-manual-then-auto-ai" : plunder != null ? "original-plunder-automatic-ai" : localCompatibility ? "local-pve-no-target-finish-v1" : (wire.defender1 != null ? "original-pvp-automatic-ai" : (forceAutomatic ? "original-pve-automatic-ai" : "original-pve-playback-ai"));
            output.WriteLine("{\"kind\":\"input\",\"schemaRevision\":4,\"scope\":" + Json(scope) + ",\"fightId\":" + Json(wire.fightid) + ",\"seed\":" + wire.randSeed + ",\"battleType\":" + wire.battleType
                + ",\"manualControlSource\":" + Json(manualAutoStart < 0 ? null : wire.optRecord == null ? "prepublished-diagnostic-schedule-v1" : "actual-original-End-evidence") + ",\"typedCallbackSHA256\":" + Json(typedSHA) + ",\"typedTargetSHA256\":" + Json(targetSHA) + ",\"typedDriverPolicySHA256\":" + Json(manualPolicySHA) + ",\"typedDriverSHA256\":" + Json(kfDriverSHA ?? manualDriverSHA) + ",\"attackerFactor\":" + Number(wire.attackerFactor) + ",\"defenderFactor\":" + Number(wire.defenderFactor) + "}");
            if (kfProjectionOnly)
            {
                output.WriteLine("{\"kind\":\"wrapper_projection\",\"data\":" + OriginalHeadlessPlaybackCaller.InspectKFNumericWrapper(kf, kfPID) + "}");
                output.WriteLine("{\"kind\":\"projection_result\",\"engineExecuted\":false}");
                return;
            }
            Action<string, List<FExport>> exports =
                delegate(string kind, List<FExport> values)
                {
                    if (kind == "tick" && values.Count == 0) return;
                    // Exports are copied to text immediately before the original
                    // engine clears its mutable export list on the next tick.
                    output.WriteLine("{\"kind\":" + Json(kind)
                        + ",\"round\":" + (latestData == null ? "null" : latestData.roundNum.ToString(CultureInfo.InvariantCulture))
                        + ",\"time\":" + (latestData == null ? "null" : Number(latestData.roundTime))
                        + ",\"exports\":" + Json(values) + "}");
                };
            Action<EngineData> snapshots =
                delegate(EngineData data)
                {
                    // Keep a read-only reference even when the serialized state
                    // is unchanged, so each export carries its actual clock.
                    latestData = data;
                    string state = State(data);
                    if (state == lastState) return;
                    lastState = state;
                    output.WriteLine("{\"kind\":\"state\",\"roundTime\":" + Number(data.roundTime) + ",\"state\":" + state + "}");
                };
            Action<string, EngineData, object> events =
                delegate(string kind, EngineData data, object value)
                {
                    output.WriteLine("{\"kind\":" + Json(kind) + ",\"round\":" + data.roundNum
                        + ",\"time\":" + Number(data.roundTime) + ",\"data\":" + EventData(value) + "}");
                    // Commit caller diagnostics and preceding native rows so
                    // a stalled loop is distinguishable from a buffered tail.
                    if (kind == "ai_notify" || kind == "caller_progress" || kind == "caller_exhausted" || kind.StartsWith("compat_", StringComparison.Ordinal)) output.Flush();
                };
            bool result = wanted != null
                ? OriginalHeadlessPlaybackCaller.RunWantedAutomatic(wire, wanted, exports, snapshots, events)
                : awake != null
                ? OriginalHeadlessPlaybackCaller.RunAwakeAutomatic(wire, awake, exports, snapshots, events)
                : fieldPVP != null
                ? OriginalHeadlessPlaybackCaller.RunFieldPVPAutomatic(wire, fieldPVP, exports, snapshots, events)
                : worldboss != null
                ? OriginalHeadlessPlaybackCaller.RunWorldBossAutomatic(wire, worldboss, exports, snapshots, events)
                : grab != null
                ? OriginalHeadlessPlaybackCaller.RunGrabAutomatic(wire, grab, exports, snapshots, events)
                : limit != null
                ? OriginalHeadlessPlaybackCaller.RunLimitAutomatic(wire, limit, exports, snapshots, events)
                : kf != null
                ? (kfSameRealm ? OriginalHeadlessPlaybackCaller.RunKFSameRealmAutomatic(wire, kf, kfRealm, kfPID, exports, snapshots, events) : OriginalHeadlessPlaybackCaller.RunKFRealmAutomatic(wire, kf, kfRealm, kfPID, exports, snapshots, events))
                : manualAutoStart >= 0
                ? OriginalHeadlessPlaybackCaller.RunRushManualAutoStart(wire, rush, manualAutoStart, exports, snapshots, events)
                : plunder != null
                ? OriginalHeadlessPlaybackCaller.RunPlunderAutomatic(wire, plunder, targetFrag, exports, snapshots, events)
                : arena != null
                ? OriginalHeadlessPlaybackCaller.RunArenaAutomatic(wire, arena, exports, snapshots, events)
                : rush != null
                ? (forceAutomatic
                    ? OriginalHeadlessPlaybackCaller.RunRushAutomatic(wire, rush, exports, snapshots, events)
                    : OriginalHeadlessPlaybackCaller.RunRushRecorded(wire, rush, exports, snapshots, events))
                : localCompatibility
                ? OriginalHeadlessPlaybackCaller.RunPVECompatibility(wire, exports, snapshots, events)
                : forceAutomatic
                ? OriginalHeadlessPlaybackCaller.RunAutomatic(wire, exports, snapshots, events)
                : OriginalHeadlessPlaybackCaller.RunOneWithObservers(wire, exports, snapshots, events);
            output.WriteLine("{\"kind\":\"result\",\"attackerWon\":" + (result ? "true" : "false") + "}");
        }
    }

    // Export only the live callback identity/state, preserving comparison keys.
    // Static skill tables and recursive release lists already belong to the
    // frozen input; serializing them at every callback repeats hundreds of KB.
    private static string EventData(object value)
    {
        FSkill skill = value as FSkill;
        if (skill != null)
            return "{\"type\":" + Json(skill.GetType().FullName) + ",\"id\":" + Json(skill.id)
                + ",\"index\":" + skill.index + ",\"IsUltimate\":" + Json(skill.IsUltimate)
                + ",\"state\":" + Json(skill.state) + ",\"stepState\":" + Json(skill.stepState)
                + ",\"cooldownCur\":" + Number(skill.cooldownCur) + ",\"target\":" + Json(skill.target)
                + ",\"castTargets\":" + Json(skill.castTargets) + "}";
        FRelease release = value as FRelease;
        if (release != null)
            return "{\"type\":" + Json(release.GetType().FullName) + ",\"cast\":{\"id\":" + Json(release.cast.id)
                + ",\"skill\":{\"id\":" + Json(release.cast.skill.id) + "}},\"isCast\":" + Json(release.isCast)
                + ",\"isLast\":" + Json(release.isLast) + ",\"oritime\":" + Number(release.oritime)
                + ",\"time\":" + Number(release.time) + "}";
        return Json(value);
    }

    private static string State(EngineData data)
    {
        return "{\"round\":" + data.roundNum + ",\"camp\":" + (int)data.curRoundCamp
            + ",\"randomSeed\":" + OriginalHeadlessPlaybackCaller.ReadOriginalRandomSeed(data)
            + ",\"attacker\":" + Player(data.attacker) + ",\"defender\":" + Player(data.defender) + "}";
    }

    private static string Player(FPlayer player)
    {
        StringBuilder s = new StringBuilder("{\"anger\":" + Number(player.angerNum) + ",\"damageFactor\":" + Number(player.damageFactor) + ",\"actors\":[");
        bool first = true;
        foreach (FActor actor in player.actors)
        {
            if (!first) s.Append(','); first = false;
            s.Append("{\"uid\":").Append(Json(actor.uniqueId)).Append(",\"heroId\":").Append(Json(actor.heroId));
            s.Append(",\"state\":").Append(Json(actor.state)).Append(",\"position\":").Append(Json(actor.position));
            s.Append(",\"attribute\":").Append(Json(actor.attribute)).Append(",\"skills\":[");
            bool firstSkill = true;
            foreach (FSkill skill in actor.skills)
            {
                if (!firstSkill) s.Append(','); firstSkill = false;
                s.Append("{\"id\":").Append(Json(skill.id)).Append(",\"state\":").Append(Json(skill.state))
                    .Append(",\"cooldown\":").Append(Number(skill.cooldownCur)).Append(",\"step\":").Append(Json(skill.stepState)).Append('}');
            }
            s.Append("],\"buffs\":").Append(Json(actor.buffManager)).Append(",\"specialState\":").Append(Json(actor.speState))
                .Append(",\"unableStates\":").Append(Json(actor.unableStates)).Append('}');
        }
        return s.Append("]}").ToString();
    }

    private static string Number(object value)
    {
        if (value is double) return ((double)value).ToString("R", CultureInfo.InvariantCulture);
        if (value is float) return ((float)value).ToString("R", CultureInfo.InvariantCulture);
        return Convert.ToString(value, CultureInfo.InvariantCulture);
    }

    private static string Json(object value) { return Json(value, 0); }
    private static string Json(object value, int depth)
    {
        if (value == null) return "null";
        if (value is string)
        {
            StringBuilder quoted = new StringBuilder("\"");
            foreach (char c in (string)value)
            {
                if (c == '\\' || c == '"') quoted.Append('\\').Append(c);
                else if (c < 32) quoted.Append("\\u").Append(((int)c).ToString("x4"));
                else quoted.Append(c);
            }
            return quoted.Append('"').ToString();
        }
        if (value is bool) return (bool)value ? "true" : "false";
        Type type = value.GetType();
        if (type.IsEnum) return Convert.ToInt64(value).ToString(CultureInfo.InvariantCulture);
        if (type.IsPrimitive || value is decimal) return Number(value);
        FActor actor = value as FActor;
        if (actor != null) return "{\"actor\":" + Json(actor.uniqueId) + ",\"camp\":" + (int)actor.player.camp + "}";
        FPlayer player = value as FPlayer;
        if (player != null) return "{\"camp\":" + (int)player.camp + "}";
        if (depth > 6) return "{\"truncatedType\":" + Json(type.FullName) + "}";
        IDictionary dictionary = value as IDictionary;
        if (dictionary != null)
        {
            StringBuilder entries = new StringBuilder("{"); bool firstEntry = true;
            foreach (DictionaryEntry entry in dictionary)
            {
                if (!firstEntry) entries.Append(','); firstEntry = false;
                entries.Append(Json(Convert.ToString(entry.Key, CultureInfo.InvariantCulture)))
                    .Append(':').Append(Json(entry.Value, depth + 1));
            }
            return entries.Append('}').ToString();
        }
        IEnumerable list = value as IEnumerable;
        if (list != null)
        {
            StringBuilder array = new StringBuilder("["); bool first = true;
            foreach (object element in list) { if (!first) array.Append(','); first = false; array.Append(Json(element, depth + 1)); }
            return array.Append(']').ToString();
        }
        StringBuilder result = new StringBuilder("{\"type\":" + Json(type.FullName));
        FieldInfo[] fields = type.GetFields(BindingFlags.Public | BindingFlags.Instance);
        Array.Sort(fields, delegate(FieldInfo a, FieldInfo b) { return StringComparer.Ordinal.Compare(a.Name, b.Name); });
        foreach (FieldInfo field in fields)
        {
            if (field.Name == "actor" || field.Name == "player" || field.Name == "compute" || field.Name == "engineData" || field.Name == "callback") continue;
            result.Append(',').Append(Json(field.Name)).Append(':').Append(Json(field.GetValue(value), depth + 1));
        }
        return result.Append('}').ToString();
    }
}
