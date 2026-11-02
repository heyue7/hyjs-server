using System;
using System.Collections.Generic;
using System.Diagnostics;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;

namespace HYJS.DesertV4
{
    // A ping on the original connection is a fence: the server dispatches its
    // packets in order. An HTTP empty-active result alone cannot fence a Start.
    internal static class EncounterStartRecovery
    {
        static readonly object gate=new object();
        static readonly Dictionary<Packet,int> queued=new Dictionary<Packet,int>();
        static object dispatched,resuming;
        static int dispatchedEpoch,type,scene,epoch;
        static RoomBase room;
        static FieldWire.Session owner;
        static bool recovering,fenced;
        static long barrier,nextBarrier;
        static int retries;
        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static bool Current()
        {
            FieldWire.Session live;
            return owner!=null && epoch==PositionSync.ConnectionEpoch && FieldWire.Current(out live) && Same(owner,live);
        }
        internal static bool Pending { get { return owner!=null && Current(); } }
        internal static bool WaitingForFence { get { return Pending && recovering && !fenced; } }
        internal static void Sent(int kind,RoomBase source,int version,FieldWire.Session who)
        {
            type=kind; room=source; scene=version; owner=who; epoch=PositionSync.ConnectionEpoch;
            recovering=fenced=false; barrier=nextBarrier=0; retries=0;
        }
        internal static void ConnectionStarting()
        {
            // Old socket callbacks and queued bodies remain epoch-tagged. Cold
            // recovery discovers the accepted fight on the new connection.
            owner=null; room=null; resuming=null; recovering=fenced=false;
            lock(gate) { dispatched=null; dispatchedEpoch=0; }
        }
        internal static void FenceConnection(RoomBase source,int version,FieldWire.Session who)
        {
            Sent(0,source,version,who);
            Timeout();
        }
        internal static void TrackPacket(object value)
        {
            Packet packet=value as Packet;
            if (packet==null || (!(packet.kBody is SCSidePveFightStart) &&
                !(packet.kBody is SCSidePveBossFightStart) && !(packet.kBody is SCPingTcp))) return;
            lock(gate)
            {
                // Removing a stale entry cannot authorize an untracked Start.
                if (queued.Count>=512) queued.Clear();
                queued[packet]=PositionSync.ConnectionEpoch;
            }
        }
        internal static bool DropPacket(object value)
        {
            Packet packet=value as Packet;
            if (packet==null) return false;
            bool start=packet.kBody is SCSidePveFightStart || packet.kBody is SCSidePveBossFightStart;
            if (!start && !(packet.kBody is SCPingTcp)) return false;
            int observed=0; bool found;
            lock(gate)
            {
                found=queued.TryGetValue(packet,out observed); queued.Remove(packet);
                dispatched=start && found && observed==PositionSync.ConnectionEpoch ? packet.kBody : null;
                dispatchedEpoch=dispatched==null ? 0 : observed;
            }
            if (found && observed!=PositionSync.ConnectionEpoch) return start;
            SCPingTcp ping=packet.kBody as SCPingTcp;
            if (ping!=null && found && Current() && recovering && barrier!=0 && ping.clientTime==barrier)
            {
                fenced=true;
                // The original Start finished before this reply. Discovery may
                // now safely distinguish no acceptance from an in-flight Start.
                owner=null; room=null;
                FieldRuntime.ClearEncounterWait();
                ColdBattleRecovery.RefreshAfterStart();
            }
            return start && StaticData.fieldId=="field_003" && !found;
        }
        internal static bool DropStart(int kind,object net,object body)
        {
            if (resuming!=null && System.Object.ReferenceEquals(resuming,body)) { resuming=null; return false; }
            if (Pending && type==kind && (StaticData.fieldId!="field_003" || room!=RoomManager.currentRoom || scene!=FieldRuntime.SceneVersion)) return true;
            if (StaticData.fieldId!="field_003") return false;
            bool delivered;
            lock(gate)
            {
                delivered=System.Object.ReferenceEquals(dispatched,body) && dispatchedEpoch==PositionSync.ConnectionEpoch;
                dispatched=null; dispatchedEpoch=0;
            }
            if (!delivered || !Current() || type!=kind || room==null || room!=RoomManager.currentRoom ||
                scene!=FieldRuntime.SceneVersion || !System.Object.ReferenceEquals(room.netManager,net)) return true;
            if (recovering) return true; // Resume the frozen Start after the fence.
            owner=null; room=null;
            return false;
        }
        internal static void AdmitResume(object body) { resuming=body; }
        internal static void Timeout()
        {
            if (!Pending) { FieldRuntime.ClearEncounterWait(); return; }
            recovering=true; fenced=false; nextBarrier=0;
            ColdBattleRecovery.SuspendForStart();
            FieldRuntime.ClearEncounterWait();
            Tick();
        }
        internal static void Tick()
        {
            if (!WaitingForFence || !NetAPI.SocketIsConnected() || Stopwatch.GetTimestamp()<nextBarrier) return;
            // Reuse one exact echo token for retries. Never resend the Start.
            if (barrier==0) barrier=Int64.MinValue+(Stopwatch.GetTimestamp()&0x3fffffffffffffffL)+1;
            nextBarrier=Stopwatch.GetTimestamp()+5L*Stopwatch.Frequency;
            if (retries++>=3)
            {
                nextBarrier=Int64.MaxValue;
                // Use the original reconnect UI; a new epoch resumes discovery.
                NetAPI.Reconnect();
                return;
            }
            try { NetAPI.SendSocket(30020,new CSPingTcp { clientTime=barrier }); }
            catch (Exception) { }
        }
    }
}
