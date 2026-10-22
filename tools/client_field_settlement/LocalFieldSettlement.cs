// Named local recovery adapter for the original type12 field-boss settlement.
// The original AskFieldPVEFightResult sends opcode 30085 once and waits for
// opcode 30086 forever. This adapter keeps that original request/registration
// untouched and only adds a bounded resend of the same frozen request plus an
// original-style error dialog when the server still has not answered.
using System;
using System.Collections;
using System.IO;
using System.Reflection;
using ProtoBuf.Message;
using UnityEngine;

public sealed class LocalFieldSettlement : MonoBehaviour
{
    private const int SendOpcode = 30085;
    private const float WaitSeconds = 10f;
    private const int MaxAutoResend = 2;

    private static LocalFieldSettlement instance;
    private static Job current;

    private sealed class Job
    {
        public object owner;
        public CSSidePveBossFightEnd request;
        public BattleData battle;
        public long player;
        public byte[] session;
        public string fightId;
        public bool running;
        public bool finished;
        public bool dialogShown;
        public int resends;
        public float deadline;
        public GameObject dialog;
    }

    // Observe is injected after the original SendSocket(30085, request). It
    // freezes the exact request that was just sent and starts the bounded wait.
    public static void Observe(object owner, object request)
    {
        CSSidePveBossFightEnd original = request as CSSidePveBossFightEnd;
        BattleData battle = StaticData.curBattleData;
        long player = Player();
        byte[] session = Session();
        if (owner == null || original == null || battle == null || (int)battle.battleType != 12 ||
            battle.isPlayBack || String.IsNullOrEmpty(original.fightId) || original.fightId.Length > 128 ||
            String.IsNullOrEmpty(original.uuid) || !Same(player, session))
            return;
        LocalFieldSettlement host = Host();
        if (current != null && !current.finished && current.fightId == original.fightId &&
            current.player == player && System.Object.ReferenceEquals(current.battle, battle) &&
            Same(current.player, current.session))
            return;
        host.CancelCurrent();
        Job job = new Job();
        job.owner = owner;
        job.request = Freeze(original);
        job.battle = battle;
        job.player = player;
        job.session = session;
        job.fightId = original.fightId;
        job.running = true;
        job.deadline = Time.realtimeSinceStartup + WaitSeconds;
        current = job;
        host.StartCoroutine(host.Wait(job));
    }

    // Completed is injected at the first instruction of the original success
    // handler. It stops resends and closes our dialog before the original
    // handler stores StaticData.pveBossEnd, unregisters 30086 and calls End.
    public static void Completed(object owner, object response)
    {
        Job job = current;
        SCSidePveBossFightEnd ended = response as SCSidePveBossFightEnd;
        if (job == null || ended == null || !IsCurrent(job)) return;
        if (job.owner != null && !System.Object.ReferenceEquals(job.owner, owner)) return;
        job.finished = true;
        job.running = false;
        current = null;
        CloseDialog(job);
    }

    private static CSSidePveBossFightEnd Freeze(CSSidePveBossFightEnd source)
    {
        // Serialize and read back so later changes to the live message cannot
        // alter the identity or payload of a resend.
        using (MemoryStream stream = new MemoryStream())
        {
            ProtoBuf.Serializer.Serialize<CSSidePveBossFightEnd>(stream, source);
            stream.Position = 0;
            return ProtoBuf.Serializer.Deserialize<CSSidePveBossFightEnd>(stream);
        }
    }

    private static bool IsCurrent(Job job)
    {
        if (job == null || job.finished || !System.Object.ReferenceEquals(current, job)) return false;
        BattleData battle = StaticData.curBattleData;
        if (battle == null || !System.Object.ReferenceEquals(battle, job.battle)) return false;
        if ((int)battle.battleType != 12 || battle.isPlayBack) return false;
        if (battle.fightId != job.fightId) return false;
        return Same(job.player, job.session);
    }

    private static long Player()
    {
        return StaticData.playerData == null ? 0 : StaticData.playerData.playerId;
    }

