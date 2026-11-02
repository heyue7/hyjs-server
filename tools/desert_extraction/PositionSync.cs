using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Reflection;
using System.Threading;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Original SCCurrentPosition has no request ID. Track send order for the
    // connection, tag queued replies by epoch, and gate unlock on server version.
    public static class PositionSync
    {
        static readonly object gate=new object();
        static readonly Dictionary<Packet,int> packetEpoch=new Dictionary<Packet,int>();
        static readonly Queue<long> sent=new Queue<long>();
        sealed class DeferredPosition
        {
            public int epoch;
            public SCCurrentPosition frame;
        }
        static readonly Queue<DeferredPosition> deferred=new Queue<DeferredPosition>();
        // Only a body taken from the original socket event queue can satisfy a
        // request. RoomNetBase.Tick also calls the position handler directly
        // with a locally fabricated fallback after its scene-load timeout.
        static object dispatchedBody;
        static int dispatchedEpoch;
        static int epoch;
        static long sequence,expected,lockGeneration;
        static FieldWire.Session owner;
        static bool overflow,confirmedBase,touchWasEnabled,authenticated,unboundLogin,reconnectRequested;
        static long nextBaseRetry,firstOutstandingAt;
        static volatile bool fieldActive,leaveActive,locked;
        static Action<Vector3,string> onResponse;
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        static bool Current()
        {
            FieldWire.Session live;
            if (!FieldWire.Current(out live)) return false;
            if (!SameSession(owner,live))
            {
                if (!SameActor(owner,live)) Release();
                if (owner!=null)
                {
                    authenticated=false; unboundLogin=false; sent.Clear(); deferred.Clear(); sequence=expected=0;
                    confirmedBase=false; overflow=false; onResponse=null; nextBaseRetry=firstOutstandingAt=0; reconnectRequested=false;
                }
                // Keep sends observed on this connection before playerData
                // became available. An established owner's change resets them.
                owner=live;
            }
            return true;
        }
        public static void SetFieldActive(bool enabled) { fieldActive=enabled; }
        // Leave recovery can outlive the desert room and still receive an old
        // socket callback after a same-process reconnect in town.
        internal static void SetLeaveActive(bool enabled) { leaveActive=enabled; }
        public static int ConnectionEpoch { get { return Thread.VolatileRead(ref epoch); } }
        public static bool Locked { get { return locked; } }
        public static void ConnectionStarting()
        {
            Interlocked.Increment(ref epoch);
            lock(gate) { packetEpoch.Clear(); dispatchedBody=null; dispatchedEpoch=0; }
            sent.Clear(); deferred.Clear(); sequence=expected=0; confirmedBase=false; overflow=false; authenticated=false; unboundLogin=false; onResponse=null; nextBaseRetry=firstOutstandingAt=0; reconnectRequested=false;
        }
        public static void LoginResponse(object value)
        {
            var response=value as SCSocketLogin;
            if (response==null || response.result!=100) return;
            if (Current()) { authenticated=true; unboundLogin=false; }
            else unboundLogin=true;
        }
        // Called before SocketTool.ReceiveHandler on its network callback thread.
        // A late callback from an old socket may not enqueue into the new one.
        public static bool DropOldSocket(object tool,IAsyncResult asyncResult)
        {
            if (!fieldActive && !leaveActive) return false;
            try
            {
                object helper=asyncResult==null ? null : asyncResult.AsyncState;
                if (tool==null || helper==null) return true;
                const BindingFlags flags=BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public;
                FieldInfo current=tool.GetType().GetField("socket",flags), source=helper.GetType().GetField("socket",flags);
                if (current==null || source==null) return true;
                object a=current.GetValue(tool),b=source.GetValue(helper);
                return a==null || b==null || !System.Object.ReferenceEquals(a,b);
            }
            catch (Exception) { return true; }
        }
        public static void TrackPacket(object value)
        {
            Packet p=value as Packet;
            if (p==null || !(p.kBody is SCCurrentPosition)) return;
            lock(gate)
            {
                if (packetEpoch.Count>=512) { overflow=true; return; }
                packetEpoch[p]=ConnectionEpoch;
            }
        }
        public static bool DropPacket(object value)
        {
            Packet p=value as Packet;
            if (p==null || !(p.kBody is SCCurrentPosition)) return false;
            int generation; bool found;
            lock(gate)
            {
                found=packetEpoch.TryGetValue(p,out generation); packetEpoch.Remove(p);
                // EventManger invokes the handler synchronously with this same
                // body. Unobserved or old-epoch packets never prove a reply.
                dispatchedBody=found && generation==ConnectionEpoch ? p.kBody : null;
                dispatchedEpoch=dispatchedBody==null ? 0 : generation;
            }
            return fieldActive && (!found || generation!=ConnectionEpoch);
        }
        static bool TakeDispatched(object value,out int dispatched)
        {
            dispatched=0;
            lock(gate)
            {
                if (dispatchedBody==null || !System.Object.ReferenceEquals(value,dispatchedBody) || dispatchedEpoch!=ConnectionEpoch) return false;
                dispatched=dispatchedEpoch;
                dispatchedBody=null; dispatchedEpoch=0; return true;
            }
        }
        // These hooks are observational in all scenes and never throw into the
        // original TCP methods; overflow leaves relocation unavailable.
        public static void TrackSend(int opcode)
        {
            if (opcode!=30024) return;
            try
            {
                // A native request may precede playerData. Preserve its order
                // under this connection epoch and bind its reply once the
                // owner is known; ConnectionStarting always clears the queue.
                Current();
                if (sequence==Int64.MaxValue || sent.Count>=64) { overflow=true; return; }
                if (sent.Count==0) firstOutstandingAt=Stopwatch.GetTimestamp();
                sent.Enqueue(++sequence);
            }
            catch (Exception) { overflow=true; }
        }
        public static void TrackBaseResponse(object net,object value)
        { if (!(net is RoomNetField)) TrackResponse(value); }
        public static void TrackFieldResponse(object value) { TrackResponse(value); }
        static void TrackResponse(object value)
        {
            try
            {
                // The timeout fallback is a visual recovery, not a reply to
                // 30024. Leave the real request queued for its eventual frame.
                int dispatched;
                if (!TakeDispatched(value,out dispatched) || dispatched!=ConnectionEpoch) return;
                var rsp=value as SCCurrentPosition;
                bool current=Current();
                if (dispatched!=ConnectionEpoch) return;
                if (!current || deferred.Count>0)
                {
                    // Every saved response reserves one existing request.
                    // No unsolicited or locally fabricated frame enters it.
                    if (rsp==null || deferred.Count>=64 || deferred.Count>=sent.Count) { confirmedBase=false; return; }
                    deferred.Enqueue(new DeferredPosition { epoch=dispatched,frame=rsp });
                    return;
                }
                AcceptResponse(rsp);
            }
            catch (Exception) { confirmedBase=false; }
        }
        static bool AcceptResponse(SCCurrentPosition rsp)
        {
            if (sent.Count==0) { confirmedBase=false; return false; }
            long actual=sent.Dequeue();
            firstOutstandingAt=sent.Count==0 ? 0 : Stopwatch.GetTimestamp();
            if (rsp==null || rsp.self==null || rsp.self.info==null || rsp.self.position==null || rsp.self.position.current==null ||
                rsp.self.info.playerId!=owner.playerId) { confirmedBase=false; return false; }
            PointNew p=rsp.self.position.current;
            Vector3 position=new Vector3((float)p.x,(float)p.y,(float)p.z);
            if (Single.IsNaN(position.x)||Single.IsNaN(position.y)||Single.IsNaN(position.z)||
                Single.IsInfinity(position.x)||Single.IsInfinity(position.y)||Single.IsInfinity(position.z))
            { confirmedBase=false; return false; }
            if (unboundLogin) { authenticated=true; unboundLogin=false; }
            confirmedBase=true;
            if (expected!=0 && actual==expected)
            {
                expected=0; Action<Vector3,string> callback=onResponse; onResponse=null;
                if (callback!=null) callback(position,rsp.self.position.scene);
            }
            return true;
        }
        static void DrainDeferred()
        {
            try
            {
                if (deferred.Count==0) return;
                RoomBase room=RoomManager.currentRoom;
                if (room==null || room.netManager==null || room.charactorManager==null || room.charactorManager.GetSelfCharactor()==null) return;
                SCCurrentPosition last=null;
                while (deferred.Count>0)
                {
                    if (!Current() || deferred.Count==0 || room!=RoomManager.currentRoom) return;
                    DeferredPosition value=deferred.Dequeue();
                    if (value.epoch!=ConnectionEpoch) { deferred.Clear(); confirmedBase=false; return; }
                    if (AcceptResponse(value.frame)) last=value.frame;
                }
                // The native handler ran before playerData was available, so its
                // original readiness hook could not bind this real frame. Repeat
                // only the readiness observation after ordered validation.
                if (last!=null && fieldActive && room==RoomManager.currentRoom && ChannelReady)
                    FieldRuntime.NativePositionReady(room.netManager,last);
            }
            catch (Exception) { confirmedBase=false; }
        }
        public static bool ChannelReady
        { get { return Current() && !overflow && confirmedBase && authenticated && sent.Count==0 && NetAPI.SocketIsConnected(); } }
        public static bool TryLock(RoomCharactor actor)
        {
            if (!ChannelReady || actor==null || locked) return false;
            return LockAfterTeleport(actor);
        }
        // Unity-thread owners may retain a lease across async callbacks. A
        // callback for an older action must never release a newer action's lock.
        public static bool TryLock(RoomCharactor actor,out long lease)
        {
            lease=0;
            if (!TryLock(actor)) return false;
            lease=lockGeneration; return true;
        }
        public static bool OwnsLock(long lease) { return lease>0 && locked && lease==lockGeneration; }
        public static bool LockAfterTeleport(RoomCharactor actor,out long lease)
        {
            lease=0;
            if (locked || !LockAfterTeleport(actor)) return false;
            lease=lockGeneration; return true;
        }
        public static bool ReleaseIfOwned(long lease)
        {
            if (!OwnsLock(lease)) return false;
            Release(); return true;
        }
        public static bool LockAfterTeleport(RoomCharactor actor)
        {
            if (locked) return Current();
            if (!Current() || actor==null || lockGeneration==Int64.MaxValue) return false;
            actor.StopMove(); lockGeneration++; locked=true;
            try
            {
                touchWasEnabled=EasyTouchTool.Instance!=null && EasyTouchTool.Instance.Enable;
                if (touchWasEnabled) EasyTouchTool.Instance.Hide();
            }
            catch (Exception) { Release(); return false; }
            return true;
        }
        public static void RequestCurrent(RoomNetBase net,Action<Vector3> callback)
        {
            if (callback==null) throw new ArgumentNullException("callback");
            RequestCurrentState(net,delegate(Vector3 position,string scene) { callback(position); });
        }
        // Recovery also needs the scene carried by this exact authenticated
        // native frame; a cached fieldId can outlive the room that set it.
        internal static void RequestCurrentState(RoomNetBase net,Action<Vector3,string> callback)
        {
            if (!Current() || !locked || overflow || !authenticated || net==null || callback==null || expected!=0 || sent.Count!=0 || !NetAPI.SocketIsConnected())
                throw new InvalidOperationException("Original position channel not ready");
            long request=sequence+1;
            expected=request; onResponse=callback;
            try
            {
                net.SendCSCurrentPosition(null);
                if (sequence!=request || sent.Count!=1) throw new InvalidOperationException("Original position request was not observed");
            }
            catch (Exception)
            {
                if (expected==request) { expected=0; onResponse=null; }
                throw;
            }
        }
        public static void Tick() { TickCore(false); }
        // Pending leave recovery can run in a town after the desert runtime has
        // stopped. It still needs one real 30024 baseline, with bounded retries.
        internal static void TickRecovery() { TickCore(true); }
        static void TickCore(bool recovery)
        {
            if (!Current()) return;
            DrainDeferred();
            if ((fieldActive || leaveActive || locked) && sent.Count>0 && firstOutstandingAt>0 &&
                !reconnectRequested && NetAPI.SocketIsConnected() &&
                Stopwatch.GetTimestamp()-firstOutstandingAt>=15L*Stopwatch.Frequency)
            {
                // 30024 has no request ID; keep its queue and lease intact
                // until the original reconnect creates a fresh connection epoch.
                reconnectRequested=true;
                try { NetAPI.Reconnect(); } catch (Exception) { }
            }
            if ((!locked || recovery) && !confirmedBase && !overflow && (authenticated || unboundLogin) && sent.Count==0 &&
                NetAPI.SocketIsConnected() && Stopwatch.GetTimestamp()>=nextBaseRetry)
            {
                // Recover an unobserved or invalid initial frame by requesting
                // a fresh native frame. Never replace an outstanding request or
                // accept a local timeout fallback as an acknowledgement.
                RoomBase candidate=RoomManager.currentRoom;
                RoomNetBase net=candidate==null ? null : recovery ? candidate.netManager : candidate.netManager as RoomNetField;
                if ((fieldActive || recovery) && net!=null)
                { nextBaseRetry=Stopwatch.GetTimestamp()+2L*Stopwatch.Frequency; try { net.SendCSCurrentPosition(null); } catch (Exception) { } }
            }
            if (!locked) return;
            RoomBase room=RoomManager.currentRoom;
            if (room!=null && room.charactorManager!=null)
            {
                RoomCharactor self=room.charactorManager.GetSelfCharactor();
                if (self!=null && self.IsMoving()) self.StopMove();
            }
        }
        public static bool BlocksMove(RoomCharactor actor)
        {
            if (!locked || actor==null) return false;
            RoomBase room=RoomManager.currentRoom;
            return room!=null && room.charactorManager!=null && room.charactorManager.GetSelfCharactor()==actor;
        }
        public static void Release()
        {
            bool restoreTouch=touchWasEnabled;
            locked=false; touchWasEnabled=false; expected=0; onResponse=null;
            try { if (restoreTouch && EasyTouchTool.Instance!=null) EasyTouchTool.Instance.ShowJoystick(); }
            catch (Exception) { }
        }
    }
}
