using System;
using System.Collections.Generic;
using HYJS.WantedSkip87;

public enum BattleType { Other = 1, Wanted = 8 }
public class BattleData { public BattleType battleType; public bool isPlayBack; }
public static class StaticData { public static BattleData curBattleData; }
namespace Battle.Engine {
    public class BaseCompute { }
    public class ComputeCore : BaseCompute {
        protected bool end;
        public int Round = 1;
        public void SetEnd(bool value) { end = value; }
        public void FinishRound() { Round++; }
    }
    public class EngineDefault { public BaseCompute compute; }
}
namespace Battle {
    public class BattlePacket { public int opcode; public int Id; }
    public class BattleManager {
        public static BattleManager Instance;
        public bool isSkip;
        public Engine.EngineDefault battleEngine;
    }
    public class BattleMsgCtrl {
        public static BattleMsgCtrl Instance;
        public Communication.MsgCtrl msgCtrl;
        public static BattleMsgCtrl GetInstance() { return Instance; }
    }
}
namespace Battle.Communication {
    public delegate void PushMessage(Battle.BattlePacket packet);
    public class MsgCtrl {
        private Queue<Battle.BattlePacket> ViewToEngineMsg = new Queue<Battle.BattlePacket>();
        private PushMessage viewToEngineHandler;
        public Queue<Battle.BattlePacket> EngineToView = new Queue<Battle.BattlePacket>();
        public void SetHandler(PushMessage handler) { viewToEngineHandler = handler; }
        public void Add(int opcode, int id) { ViewToEngineMsg.Enqueue(new Battle.BattlePacket { opcode = opcode, Id = id }); }
        public int Count { get { return ViewToEngineMsg.Count; } }
        public void ReplaceQueue() { ViewToEngineMsg = new Queue<Battle.BattlePacket>(); }
    }
}
public static class ModelTests {
    private static int checks;
    private static void Check(bool value, string message) { if (!value) throw new Exception(message); checks++; }
    public static int Main() {
        var core = new Battle.Engine.ComputeCore();
        var manager = new Battle.BattleManager { isSkip = true, battleEngine = new Battle.Engine.EngineDefault { compute = core } };
        Battle.BattleManager.Instance = manager;
        var messages = new Battle.Communication.MsgCtrl();
        Battle.BattleMsgCtrl.Instance = new Battle.BattleMsgCtrl { msgCtrl = messages };
        StaticData.curBattleData = new BattleData { battleType = BattleType.Wanted };
        foreach (int opcode in new[] { 51003, 51006, 51008, 51010, 51012, 51021 }) {
            core.SetEnd(false);
            Check(!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = opcode }), "live control blocked");
            core.SetEnd(true);
            Check(WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = opcode }), "ended control accepted");
        }
        Check(!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = 55011 }), "settlement intercepted");
        Check(!WantedSkip.IgnoreEndedControl(null), "null changed");
        core.FinishRound();
        Check(core.Round == 2, "original unguarded tail model missing");
        core.Round = 1;
        if (!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = 51006 })) core.FinishRound();
        Check(core.Round == 1, "late finish changed terminal");
        manager.isSkip = false;
        Check(!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = 51006 }), "ordinary wanted changed");
        manager.isSkip = true;
        StaticData.curBattleData.isPlayBack = true;
        Check(!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = 51006 }), "wanted playback changed");
        var playback = new Battle.Communication.MsgCtrl();
        playback.Add(51012, 99);
        Battle.BattleMsgCtrl.Instance.msgCtrl = playback;
        WantedSkip.BeforeSkip(manager);
        Check(playback.Count == 1, "wanted playback queue drained");
        Battle.BattleMsgCtrl.Instance.msgCtrl = messages;
        StaticData.curBattleData.isPlayBack = false;
        StaticData.curBattleData.battleType = BattleType.Other;
        Check(!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = 51006 }), "other mode changed");
        messages.Add(51012, 1);
        WantedSkip.BeforeSkip(manager);
        Check(messages.Count == 1, "other mode queue drained");
        StaticData.curBattleData.battleType = BattleType.Wanted;
        core.SetEnd(false);
        var order = new List<int>();
        messages.EngineToView.Enqueue(new Battle.BattlePacket { opcode = 51020 });
        messages.SetHandler(delegate(Battle.BattlePacket packet) {
            if (!WantedSkip.IgnoreEndedControl(packet)) {
                order.Add(packet.Id);
                if (packet.Id == 2) core.SetEnd(true);
            }
        });
        messages.Add(51012, 2);
        messages.Add(51006, 3);
        WantedSkip.BeforeSkip(manager);
        Check(order.Count == 2 && order[0] == 1 && order[1] == 2, "FIFO or terminal boundary changed");
        Check(messages.Count == 0 && messages.EngineToView.Count == 1, "wrong queue consumed");
        messages.SetHandler(null);
        messages.Add(51012, 4);
        WantedSkip.BeforeSkip(manager);
        Check(messages.Count == 0, "original absent-handler dequeue changed");
        messages.Add(51012, 5);
        messages.Add(51012, 6);
        var expected = new InvalidOperationException("original handler failure");
        messages.SetHandler(delegate(Battle.BattlePacket packet) { throw expected; });
        try { WantedSkip.BeforeSkip(manager); throw new Exception("handler failure swallowed"); }
        catch (InvalidOperationException error) { Check(Object.ReferenceEquals(error, expected), "handler exception replaced"); }
        Check(messages.Count == 1 && messages.EngineToView.Count == 1, "failure consumed unrelated messages");
        messages.SetHandler(delegate(Battle.BattlePacket packet) { messages.ReplaceQueue(); messages.Add(51012, 8); messages.SetHandler(delegate(Battle.BattlePacket next) { order.Add(next.Id); }); });
        WantedSkip.BeforeSkip(manager);
        Check(order[order.Count - 1] == 8 && messages.Count == 0, "queue replacement did not follow original dispatcher");
        StaticData.curBattleData = null;
        Check(!WantedSkip.IgnoreEndedControl(new Battle.BattlePacket { opcode = 51006 }), "no battle changed");
        Console.WriteLine("PASS " + checks + " Wanted skip boundary checks; models are not Unity/device execution");
        return 0;
    }
}
