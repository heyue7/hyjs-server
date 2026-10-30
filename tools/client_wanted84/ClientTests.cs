using System;
using System.IO;
using HYJS.Feedback65;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using UI;

public static class ClientTests
{
    static void Check(bool condition, string message) { if (!condition) throw new Exception(message); }
    static byte[] Encode(CSRebelFightEnd end)
    {
        using (MemoryStream stream = new MemoryStream()) { ProtoBuf.Serializer.Serialize<CSRebelFightEnd>(stream, end); return stream.ToArray(); }
    }
    static bool Same(byte[] a, byte[] b)
    {
        if (a.Length != b.Length) return false;
        for (int i = 0; i < a.Length; i++) if (a[i] != b[i]) return false;
        return true;
    }
    static void Reset()
    {
        NetAPI.Reset(); TestScheduler.Reset();
        StaticData.playerData = new UIPlayerData { playerId = 41, score = 100, attempts = 9 };
        StaticData.sessionKey = new byte[] { 1, 2, 3, 4 };
        StaticData.curBattleData = new BattleData();
        ManagerController.Instance = new ManagerController();
        BoxManager.lastMessage = null;
    }
    static Tuple<WantedMission, object, CSRebelFightEnd, HttpHandler> Begin()
    {
        WantedMission page = new WantedMission("wanted-A");
        ManagerController.Instance.page = page;
        Runtime.WantedRequest(page, 1);
        SCRebelFightStart start = new SCRebelFightStart { fightId = "paid-fight", seed = 1234 };
        Check(!Runtime.WantedStart(page, new Packet(55004, start), true), "original Start callback suppressed");
        StaticData.curBattleData.SetData(start);
        object owner = new object();
        Runtime.WantedSettlement(owner);
        CSRebelFightEnd end = new CSRebelFightEnd { fightId = start.fightId, result = 0, record = "actual frozen terminal" };
        Runtime.WantedCaptureEnd(owner, end);
        HttpHandler original = delegate(Packet packet, bool success) { Runtime.WantedResult(owner, packet, success); };
        return Tuple.Create(page, owner, end, Runtime.WantedWrapEnd(original));
    }
    static void ListSnapshot(int score, int attempts)
    {
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55001, "expected original list request");
        SCGetRebels list = new SCGetRebels();
        list.rebels.AddRange(ManagerController.Instance.page.BossSnapshot());
        NetAPI.Reply(new Packet(55002, list), true);
        // Original HTTPManager applies its external SCPlayer after the list callback.
        StaticData.playerData.score = score;
        StaticData.playerData.attempts = attempts;
    }
    static void RunAutoRetryAfterList()
    {
        TestScheduler.Tick();
        Check(NetAPI.pending == null, "retry raced list snapshot frame");
        TestScheduler.Tick();
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55011, "same End was not retried after list snapshot");
    }
    static void RetryableReplaysPaidFight()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        Check(ManagerController.Instance.leaveCount == 1, "retryable failure did not return to list");
        ListSnapshot(100, 9);
        TestScheduler.Tick();
        Check(NetAPI.ends == 0, "rejected recording was automatically resubmitted");
        Check(Runtime.WantedResume(flow.Item1, 1), "original paid attack was not intercepted");
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55003 &&
              ((CSRebelFightStart)NetAPI.pending.body).rebelId == "wanted-recover:paid-fight" &&
              ((CSRebelFightStart)NetAPI.pending.body).type == 1 && ManagerController.Instance.enterCount == 0,
              "paid fight was not named in original Start request");
        NetAPI.Reply(new Packet(55004, new SCRebelFightStart { fightId = "paid-fight", seed = 1234 }), true);
        Check(ManagerController.Instance.enterCount == 1 && StaticData.curBattleData.fightId == "paid-fight" &&
              StaticData.curBattleData.seed == 1234 && NetAPI.ends == 0 && StaticData.playerData.attempts == 9,
              "server-confirmed Start was not restored exactly");
    }
    static void InvalidRecoveryReplyNeverEntersBattle()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        ListSnapshot(100, 9);
        Check(Runtime.WantedResume(flow.Item1, 1), "expected paid recovery request");
        NetAPI.Reply(new Packet(55004, new SCRebelFightStart { fightId = "different-fight", seed = 1234 }), true);
        Check(ManagerController.Instance.enterCount == 0 && StaticData.playerData.attempts == 9,
              "mismatched paid Start entered battle");
        Check(Runtime.WantedResume(flow.Item1, 1), "recovery ownership was lost after mismatch");
        Check(((CSRebelFightStart)NetAPI.pending.body).rebelId == "wanted-recover:paid-fight", "recovery fell through to paid new Start");
        NetAPI.Reply(new Packet(10001, new ErrorMessage { code = "90003", msg = "already committed" }), false);
        Check(ManagerController.Instance.enterCount == 0 && StaticData.playerData.attempts == 9,
              "committed fight error entered battle or deducted locally");
    }
    static void ConfirmedCommittedRefreshesAndAllowsNewAttack()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        ListSnapshot(100, 9);
        Check(Runtime.WantedResume(flow.Item1, 1), "paid recovery not sent");
        NetAPI.Reply(new Packet(10001, new ErrorMessage { code = "90003", msg = "本次悬赏挑战已结算(其他)" }), false);
        Check(Runtime.WantedResume(flow.Item1, 1), "similar error incorrectly cleared recovery");
        Check(((CSRebelFightStart)NetAPI.pending.body).rebelId == "wanted-recover:paid-fight", "similar error became a new attack");
        NetAPI.Reply(new Packet(10001, new ErrorMessage { code = "90003", msg = "本次悬赏挑战已结算" }), false);
        TestScheduler.Tick();
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55001, "confirmed settlement did not refresh player snapshot");
        ListSnapshot(130, 9);
        Check(!Runtime.WantedResume(flow.Item1, 1), "confirmed settlement still blocks new attack");
        flow.Item1.SendEnterBattle(1);
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55003 && ((CSRebelFightStart)NetAPI.pending.body).rebelId == "wanted-A",
              "new user attack did not retain the original instance ID");
    }
    static void SupersededStartCallbackCannotEnterBattle()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        ListSnapshot(100, 9);
        Check(Runtime.WantedResume(flow.Item1, 1), "first paid recovery not sent");
        HttpHandler previous = NetAPI.pending.handler;
        NetAPI.Reply(new Packet(10001, new ErrorMessage { code = "90003", msg = "busy" }), false);
        Check(Runtime.WantedResume(flow.Item1, 1), "second paid recovery not sent");
        previous(new Packet(55004, new SCRebelFightStart { fightId = "paid-fight", seed = 1234 }), true);
        Check(ManagerController.Instance.enterCount == 0, "superseded Start callback entered battle");
        NetAPI.Reply(new Packet(55004, new SCRebelFightStart { fightId = "paid-fight", seed = 1234 }), true);
        Check(ManagerController.Instance.enterCount == 1 && StaticData.playerData.attempts == 9,
              "current Start callback was lost or deducted again");
    }
    static void RecoveredBattleFinishedElsewhereRefreshesWithoutAward()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        ListSnapshot(100, 9);
        Check(Runtime.WantedResume(flow.Item1, 1), "paid recovery not sent");
        NetAPI.Reply(new Packet(55004, new SCRebelFightStart { fightId = "paid-fight", seed = 1234 }), true);
        object newOwner = new object();
        Runtime.WantedSettlement(newOwner);
        Runtime.WantedCaptureEnd(newOwner, new CSRebelFightEnd { fightId = "paid-fight", result = 1, record = "different replay" });
        HttpHandler callback = Runtime.WantedWrapEnd(delegate(Packet packet, bool success) { Runtime.WantedResult(newOwner, packet, success); });
        callback(new Packet(10001, new ErrorMessage { code = "90003", msg = "本次悬赏挑战已结算" }), false);
        Check(ManagerController.Instance.leaveCount == 2 && NetAPI.pending != null && NetAPI.pending.opcode == 55001,
              "confirmed concurrent settlement did not return to the original list");
        ListSnapshot(130, 9);
        Check(StaticData.playerData.score == 130 && StaticData.playerData.attempts == 9 &&
              StaticData.playerData.addCalls == 0 && StaticData.playerData.deleteCalls == 0 && !Runtime.WantedResume(flow.Item1, 1),
              "confirmed concurrent settlement applied a duplicate reward or blocked new attacks");
    }
    static void ExpiredRejectedBossDoesNotBlockAnotherAttack()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        flow.Item1.SetBoss(null);
        ListSnapshot(100, 9);
        TestScheduler.Tick();
        flow.Item1.SetBoss("wanted-B");
        Check(!Runtime.WantedResume(flow.Item1, 1), "absent rejected boss still blocked the original attack button");
        flow.Item1.SendEnterBattle(1);
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55003 && ((CSRebelFightStart)NetAPI.pending.body).rebelId == "wanted-B" &&
              StaticData.playerData.attempts == 9, "new selected boss did not use the original paid-attack request");
    }
    static void UnknownEndThenRejectedMissingBossAllowsNewAttack()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90003", msg = "unknown" }), false);
        flow.Item1.SetBoss(null);
        ListSnapshot(100, 9);
        RunAutoRetryAfterList();
        NetAPI.Reply(new Packet(10001, new ErrorMessage { code = "90005", msg = "record conflict" }), false);
        TestScheduler.Tick();
        flow.Item1.SetBoss("wanted-B");
        Check(!Runtime.WantedResume(flow.Item1, 1), "missing boss after confirmed rejection still blocked a new attack");
        flow.Item1.SendEnterBattle(1);
        Check(NetAPI.pending != null && ((CSRebelFightStart)NetAPI.pending.body).rebelId == "wanted-B" &&
              StaticData.playerData.attempts == 9, "missing boss recovery changed or duplicated the next attempt");
    }
    static void UnknownEndUsesExactRequestAndAuthoritativeSnapshot()
    {
        Reset();
        var flow = Begin();
        byte[] original = Encode(flow.Item3);
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90003", msg = "unknown" }), false);
        Check(ManagerController.Instance.leaveCount == 1 && NetAPI.pending.opcode == 55001, "unknown result did not return to original list");
        // A killed or expired boss can disappear from that refreshed list.
        flow.Item1.SetBoss(null);
        ListSnapshot(130, 9);
        RunAutoRetryAfterList();
        Check(Same(original, Encode((CSRebelFightEnd)NetAPI.pending.body)), "retry changed End bytes");
        HttpHandler retry = NetAPI.pending.handler;
        NetAPI.Reply(new Packet(55012, new SCRebelFightEnd { get = 30, cost = 1 }), true);
        Check(StaticData.playerData.score == 130 && StaticData.playerData.attempts == 9 &&
              StaticData.playerData.addCalls == 0 && StaticData.playerData.deleteCalls == 0, "authoritative list snapshot had End deltas applied twice");
        retry(new Packet(55012, new SCRebelFightEnd { get = 30, cost = 1 }), true);
        Check(StaticData.playerData.addCalls == 0 && StaticData.playerData.deleteCalls == 0, "duplicate callback changed assets");
        TestScheduler.Tick();
        Check(NetAPI.pending != null && NetAPI.pending.opcode == 55001, "accepted receipt did not request a fresh player snapshot");
        ListSnapshot(130, 9);
        Check(flow.Item1.showCalls == 2, "unexpected recovery loop");
    }
    static void NewLoginRejectsLateReceipt()
    {
        Reset();
        var flow = Begin();
        flow.Item4(new Packet(10001, new ErrorMessage { code = "90003", msg = "unknown" }), false);
        ListSnapshot(130, 9);
        RunAutoRetryAfterList();
        // BackToLogin -> ClearAllData creates a new player object even for the same role.
        StaticData.playerData = new UIPlayerData { playerId = 41, score = 130, attempts = 9 };
        NetAPI.Reply(new Packet(55012, new SCRebelFightEnd { get = 30, cost = 1 }), true);
        TestScheduler.Tick();
        Check(StaticData.playerData.score == 130 && StaticData.playerData.attempts == 9 &&
              StaticData.playerData.addCalls == 0 && StaticData.playerData.deleteCalls == 0 && NetAPI.pending == null,
              "stale response affected new login");
    }
    static void InBattleSuccessKeepsOriginalCallback()
    {
        Reset();
        var flow = Begin();
        bool intercepted = Runtime.WantedResult(flow.Item2, new Packet(55012, new SCRebelFightEnd { get = 30, cost = 1 }), true);
        Check(!intercepted && ManagerController.Instance.leaveCount == 0 && NetAPI.ends == 0,
              "successful in-battle receipt did not keep its original callback");
    }
    public static void Main()
    {
        RetryableReplaysPaidFight();
        InvalidRecoveryReplyNeverEntersBattle();
        ConfirmedCommittedRefreshesAndAllowsNewAttack();
        SupersededStartCallbackCannotEnterBattle();
        RecoveredBattleFinishedElsewhereRefreshesWithoutAward();
        ExpiredRejectedBossDoesNotBlockAnotherAttack();
        UnknownEndThenRejectedMissingBossAllowsNewAttack();
        UnknownEndUsesExactRequestAndAuthoritativeSnapshot();
        NewLoginRejectsLateReceipt();
        InBattleSuccessKeepsOriginalCallback();
        Console.WriteLine("Wanted 84 callback and recovery scenarios passed");
    }
}
