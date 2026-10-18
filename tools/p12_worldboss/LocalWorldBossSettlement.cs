// Named local compatibility adapter. Shared Boss HP commits only after the
// original type18 end handler; the original battle calculator remains intact.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Reflection;
using System.Text;
using LitJson;
using ProtoBuf.Message;
using UnityEngine;

public sealed class LocalWorldBossSettlement : MonoBehaviour
{
    private static bool adapterEnabled;
    private static string origin;
    private static LocalWorldBossSettlement instance;
    private Job current;
    private readonly HashSet<string> completed = new HashSet<string>();
    private long scopePlayer;
    private byte[] scopeSession;
    private object blockedOwner;
    private SCWorldBossFight blockedStart;
    private long blockedPlayer;
    private Recovery recovery;
    private object checkedFieldNet;
    private string checkedScene;
    private bool recoveryChecked;

    private sealed class Recovery
    {
        public object net, battle, start;
        public long player;
        public byte[] session;
        public string scene, fieldId, server, pendingScene, status;
        public SCWorldBossFight pendingStart;
        public bool running, complete, mustRefresh;
    }

    private sealed class Job
    {
        public object owner, battle, fieldNet;
        public SCWorldBossFight start;
        public Delegate callback;
        public long player;
        public byte[] session;
        public string fightId, server;
        public bool running, finished;
        public string status;
    }

    // Configure is injected only by the explicit local APK build option.
    public static void Configure(string baseURI, bool active)
    {
        Uri uri;
        if (!Uri.TryCreate(baseURI, UriKind.Absolute, out uri) ||
            (uri.Scheme != "http" && uri.Scheme != "https") ||
            !String.IsNullOrEmpty(uri.UserInfo) || !String.IsNullOrEmpty(uri.Query) ||
            !String.IsNullOrEmpty(uri.Fragment) || uri.AbsolutePath != "/")
            throw new ArgumentException("Invalid local world boss origin");
        string next = uri.GetLeftPart(UriPartial.Authority);
        if (instance != null && (!active || origin != next)) instance.Cancel();
        origin = next;
        adapterEnabled = active;
    }

    private static LocalWorldBossSettlement Host()
    {
        if (instance == null)
        {
            GameObject obj = new GameObject("LocalWorldBossSettlement");
            UnityEngine.Object.DontDestroyOnLoad(obj);
            instance = obj.AddComponent<LocalWorldBossSettlement>();
        }
        return instance;
    }

