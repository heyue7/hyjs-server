using System;
using System.Text;
using UnityEngine;

// The overlay receives the current game's own session in memory. It never
// creates another login or stores account credentials in the APK or on disk.
public sealed class LocalWalletSessionBridge : MonoBehaviour
{
    private static LocalWalletSessionBridge instance;
    private static string blockedSession;
    private string lastSession;
    private long lastPlayer;
    private float nextTick;

    public static void Install()
    {
        try
        {
            if (instance != null) return;
            GameObject owner = new GameObject("LocalWalletSession");
            DontDestroyOnLoad(owner);
            instance = owner.AddComponent<LocalWalletSessionBridge>();
        }
        catch (Exception) { Debug.LogWarning("Local wallet session bridge unavailable"); }
    }

    // A login/account switch invalidates the visible wallet immediately. A
    // retained StaticData value cannot rebind the previous account's session.
    public static void Pause()
    {
        blockedSession = Hex(StaticData.sessionKey);
        if (instance == null) return;
        instance.lastSession = null;
        instance.lastPlayer = 0;
        Bind(0, "");
    }

    private static string Hex(byte[] bytes)
    {
        if (bytes == null || bytes.Length != 16) return "";
        StringBuilder value = new StringBuilder(32);
        foreach (byte part in bytes) value.Append(part.ToString("x2"));
        return value.ToString();
    }

    // The native account page calls this only after the logout endpoint acknowledges release.
    public void LogoutAccount(string ignored)
    {
        Pause();
        ManagerController manager = ManagerController.Instance;
        if (manager != null) manager.BackToLogin();
    }

    private static void Bind(long player, string session)
    {
        try
        {
            using (AndroidJavaClass wallet = new AndroidJavaClass("com.hyjs.wallet.WalletActivity"))
                wallet.CallStatic("bindSession", player, session);
        }
        catch (Exception) { Debug.LogWarning("Local wallet native binding unavailable"); }
    }

    private void Update()
    {
        if (Time.realtimeSinceStartup < nextTick) return;
        nextTick = Time.realtimeSinceStartup + .5f;
        long player = StaticData.playerData == null ? 0 : StaticData.playerData.playerId;
        string session = Hex(StaticData.sessionKey);
        if (player <= 0 || session.Length != 32 || session == blockedSession)
        {
            player = 0;
            session = "";
        }
        // Catch at the caller too: optional helper JIT/type loading can fail
        // before the helper's own try block is entered on the old Unity CLR.
        try { PollFormationUI(player, session); }
        catch (Exception error)
        {
            try { FormationLog("bootstrap-load", error.GetType().FullName); }
            catch (Exception) { }
        }
        if (lastPlayer == player && lastSession == session) return;
        lastPlayer = player;
        lastSession = session;
        Bind(player, session);
    }

    // Optional UI bootstrap: reflection isolates a missing/invalid module from
    // original ArrayPage JIT, wallet binding and all original business methods.
    private object formationPage;
    private readonly string[] formationDiagnostics = new string[16];
    private int formationDiagnosticCount;

    private void FormationLog(string stage, string reason)
    {
        string line = stage + " " + reason;
        for (int i = 0; i < formationDiagnosticCount; ++i)
            if (formationDiagnostics[i] == line) return;
        if (formationDiagnosticCount >= formationDiagnostics.Length) return;
        formationDiagnostics[formationDiagnosticCount++] = line;
        try
        {
            using (AndroidJavaClass log = new AndroidJavaClass("android.util.Log"))
                log.CallStatic<int>("w", "HYJSFormation", line);
        }
        catch (Exception) { } // Optional diagnostics never interrupt binding.
    }

    private void PollFormationUI(long player, string session)
    {
        try
        {
            // Update already checked blockedSession/current StaticData and its
            // original .5s timer. Require the native wallet's actual bound pair.
            if (player <= 1 || player != lastPlayer || session != lastSession || session == blockedSession)
            { formationPage = null; return; }
            object page = UI.PageManager.Instance == null ? null : UI.PageManager.Instance.currentPage;
            if (page == null || page.GetType().FullName != "UI.ArrayPage")
            { formationPage = null; return; }
            if (System.Object.ReferenceEquals(page, formationPage)) return;
            formationPage = page; // One attempt per page entry, including failure.
            FormationLog("probe", "current ArrayPage reflection bootstrap");
            Type owner = Type.GetType("HYJS.LocalFormation.LocalFormationUI, LocalFormationUI", true);
            System.Reflection.MethodInfo method = owner.GetMethod("Ensure",
                System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.Static,
                null, new Type[] { page.GetType() }, null);
            if (method == null) throw new MissingMethodException("Formation Ensure");
            object result = method.Invoke(null, new object[] { page });
            FormationLog("result", result is bool && (bool)result ? "attached" : "declined");
        }
        catch (Exception error)
        {
            Exception cause = error;
            while (cause is System.Reflection.TargetInvocationException && cause.InnerException != null)
                cause = cause.InnerException;
            FormationLog("failure", cause.GetType().FullName); // No message, GUID or session.
        }
    }

    // Java calls this on Unity's normal message queue only after the server has
    // committed payment. Refresh the existing player model through its own API.
    public void RefreshAfterPayment(string ignored)
    {
        long player = lastPlayer;
        string session = lastSession;
        if (player <= 0 || session == null || session.Length != 32) return;
        try
        {
            NetAPI.SendHttp(20101, new ProtoBuf.Message.CSPlayer(),
                delegate(NetWork.Packets.Packet packet, bool success)
                {
                    if (!success || packet == null || StaticData.playerData == null ||
                        StaticData.playerData.playerId != player || Hex(StaticData.sessionKey) != session) return;
                    ProtoBuf.Message.SCPlayer reply = packet.kBody as ProtoBuf.Message.SCPlayer;
                    if (reply != null && reply.player != null) StaticData.playerData.ResetPlayer(reply.player);
                });
        }
        catch (Exception) { Debug.LogWarning("Local wallet player refresh unavailable"); }
    }

    // Calendar controls update the original client clock as soon as the
    // authenticated server operation completes, before activity pages reopen.
    public void RefreshGameTime(string gameTime)
    {
        long value;
        if (lastPlayer <= 0 || !Int64.TryParse(gameTime, out value) || value <= 0) return;
        ServerTimerTool.CorrectTime(value);
    }

    // Reuse the original town-HUD and currently open activity-page requests.
    // This creates no rewards or visibility flags in the client.
    public void RefreshActivities(string ignored)
    {
        if (lastPlayer <= 0 || Hex(StaticData.sessionKey) != lastSession) return;
        foreach (string entry in new string[] { "UI.TownPage|OpenStateCheck", "UI.UIActivity|DoOpen" })
        {
            try
            {
                string[] names = entry.Split('|');
                Type owner = typeof(StaticData).Assembly.GetType(names[0]);
                if (owner == null) continue;
                System.Reflection.MethodInfo method = owner.GetMethod(names[1],
                    System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance,
                    null, Type.EmptyTypes, null);
                if (method == null) continue;
                foreach (UnityEngine.Object value in UnityEngine.Object.FindObjectsOfType(owner))
                {
                    Component component = value as Component;
                    if (component != null && component.gameObject.activeInHierarchy) method.Invoke(value, null);
                }
            }
            catch (Exception) { Debug.LogWarning("Local activity view refresh unavailable"); }
        }
    }
}
