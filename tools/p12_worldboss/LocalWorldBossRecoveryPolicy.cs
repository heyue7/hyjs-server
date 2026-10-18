using System;

// Pure scope checks shared by the Unity adapter and executable offline tests.
public static class LocalWorldBossRecoveryPolicy
{
    public static bool FieldScene(string scene)
    {
        // Original protocol scene IDs admitted by CSTransferScene. Resource
        // names such as town_side_002 are a separate namespace.
        return scene == "field_001" || scene == "field_002" || scene == "field_003" ||
            scene == "field_004" || scene == "field_005" || scene == "field_006";
    }
    public static bool SameSession(long capturedPlayer, byte[] capturedSession, long player, byte[] session)
    {
        if (capturedPlayer <= 0 || capturedPlayer != player || capturedSession == null || session == null || capturedSession.Length != 16 || session.Length != 16) return false;
        int diff = 0;
        for (int i = 0; i < 16; i++) diff |= capturedSession[i] ^ session[i];
        return diff == 0;
    }

    public static bool CanRestore(string originalScene, string pendingScene,
        object capturedNet, object activeNet, object capturedBattle, object activeBattle,
        object capturedStart, object activeStart, bool fieldReady, bool privateRun)
    {
        return fieldReady && !privateRun && capturedNet != null &&
            Object.ReferenceEquals(capturedNet, activeNet) &&
            Object.ReferenceEquals(capturedBattle, activeBattle) &&
            Object.ReferenceEquals(capturedStart, activeStart) &&
            FieldScene(originalScene) &&
            originalScene == pendingScene;
    }
}