    private static byte[] Session()
    {
        return StaticData.sessionKey == null ? new byte[0] : (byte[])StaticData.sessionKey.Clone();
    }

    private static bool Same(long player, byte[] session)
    {
        long livePlayer = Player();
        byte[] liveSession = StaticData.sessionKey;
        if (player <= 0 || player != livePlayer) return false;
        if (session == null || liveSession == null || session.Length != 16 || liveSession.Length != 16) return false;
        int diff = 0;
        for (int i = 0; i < 16; i++) diff |= session[i] ^ liveSession[i];
        return diff == 0;
    }

    private static LocalFieldSettlement Host()
    {
        if (instance == null)
        {
            GameObject obj = new GameObject("LocalFieldSettlement");
            UnityEngine.Object.DontDestroyOnLoad(obj);
            instance = obj.AddComponent<LocalFieldSettlement>();
        }
        return instance;
    }

    private IEnumerator Wait(Job job)
    {
        while (IsCurrent(job))
        {
            if (Time.realtimeSinceStartup >= job.deadline)
            {
                if (job.resends < MaxAutoResend)
                {
                    job.resends++;
                    Debug.LogWarning("Local field boss settlement timed out; resending the same request");
                    Resend(job);
                    job.deadline = Time.realtimeSinceStartup + WaitSeconds;
                }
                else
                {
                    job.running = false;
                    ShowDialog(job);
                    yield break;
                }
            }
            yield return null;
        }
    }

    private static void Resend(Job job)
    {
        if (!IsCurrent(job)) return;
        try
        {
            // The original 30086 handler stays registered; only the request is
            // sent again. Nothing else is registered or recomputed.
            NetAPI.SendSocket(SendOpcode, job.request);
        }
        catch (Exception error)
        {
            Debug.LogWarning("Local field boss settlement resend failed: " + error.Message);
        }
    }

    private void ShowDialog(Job job)
    {
        if (!IsCurrent(job) || job.dialogShown) return;
        job.dialogShown = true;
        try
        {
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton(
                "首领结算暂未完成，服务器尚未返回结果。可重试同一结算请求，或取消返回登录。",
                true,
                new BoxManager.MessageBoxDelegate(delegate(bool confirmed) { OnDialog(job, confirmed); }),
                "首领结算",
                0);
            job.dialog = BoxObject();
        }
        catch (Exception error)
        {
            job.dialogShown = false;
            Debug.LogError("Local field boss settlement dialog failed: " + error.Message);
        }
    }

    private void OnDialog(Job job, bool confirmed)
    {
        if (!IsCurrent(job)) return;
        job.dialog = null;
        job.dialogShown = false;
        if (!confirmed)
        {
            CancelCurrent();
            ManagerController.Instance.BackToLogin();
            return;
        }
        job.resends = 0;
        job.running = true;
        job.deadline = Time.realtimeSinceStartup + WaitSeconds;
        Resend(job);
        StartCoroutine(Wait(job));
    }

    private static GameObject BoxObject()
    {
        // BoxManager keeps the box it created in its private static m_kBox.
        FieldInfo field = typeof(BoxManager).GetField("m_kBox",
            BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic);
        return field == null ? null : field.GetValue(null) as GameObject;
    }

    private static void CloseDialog(Job job)
    {
        GameObject box = job.dialog;
        job.dialog = null;
        job.dialogShown = false;
        if (box == null) return;
        try
        {
            // Only destroy the box this job created; another box belongs to the
            // original UI and must keep its own lifetime.
            if (System.Object.ReferenceEquals(BoxObject(), box)) UnityEngine.Object.Destroy(box);
        }
        catch (Exception error)
        {
            Debug.LogWarning("Local field boss settlement dialog cleanup failed: " + error.Message);
        }
    }

    private void CancelCurrent()
    {
        Job job = current;
        current = null;
        if (job == null) return;
        job.finished = true;
        job.running = false;
        CloseDialog(job);
    }

    private void Update()
    {
        if (current != null && !IsCurrent(current)) CancelCurrent();
    }

    private void OnDestroy()
    {
        if (instance == this)
        {
            instance = null;
            current = null;
        }
    }
}
