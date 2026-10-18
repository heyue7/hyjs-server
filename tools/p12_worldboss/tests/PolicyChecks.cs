using System;

public static class PolicyChecks
{
    private static int count;
    private static void Check(bool expected, bool actual, string name)
    {
        count++;
        if (expected != actual) throw new Exception("Failed: " + name);
    }
    private sealed class EqualByValue
    {
        public override bool Equals(object other) { return other is EqualByValue; }
        public override int GetHashCode() { return 1; }
    }
    public static void Main()
    {
        byte[] session = new byte[16];
        byte[] copy = (byte[])session.Clone();
        Check(true, LocalWorldBossRecoveryPolicy.SameSession(7, session, 7, copy), "same owner session by content");
        copy[15] = 1;
        Check(false, LocalWorldBossRecoveryPolicy.SameSession(7, session, 7, copy), "rotated final session byte");
        Check(false, LocalWorldBossRecoveryPolicy.SameSession(7, session, 8, session), "different player");
        Check(false, LocalWorldBossRecoveryPolicy.SameSession(0, session, 0, session), "unlogged player");
        Check(false, LocalWorldBossRecoveryPolicy.SameSession(7, null, 7, session), "missing captured session");
        Check(false, LocalWorldBossRecoveryPolicy.SameSession(7, session, 7, null), "missing current session");
        Check(false, LocalWorldBossRecoveryPolicy.SameSession(7, session, 7, new byte[15]), "invalid current key length");
        object net = new EqualByValue(), battle = new EqualByValue(), start = new EqualByValue();
        // This exact field_002 value is the server's real Boss preparation and
        // current-position protocol value, not a client scene resource name.
        string scene = "field_002";
        Check(true, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, net, battle, battle, start, start, true, false), "actual field_002 pending restores");
        Check(true, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, net, null, null, null, null, true, false), "fresh field with no old battle");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, null, null, battle, battle, start, start, true, false), "missing field net");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, new EqualByValue(), battle, battle, start, start, true, false), "field replaced though value-equal");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, net, battle, new EqualByValue(), start, start, true, false), "new combat after query");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, net, battle, battle, start, new EqualByValue(), true, false), "new start after query");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, net, battle, battle, start, start, false, false), "field not foreground-ready");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, scene, net, net, battle, battle, start, start, true, true), "private runner owns engine");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, "field_003", net, net, battle, battle, start, start, true, false), "different admitted world scene");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, "field_002#2", net, net, battle, battle, start, start, true, false), "scene suffix cannot alias protocol scene");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore("town_main", "town_main", net, net, battle, battle, start, start, true, false), "town cannot restore boss");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(null, null, net, net, battle, battle, start, start, true, false), "missing scene");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore(scene, null, net, net, battle, battle, start, start, true, false), "missing pending scene");
        Check(false, LocalWorldBossRecoveryPolicy.CanRestore("town_side_002", "town_side_002", net, net, battle, battle, start, start, true, false), "resource town_side_002 cannot restore pending");
        foreach (string admitted in new[] { "field_001", "field_002", "field_003", "field_004", "field_005", "field_006" })
            Check(true, LocalWorldBossRecoveryPolicy.FieldScene(admitted), "admitted protocol scene " + admitted);
        foreach (string rejected in new[] { "field_000", "field_007", "field_2", "field_002#1", "town_side_002" })
            Check(false, LocalWorldBossRecoveryPolicy.FieldScene(rejected), "rejected non-protocol scene " + rejected);
        Console.WriteLine("World boss recovery scope checks passed: " + count);
    }
}
