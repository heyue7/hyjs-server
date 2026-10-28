// Caller template for the original APK assembly. Its EngineDefault/ComputeCore,
// AI, actors, releases, buffs, tables and random implementation stay original.
// Compile/inject into the client harness and verify against both captured Daily
// goldens before treating new batch timelines as native evidence.
using System;
using System.Globalization;
using System.Collections.Generic;
using System.Collections;
using System.Reflection;
using System.Diagnostics;
using Battle.Engine;
using Battle.Engine.AI;
using Battle.Engine.InputData;
using Battle.Engine.LogicModel;
using ProtoBuf.Message;

public static class OriginalHeadlessPlaybackCaller
{
    private static Dictionary<int,double> RoundAutoControls66;
    private static bool Formation87;
    public static bool RunRushFormation87(SCFightPlayback identity, SCRushFightStart start, Dictionary<int,double> controls, Action<string,List<FExport>> exports, Action<EngineData> snapshots, Action<string,EngineData,object> events) {
        Formation87=true;
        try { return RunRushRoundLiveAuto66(identity,start,controls,exports,snapshots,events); }
        finally { Formation87=false; }
    }
    public static bool RunRushRoundLiveAuto66(SCFightPlayback identity, SCRushFightStart start, Dictionary<int,double> controls, Action<string,List<FExport>> exports, Action<EngineData> snapshots, Action<string,EngineData,object> events)
    {
        if(RoundAutoControls66!=null || controls==null || controls.Count==0)throw new InvalidOperationException("exclusive bounded round controls required");
        RoundAutoControls66=controls;
        try { return RunRushManualAutoStart(identity,start,0,exports,snapshots,events); }
        finally { RoundAutoControls66=null; }
    }

    public static bool RunWantedRoundLive86(SCFightPlayback identity, SCRebelFightStart start, Dictionary<int,double> controls, Action<string,List<FExport>> exports, Action<EngineData> snapshots, Action<string,EngineData,object> events)
    {
        if(identity==null || start==null || identity.battleType!=8 || identity.maxRound!=20 || start.maxround!=20 || start.battleMonsterList.Count!=1 || RoundAutoControls66!=null || controls==null || controls.Count==0) throw new InvalidOperationException("typed wanted bounded controls required");
        bool previous=StaticData.isPveAutoCache;
        StaticData.isPveAutoCache=false;
        RoundAutoControls66=controls;
        try { BattleData data=new BattleData(); data.SetData(start); return RunWithManualAutoDelay(identity,exports,snapshots,events,true,false,data,0); }
        finally { RoundAutoControls66=null; StaticData.isPveAutoCache=previous; }
    }

