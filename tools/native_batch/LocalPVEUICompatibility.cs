// Named opt-in controller recovery for the original animated PVE UI.
// Observe actual presses and original engine/view state. The sole control
// action uses the existing AskRoundOver message route after Update returns.
using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using System.Reflection;
using Battle.Engine;
using Battle.Engine.AI;
using Battle.Engine.LogicModel;
using UnityEngine;

public static class LocalPVEUICompatibility
{
    public const string Policy = "local-pve-no-target-finish-v1";
    private sealed class Press
    {
        public EngineDefault Engine;
        public int Round;
        public CampType Camp;
    }
    private static readonly Dictionary<FSkill, Press> attempts = new Dictionary<FSkill, Press>();
    private static EngineDefault lastEngine;

    public static bool Enabled()
    {
        string path = Path.Combine(Application.persistentDataPath, "local-pve-round-compat-policy.txt");
        return File.Exists(path) && File.ReadAllText(path).Trim() == Policy;
    }

    private static object Field(object value, string name)
    {
        if (value == null) return null;
        for (Type type = value.GetType(); type != null; type = type.BaseType)
        {
            FieldInfo field = type.GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly);
            if (field != null) return field.GetValue(value);
        }
        return null;
    }

    private static bool OriginalPVE()
    {
        if (StaticData.curBattleData == null) return false;
        object value = Field(StaticData.curBattleData, "battleType");
        if (!(value is Enum) && !(value is int)) return false;
        switch (Convert.ToInt32(value))
        {
            case 1: case 4: case 6: case 8: case 9: case 10: case 16: return true;
            default: return false;
        }
    }

    public static void ObservePress(int camp, string actorUid, int skillIndex)
    {
        if (!Enabled() || !OriginalPVE()) return;
        Battle.BattleManager manager = Battle.BattleManager.Instance;
        EngineDefault engine = manager == null ? null : manager.battleEngine;
        if (engine == null) return;
        if (lastEngine != engine) { attempts.Clear(); lastEngine = engine; }
        EngineData data = engine.GetEngineData();
        if (data == null || camp != 1 || data.curRoundCamp != (CampType)1) return;
        if (data.attacker.actors.Count == 0) return;
        foreach (FActor actor in data.attacker.actors)
            if ((int)actor.state != 3 && (int)actor.state != 4) return;
        foreach (FActor actor in data.defender.actors)
        {
            if (actor.uniqueId != actorUid) continue;
            foreach (FSkill skill in actor.skills)
                if (skill.index == skillIndex && (int)skill.state == 0 && skill.target == null)
                    attempts[skill] = new Press { Engine = engine, Round = data.roundNum, Camp = data.curRoundCamp };
        }
    }

    public static void ObserveBegin(object skill)
    {
        FSkill original = skill as FSkill;
        if (original != null) attempts.Remove(original);
    }

    private static bool Empty(object value)
    {
        ICollection queue = value as ICollection;
        return queue != null && queue.Count == 0;
    }

    private static bool OriginalViewIdle(Battle.View.BattleViewCtrl view, CampType camp)
    {
        if (!Empty(Field(view, "behaviourQueue")) || !Empty(Field(view, "currentBehaviours"))
            || Field(view, "ultimateBehaviour") != null) return false;
        MethodInfo list = view.GetType().GetMethod("GetActorUids");
        MethodInfo character = view.GetType().GetMethod("GetBattleCharacter");
        if (list == null || character == null) return false;
        Type declared = list.GetParameters()[0].ParameterType;
        if (!declared.IsEnum) return false;
        IEnumerable ids = list.Invoke(view, new object[] { Enum.ToObject(declared, (int)camp) }) as IEnumerable;
        if (ids == null) return false;
        foreach (object id in ids)
        {
            object actor = character.Invoke(view, new object[] { id });
            if (actor == null) return false;
            object alive = Field(actor, "isAlive");
            if (!(alive is bool)) return false;
            if (!(bool)alive) continue;
            MethodInfo state = actor.GetType().GetMethod("GetCurrrentStateType");
            if (state == null || Convert.ToInt32(state.Invoke(actor, new object[0])) != 2) return false;
        }
        return true;
    }

    public static void AfterUpdate(object value)
    {
        if (attempts.Count == 0) return;
        Battle.BattleManager manager = value as Battle.BattleManager;
        if (manager == null || manager.battleEngine != lastEngine) { attempts.Clear(); return; }
        if (!Enabled() || !OriginalPVE() || manager.isPause || manager.battleViewCtrl == null) return;
        EngineDefault engine = manager.battleEngine;
        EngineData data = engine.GetEngineData();
        if (data == null || data.curRoundCamp != (CampType)1) return;
        object core = Field(engine, "compute");
        EngineCopyEnemyAI ai = Field(core, "defenderAI") as EngineCopyEnemyAI;
        if (ai == null || Field(ai, "curMsg") != null || Field(ai, "curUltiMsg") != null
            || !Empty(Field(ai, "msgQueue")) || !Empty(Field(ai, "ultiMsgQueue"))) return;
        object step = Field(ai, "curStep");
        bool normal = Field(ai, "isRunning") is bool && (bool)Field(ai, "isRunning")
            && step is Enum && step.GetType().Name == "CopyEnenyAIStep"
            && step.GetType().DeclaringType == typeof(EngineCopyEnemyAI) && Convert.ToInt32(step) == 1;
        bool active = data.ultimateSkill != null;
        foreach (FActor actor in data.GetCurRoundPlayer().actors)
            foreach (FSkill skill in actor.skills)
                if ((int)skill.state == 1 || (int)skill.state == 2 || (int)skill.state == 3) active = true;
        bool proven = false;
        bool accounted = true;
        IEnumerable dealt = Field(ai, "dealedMsgs") as IEnumerable;
        if (dealt == null) return;
        foreach (object message in dealt)
        {
            FSkill skill = Field(message, "skill") as FSkill;
            if (skill == null) { accounted = false; continue; }
            if ((int)skill.state == 4) continue;
            Press proof;
            if ((int)skill.state != 0 || skill.target != null || !attempts.TryGetValue(skill, out proof)
                || proof.Engine != engine || proof.Round != data.roundNum || proof.Camp != data.curRoundCamp
                || Field(skill.actor, "doSkill") != null || Field(skill.actor, "doUltimateSkill") != null)
            { accounted = false; continue; }
            proven = true;
        }
        object rounding = Field(core, "rounding");
        object ended = Field(core, "end");
        if (!OriginalHeadlessPlaybackCaller.PVECompatibilityGate(true, true, rounding is bool && (bool)rounding,
            !(ended is bool) || (bool)ended, true, normal, !active, proven, accounted)
            || !OriginalViewIdle(manager.battleViewCtrl, data.curRoundCamp)) return;
        // Clear observer state before enqueueing the existing original public
        // command. The engine receives it via its normal view/message update.
        attempts.Clear();
        UnityEngine.Debug.Log("HYJS local-pve-no-target-finish-v1 AskRoundOver round=" + data.roundNum + " time=" + data.roundTime);
        manager.battleViewCtrl.AskRoundOver();
    }
}
