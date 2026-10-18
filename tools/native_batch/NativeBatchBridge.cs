// Stable diagnostic ABI. Only a developer-pushed caller in this application's
// private files directory can be reloaded; original game assemblies stay loaded
// and unchanged. This avoids a full APK installation for each caller correction.
using System;
using System.IO;
using System.Reflection;
using System.Security.Cryptography;
using UnityEngine;

public static class NativeBatchRunner
{
    private static string loadedDigest;
    private static Type implementation;
    private static bool uiObserved;

    private static bool UIEnabled()
    {
        string path = Path.Combine(Application.persistentDataPath, "local-pve-round-compat-policy.txt");
        return File.Exists(path) && File.ReadAllText(path).Trim() == "local-pve-no-target-finish-v1";
    }

    public static void ObservePVEUIPress(int camp, string actorUid, int skillIndex)
    {
        if (!UIEnabled()) return;
        Type target = Load();
        MethodInfo method = target == null ? null : target.GetMethod("ObservePVEUIPress");
        if (method == null) return;
        method.Invoke(null, new object[] { camp, actorUid, skillIndex });
        uiObserved = true;
    }

    public static void ObservePVEUIBegin(object skill)
    {
        if (!uiObserved || implementation == null) return;
        MethodInfo method = implementation.GetMethod("ObservePVEUIBegin");
        if (method != null) method.Invoke(null, new object[] { skill });
    }

    public static void AfterPVEUIUpdate(object manager)
    {
        if (!uiObserved || implementation == null) return;
        MethodInfo method = implementation.GetMethod("AfterPVEUIUpdate");
        if (method != null) method.Invoke(null, new object[] { manager });
    }

    private static Type Load()
    {
        string path = Path.Combine(Application.persistentDataPath, "native-runner-current.dll");
        if (!File.Exists(path)) return null;
        byte[] data = File.ReadAllBytes(path);
        string digest;
        using (SHA256 hash = SHA256.Create()) digest = Convert.ToBase64String(hash.ComputeHash(data));
        if (digest == loadedDigest) return implementation;
        // Each compiled implementation has a content-addressed assembly name,
        // so Mono cannot silently reuse the previously loaded assembly identity.
        Type next = Assembly.Load(data).GetType("NativeBatchRunner", true);
        implementation = next;
        loadedDigest = digest;
        return implementation;
    }

    public static void ApplyActivityInformation(object panel, object packet, bool success)
    {
        Type target = Load();
        if (target == null) return;
        MethodInfo method = target.GetMethod("ApplyActivityInformation");
        if (method != null) method.Invoke(null, new object[] { panel, packet, success });
    }

    public static void ApplyExpeditionPreview(object panel)
    {
        Type target = Load();
        if (target == null) return;
        MethodInfo method = target.GetMethod("ApplyExpeditionPreview");
        if (method != null) method.Invoke(null, new object[] { panel });
    }

    public static bool TryStart(object candidate)
    {
        Type target = Load();
        if (target == null) return false;
        return (bool)target.GetMethod("TryStart", BindingFlags.Public | BindingFlags.Static)
            .Invoke(null, new object[] { candidate });
    }

    public static void LogRoomPosition(object net, object response, int phase)
    {
        Type target = Load();
        if (target == null) return;
        target.GetMethod("LogRoomPosition", BindingFlags.Public | BindingFlags.Static)
            .Invoke(null, new object[] { net, response, phase });
    }
}