    private static long Player() { return StaticData.playerData == null ? 0 : StaticData.playerData.playerId; }
    private static byte[] Session() { return StaticData.sessionKey == null ? new byte[0] : (byte[])StaticData.sessionKey.Clone(); }
    private static bool Same(long player, byte[] session)
    {
        return LocalWorldBossRecoveryPolicy.SameSession(player, session, Player(), StaticData.sessionKey);
    }
    private static Dictionary<string, string> Headers(long player, byte[] session)
    {
        Dictionary<string, string> headers = new Dictionary<string, string>();
        headers["X-Hyjs-Player-ID"] = player.ToString(CultureInfo.InvariantCulture);
        headers["X-Hyjs-Session-Key"] = Convert.ToBase64String(session);
        headers["Content-Type"] = "application/json";
        headers["Cache-Control"] = "no-store";
        return headers;
    }
    private static Delegate EndCallback(object owner)
    {
        FieldInfo field = owner.GetType().GetField("onResult", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        return field == null ? null : field.GetValue(owner) as Delegate;
    }
    private static object ExistingFieldNet()
    {
        Type manager = typeof(Room.RoomManager);
        FieldInfo field = manager.GetField("sceneLogics", BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic);
        IDictionary rooms = field == null ? null : field.GetValue(null) as IDictionary;
        MethodInfo getter = manager.GetMethod("GetRoom", BindingFlags.Static | BindingFlags.Public);
        if (rooms == null || getter == null) return null;
        object key = Enum.ToObject(getter.GetParameters()[0].ParameterType, 4);
        // GetRoom creates absent rooms. Read only the already-loaded field room.
        Room.RoomBase room = rooms.Contains(key) ? rooms[key] as Room.RoomBase : null;
        return room == null ? null : room.netManager as Room.RoomNetField;
    }
    private static bool FieldReady(object net)
    {
        Room.RoomBase room = Room.RoomManager.currentRoom;
        return (int)Room.RoomManager.curSceneType == 4 && room != null && room.foreGround &&
            System.Object.ReferenceEquals(room.netManager, net) && System.Object.ReferenceEquals(ExistingFieldNet(), net);
    }
    private static bool PrivateRun()
    {
        foreach (Assembly assembly in AppDomain.CurrentDomain.GetAssemblies())
        {
            Type runner = assembly.GetType("NativeBatchRunner", false);
            if (runner == null) continue;
            FieldInfo running = runner.GetField("running", BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic);
            if (running != null && running.FieldType == typeof(bool) && (bool)running.GetValue(null)) return true;
        }
        return false;
    }
    private void SetScope(long player, byte[] session)
    {
        if (!Same(scopePlayer, scopeSession))
        {
            Cancel();
            scopePlayer = player;
            scopeSession = session;
        }
    }

    // Called only after the original field position handler completed its self,
    // side, RoomLoadEnd and _CompleteTownToField work. Never enters a new room.
    public static void ObserveFieldReady(object owner, object response)
    {
        if (!adapterEnabled) return;
        Room.RoomNetField net = owner as Room.RoomNetField;
        SCCurrentPosition position = response as SCCurrentPosition;
        long player = Player();
        byte[] session = Session();
        if (net == null || position == null || position.self == null || position.self.position == null ||
            position.self.info == null || position.self.info.playerId != player || position.self.info.playerSide == null ||
            !Same(player, session) || !FieldReady(net) || PrivateRun()) return;
        string scene = position.self.position.scene;
        if (!LocalWorldBossRecoveryPolicy.FieldScene(scene)) return;
        LocalWorldBossSettlement host = Host();
        host.SetScope(player, session);
        if (host.current != null && !host.current.finished) return;
        if (host.recoveryChecked && System.Object.ReferenceEquals(host.checkedFieldNet, net) && host.checkedScene == scene) return;
        host.checkedFieldNet = net;
        host.checkedScene = scene;
        host.recoveryChecked = true;
        Recovery state = new Recovery();
        state.net = net;
        state.battle = StaticData.curBattleData;
        state.start = StaticData.pveWorldBossEnd;
        state.player = player;
        state.session = session;
        state.scene = scene;
        state.fieldId = StaticData.fieldId;
        state.server = origin;
        host.recovery = state;
        host.BeginRecovery(state);
    }
    private bool IsRecoveryCurrent(Recovery state)
    {
        return adapterEnabled && origin == state.server && System.Object.ReferenceEquals(recovery, state) &&
            Same(state.player, state.session) && state.fieldId == StaticData.fieldId &&
            LocalWorldBossRecoveryPolicy.CanRestore(state.scene, state.scene, state.net,
                Room.RoomManager.currentRoom == null ? null : Room.RoomManager.currentRoom.netManager,
                state.battle, StaticData.curBattleData, state.start, StaticData.pveWorldBossEnd,
                FieldReady(state.net), PrivateRun());
    }
    private void BeginRecovery(Recovery state)
    {
        if (!IsRecoveryCurrent(state) || state.running || state.complete) return;
        state.running = true;
        state.status = "正在检查未结束的世界首领挑战…";
        StartCoroutine(CheckRecovery(state));
    }
    private static JsonData Data(string text)
    {
        if (String.IsNullOrEmpty(text) || text.Length > 16000000) throw new InvalidOperationException();
        JsonData envelope = JsonMapper.ToObject(text);
        if (Convert.ToInt32(envelope["code"].ToString(), CultureInfo.InvariantCulture) != 0) throw new InvalidOperationException();
        return envelope["data"];
    }
    private static SCWorldBossFight StartFrame(JsonData data, long player)
    {
        byte[] raw = Convert.FromBase64String((string)data["startBase64"]);
        if (raw.Length == 0 || raw.Length > 8000000) throw new InvalidOperationException();
        SCWorldBossFight result;
        using (MemoryStream stream = new MemoryStream(raw)) result = ProtoBuf.Serializer.Deserialize<SCWorldBossFight>(stream);
        if (result == null || String.IsNullOrEmpty(result.fightId) || result.fightId.Length > 64 ||
            (result.code != 1 && result.code != 2) || result.playerSide == null || result.playerSide.playerId != player)
            throw new InvalidOperationException();
        return result;
    }
    private IEnumerator CheckRecovery(Recovery state)
    {
        // One frame lets the original room callback and queued side finish.
        yield return null;
        for (int attempt = 0; attempt < 3 && IsRecoveryCurrent(state); attempt++)
        {
            bool valid = false;
            bool pending = false;
            SCWorldBossFight start = null;
            string scene = null;
            using (WWW request = new WWW(state.server + "/world-boss/resume", Encoding.UTF8.GetBytes("{}"), Headers(state.player, state.session)))
            {
                float deadline = Time.realtimeSinceStartup + 15;
                while (!request.isDone && Time.realtimeSinceStartup < deadline && IsRecoveryCurrent(state)) yield return null;
                if (!IsRecoveryCurrent(state)) yield break;
                if (request.isDone && String.IsNullOrEmpty(request.error))
                {
                    try
                    {
                        JsonData data = Data(request.text);
                        pending = (bool)data["pending"];
                        if (pending) { start = StartFrame(data, state.player); scene = (string)data["scene"]; if (String.IsNullOrEmpty(scene)) throw new InvalidOperationException(); }
                        valid = true;
                    }
                    catch { Debug.LogWarning("Local world boss recovery response was rejected"); }
                }
            }
            if (!IsRecoveryCurrent(state)) yield break;
            if (valid)
            {
                state.running = false;
                state.mustRefresh = false;
                state.pendingStart = start;
                state.pendingScene = scene;
                state.complete = !pending;
                state.status = pending ? "发现尚未结束的世界首领挑战。" : null;
                // Expired pending frames can be released by /resume. Refresh
                // through the original authenticated current-position route.
                if (!pending) ((Room.RoomNetField)state.net).SendCSCurrentPosition(null);
                yield break;
            }
            if (attempt < 2)
            {
                float retryAt = Time.realtimeSinceStartup + attempt + 1;
                while (Time.realtimeSinceStartup < retryAt && IsRecoveryCurrent(state)) yield return null;
            }
        }
        if (!IsRecoveryCurrent(state)) yield break;
        state.running = false;
        state.mustRefresh = true;
        state.status = "未完成挑战状态暂时无法读取，请重试。";
    }
    private void Restore(Recovery state)
    {
        if (!IsRecoveryCurrent(state) || state.running || state.mustRefresh || state.pendingStart == null ||
            !LocalWorldBossRecoveryPolicy.CanRestore(state.scene, state.pendingScene, state.net,
                Room.RoomManager.currentRoom.netManager, state.battle, StaticData.curBattleData,
                state.start, StaticData.pveWorldBossEnd, FieldReady(state.net), PrivateRun())) return;
        MethodInfo handler = state.net.GetType().GetMethod("SCWorldBossFight", BindingFlags.Instance | BindingFlags.NonPublic);
        if (handler == null) { state.mustRefresh = true; state.status = "世界首领恢复入口暂不可用，请重新检查。"; return; }
        // Consume the selection before invoking the original scene transition.
        // No second start request, number consumption or client damage exists.
        state.complete = true;
        try { handler.Invoke(state.net, new object[] { state.pendingStart }); }
        catch { Debug.LogError("Local world boss original recovery entry failed"); }
    }

    // True consumes End. The original callback is held until an authenticated
    // finish succeeds, including when Skip reached the same original end path.
    public static bool TryFinish(object owner)
    {
        if (!adapterEnabled) return false;
        if (owner == null || owner.GetType().FullName != "Battle.FieldPVEWorldBossBattleOver") return false;
        if (StaticData.curBattleData == null || (int)StaticData.curBattleData.battleType != 18 || StaticData.curBattleData.isPlayBack) return false;
        LocalWorldBossSettlement host = Host();
        long player = Player();
        byte[] session = Session();
        host.SetScope(player, session);
        SCWorldBossFight start = StaticData.pveWorldBossEnd;
        if (start == null || String.IsNullOrEmpty(start.fightId) || start.fightId.Length > 128 ||
            start.playerSide == null || start.playerSide.playerId != player || !Same(player, session))
        {
            // An enabled local adapter must never resume the original callback
            // with an uncommitted result or guess a player's current pending ID.
            Debug.LogError("Local world boss settlement has no valid authenticated fight");
            host.CancelCurrent();
            host.blockedOwner = owner;
            host.blockedStart = start;
            host.blockedPlayer = player;
            return true;
        }
        host.blockedOwner = null;
        host.blockedStart = null;
        if (host.completed.Contains(start.fightId)) return true;
        if (host.current != null && host.current.fightId == start.fightId)
        {
            if (!host.current.running && !host.current.finished && host.IsCurrent(host.current)) host.Begin(host.current);
            return true;
        }
        host.CancelCurrent();
        Job job = new Job();
        job.owner = owner;
        job.battle = StaticData.curBattleData;
        job.fieldNet = ExistingFieldNet();
        job.start = start;
        job.callback = EndCallback(owner);
        job.player = player;
        job.session = session;
        job.fightId = start.fightId;
        job.server = origin;
        job.status = "正在结算世界首领伤害…";
        host.current = job;
        host.Begin(job);
        return true;
    }

    private bool IsCurrent(Job job)
    {
        return adapterEnabled && origin == job.server && System.Object.ReferenceEquals(current, job) &&
            Same(job.player, job.session) && System.Object.ReferenceEquals(StaticData.curBattleData, job.battle) &&
            System.Object.ReferenceEquals(StaticData.pveWorldBossEnd, job.start) &&
            StaticData.pveWorldBossEnd.fightId == job.fightId;
    }

    private void Begin(Job job)
    {
        if (!IsCurrent(job) || job.running || job.finished) return;
        job.running = true;
        job.status = "正在结算世界首领伤害…";
        StartCoroutine(Commit(job));
    }

    private IEnumerator Commit(Job job)
    {
        JsonData payload = new JsonData();
        payload["fightId"] = job.fightId;
        byte[] body = Encoding.UTF8.GetBytes(JsonMapper.ToJson(payload));
        for (int attempt = 0; attempt < 3 && IsCurrent(job); attempt++)
        {
            SCWorldBossFight result = null;
            using (WWW request = new WWW(job.server + "/world-boss/finish", body, Headers(job.player, job.session)))
            {
                float deadline = Time.realtimeSinceStartup + 15;
                while (!request.isDone && Time.realtimeSinceStartup < deadline && IsCurrent(job)) yield return null;
                if (!IsCurrent(job)) yield break;
                if (request.isDone && String.IsNullOrEmpty(request.error))
                {
                    try { result = Decode(request.text, job); }
                    catch { Debug.LogWarning("Local world boss settlement response was rejected"); }
                }
            }
            if (!IsCurrent(job)) yield break;
            if (result != null)
            {
                Apply(job, result);
                yield break;
            }
            if (attempt < 2)
            {
                float retryAt = Time.realtimeSinceStartup + attempt + 1;
                while (Time.realtimeSinceStartup < retryAt && IsCurrent(job)) yield return null;
            }
        }
        if (!IsCurrent(job)) yield break;
        job.running = false;
        job.status = "世界首领结算暂未完成，请重试。";
    }

    private static SCWorldBossFight Decode(string text, Job job)
    {
        JsonData data = Data(text);
        long hp = Convert.ToInt64(data["currentHp"].ToString(), CultureInfo.InvariantCulture);
        long total = Convert.ToInt64(data["totalHp"].ToString(), CultureInfo.InvariantCulture);
        long applied = Convert.ToInt64(data["appliedDamage"].ToString(), CultureInfo.InvariantCulture);
        if (!data["alreadyCommitted"].IsBoolean) throw new InvalidOperationException();
        if (hp < 0 || total <= 0 || hp > total || applied < 0) throw new InvalidOperationException();
        SCWorldBossFight result = StartFrame(data, job.player);
        if (result.fightId != job.fightId || result.randSeed != job.start.randSeed)
            throw new InvalidOperationException();
        return result;
    }

    private void BeginAbandon(Job job)
    {
        if (!IsCurrent(job) || job.running || job.finished) return;
        job.running = true;
        job.status = "正在放弃本次挑战…";
        StartCoroutine(Abandon(job, null));
    }
    private void BeginAbandon(Recovery state)
    {
        if (!IsRecoveryCurrent(state) || state.running || state.complete || state.pendingStart == null) return;
        state.running = true;
        // A lost acknowledgement must be rechecked before any cached playback
        // can resume; the server may already have accepted this abandonment.
        state.mustRefresh = true;
        state.status = "正在放弃本次挑战…";
        StartCoroutine(Abandon(null, state));
    }
    private bool AbandonScope(Job job, Recovery state)
    {
        return job != null ? IsCurrent(job) && !job.finished : IsRecoveryCurrent(state) && !state.complete;
    }
    private IEnumerator Abandon(Job job, Recovery state)
    {
        long player = job != null ? job.player : state.player;
        byte[] session = job != null ? job.session : state.session;
        string fightId = job != null ? job.fightId : state.pendingStart.fightId;
        string server = job != null ? job.server : state.server;
        JsonData payload = new JsonData();
        payload["fightId"] = fightId;
        byte[] body = Encoding.UTF8.GetBytes(JsonMapper.ToJson(payload));
        for (int attempt = 0; attempt < 3 && AbandonScope(job, state); attempt++)
        {
            bool abandoned = false;
            using (WWW request = new WWW(server + "/world-boss/abandon", body, Headers(player, session)))
            {
                float deadline = Time.realtimeSinceStartup + 15;
                while (!request.isDone && Time.realtimeSinceStartup < deadline && AbandonScope(job, state)) yield return null;
                if (!AbandonScope(job, state)) yield break;
                if (request.isDone && String.IsNullOrEmpty(request.error))
                {
                    try { abandoned = (bool)Data(request.text)["abandoned"]; }
                    catch { Debug.LogWarning("Local world boss abandon response was rejected"); }
                }
            }
            if (!AbandonScope(job, state)) yield break;
            if (abandoned)
            {
                completed.Add(fightId);
                Room.RoomNetField net = (Room.RoomNetField)(job != null ? job.fieldNet : state.net);
                if (job != null)
                {
                    job.callback = null;
                    job.finished = true;
                    job.running = false;
                    job.status = null;
                    // This is the same original return used by the Boss result
                    // page, only after a server-confirmed abandonment.
                    ManagerController.Instance.BattleToField();
                }
                else
                {
                    state.complete = true;
                    state.running = false;
                    state.pendingStart = null;
                    state.status = null;
                }
                if (Same(player, session) && FieldReady(net)) net.SendCSCurrentPosition(null);
                yield break;
            }
            if (attempt < 2)
            {
                float retryAt = Time.realtimeSinceStartup + attempt + 1;
                while (Time.realtimeSinceStartup < retryAt && AbandonScope(job, state)) yield return null;
            }
        }
        if (!AbandonScope(job, state)) yield break;
        if (job != null) { job.running = false; job.status = "放弃尚未确认，可重试结算或放弃。"; }
        else { state.running = false; state.status = "放弃尚未确认，请重新检查挑战状态或重试放弃。"; }
    }

    private static bool QueueFinishedSide(Job job, PlayerSide side)
    {
        object net = ExistingFieldNet();
        if (job.fieldNet == null || !System.Object.ReferenceEquals(job.fieldNet, net)) return false;
        MethodInfo handler = net.GetType().GetMethod("SCFieldPlayerState", BindingFlags.Instance | BindingFlags.NonPublic);
        if (handler == null) return false;
        SCFieldPlayerState update = new SCFieldPlayerState();
        update.player.Add(side);
        handler.Invoke(net, new object[] { update });
        return true;
    }

    private void Apply(Job job, SCWorldBossFight result)
    {
        if (!IsCurrent(job) || job.finished) return;
        // Reuse the original full-state receive callback. Its queue may already
        // have consumed the initial snapshot; a fresh complete side must follow.
        try
        {
            if (!QueueFinishedSide(job, result.playerSide)) throw new InvalidOperationException();
        }
        catch
        {
            job.running = false;
            job.status = "世界首领状态尚未同步，请重试结算。";
            return;
        }
        StaticData.pveWorldBossEnd = result;
        StaticData.lastBattleEndMsg = result;
        StaticData.isLastBattleWin = result.code == 1;
        job.finished = true;
        job.running = false;
        job.status = null;
        completed.Add(job.fightId);
        Delegate callback = job.callback;
        job.callback = null;
        if (callback != null)
        {
            try { callback.DynamicInvoke(StaticData.isLastBattleWin); }
            catch { Debug.LogError("Local world boss original result callback failed"); }
        }
    }

    private void CancelCurrent()
    {
        StopAllCoroutines();
        if (current != null) current.callback = null;
        current = null;
    }
    private void Cancel()
    {
        CancelCurrent();
        completed.Clear();
        scopePlayer = 0;
        scopeSession = null;
        blockedOwner = null;
        blockedStart = null;
        recovery = null;
        checkedFieldNet = null;
        checkedScene = null;
        recoveryChecked = false;
    }
    private void Update()
    {
        if (current != null && !current.finished && !IsCurrent(current)) CancelCurrent();
        if (recovery != null && !recovery.complete && !IsRecoveryCurrent(recovery)) recovery = null;
        if (scopePlayer > 0 && (current != null || recovery != null) && !Same(scopePlayer, scopeSession)) Cancel();
    }
    private void OnGUI()
    {
        Job job = current;
        Recovery state = recovery;
        if (state != null && !state.complete && IsRecoveryCurrent(state))
        {
            float boxWidth = Math.Min(650, Screen.width - 24);
            GUILayout.BeginArea(new Rect((Screen.width - boxWidth) / 2, Screen.height / 2 - 105, boxWidth, 210), GUI.skin.box);
            GUILayout.Label(state.status);
            if (!state.running)
            {
                bool matchingScene = state.scene == state.pendingScene;
                if (state.pendingStart != null && !matchingScene) GUILayout.Label("该挑战属于其他场景，可放弃后重新挑战。当前场景不会自动切换。");
                if (state.pendingStart != null && !state.mustRefresh && matchingScene && GUILayout.Button("继续世界首领战斗")) Restore(state);
                if ((state.mustRefresh || state.pendingStart == null) && GUILayout.Button("重新检查挑战状态")) BeginRecovery(state);
                if (state.pendingStart != null && GUILayout.Button("放弃本次挑战（不返还次数）")) BeginAbandon(state);
                if (GUILayout.Button("返回登录")) { Cancel(); ManagerController.Instance.BackToLogin(); }
            }
            GUILayout.EndArea();
            return;
        }
        if (blockedOwner != null && job == null && blockedPlayer == Player() && System.Object.ReferenceEquals(blockedStart, StaticData.pveWorldBossEnd))
        {
            float boxWidth = Math.Min(600, Screen.width - 24);
            GUILayout.BeginArea(new Rect((Screen.width - boxWidth) / 2, Screen.height / 2 - 80, boxWidth, 160), GUI.skin.box);
            GUILayout.Label("世界首领结算凭据暂不可用，请重试或返回登录。未确认的伤害不会在此处重复计算。");
            if (GUILayout.Button("重试结算")) TryFinish(blockedOwner);
            if (GUILayout.Button("返回登录")) { Cancel(); ManagerController.Instance.BackToLogin(); }
            GUILayout.EndArea();
            return;
        }
        if (job == null || job.finished || !IsCurrent(job)) return;
        float width = Math.Min(600, Screen.width - 24);
        GUILayout.BeginArea(new Rect((Screen.width - width) / 2, Screen.height / 2 - 80, width, 160), GUI.skin.box);
        GUILayout.Label(job.status);
        if (!job.running)
        {
            if (GUILayout.Button("重试结算")) Begin(job);
            if (GUILayout.Button("放弃本次挑战（不返还次数）")) BeginAbandon(job);
            if (GUILayout.Button("返回登录")) { Cancel(); ManagerController.Instance.BackToLogin(); }
        }
        GUILayout.EndArea();
    }
    private void OnDestroy() { Cancel(); if (instance == this) instance = null; }
}
