using System;
using System.Collections.Generic;
using System.Reflection;
using Battle;
using Battle.Engine;
using Battle.Communication;

namespace HYJS.WantedSkip87
{
    public static class WantedSkip
    {
        private static readonly BindingFlags Fields = BindingFlags.Instance | BindingFlags.NonPublic;
        private static readonly FieldInfo QueueField = typeof(MsgCtrl).GetField("ViewToEngineMsg", Fields);
        private static readonly FieldInfo HandlerField = typeof(MsgCtrl).GetField("viewToEngineHandler", Fields);
        private static readonly FieldInfo EndField = typeof(ComputeCore).GetField("end", Fields);

        private static bool IsWanted()
        {
            return StaticData.curBattleData != null && !StaticData.curBattleData.isPlayBack &&
                (int)StaticData.curBattleData.battleType == 8;
        }

        // isSkip is already set. Keep the original ViewToEngine FIFO/delegate
        // order; leave EngineToView with the original frame dispatcher.
        public static void BeforeSkip(BattleManager manager)
        {
            if (!IsWanted()) return;
            MsgCtrl messages = BattleMsgCtrl.GetInstance().msgCtrl;
            while (((Queue<BattlePacket>)QueueField.GetValue(messages)).Count > 0)
            {
                BattlePacket packet = ((Queue<BattlePacket>)QueueField.GetValue(messages)).Dequeue();
                PushMessage handler = (PushMessage)HandlerField.GetValue(messages);
                if (handler != null) handler(packet);
            }
        }

        // Original FinishRound can increment roundNum after BattleEnd.
        // Completed skipped battles no longer accept view control messages.
        public static bool IgnoreEndedControl(BattlePacket packet)
        {
            if (packet == null || !IsWanted()) return false;
            switch (packet.opcode)
            {
                case 51003: case 51006: case 51008: case 51010: case 51012: case 51021: break;
                default: return false;
            }
            BattleManager manager = BattleManager.Instance;
            return manager != null && manager.isSkip && manager.battleEngine != null &&
                manager.battleEngine.compute is ComputeCore &&
                (bool)EndField.GetValue(manager.battleEngine.compute);
        }
    }
}
