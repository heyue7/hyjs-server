using System;

public sealed class Button
{
    public bool Active;
    public void SetActive(bool value) { Active = value; }
}
public sealed class Tween { public float duration; }
namespace UI
{
    public sealed class UICopySweep
    {
        public bool needStop;
        public int count;
        public string levelId = "level-a";
        public Button stopBtn = new Button(), submitBtn = new Button();
        public Tween titleAlpha = new Tween(), bodyScale = new Tween();
        public int sends, showCalls, stopCalls;
        public void StopAllCoroutines() { stopCalls++; }
    }
    public sealed class UILevelData { public int level_power_cost = 7; }
    public static class UICopyData
    {
        public static string LastLevel;
        public static int Added;
        public static UILevelData GetUILevelData(string id) { LastLevel = id; return new UILevelData(); }
        public static void AddCurrentLevelChallengeCount(string id, int n) { LastLevel = id; Added += n; }
    }
}
namespace NetWork.Packets { public sealed class Packet { public object kBody; } }
namespace ProtoBuf.Message
{
    public sealed class SCCopySweep { public object gain = new object(), petAdds = new object(); public string wantedId; }
}
public sealed class Player
{
    public int Stamina = 100, gainCount, petCount;
    public void AddResultObj(object x) { gainCount++; }
    public void AddPetAddIntimacy(object x) { petCount++; }
}
public static class StaticData { public static Player playerData = new Player(); }
public static class BoxManager
{
    public static int boxes;
    public static void CreateMeetBossBox(string id) { boxes++; }
}
