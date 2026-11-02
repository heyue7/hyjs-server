using System;
using System.Collections;
using System.Diagnostics;
using System.Reflection;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UI;

namespace HYJS.DesertV4
{
    // Scope the original HTTP callback and coroutine together. Retrying the
    // same monster uses the server's existing accepted-fight resume contract.
    public static class AwakeStartRecovery
    {
        sealed class Attempt
        {
            public FieldWire.Session owner;
            public RoomBase room;
            public object battle;
            public string monster;
            public long deadline;
            public bool received;
        }
        static Attempt current;
        static readonly FieldInfo Enter=typeof(ManagerController).GetField("enterBattle",BindingFlags.Instance|BindingFlags.NonPublic);
        static readonly FieldInfo Receive=typeof(ManagerController).GetField("receivePveMsg",BindingFlags.Instance|BindingFlags.NonPublic);
        static bool Owned(Attempt a)
        {
            FieldWire.Session live;
            return a!=null && current==a && FieldWire.Current(out live) && a.owner!=null &&
                live.playerId==a.owner.playerId && live.sessionHex==a.owner.sessionHex && live.origin==a.owner.origin;
        }
        static bool Scope(Attempt a)
        {
            return Owned(a) && System.Object.ReferenceEquals(a.battle,StaticData.curBattleData) &&
                a.room==RoomManager.currentRoom && a.monster==StaticData.awakeCopyFightMonGuid;
        }
        static void Cancel(Attempt a,bool report)
        {
            if (current!=a) return;
            bool owned=Scope(a); current=null;
            if (!owned) return;
            ManagerController manager=ManagerController.Instance;
            if (manager!=null)
            {
                if (Enter!=null) Enter.SetValue(manager,false);
                if (Receive!=null) Receive.SetValue(manager,false);
            }
            LoadingPanel loading=LoadingPanel.Instance;
            if (loading!=null) loading.HideMask(.5f);
            UIChallengeRunMap page=PageManager.instance==null ? null : PageManager.instance.currentPage as UIChallengeRunMap;
            if (page!=null) page.EndIntoBattleEffect();
            // This is the original failed-entry action. Login/exploration can
            // resume an accepted monster; no second result or reward is made.
            if (report) BoxManager.CreateReloginBox(LanguageTool.GetTextNoUpdate(1047));
        }
        public static bool BeforeStart()
        {
            if (current==null) return false;
            if (!Scope(current)) { current=null; return false; }
            BoxManager.CreatePopupTextBox("本场战斗正在确认",0f); return true;
        }
        public static bool SendHttp(int opcode,object request,HttpHandler callback)
        {
            CSAwakeCopyFightBegin begin=request as CSAwakeCopyFightBegin;
            if (opcode!=81009 || begin==null) return NetAPI.SendHttp(opcode,request,callback);
            FieldWire.Session owner;
            if (!FieldWire.Current(out owner)) return false;
            Attempt attempt=new Attempt { owner=owner,room=RoomManager.currentRoom,battle=StaticData.curBattleData,
                monster=begin.monGuid,deadline=Stopwatch.GetTimestamp()+15L*Stopwatch.Frequency };
            current=attempt;
            try
            {
                bool sent=NetAPI.SendHttp(opcode,request,delegate(Packet packet,bool success)
                {
                    if (!Scope(attempt) || attempt.received) return;
                    if (Stopwatch.GetTimestamp()>=attempt.deadline) { Cancel(attempt,true); return; }
                    SCAwakeCopyFightBegin reply=packet==null ? null : packet.kBody as SCAwakeCopyFightBegin;
                    if (!success || reply==null || String.IsNullOrEmpty(reply.fightId)) { Cancel(attempt,true); return; }
                    attempt.received=true;
                    try { callback(packet,true); }
                    catch (Exception) { Cancel(attempt,true); }
                });
                if (!sent) Cancel(attempt,true);
                return sent;
            }
            catch (Exception) { Cancel(attempt,true); return false; }
        }
        public static IEnumerator Guard(IEnumerator original)
        {
            Attempt attempt=current;
            // Playback has no HTTP request; preserve its original iterator.
            if (attempt==null)
            {
                while (original.MoveNext()) yield return original.Current;
                yield break;
            }
            while (Scope(attempt))
            {
                if (Stopwatch.GetTimestamp()>=attempt.deadline) { Cancel(attempt,true); yield break; }
                bool more;
                try { more=original.MoveNext(); }
                catch (Exception) { Cancel(attempt,true); yield break; }
                if (!more) { if (current==attempt) current=null; yield break; }
                yield return original.Current;
            }
            Cancel(attempt,false);
        }
    }
}