    // Read fields through the original inheritance chain. Diagnostics never
    // invoke AI actions, setters, target selectors or random-number methods.
    private static object ReadField(object value, string name)
    {
        if (value == null) return null;
        for (Type type = value.GetType(); type != null; type = type.BaseType)
        {
            FieldInfo field = type.GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly);
            if (field != null) return field.GetValue(value);
        }
        return null;
    }

    private static object MessageState(object message)
    {
        if (message == null) return null;
        FSkill skill = ReadField(message, "skill") as FSkill;
        Dictionary<string, object> result = new Dictionary<string, object>();
        result["type"] = message.GetType().FullName;
        result["actor"] = ReadField(message, "actor");
        if (skill != null)
        {
            result["skill"] = skill.id;
            result["state"] = skill.state;
            result["step"] = skill.stepState;
            result["target"] = skill.target;
            result["cooldown"] = skill.cooldownCur;
            result["actorState"] = skill.actor.state;
            object stateManager = ReadField(skill.actor, "stateManager");
            Dictionary<string, object> flags = new Dictionary<string, object>();
            foreach (string flag in new string[] { "IsDizzy", "IsForbidUltimateSkill", "isForbidFirstSkill", "IsForbidSecondSkill", "IsForbidThirdSkill" }) flags[flag] = ReadField(stateManager, flag);
            result["actorStateManager"] = flags;
        }
        IEnumerable skills = ReadField(message, "skills") as IEnumerable;
        if (skills != null)
        {
            List<object> queued = new List<object>();
            foreach (object item in skills)
            {
                FSkill queuedSkill = item as FSkill;
                if (queuedSkill != null) queued.Add(new Dictionary<string, object> { { "skill", queuedSkill.id }, { "state", queuedSkill.state }, { "step", queuedSkill.stepState } });
            }
            result["skills"] = queued;
        }
        return result;
    }

    private static object AIState(EngineAI ai)
    {
        if (ai == null) return null;
        Dictionary<string, object> result = new Dictionary<string, object>();
        result["type"] = ai.GetType().FullName;
        foreach (string name in new string[] { "isRunning", "curStep", "cdLeftTime", "isUltimating", "curAiMsgDeltaTime" }) result[name] = ReadField(ai, name);
        foreach (string name in new string[] { "curMsg", "curUltiMsg", "curAiMsg" }) result[name] = MessageState(ReadField(ai, name));
        foreach (string name in new string[] { "msgQueue", "dealedMsgs", "ultiMsgQueue", "aiMsgQueue", "dealingSkills" })
        {
            IEnumerable queue = ReadField(ai, name) as IEnumerable;
            if (queue == null) { result[name] = null; continue; }
            List<object> values = new List<object>();
            foreach (object item in queue)
            {
                FSkill skill = item as FSkill;
                values.Add(skill == null ? MessageState(item) : new Dictionary<string, object> { { "skill", skill.id }, { "state", skill.state }, { "step", skill.stepState } });
            }
            result[name] = values;
        }
        return result;
    }

    // A separately named controller recovery may issue the original public
    // round-over action. These facts never authorize a skill/HP/RNG edit.
    public static bool PVECompatibilityGate(bool enabled, bool originalCopyAI, bool rounding, bool ended,
        bool queuesEmpty, bool normalStep, bool noActiveSkills, bool failedPressProven, bool allDealtAccounted)
    {
        return enabled && originalCopyAI && rounding && !ended && queuesEmpty && normalStep
            && noActiveSkills && failedPressProven && allDealtAccounted;
    }

    private sealed class NoTargetAttempt
    {
        public int Round;
        public double Time;
        public CampType Camp;
        public FSkill Skill;
        public List<object> Opponents;
    }

    // Reading the original AtomicLong changes no seed and consumes no draw.
    // Read at initialization and each original Tick/boundary, before Clear.
    public static long ReadOriginalRandomSeed(EngineData data)
    {
        FieldInfo field = typeof(Battle.Engine.util.JRandom).GetField("seed", BindingFlags.Instance | BindingFlags.NonPublic);
        AtomicLong seed = (AtomicLong)field.GetValue(data.random.innerRandom);
        return seed.Get();
    }

    // NotifyCanFinishRound also comes from SkillEnd when normal skills end,
    // while a future playback ultimate may still be queued. The original view
    // merely enables its round-over button. Wait for the original playback AI
    // to exhaust its own queues before issuing the harness's round-over action.
    private static bool PlaybackExhausted(EnginePlaybackAI ai)
    {
        if (ai == null) return true;
        BindingFlags flags = BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public;
        Type type = typeof(EnginePlaybackAI);
        if (type.GetField("curMsg", flags).GetValue(ai) != null) return false;
        foreach (string name in new string[] { "aiMsgQueue", "dealingSkills" })
        {
            System.Collections.ICollection queue = (System.Collections.ICollection)type.GetField(name, flags).GetValue(ai);
            if (queue.Count != 0) return false;
        }
        return true;
    }

    private sealed class Capture : EngineCallback
    {
        public bool Done;
        public bool PendingNext;
        public bool PendingFinish;
        public EngineData FinalData;
        public EngineData Data;
        public Action<string, List<FExport>> Exports;
        public Action<EngineData> Snapshot;
        public Action<string, EngineData, object> Events;
        public bool OriginalAutomatic;
        public EngineDefault Engine;
        public EngineAI AttackerAI;
        public EngineAI DefenderAI;
        public int NotifyCount;
        public bool LocalNoTargetFinish;
        private Dictionary<FSkill, NoTargetAttempt> noTargetAttempts = new Dictionary<FSkill, NoTargetAttempt>();
        private bool AllOpponentsWeakOrDead(CampType camp, out List<object> states)
        {
            states = new List<object>();
            FPlayer opponent = camp == (CampType)0 ? Data.defender : Data.attacker;
            if (opponent.actors.Count == 0) return false;
            foreach (FActor actor in opponent.actors)
            {
                states.Add(new Dictionary<string, object> { { "actor", actor.uniqueId }, { "state", actor.state } });
                if ((int)actor.state != 3 && (int)actor.state != 4) return false;
            }
            return true;
        }

        public bool TryLocalNoTargetFinish(int ticks)
        {
            if (!LocalNoTargetFinish || Done) return false;
            EngineCopyEnemyAI ai = (Data.curRoundCamp == (CampType)0 ? AttackerAI : DefenderAI) as EngineCopyEnemyAI;
            if (ai == null) return false;
            bool queuesEmpty = ReadField(ai, "curMsg") == null && ReadField(ai, "curUltiMsg") == null;
            foreach (string name in new string[] { "msgQueue", "ultiMsgQueue" })
            {
                ICollection queue = ReadField(ai, name) as ICollection;
                queuesEmpty = queuesEmpty && queue != null && queue.Count == 0;
            }
            bool active = Data.ultimateSkill != null;
            foreach (FActor actor in Data.GetCurRoundPlayer().actors)
                foreach (FSkill skill in actor.skills)
                    if ((int)skill.state == 1 || (int)skill.state == 2 || (int)skill.state == 3) active = true;
            bool proven = false, accounted = true;
            List<object> failures = new List<object>();
            IEnumerable dealt = ReadField(ai, "dealedMsgs") as IEnumerable;
            if (dealt == null) accounted = false;
            else foreach (object message in dealt)
            {
                FSkill skill = ReadField(message, "skill") as FSkill;
                if (skill == null) { accounted = false; continue; }
                if ((int)skill.state == 4) continue;
                NoTargetAttempt attempt;
                if ((int)skill.state != 0 || skill.target != null || !noTargetAttempts.TryGetValue(skill, out attempt)
                    || attempt.Round != Data.roundNum || attempt.Camp != Data.curRoundCamp
                    || ReadField(skill.actor, "doSkill") != null || ReadField(skill.actor, "doUltimateSkill") != null)
                { accounted = false; continue; }
                proven = true;
                failures.Add(new Dictionary<string, object> { { "actor", skill.actor.uniqueId }, { "skill", skill.id },
                    { "pressedRound", attempt.Round }, { "pressedTime", attempt.Time }, { "state", skill.state },
                    { "target", skill.target }, { "opponentsAtPress", attempt.Opponents } });
            }
            object core = ReadField(Engine, "compute");
            object rounding = ReadField(core, "rounding"), ended = ReadField(core, "end"), step = ReadField(ai, "curStep");
            // Reflection boxes the original nested step enum as an enum, not
            // Int32. Preserve its exact declared type and original Normal=1.
            bool normal = ReadField(ai, "isRunning") is bool && (bool)ReadField(ai, "isRunning")
                && step is Enum && step.GetType().Name == "CopyEnenyAIStep"
                && step.GetType().DeclaringType == typeof(EngineCopyEnemyAI)
                && Convert.ToInt32(step) == 1;
            if (!PVECompatibilityGate(LocalNoTargetFinish, true, rounding is bool && (bool)rounding,
                !(ended is bool) || (bool)ended, queuesEmpty, normal, !active, proven, accounted)) return false;
            if (Events != null) Events("compat_no_target_finish", Data, new Dictionary<string, object> {
                { "policy", "local-pve-no-target-finish-v1" }, { "action", "EngineDefault.FinishRound" },
                { "ticks", ticks }, { "failedActualPresses", failures } });
            // Invoke the same public control action as an original round-over
            // request, after the original Update has unwound. No skill changes.
            Engine.FinishRound();
            noTargetAttempts.Clear();
            if (Events != null) Events("compat_original_finish_return", Data, new Dictionary<string, object> {
                { "policy", "local-pve-no-target-finish-v1" }, { "nextRound", Data.roundNum }, { "done", Done } });
            return true;
        }
        public void Progress(string kind, int ticks, List<string> stack)
        {
            if (Events == null) return;
            object core = ReadField(Engine, "compute");
            Dictionary<string, object> values = new Dictionary<string, object>();
            values["ticks"] = ticks;
            values["notifyCount"] = NotifyCount;
            values["pendingFinish"] = PendingFinish;
            values["pendingNext"] = PendingNext;
            values["done"] = Done;
            values["engineRun"] = ReadField(Engine, "run");
            values["engineRounding"] = ReadField(Engine, "rounding");
            values["coreType"] = core == null ? null : core.GetType().FullName;
            foreach (string name in new string[] { "end", "rounding", "currentTime", "realTime", "autoFinishARound" }) values["core_" + name] = ReadField(core, name);
            values["attackerAI"] = AIState(AttackerAI);
            values["defenderAI"] = AIState(DefenderAI);
            FSkill ultimate = ReadField(Data, "ultimateSkill") as FSkill;
            values["ultimateSkill"] = ultimate == null ? null : new Dictionary<string, object> { { "skill", ultimate.id }, { "state", ultimate.state }, { "actor", ultimate.actor } };
            values["stack"] = stack;
            Events(kind, Data, values);
        }
        // Read-only phase diagnostics for the one collected real Game1 tail.
        // No AI method, skill action, timestamp assignment or RNG call is made.
        public void TailPhase(string kind, int ticks)
        {
            if (Events == null) return;
            Dictionary<string, object> values = new Dictionary<string, object>();
            values["ticks"] = ticks;
            values["pendingFinish"] = PendingFinish;
            values["pendingNext"] = PendingNext;
            values["activeCamp"] = (int)Data.curRoundCamp;
            values["attackerAI"] = AIState(AttackerAI);
            values["defenderAI"] = AIState(DefenderAI);
            List<object> pointers = new List<object>();
            foreach (FPlayer player in new FPlayer[] { Data.attacker, Data.defender })
                foreach (FActor actor in player.actors)
                {
                    Dictionary<string, object> row = new Dictionary<string, object>();
                    row["uid"] = actor.uniqueId;
                    row["state"] = actor.state;
                    foreach (string field in new string[] { "doSkill", "doUltimateSkill" })
                    {
                        FSkill skill = ReadField(actor, field) as FSkill;
                        row[field] = skill == null ? null : new Dictionary<string, object> { { "id", skill.id }, { "state", skill.state }, { "step", skill.stepState }, { "stepEnd", ReadField(skill, "stepEnd") } };
                    }
                    ICollection waiting = ReadField(actor, "waitSkills") as ICollection;
                    row["waitCount"] = waiting == null ? -1 : waiting.Count;
                    pointers.Add(row);
                }
            values["actors"] = pointers;
            Events(kind, Data, values);
        }
        public override void BattleEnd(EngineData data) { FinalData = data; Done = true; if (Snapshot != null) Snapshot(data); }
        public override void RoundEnd(CampType nextCamp) { PendingNext = true; }
        public override void HaventActiveNormalSkill()
        {
            NotifyCount++;
            List<string> origin = new List<string>();
            if (OriginalAutomatic)
            {
                // Automatic AI's own Update checks its normal, ultimate and
                // actor queues before this notification. SkillEnd's weaker
                // notification is only a view hint and must not end the round.
                StackFrame[] frames = new StackTrace().GetFrames();
                if (frames == null) throw new InvalidOperationException("original automatic AI notification origin unavailable");
                foreach (StackFrame frame in frames)
                {
                    MethodBase method = frame.GetMethod();
                    if (method != null) origin.Add((method.DeclaringType == null ? "<null>" : method.DeclaringType.FullName) + "::" + method.Name);
                    if (method != null && method.Name == "Update" && (method.DeclaringType == typeof(EnginePlayerAI) || method.DeclaringType == typeof(EngineCopyEnemyAI)))
                    { PendingFinish = true; }
                }
            }
            else PendingFinish = true;
            if (NotifyCount <= 8 || NotifyCount % 100 == 0) Progress("ai_notify", -1, origin);
        }
        public override void SkillPress(CampType camp, string actorUid, int skillIndex)
        {
            if (LocalNoTargetFinish && camp == Data.curRoundCamp)
            {
                FPlayer player = camp == (CampType)0 ? Data.attacker : Data.defender;
                foreach (FActor actor in player.actors)
                {
                    if (actor.uniqueId != actorUid) continue;
                    foreach (FSkill skill in actor.skills)
                    {
                        List<object> opponents;
                        if (skill.index == skillIndex && (int)skill.state == 0 && skill.target == null
                            && AllOpponentsWeakOrDead(camp, out opponents))
                        {
                            noTargetAttempts[skill] = new NoTargetAttempt { Round = Data.roundNum, Time = Data.roundTime,
                                Camp = camp, Skill = skill, Opponents = opponents };
                            if (Events != null) Events("compat_no_target_candidate", Data, new Dictionary<string, object> {
                                { "policy", "local-pve-no-target-finish-v1" }, { "actor", actorUid }, { "skill", skill.id },
                                { "opponentsAtPress", opponents } });
                        }
                    }
                }
            }
            if (Events != null) Events("skill_press", Data, new Dictionary<string, object> { { "camp", (int)camp }, { "actorUid", actorUid }, { "skillIndex", skillIndex } });
        }
        public override void SkillBegin(FSkill skill, List<FActor> targets)
        {
            noTargetAttempts.Remove(skill);
            if (Events != null) Events("skill_begin", Data, skill);
        }
        public override void SkillCast(FRelease release)
        {
            if (Events != null) Events("skill_cast", Data, release);
        }
        public override void Tick(List<FExport> values) { if (Exports != null) Exports("tick", values); if (Snapshot != null) Snapshot(Data); }
        public override void RoundBeginTick(List<FExport> values) { if (Exports != null) Exports("round_begin", values); if (Snapshot != null) Snapshot(Data); }
        public override void RoundEndTick(List<FExport> values) { if (Exports != null) Exports("round_end", values); if (Snapshot != null) Snapshot(Data); }
    }

    public static bool RunOne(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog)
    {
        return Run(originalWire, exportLog, snapshotLog, null, originalWire.defender1 != null, false);
    }

    public static bool RunOneWithObservers(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog, Action<string, EngineData, object> eventLog)
    {
        return Run(originalWire, exportLog, snapshotLog, eventLog, originalWire.defender1 != null, false);
    }

    public static bool RunAutomatic(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog, Action<string, EngineData, object> eventLog)
    {
        return Run(originalWire, exportLog, snapshotLog, eventLog, true, false);
    }

    public static bool RunPVECompatibility(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog, Action<string, EngineData, object> eventLog)
    {
        if (originalWire.defender1 != null || originalWire.defender2 == null)
            throw new InvalidOperationException("local PVE compatibility requires the original monster fighter input");
        return Run(originalWire, exportLog, snapshotLog, eventLog, true, true);
    }

    // UF14 has its own original replay overload. SetPlaybackData does not
    // create defender fighters for14. Preserve local frozen players/factors and
    // let the original UF overload handle room/default40/FirstOrder0,0 fallback.
    private static void SetDiagnosticBattleData(BattleData data, SCFightPlayback originalWire)
    {
        if (originalWire.battleType == 17)
        {
            // Cross-realm match17 has an original typed callback distinct from
            // KFReplayData and generic SetPlaybackData. Preserve the two source
            // snapshots and let Clear/SetData own factors1/1, viewfalse, initiative
            // and round defaults. The diagnostic supplies no server winner.
            if (originalWire.attacker == null || originalWire.defender1 == null || originalWire.defender2 != null
                || originalWire.optRecord != null || originalWire.winConditions.Count != 0
                || originalWire.attackerFactor != 1.0 || originalWire.defenderFactor != 1.0)
                throw new InvalidOperationException("original Kuafu17 adapter requires exactly two automatic player sources and factors1/1");
            SCKuafuMatchPlayer match = new SCKuafuMatchPlayer();
            match.fightId = originalWire.fightid;
            // Leave the source room absent; original SetData applies map_forest_001.
            match.randomSeed = originalWire.randSeed;
            match.maxRoundNum = originalWire.maxRound;
            match.playerList.Add(originalWire.attacker);
            match.playerList.Add(originalWire.defender1);
            data.SetData(match);
            return;
        }
        if (originalWire.battleType == 11)
        {
            // Friend duels have their own original typed callback. Clear sets
            // factors1/1 and viewfalse; preserve both original player snapshots,
            // legal initiative/seed/maxRound and leave the server winner absent.
            if (originalWire.attacker == null || originalWire.defender1 == null || originalWire.defender2 != null
                || originalWire.optRecord != null || originalWire.winConditions.Count != 0
                || originalWire.attackerFactor != 1.0 || originalWire.defenderFactor != 1.0)
                throw new InvalidOperationException("original Friend11 adapter requires two automatic player sources and factors1/1");
            SCFriendFight friend = new SCFriendFight();
            friend.fightId = originalWire.fightid;
            friend.roomId = "map_forest_001";
            friend.randSeed = originalWire.randSeed;
            friend.maxRoundNum = originalWire.maxRound;
            friend.playerList.Add(originalWire.attacker);
            friend.playerList.Add(originalWire.defender1);
            data.SetData(friend);
            return;
        }
        if (originalWire.battleType != 14)
        {
            data.SetPlaybackData(originalWire);
            return;
        }
        if (originalWire.attacker == null || originalWire.optRecord != null || ((originalWire.defender1 == null) == (originalWire.defender2 == null)))
            throw new InvalidOperationException("local UF14 adapter requires exactly one frozen player or monster defender and original automatic inputs");
        if (originalWire.defender2 != null)
        {
            // The original guard replay overload builds both automatic fighters.
            // Keep the finite local monster/HP/seed/factors; no winner is supplied.
            SCUFPveFightRecordReplay guard = new SCUFPveFightRecordReplay();
            guard.fightId = originalWire.fightid;
            guard.randomSeed = originalWire.randSeed;
            guard.maxRound = originalWire.maxRound;
            guard.attackerDamageFactor = originalWire.attackerFactor;
            guard.defenderDamageFactor = originalWire.defenderFactor;
            guard.player = originalWire.attacker;
            guard.monster = originalWire.defender2;
            data.SetData(guard);
            return;
        }
        SCUFFightRecordReplay typed = new SCUFFightRecordReplay();
        typed.fightId = originalWire.fightid;
        typed.randomSeed = originalWire.randSeed;
        typed.maxRound = originalWire.maxRound;
        typed.attackerDamageFactor = originalWire.attackerFactor;
        typed.defenderDamageFactor = originalWire.defenderFactor;
        typed.playerList.Add(originalWire.attacker);
        typed.playerList.Add(originalWire.defender1);
        // Original SetData clears viewfalse and applies the published0,0 fallback.
        // No server winner is supplied; Capture computes original subjectcamp0.
        data.SetData(typed);
    }


    // Read original FPlayer record lists through the original EngineDefault getter.
    // This is the original BattleNetOver.ParseOptRecord shape without its live
    // BattleManager singleton or network/UI dispatch. The engine is never modified.
    private static BattleOperate OriginalFinalRecord(EngineDefault engine)
    {
        EngineData data = engine.GetEngineData();
        BattleOperate record = new BattleOperate();
        record.finalRoundNum = data.roundNum;
        record.attackerTotalLeftHp = data.GetNumHP((CampType)0);
        record.defenderTotalLeftHp = data.GetNumHP((CampType)1);
        for (int camp = 0; camp < 2; camp++)
        {
            List<FOperateSkill> skills;
            List<FOperateTrans> transforms;
            engine.GetRecordOperates((CampType)camp, out skills, out transforms);
            if (skills != null)
                foreach (FOperateSkill source in skills)
                {
                    BattleSkillOperate skill = new BattleSkillOperate();
                    skill.roundNum = source.round;
                    skill.roundTime = source.roundTime;
                    skill.camp = (int)source.camp;
                    skill.actorUid = source.uniquedId.Split('_')[1];
                    skill.skillIndex = source.skillIndex;
                    record.skillOpts.Add(skill);
                }
            if (transforms != null && transforms.Count > 0)
            {
                FOperateTrans source = transforms[0];
                BattleTransOperate transform = new BattleTransOperate();
                transform.roundNum = source.round;
                transform.fpids.AddRange(source.fpids);
                foreach (string uid in source.uids) transform.actorUids.Add(uid.Split('_')[1]);
                if (camp == 0) record.attackerTransOpt = transform;
                else record.defenderTransOpt = transform;
            }
        }
        return record;
    }

    // Execute the actual original Rush callback for one frozen diagnostic pair.
    // Preserve scene/defaults/source actors; automatic mode is an explicit caller
    // choice, separate from original typed SetData's manual-player default.
    // Explicit typed sidecar entry keeps legacy generic PVP2 diagnostics intact.
    // Explicit local wrapper compatibility uses the exact original KF PB. The
    // cloned viewer context never invokes the ZoneId setter or persists prefs.
    // Same-PID original duplicate pairs are not represented as a full result.
    // This deliberately runs only original wrapper projection, never an Engine
    // or whole multi-pair result. Zone0 selects the original PID-only branch.
    public static string InspectKFNumericWrapper(KFReplayData source, long viewerPID)
    {
        if (source == null || source.playerList.Count != 2 || viewerPID <= 0)
            throw new InvalidOperationException("original KF numeric wrapper source missing");
        FieldInfo holder = typeof(StaticData).GetField("playerData", BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic);
        object previous = holder == null ? null : holder.GetValue(null);
        if (previous == null) throw new InvalidOperationException("original KF viewer context missing");
        object viewer = typeof(object).GetMethod("MemberwiseClone", BindingFlags.Instance | BindingFlags.NonPublic).Invoke(previous, new object[0]);
        FieldInfo realm = viewer.GetType().GetField("zoneId", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        FieldInfo pid = viewer.GetType().GetField("playerId", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        if (realm == null || pid == null) throw new InvalidOperationException("original viewer context fields missing");
        realm.SetValue(viewer, Convert.ChangeType(0, realm.FieldType, CultureInfo.InvariantCulture));
        pid.SetValue(viewer, Convert.ChangeType(viewerPID, pid.FieldType, CultureInfo.InvariantCulture));
        try
        {
            holder.SetValue(null, viewer);
            BattleData data = new BattleData(); data.SetData(source);
            IList a = typeof(BattleData).GetField("attackFighters", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic).GetValue(data) as IList;
            IList d = typeof(BattleData).GetField("defendFighters", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic).GetValue(data) as IList;
            if (a == null || d == null) throw new InvalidOperationException("original KF fighter lists missing");
            return "{\"attackFighterCount\":" + a.Count + ",\"defendFighterCount\":" + d.Count
                + ",\"isAttToDef\":" + (data.isAttToDef ? "true" : "false") + ",\"attackFirst\":" + (data.attackFirst ? "true" : "false")
                + ",\"engineExecuted\":false}";
        }
        finally { holder.SetValue(null, previous); }
    }

    public static bool RunKFRealmAutomatic(SCFightPlayback identity, KFReplayData source, long viewerRealm, long viewerPID, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        return RunKFJointAutomatic(identity, source, viewerRealm, viewerPID, exports, snapshots, events, false);
    }

    // Same-realm input is an independent diagnostic scope with two distinct actual PIDs.
    public static bool RunKFSameRealmAutomatic(SCFightPlayback identity, KFReplayData source, long viewerRealm, long viewerPID, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        return RunKFJointAutomatic(identity, source, viewerRealm, viewerPID, exports, snapshots, events, true);
    }

    private static bool RunKFJointAutomatic(SCFightPlayback identity, KFReplayData source, long viewerRealm, long viewerPID, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events, bool sameRealm)
    {
        if (identity == null || source == null || source.playerList.Count != 2 || viewerRealm <= 0 || viewerPID <= 0
            || identity.battleType != 17 || identity.fightid != source.warId || identity.randSeed != source.randSeed
            || identity.maxRound != (source.maxRoundNum > 0 ? source.maxRoundNum : 40)
            || identity.defender1 == null || identity.defender2 != null || identity.optRecord != null
            || identity.winConditions.Count != 0 || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || source.playerList[0].serverId <= 0 || source.playerList[1].serverId <= 0
            || (sameRealm ? (source.playerList[0].serverId != source.playerList[1].serverId || source.playerList[0].playerId <= 0 || source.playerList[1].playerId <= 0 || source.playerList[0].playerId == source.playerList[1].playerId) : source.playerList[0].serverId == source.playerList[1].serverId))
            throw new InvalidOperationException("KF realm diagnostic requires its exact independent realm/player boundary");
        int selected = -1;
        for (int i = 0; i < 2; i++)
            if (source.playerList[i].serverId == viewerRealm && source.playerList[i].playerId == viewerPID)
            { if (selected >= 0) throw new InvalidOperationException("ambiguous KF joint viewer"); selected = i; }
        if (selected < 0 || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(source.playerList[selected])
            || OriginalMessageBytes(identity.defender1) != OriginalMessageBytes(source.playerList[1-selected]))
            throw new InvalidOperationException("KF diagnostic actors differ from the explicit joint viewer");
        BindingFlags flags = BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic;
        FieldInfo holder = typeof(StaticData).GetField("playerData", flags);
        if (holder == null) throw new InvalidOperationException("original playerData context field missing");
        object previous = holder.GetValue(null);
        if (previous == null) throw new InvalidOperationException("original playerData context is not initialized");
        object viewer = typeof(object).GetMethod("MemberwiseClone", BindingFlags.Instance | BindingFlags.NonPublic).Invoke(previous, new object[0]);
        FieldInfo realm = viewer.GetType().GetField("zoneId", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        if (realm == null) throw new InvalidOperationException("original cached ZoneId field missing");
        realm.SetValue(viewer, Convert.ChangeType(viewerRealm, realm.FieldType, CultureInfo.InvariantCulture));
        FieldInfo pid = viewer.GetType().GetField("playerId", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        if (pid != null) pid.SetValue(viewer, Convert.ChangeType(viewerPID, pid.FieldType, CultureInfo.InvariantCulture));
        else
        {
            PropertyInfo property = viewer.GetType().GetProperty("playerId", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (property == null || !property.CanWrite) throw new InvalidOperationException("original viewer playerId is not writable on clone");
            property.SetValue(viewer, Convert.ChangeType(viewerPID, property.PropertyType, CultureInfo.InvariantCulture), null);
        }
        try
        {
            holder.SetValue(null, viewer);
            BattleData data = new BattleData();
            data.SetData(source);
            FieldInfo attackers = typeof(BattleData).GetField("attackFighters", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            FieldInfo defenders = typeof(BattleData).GetField("defendFighters", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            IList a = attackers == null ? null : attackers.GetValue(data) as IList;
            IList d = defenders == null ? null : defenders.GetValue(data) as IList;
            bool expectedFirst = !(source.playerList[0].inst.firstOrder == 0 && source.playerList[1].inst.firstOrder == 1);
            if (a == null || d == null || a.Count != 1 || d.Count != 1 || data.isAttToDef != (selected == 1) || data.attackFirst != expectedFirst)
                throw new InvalidOperationException("KF realm wrapper did not produce exactly one pair with original global initiative; install independently verified wrapper first");
            return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
        }
        finally { holder.SetValue(null, previous); }
    }

    // Original SCGrab.SetData fixes type19/forest room and both automatic
    // fighters; bookkeeping/winner reply fields cannot replace its raw pair.
    // Original WorldBoss SetData fixes attackFirst/type18 and takes exactly
    // the first monster group. A nonempty scene selects map_arena_001 in original IL.
    public static bool RunWorldBossAutomatic(SCFightPlayback identity, SCWorldBossFight start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 18
            || start.battlePlayer == null || start.battlePlayer.inst == null
            || start.battleMonsterList.Count == 0 || start.battleMonsterList[0] == null || start.battleMonsterList[0].inst == null
            || String.IsNullOrEmpty(start.fightScene) || identity.defender1 != null || identity.defender2 == null
            || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed
            || identity.maxRound != (start.maxround > 0 ? start.maxround : 40)
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.battlePlayer)
            || OriginalMessageBytes(identity.defender2) != OriginalMessageBytes(start.battleMonsterList[0]))
            throw new InvalidOperationException("WorldBoss18 exact typed projection or independent scene context missing");
        BattleData data = new BattleData();
        data.SetData(start);
        if ((int)data.battleType != 18 || !data.attackFirst || data.isAttToDef || data.mapSceneName != "map_arena_001")
            throw new InvalidOperationException("Original WorldBoss wrapper differs from recovered type18 branch");
        return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
    }

    // Awake16 uses the original typed overload and its saved auto-cache branch.
    // Restore the diagnostic static context after the original engine stops.
    public static bool RunAwakeAutomatic(SCFightPlayback identity, SCAwakeCopyFightBegin start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 16
            || start.battlePlayer == null || start.battlePlayer.inst == null
            || start.battleMonser == null || start.battleMonser.inst == null
            || String.IsNullOrEmpty(start.roomId) || identity.defender1 != null || identity.defender2 == null
            || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || identity.fightid != start.fightId || identity.randSeed != start.randomSeed
            || identity.maxRound != (start.maxRoundNum > 0 ? start.maxRoundNum : 40)
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.battlePlayer)
            || OriginalMessageBytes(identity.defender2) != OriginalMessageBytes(start.battleMonser))
            throw new InvalidOperationException("Awake16 exact typed projection or room context missing");
        bool previousAuto = StaticData.isPveAutoCache;
        try
        {
            StaticData.isPveAutoCache = true;
            BattleData data = new BattleData();
            data.SetData(start);
            if ((int)data.battleType != 16 || !data.attackFirst || data.isAttToDef || data.mapSceneName != start.roomId
                || data.GetFighter(0, 0) == null || !data.GetFighter(0, 0).isAuto)
                throw new InvalidOperationException("Original Awake16 wrapper or automatic branch differs");
            return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
        }
        finally { StaticData.isPveAutoCache = previousAuto; }
    }

    // Field PK has a separate type5 overload. The original field002 table
    // supplies its ice scene; no result, loot or initiative hint is supplied.
    public static bool RunFieldPVPAutomatic(SCFightPlayback identity, SidePvpFightData start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 5
            || start.attPlayer == null || start.attPlayer.inst == null || start.defPlayer == null || start.defPlayer.inst == null
            || identity.defender1 == null || identity.defender2 != null || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.fightid != start.id || identity.randSeed != start.randSeed || start.winPlayerId != 0
            || start.attGet.Count != 0 || start.defLose.Count != 0 || start.attFirstPlayerId != 0
            || identity.maxRound != (start.maxRoundCount > 0 ? start.maxRoundCount : 40)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.attPlayer)
            || OriginalMessageBytes(identity.defender1) != OriginalMessageBytes(start.defPlayer))
            throw new InvalidOperationException("Field5 exact typed source differs or supplies a preset result");
        string previousField = StaticData.fieldId;
        try
        {
            StaticData.fieldId = "field_002";
            BattleData data = new BattleData();
            data.SetData(start);
            bool expectedFirst = !(start.attPlayer.inst.firstOrder == 0 && start.defPlayer.inst.firstOrder == 1);
            if ((int)data.battleType != 5 || data.attackFirst != expectedFirst || data.isAttToDef || data.mapSceneName != "map_ice_001")
                throw new InvalidOperationException("Original field002 PK5 wrapper differs");
            return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
        }
        finally { StaticData.fieldId = previousField; }
    }

    public static bool RunWantedAutomatic(SCFightPlayback identity, SCRebelFightStart start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 8 || identity.maxRound != 20
            || start.battlePlayer == null || start.battlePlayer.inst == null
            || start.battleMonsterList.Count != 1 || start.battleMonsterList[0] == null || start.battleMonsterList[0].inst == null
            || identity.defender1 != null || identity.defender2 == null || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.attackerFactor != start.damageFactor || (start.damageFactor != 1.0 && start.damageFactor != 2.5)
            || identity.defenderFactor != 1.0 || identity.fightid != start.fightId || identity.randSeed != start.randSeed
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.battlePlayer)
            || OriginalMessageBytes(identity.defender2) != OriginalMessageBytes(start.battleMonsterList[0]))
            throw new InvalidOperationException("Wanted8 exact single-wave typed projection or original factor differs");
        bool previousAuto = StaticData.isPveAutoCache;
        try
        {
            StaticData.isPveAutoCache = true;
            BattleData data = new BattleData();
            data.SetData(start);
            string room = String.IsNullOrEmpty(start.fightRoomId) ? "map_arena_001" : start.fightRoomId;
            if ((int)data.battleType != 8 || !data.attackFirst || data.maxRound != 20 || data.mapSceneName != room
                || data.GetFighter(0, 0) == null || !data.GetFighter(0, 0).isAuto)
                throw new InvalidOperationException("Original Wanted8 typed wrapper or fixed20 differs");
            return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
        }
        finally { StaticData.isPveAutoCache = previousAuto; }
    }

    public static bool RunGrabAutomatic(SCFightPlayback identity, SCGrab start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 19 || start.attPlayer == null || start.defPlayer == null
            || identity.defender1 == null || identity.defender2 != null || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed || start.winPlayerId != 0
            || identity.maxRound != (start.maxRoundNum > 0 ? start.maxRoundNum : 40)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.attPlayer)
            || OriginalMessageBytes(identity.defender1) != OriginalMessageBytes(start.defPlayer))
            throw new InvalidOperationException("Grab19 original typed source differs or supplies a preset result");
        BattleData data = new BattleData();
        data.SetData(start);
        return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
    }

    public static bool RunLimitAutomatic(SCFightPlayback identity, SCLimitPvpFightResult start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 20 || start.playerList.Count != 2
            || identity.defender1 == null || identity.defender2 != null || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed || start.winPlayerId != 0
            || identity.maxRound != (start.maxRoundNum > 0 ? start.maxRoundNum : 40)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.playerList[0])
            || OriginalMessageBytes(identity.defender1) != OriginalMessageBytes(start.playerList[1]))
            throw new InvalidOperationException("Limit20 original typed callback and diagnostic identity differ or preset server winner");
        BattleData data = new BattleData();
        data.SetData(start);
        return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
    }

    public static bool RunArenaAutomatic(SCFightPlayback identity, SCPvpFightResult start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 2 || start.playerList.Count != 2
            || identity.defender1 == null || identity.defender2 != null || identity.optRecord != null || identity.winConditions.Count != 0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed || start.winPlayerId != 0
            || identity.maxRound != (start.maxRoundNum > 0 ? start.maxRoundNum : 40)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.playerList[0])
            || OriginalMessageBytes(identity.defender1) != OriginalMessageBytes(start.playerList[1]))
            throw new InvalidOperationException("Arena original typed callback and diagnostic identity differ or preset server winner");
        BattleData data = new BattleData();
        data.SetData(start);
        return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
    }

    // Original SetData(SCFightTreasureStartResult, targetFrag) fixes mode3,
    // Max20 and both automatic fighters. Room/max/result reply hints cannot
    // replace those original wrapper rules. No resource/fragment UI is invoked.
    public static bool RunPlunderAutomatic(SCFightPlayback identity, SCFightTreasureStartResult start, string targetFrag, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || identity.battleType != 3 || start.playerList.Count != 2
            || String.IsNullOrEmpty(targetFrag) || identity.defender1 == null || identity.defender2 != null
            || identity.optRecord != null || identity.winConditions.Count != 0 || identity.maxRound != 20
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed
            || start.isWin != 0 || start.isGetFrag != 0 || start.get != null || start.cost.Count != 0
            || !String.IsNullOrEmpty(start.reward) || !String.IsNullOrEmpty(start.rewardFake1) || !String.IsNullOrEmpty(start.rewardFake2)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.playerList[0])
            || OriginalMessageBytes(identity.defender1) != OriginalMessageBytes(start.playerList[1]))
            throw new InvalidOperationException("Plunder original typed source differs or contains preset result");
        BattleData data = new BattleData();
        data.SetData(start, targetFrag);
        return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
    }

    public static bool RunRushAutomatic(SCFightPlayback identity, SCRushFightStart start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || (identity.battleType != 9 && identity.battleType != 10)
            || start.battlePlayer == null || start.battleMonsterList.Count != 1 || identity.defender1 != null
            || identity.optRecord != null || identity.winConditions.Count != 0 || start.winKey != 0 || start.winValue != 0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed
            || identity.maxRound != (start.maxRound > 0 ? start.maxRound : 40)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.battlePlayer)
            || OriginalMessageBytes(identity.defender2) != OriginalMessageBytes(start.battleMonsterList[0]))
            throw new InvalidOperationException("Rush original typed callback and diagnostic identity differ or unsupported full-source condition");
        BattleData data = new BattleData();
        data.SetData(start, identity.battleType == 9);
        BattleFighterData player = data.GetFighter(0, 0);
        if (player == null) throw new InvalidOperationException("original Rush callback lacks its first player");
        player.isAuto = true;
        return RunWithInitializedData(identity, exports, snapshots, events, true, false, data);
    }

    // Independently named record playback diagnostic. The actual typed source
    // keeps its original manual-player constructor; the replay driver explicitly
    // projects isPlayBack/operateRecord and isAuto=true as original
    // SetPlaybackData(0012..0020,006c..007b) plus SetFightEngineData(00e3..0141).
    // This is not the original live UI driver and never changes source actors,
    // record clocks, numerical values or original Engine/AI methods.
    public static bool RunRushRecorded(SCFightPlayback identity, SCRushFightStart start, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || (identity.battleType != 9 && identity.battleType != 10)
            || start.battlePlayer == null || start.battleMonsterList.Count != 1 || identity.defender1 != null
            || identity.optRecord == null || identity.winConditions.Count != 0 || start.winKey != 0 || start.winValue != 0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed
            || identity.maxRound != (start.maxRound > 0 ? start.maxRound : 40)
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.battlePlayer)
            || OriginalMessageBytes(identity.defender2) != OriginalMessageBytes(start.battleMonsterList[0]))
            throw new InvalidOperationException("Rush recorded typed source and exact identity differ");
        BattleData data = new BattleData();
        data.SetData(start, identity.battleType == 9);
        BattleFighterData player = data.GetFighter(0, 0);
        if (player == null) throw new InvalidOperationException("original Rush record source lacks its player");
        data.isPlayBack = true;
        data.operateRecord = identity.optRecord;
        player.isAuto = true;
        return RunWithInitializedData(identity, exports, snapshots, events, false, false, data);
    }

    // Reproduce one actual original live control: the typed Rush source starts
    // manual, original Update accumulates each 0.1 tick, then original SetAuto
    // runs EnginePlayerAI at the supplied observed round clock. The source End
    // record is evidence only; it is never fed to PlaybackAI or used as a result.
    public static bool RunRushManualAutoStart(SCFightPlayback identity, SCRushFightStart start, double autoStartTime, Action<string, List<FExport>> exports, Action<EngineData> snapshots, Action<string, EngineData, object> events)
    {
        if (identity == null || start == null || (identity.battleType != 9 && identity.battleType != 10)
            || start.battlePlayer == null || start.battleMonsterList.Count != 1 || identity.defender1 != null
            || identity.winConditions.Count != 0 || start.winKey != 0 || start.winValue != 0
            || identity.fightid != start.fightId || identity.randSeed != start.randSeed
            || identity.maxRound != (start.maxRound > 0 ? start.maxRound : 40)
            || Double.IsNaN(autoStartTime) || Double.IsInfinity(autoStartTime) || autoStartTime < 0 || autoStartTime > 110
            || identity.attackerFactor != 1.0 || identity.defenderFactor != 1.0
            || OriginalMessageBytes(identity.attacker) != OriginalMessageBytes(start.battlePlayer)
            || OriginalMessageBytes(identity.defender2) != OriginalMessageBytes(start.battleMonsterList[0]))
            throw new InvalidOperationException("Rush manual-to-auto source or observed control differs");
        // Observed End binding or explicit diagnostic scheduling is verified by
        // the SHA-pinned loader; the live player AI never consumes optRecord.
        BattleData data = new BattleData();
        data.SetData(start, identity.battleType == 9);
        BattleFighterData player = data.GetFighter(0, 0);
        if (player == null || player.isAuto || data.isPlayBack)
            throw new InvalidOperationException("Original typed Rush must start manual and not playback");
        return RunWithManualAutoDelay(identity, exports, snapshots, events, true, false, data, autoStartTime);
    }

    private static string OriginalMessageBytes<T>(T message)
    {
        if (message == null) return "null";
        using (System.IO.MemoryStream stream = new System.IO.MemoryStream())
        {
            ProtoBuf.Serializer.Serialize<T>(stream, message);
            return Convert.ToBase64String(stream.ToArray());
        }
    }

    private static bool OriginalCopyEnemyMode(int battleType)
    {
        switch (battleType)
        {
            case 0: case 1: case 4: case 6: case 7: case 8:
            case 9: case 10: case 13: case 16: return true;
            default: return false;
        }
    }

    private static bool Run(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog, Action<string, EngineData, object> eventLog, bool automatic, bool localNoTargetFinish)
    {
        return RunWithInitializedData(originalWire, exportLog, snapshotLog, eventLog, automatic, localNoTargetFinish, null);
    }

    private static bool RunWithInitializedData(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog, Action<string, EngineData, object> eventLog, bool automatic, bool localNoTargetFinish, BattleData initialized)
    {
        return RunWithManualAutoDelay(originalWire, exportLog, snapshotLog, eventLog, automatic, localNoTargetFinish, initialized, -1.0);
    }

    private static bool RunWithManualAutoDelay(SCFightPlayback originalWire, Action<string, List<FExport>> exportLog, Action<EngineData> snapshotLog, Action<string, EngineData, object> eventLog, bool automatic, bool localNoTargetFinish, BattleData initialized, double manualAutoDelay)
    {
        BattleData previous = StaticData.curBattleData;
        var previousEndMessage = StaticData.lastBattleEndMsg;
        bool previousLastBattleWin = StaticData.isLastBattleWin;
        EngineDefault engine = null;
        try
        {
            BattleData data = initialized ?? new BattleData();
            if (initialized == null) SetDiagnosticBattleData(data, originalWire);
            StaticData.curBattleData = data; // Original DoUpdate reads isAttToDef here.
            BattleFighterData attacker = data.GetFighter(0, 0);
            BattleFighterData defender = data.GetFighter(1, 0);
            if (attacker == null || defender == null) throw new InvalidOperationException("original playback type has no first fighter pair");
            InFightData input = InFightData.BuildInFightData(data, attacker, defender);
            engine = new EngineDefault();
            engine.SetFightData(input); // Original ComputeCore and original table/actor initialization.
            Capture capture = new Capture();
            capture.Exports = exportLog;
            capture.Data = engine.GetEngineData();
            capture.Snapshot = snapshotLog;
            capture.Events = eventLog;
            capture.OriginalAutomatic = automatic;
            capture.Engine = engine;
            capture.LocalNoTargetFinish = localNoTargetFinish;
            engine.SetCallback(capture); // Capture avoids EngineMsgCallback's room/UI/socket dispatch.
            EnginePlaybackAI attackerAI = null;
            EnginePlaybackAI defenderAI = null;
            if (automatic)
            {
                capture.AttackerAI = new EnginePlayerAI();
                // Original BattleManager.SetFightEngineData selects by mode, not
                // fighter payload shape: UF14 guard still uses PlayerAI.
                capture.DefenderAI = OriginalCopyEnemyMode((int)data.battleType) ? (EngineAI)new EngineCopyEnemyAI() : new EnginePlayerAI();
                engine.SetAI((CampType)0, capture.AttackerAI);
                engine.SetAI((CampType)1, capture.DefenderAI);
            }
            else
            {
                attackerAI = new EnginePlaybackAI();
                attackerAI.SetOperateRecord(originalWire.optRecord);
                engine.SetAI((CampType)0, attackerAI);
                capture.AttackerAI = attackerAI;
                capture.DefenderAI = new EngineCopyEnemyAI();
                engine.SetAI((CampType)1, capture.DefenderAI);
            }
            // SetPlaybackData constructs both fighters with isAuto=true, which
            // SetData already copies. SetAuto before Start would immediately
            // Run an AI whose data/player/engine have not been initialized.
            if (snapshotLog != null) snapshotLog(engine.GetEngineData());
            engine.Start(); // Original Start initializes AI but leaves rounding=false.
            if(RoundAutoControls66!=null)engine.SetAuto((CampType)0,false);
                    engine.NextRound(); // Original view would request this after its visual entry.
            capture.PendingFinish = false; // PlaybackAI loads its queue after RoundBegin notifications.
            if (Formation87) {
                if (capture.Data.roundNum != 1 || capture.Data.roundTime != 0.0 || engine.IsAuto((CampType)0)) throw new InvalidOperationException("original manual round1 required");
                List<string> uids = new List<string>(); List<int> positions = new List<int>();
                int[] slots = new int[]{4,8,5,9,11,15};
                foreach (FActor actor in capture.Data.attacker.actors) {positions.Add(slots[uids.Count]);uids.Add(actor.uniqueId);}
                engine.Pause(); engine.DoTrans(uids,positions); engine.Continue();
            }
            bool manualAutoStarted = manualAutoDelay < 0;
            for (int ticks = 0; !capture.Done && ticks < 200000; ticks++)
            {
                if (ticks % 100 == 0) capture.Progress("caller_progress", ticks, null);
                if (capture.PendingNext)
                {
                    capture.PendingNext = false;
                    capture.PendingFinish = false;
                    if(RoundAutoControls66!=null)engine.SetAuto((CampType)0,false);
                    engine.NextRound();
                    capture.PendingFinish = false;
                }
                if (capture.Done) break;
                if (RoundAutoControls66!=null && capture.Data.curRoundCamp==(CampType)0 && !engine.IsAuto((CampType)0))
                {
                    double delay;
                    if(!RoundAutoControls66.TryGetValue(capture.Data.roundNum,out delay))throw new InvalidOperationException("missing round control");
                    if(capture.Data.roundTime>=delay) {
                        if(eventLog!=null)eventLog("round_live_auto_control",capture.Data,new Dictionary<string,object>{{"round",capture.Data.roundNum},{"requested",delay},{"actual",capture.Data.roundTime}});
                        engine.SetAuto((CampType)0,true);
                    }
                }
                if (RoundAutoControls66==null && !manualAutoStarted && capture.Data.roundNum == 1 && capture.Data.roundTime >= manualAutoDelay)
                {
                    if (engine.IsAuto((CampType)0)) throw new InvalidOperationException("Manual source became automatic before the observed control");
                    manualAutoStarted = true;
                    if (eventLog != null) eventLog("manual_auto_start", capture.Data, new Dictionary<string, object> {
                        { "requestedOriginalRoundTime", manualAutoDelay }, { "actualOriginalRoundTime", capture.Data.roundTime }
                    });
                    engine.SetAuto((CampType)0, true); // Original live UI control; no clocks or actor values are assigned.
                }
                bool tailProbe = originalWire.battleType == 17
                    && originalWire.fightid == "b874063b53a8612600a866de4fcccb27bdc51d36a3fae45d00051662d77c7cd3"
                    && capture.Data.roundNum == 23 && capture.Data.roundTime >= 24.6 && capture.Data.roundTime <= 25.6;
                if (tailProbe) capture.TailPhase("caller_tail_before_update", ticks);
                engine.Update(0.1); // Original Update performs its own 0.1 clock accumulation.
                if (tailProbe) capture.TailPhase("caller_tail_after_update", ticks);
                if (capture.TryLocalNoTargetFinish(ticks)) continue;
                EnginePlaybackAI activePlayback = capture.Data.GetCurRoundPlayer().camp == (CampType)0 ? attackerAI : defenderAI;
                if (capture.PendingFinish && !capture.Done && (automatic || PlaybackExhausted(activePlayback)))
                {
                    capture.PendingFinish = false;
                    engine.FinishRound(); // Deferred until original AI/player Update unwinds.
                    if (tailProbe) capture.TailPhase("caller_tail_after_finish", ticks);
                }
            }
            if (!capture.Done)
            {
                capture.Progress("caller_exhausted", 200000, null);
                throw new InvalidOperationException("bounded original headless playback did not end");
            }
            if (eventLog != null)
            {
                BattleOperate finalRecord = OriginalFinalRecord(engine);
                using (System.IO.MemoryStream stream = new System.IO.MemoryStream())
                {
                    ProtoBuf.Serializer.Serialize<BattleOperate>(stream, finalRecord);
                    eventLog("original_final_operates", engine.GetEngineData(), new Dictionary<string, object> {
                        { "schema", "original-recorded-operates-v1" },
                        { "recordAttacker", input.recordAttackerOperate },
                        { "recordDefender", input.recordDefenderOperate },
                        { "protobufBase64", Convert.ToBase64String(stream.ToArray()) }
                    });
                }
            }
            Battle.BattleEndCalc calc = new Battle.BattleEndCalc();
            calc.Init(data.battleType, (Battle.BattleCamp)0, capture.FinalData);
            return calc.CheckResult(); // Original EngineMsgCallback.BattleEnd uses subject camp 0.
        }
        finally
        {
            if (engine != null) engine.Stop();
            StaticData.curBattleData = previous;
            StaticData.lastBattleEndMsg = previousEndMessage;
            StaticData.isLastBattleWin = previousLastBattleWin;
        }
    }
}
