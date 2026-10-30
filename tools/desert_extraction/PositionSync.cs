using System;
using System.Collections.Generic;
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
        // Only a body taken from the original socket event queue can satisfy a
        // request. RoomNetBase.Tick also calls the position handler directly
        // with a locally fabricated fallback after its scene-load timeout.
        static object dispatchedBody;
        static int dispatchedEpoch;
        static int epoch;
        static long sequence,expected,lockGeneration;
        static FieldWire.Session owner;
        static bool overflow,confirmedBase,touchWasEnabled,authenticated;
        static volatile bool fieldActive,locked;
        static Action<Vector3> onResponse;
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
                if (owner!=null) authenticated=false;
                owner=live; sent.Clear(); sequence=expected=0; confirmedBase=false; overflow=false; onResponse=null;
            }
            return true;
        }
        public static void SetFieldActive(bool enabled) { fieldActive=enabled; }
        public static int ConnectionEpoch { get { return Thread.VolatileRead(ref epoch); } }
        public static bool Locked { get { return locked; } }
        public static void ConnectionStarting()
        {
            Interlocked.Increment(ref epoch);
            lock(gate) { dispatchedBody=null; dispatchedEpoch=0; }
            sent.Clear(); sequence=expected=0; confirmedBase=false; overflow=false; authenticated=false; onResponse=null;
        }
        public static void LoginResponse(object value)
        {
            var response=value as SCSocketLogin;
            if (response!=null && response.result==100 && Current()) authenticated=true;
        }
        // Called before SocketTool.ReceiveHandler on its network callback thread.
        // A late callback from an old socket may not enqueue into the new one.
        public static bool DropOldSocket(object tool,IAsyncResult asyncResult)
        {
            if (!fieldActive) return false;
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
        static bool TakeDispatched(object value)
        {
            lock(gate)
            {
                if (dispatchedBody==null || !System.Object.ReferenceEquals(value,dispatchedBody) || dispatchedEpoch!=ConnectionEpoch) return false;
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
                if (!Current()) return;
                if (sequence==Int64.MaxValue || sent.Count>=64) { overflow=true; return; }
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
                if (!TakeDispatched(value)) return;
                if (!Current() || sent.Count==0) { confirmedBase=false; return; }
                long actual=sent.Dequeue();
                var rsp=value as SCCurrentPosition;
                if (rsp==null || rsp.self==null || rsp.self.info==null || rsp.self.position==null || rsp.self.position.current==null ||
                    rsp.self.info.playerId!=owner.playerId) { confirmedBase=false; return; }
                confirmedBase=true;
                if (expected==0 || actual!=expected) return;
                PointNew p=rsp.self.position.current;
                Vector3 position=new Vector3((float)p.x,(float)p.y,(float)p.z);
                if (Single.IsNaN(position.x)||Single.IsNaN(position.y)||Single.IsNaN(position.z)||
                    Single.IsInfinity(position.x)||Single.IsInfinity(position.y)||Single.IsInfinity(position.z)) { confirmedBase=false; return; }
                expected=0; Action<Vector3> callback=onResponse; onResponse=null;
                if (callback!=null) callback(position);
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
            if (!Current() || !locked || overflow || !authenticated || net==null || callback==null || expected!=0 || sent.Count!=0 || !NetAPI.SocketIsConnected())
                throw new InvalidOperationException("Original position channel not ready");
            expected=sequence+1; onResponse=callback; net.SendCSCurrentPosition(null);
            if (sequence!=expected || sent.Count!=1) throw new InvalidOperationException("Original position request was not observed");
        }
        public static void Tick()
        {
            if (!locked || !Current()) return;
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
