using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Map-only snapshots of the existing team replies. This class sends no
    // messages and never changes the native team cache or a room actor.
    public static class TeamMap86
    {
        public struct Key : IEquatable<Key>
        {
            public int serverId;
            public long playerId;
            public Key(int server,long player) { serverId=server; playerId=player; }
            public bool Equals(Key other) { return serverId==other.serverId && playerId==other.playerId; }
            public override bool Equals(object other) { return other is Key && Equals((Key)other); }
            public override int GetHashCode() { return serverId*397 ^ playerId.GetHashCode(); }
        }
        public sealed class Visible
        {
            public Key key;
            public int color;
            public RoomCharactor actor;
            public string actorKey;
            public Vector3 position;
        }
        sealed class Member
        {
            public Key key;
            public int state;
            public string actorKey;
        }
        sealed class Queued
        {
            public int epoch,roomGeneration;
            public bool matchedTick,canApply,discard;
            public SentTick sent;
            public float earliest;
        }
        sealed class SentTick
        {
            public int epoch,roomGeneration,mutation;
            public float at;
            public bool eligible;
            public object net;
        }
        sealed class PositionPacket
        {
            public int epoch, generation, mutation;
            public long sequence;
            public RoomBase room;
            public bool dispatched, unbound;
            public float queuedAt;
        }
        sealed class Presence
        {
            public int serverId;
            public Vector3 position;
            public bool present, hasPosition;
            public long sequence;
            public float expires;
        }

        static readonly object queueGate=new object();
        static readonly FieldInfo NativeSocket=typeof(NetAPI).GetField("_socket",BindingFlags.Static|BindingFlags.NonPublic);
        static readonly Dictionary<object,Queued> queued=new Dictionary<object,Queued>();
        static readonly Dictionary<object,PositionPacket> positionQueued=new Dictionary<object,PositionPacket>();
        static readonly Dictionary<long,Presence> presence=new Dictionary<long,Presence>();
        static readonly List<long> presenceExpired=new List<long>();
        static readonly Queue<SentTick> sentTicks=new Queue<SentTick>();
        static readonly Dictionary<Key,int> colors=new Dictionary<Key,int>();
        static readonly List<Member> members=new List<Member>();
        static int roomGeneration,mutationGeneration;
        static int receivedTicks;
        static bool tickPairingBroken,hardBroken,ambiguousBatch;
        static float pendingEarliest;
        static long positionSequence;
        static long presenceFloor;
        static RoomBase presenceRoom;
        static FieldWire.Session presenceOwner;
        static int presenceEpoch,presenceGeneration;
        static int bindFromGeneration=-1;
        static bool positionInvalid;
        const float PresenceLifetime=60f;
        const int PresenceLimit=20, PositionQueueLimit=256;
        // First failure and aggregate dispatch facts contain no account or
        // packet payload. They are sampled only when the map opens.
        static int breakReason,tickQueued,tickDequeued,tickObserved;
        static bool lastOpcode,lastTracked,lastHandlersKnown,lastHandler,lastBoundHandler,lastManagerKnown,lastCurrentManager;
        static RoomBase room;
        static FieldWire.Session owner;
        static string teamId;
        static float expires;
        static int snapshotEpoch;

        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static bool TeamMessage(object value)
        {
            return value is SCQueryTeam || value is SCQueryTeamTick || value is SCJoinTeam || value is SCTeamCreateAndInvite ||
                value is SCExitTeam || value is SCTeamKick || value is SCDisbandTeam;
        }
        static bool PositionMessage(object value)
        { return value is SCPlayerAppear || value is SCMovePush || value is SCPlayerDisappear; }
        static void ClearPresence()
        { presence.Clear(); presenceFloor=0; presenceRoom=null; presenceOwner=null; presenceEpoch=presenceGeneration=0; }
        // Call only under queueGate. Room-scoped ambiguity clears at a room
        // boundary; missing accounting stays broken until a new connection.
        static void BreakTick(int reason,bool hard)
        {
            if (!tickPairingBroken) breakReason=reason;
            tickPairingBroken=true; if (hard) hardBroken=true;
        }
        static void Count(ref int value) { if (value<Int32.MaxValue) value++; }
        static void AdvanceGeneration(bool connection)
        {
            lock (queueGate)
            {
                unchecked { roomGeneration++; }
                if (connection)
                {
                    sentTicks.Clear(); receivedTicks=0; mutationGeneration=0; tickPairingBroken=false; hardBroken=false; ambiguousBatch=false; pendingEarliest=0;
                    breakReason=tickQueued=tickDequeued=tickObserved=0;
                    lastOpcode=lastTracked=lastHandlersKnown=lastHandler=lastBoundHandler=lastManagerKnown=lastCurrentManager=false;
                    positionQueued.Clear(); positionSequence=0; positionInvalid=false;
                }
                else if (!hardBroken) { tickPairingBroken=false; breakReason=0; }
                // Keep already-queued packet tags across transitions. Their old
                // epoch/room identifies them when the native loop dispatches them.
            }
        }
        static void ClearSnapshot(bool resetColors)
        {
            teamId=null; expires=0; members.Clear();
            if (resetColors) colors.Clear();
        }

        // Called from the field runtime on the main thread, after its own room
        // and session binding. Messages queued before binding cannot seed it.
        public static void BindScene(RoomBase value)
        {
            FieldWire.Session current;
            if (value==null || !FieldWire.Current(out current)) { ReleaseScene(); return; }
            if (room==value && SameSession(owner,current)) return;
            bool keep=presenceRoom==value && SameSession(presenceOwner,current) &&
                presenceEpoch==PositionSync.ConnectionEpoch && presenceGeneration==roomGeneration;
            int prior=roomGeneration;
            AdvanceGeneration(false); bindFromGeneration=prior; ClearSnapshot(true); room=value; owner=current;
            if (keep) presenceGeneration=roomGeneration; else ClearPresence();
        }
        public static void ReleaseScene()
        { AdvanceGeneration(false); lock (queueGate) { positionQueued.Clear(); positionInvalid=false; } bindFromGeneration=-1; ClearSnapshot(true); ClearPresence(); room=null; owner=null; }
        public static void ConnectionStarting()
        { AdvanceGeneration(true); bindFromGeneration=-1; ClearSnapshot(true); ClearPresence(); }
        // Only an actual native transfer send reaches this hook. Invalidate
        // before any old-line packets can be dispatched during the transition.
        public static void ScopeStarting(int opcode)
        {
            if (opcode!=30032 && opcode!=30061) return;
            AdvanceGeneration(false); lock (queueGate) { positionQueued.Clear(); positionInvalid=false; } bindFromGeneration=-1; ClearSnapshot(true); ClearPresence();
        }

        static bool SameScope(SentTick a,SentTick b)
        { return a.epoch==b.epoch && a.roomGeneration==b.roomGeneration && a.mutation==b.mutation && a.eligible==b.eligible && System.Object.ReferenceEquals(a.net,b.net); }
        // Read the existing controller and native delegates without invoking a
        // singleton getter or changing registration.
        static bool NativeHandlerReady(object net)
        {
            try
            {
                const BindingFlags flags=BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public;
                object socket=NativeSocket==null ? null : NativeSocket.GetValue(null);
                NetWork.SocketController controller=socket as NetWork.SocketController;
                if (controller==null || !controller.IsConnected()) return false;
                FieldInfo managerField=typeof(NetWork.SocketController).GetField("eventManager",flags);
                object manager=managerField==null ? null : managerField.GetValue(controller);
                FieldInfo handlers=manager==null ? null : manager.GetType().GetField("msgHandle",flags);
                IDictionary table=handlers==null ? null : handlers.GetValue(manager) as IDictionary;
                Delegate callback=table!=null && table.Contains(22024) ? table[22024] as Delegate : null;
                if (callback==null) return false;
                foreach (Delegate item in callback.GetInvocationList())
                    if (System.Object.ReferenceEquals(item.Target,net) && item.Method.Name=="SCQueryTeamTick") return true;
            }
            catch (Exception) { }
            return false;
        }

        // Observes only calls the native client already made. Error frames
        // have no request ID; uncertain candidates never become a snapshot.
        public static void TickSent(object net)
        {
            try
            {
                if (!(net is RoomNetField)) return;
                bool eligible=NativeHandlerReady(net) && CurrentRoom(net);
                lock (queueGate)
                {
                    if (sentTicks.Count>=64) { BreakTick(2,true); return; }
                    var sent=new SentTick { epoch=PositionSync.ConnectionEpoch,roomGeneration=roomGeneration,
                        mutation=mutationGeneration,at=Time.unscaledTime,eligible=eligible,net=net };
                    if (sentTicks.Count==0) pendingEarliest=sent.at;
                    else
                    {
                        if (!SameScope(sentTicks.Peek(),sent)) ambiguousBatch=true;
                        if (sent.at<pendingEarliest) pendingEarliest=sent.at;
                    }
                    sentTicks.Enqueue(sent);
                }
            }
            catch (Exception) { lock (queueGate) { BreakTick(3,true); } }
        }

        // The original SocketController Tick moves packets from its locked
        // receive queue into AddPacket before native event dispatch.
        public static void Queue(object value)
        {
            try
            {
                Packet packet=value as Packet;
                object body=packet==null ? null : packet.kBody;
                if (PositionMessage(body))
                {
                    RoomBase target=RoomManager.currentRoom;
                    if (!(target is RoomField) || StaticData.fieldId!="field_003") return;
                    lock (queueGate)
                    {
                        if (positionQueued.Count>=PositionQueueLimit || positionQueued.ContainsKey(body))
                        { positionQueued.Clear(); positionInvalid=true; return; }
                        positionQueued.Add(body,new PositionPacket { epoch=PositionSync.ConnectionEpoch,
                            generation=roomGeneration,mutation=mutationGeneration,room=target,
                            unbound=room==null,sequence=++positionSequence,queuedAt=Time.unscaledTime });
                    }
                    return;
                }
                if (!(body is ErrorMessage) && !TeamMessage(body)) return;
                lock (queueGate)
                {
                    // The generic error has no request ID. Old-room requests
                    // remain tombstones; only a current eligible request can
                    // invalidate this room's snapshot.
                    if (body is ErrorMessage)
                    {
                        foreach (SentTick pending in sentTicks)
                            if (pending.eligible && pending.epoch==PositionSync.ConnectionEpoch && pending.roomGeneration==roomGeneration)
                            { BreakTick(1,false); break; }
                        return;
                    }
                    if (body is SCQueryTeamTick) Count(ref tickQueued);
                    int epoch=PositionSync.ConnectionEpoch;
                    if (queued.Count>=256)
                    { BreakTick(4,true); queued.Clear(); receivedTicks=0; }
                    if (queued.ContainsKey(body)) { BreakTick(5,true); return; }
                    SentTick sent=null; bool matched=false,canApply=false; float earliest=0;
                    if (body is SCQueryTeamTick)
                    {
                        if (sentTicks.Count==0) BreakTick(6,true);
                        else
                        {
                            sent=sentTicks.Peek();
                            earliest=pendingEarliest;
                            float age=Time.unscaledTime-earliest;
                            if (Single.IsNaN(age) || Single.IsInfinity(age) || age<0 || age>15f) BreakTick(10,true);
                            // Every candidate must be from this connection,
                            // room and membership generation. A mixed batch
                            // stays hidden until all of its replies arrive.
                            canApply=!tickPairingBroken && !ambiguousBatch && sent.eligible && sent.epoch==epoch &&
                                sent.roomGeneration==roomGeneration && sent.mutation==mutationGeneration;
                            sentTicks.Dequeue();
                            if (sentTicks.Count==0) { ambiguousBatch=false; pendingEarliest=0; }
                            receivedTicks++; matched=true;
                        }
                    }
                    queued.Add(body,new Queued { epoch=epoch,roomGeneration=roomGeneration,matchedTick=matched,canApply=canApply,sent=sent,earliest=earliest });
                }
            }
            catch (Exception) { lock (queueGate) { BreakTick(7,true); } }
        }
        // Called on the native event loop immediately after dequeue. This is
        // observation only: never invoke, register or alter a native handler.
        public static void InspectDispatch(object manager,object value)
        {
            try
            {
                Packet packet=value as Packet;
                object body=packet==null ? null : packet.kBody;
                if (PositionMessage(body)) { InspectPositionDispatch(manager,packet,body); return; }
                if (!TeamMessage(body)) return;
                bool tick=body is SCQueryTeamTick;
                bool opcode=packet.nOpCode==22024,tracked=false,known=false,handler=false,bound=false,managerKnown=false,currentManager=false;
                try
                {
                    const BindingFlags flags=BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public;
                    FieldInfo handlers=manager==null ? null : manager.GetType().GetField("msgHandle",flags);
                    IDictionary table=handlers==null ? null : handlers.GetValue(manager) as IDictionary;
                    if (table!=null)
                    {
                        known=true; Delegate callback=table.Contains(packet.nOpCode) ? table[packet.nOpCode] as Delegate : null;
                        handler=callback!=null;
                        if (callback!=null && room!=null)
                            foreach (Delegate item in callback.GetInvocationList())
                                if (System.Object.ReferenceEquals(item.Target,room.netManager) && item.Method.Name==body.GetType().Name)
                                { bound=true; break; }
                    }
                    FieldInfo nativeManager=typeof(NetWork.SocketController).GetField("eventManager",flags);
                    if (NativeSocket!=null && nativeManager!=null)
                    {
                        object socket=NativeSocket.GetValue(null);
                        managerKnown=true;
                        if (socket!=null) currentManager=System.Object.ReferenceEquals(nativeManager.GetValue(socket),manager);
                    }
                }
                catch (Exception) { } // diagnostics cannot alter native dispatch
                bool invalidate=false;
                lock (queueGate)
                {
                    Queued item; tracked=queued.TryGetValue(body,out item);
                    if (tick)
                    {
                        Count(ref tickDequeued); lastOpcode=opcode; lastTracked=tracked; lastHandlersKnown=known; lastHandler=handler;
                        lastBoundHandler=bound; lastManagerKnown=managerKnown; lastCurrentManager=currentManager;
                    }
                    if (tracked && known && managerKnown)
                    {
                        bool sameEpoch=item.epoch==PositionSync.ConnectionEpoch;
                        bool current=sameEpoch && item.roomGeneration==roomGeneration;
                        if (!bound || !currentManager)
                        {
                            queued.Remove(body);
                            if (sameEpoch && item.matchedTick)
                            {
                                if (receivedTicks<=0) BreakTick(8,true);
                                else receivedTicks--;
                            }
                            invalidate=current;
                        }
                        else if (!current) item.discard=true;
                    }
                }
                if (invalidate) ClearSnapshot(false);
            }
            catch (Exception) { } // diagnostics cannot disrupt the native event loop
        }
        static void InspectPositionDispatch(object manager,Packet packet,object body)
        {
            bool valid=false;
            try
            {
                const BindingFlags flags=BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public;
                RoomBase current=RoomManager.currentRoom;
                FieldInfo handlers=manager==null ? null : manager.GetType().GetField("msgHandle",flags);
                IDictionary table=handlers==null ? null : handlers.GetValue(manager) as IDictionary;
                Delegate callback=table!=null && table.Contains(packet.nOpCode) ? table[packet.nOpCode] as Delegate : null;
                FieldInfo nativeManager=typeof(NetWork.SocketController).GetField("eventManager",flags);
                object socket=NativeSocket==null ? null : NativeSocket.GetValue(null);
                bool opcode=(body is SCPlayerAppear && packet.nOpCode==30028) ||
                    (body is SCMovePush && packet.nOpCode==30027) || (body is SCPlayerDisappear && packet.nOpCode==30029);
                if (opcode && current!=null && callback!=null && socket!=null && nativeManager!=null &&
                    System.Object.ReferenceEquals(nativeManager.GetValue(socket),manager))
                    foreach (Delegate item in callback.GetInvocationList())
                        if (System.Object.ReferenceEquals(item.Target,current.netManager) && item.Method.Name==body.GetType().Name)
                        { valid=true; break; }
            }
            catch (Exception) { }
            lock (queueGate)
            {
                PositionPacket tag;
                if (positionQueued.TryGetValue(body,out tag)) tag.dispatched=valid;
            }
        }
        static bool TakePacket(object value,out Queued item)
        {
            item=null; if (value==null) return false;
            lock (queueGate)
            {
                if (!queued.TryGetValue(value,out item)) return false;
                queued.Remove(value);
                return true;
            }
        }
        static bool TakeTick(Queued packet,object net)
        {
            lock (queueGate)
            {
                if (packet.epoch!=PositionSync.ConnectionEpoch) return false;
                if (!packet.matchedTick || packet.sent==null || receivedTicks<=0)
                { BreakTick(8,true); return false; }
                receivedTicks--;
                SentTick sent=packet.sent;
                if (sent.epoch!=packet.epoch) { BreakTick(9,true); return false; }
                if (Time.unscaledTime-packet.earliest>15f) { BreakTick(10,true); return false; }
                return !packet.discard && !tickPairingBroken && packet.canApply && System.Object.ReferenceEquals(sent.net,net) && sent.roomGeneration==roomGeneration && packet.roomGeneration==roomGeneration &&
                    sent.mutation==mutationGeneration;
            }
        }
        static bool TickChannelValid()
        {
            lock (queueGate)
            {
                if (sentTicks.Count>0 && Time.unscaledTime-pendingEarliest>15f) BreakTick(11,true);
                bool aligned=true;
                if (sentTicks.Count>0)
                {
                    SentTick pending=sentTicks.Peek();
                    aligned=pending.eligible && pending.epoch==PositionSync.ConnectionEpoch && pending.roomGeneration==roomGeneration &&
                        pending.mutation==mutationGeneration && room!=null && System.Object.ReferenceEquals(pending.net,room.netManager);
                }
                foreach (Queued item in queued.Values)
                    if (item.epoch==PositionSync.ConnectionEpoch && item.matchedTick && item.sent!=null)
                    {
                        if (Time.unscaledTime-item.earliest>15f) BreakTick(11,true);
                        if (!item.canApply || item.discard || item.roomGeneration!=roomGeneration || item.sent.roomGeneration!=roomGeneration ||
                            item.sent.mutation!=mutationGeneration || room==null || !System.Object.ReferenceEquals(item.sent.net,room.netManager)) aligned=false;
                    }
                return !tickPairingBroken && !ambiguousBatch && aligned;
            }
        }
        static bool CurrentRoom(object net)
        {
            FieldWire.Session current;
            return room!=null && StaticData.fieldId=="field_003" && room==RoomManager.currentRoom &&
                room is RoomField && System.Object.ReferenceEquals(room.netManager,net) &&
                room.charactorManager!=null && room.charactorManager.GetSelfCharactor()!=null &&
                FieldWire.Current(out current) && SameSession(owner,current) && NetAPI.SocketIsConnected();
        }

        static void PrunePresence(float now)
        {
            // A stationary player sends no periodic position refresh. Retain
            // same-line appearances until native disappearance or scope loss;
            // only expired disappearance tombstones are safe to discard.
            presenceExpired.Clear();
            foreach (KeyValuePair<long,Presence> entry in presence)
                if (!entry.Value.present && entry.Value.expires<=now) presenceExpired.Add(entry.Key);
            foreach (long id in presenceExpired) presence.Remove(id);
            presenceExpired.Clear();
        }
        static bool PositionValid(ProtoBuf.Message.Position value)
        {
            if (value==null || value.playerId<=0 || value.current==null ||
                (value.scene!="field_003" && value.scene!="town_side_field_003")) return false;
            float x=value.current.x,y=value.current.y,z=value.current.z;
            return !Single.IsNaN(x) && !Single.IsNaN(y) && !Single.IsNaN(z) &&
                !Single.IsInfinity(x) && !Single.IsInfinity(y) && !Single.IsInfinity(z) &&
                Math.Abs(x)<=10000f && Math.Abs(y)<=10000f && Math.Abs(z)<=10000f;
        }
        static Vector3 PositionValue(ProtoBuf.Message.Position value)
        { return new Vector3(value.current.x,value.current.y,value.current.z); }
        static Presence PresenceSlot(long id,long sequence,float now)
        {
            if (id<=0 || sequence<=presenceFloor || presenceOwner==null || id==presenceOwner.playerId) return null;
            Presence item;
            if (!presence.TryGetValue(id,out item))
            {
                if (presence.Count>=PresenceLimit)
                {
                    long victim=0, oldest=Int64.MaxValue;
                    foreach (KeyValuePair<long,Presence> entry in presence)
                        if (!entry.Value.present && entry.Value.sequence<sequence && entry.Value.sequence<oldest)
                        { victim=entry.Key; oldest=entry.Value.sequence; }
                    if (victim==0) return null;
                    presence.Remove(victim);
                    if (oldest>presenceFloor) presenceFloor=oldest;
                }
                item=new Presence(); presence.Add(id,item);
            }
            return sequence>item.sequence ? item : null;
        }

        // These are the original same-line TCP frames, observed at their
        // native callback after the event queue authenticated its handler.
        // Candidate identities are bounded and do not grant team membership.
        public static void ObservePosition(object net,object value)
        {
            try
            {
                PositionPacket tag; bool invalid;
                lock (queueGate)
                {
                    invalid=positionInvalid;
                    if (!positionQueued.TryGetValue(value,out tag)) return;
                    positionQueued.Remove(value);
                }
                if (invalid) { ClearPresence(); return; }
                RoomBase current=RoomManager.currentRoom; FieldWire.Session who;
                float age=Time.unscaledTime-tag.queuedAt;
                bool generation=tag.generation==roomGeneration ||
                    (tag.unbound && room==tag.room && tag.generation==bindFromGeneration);
                if (!tag.dispatched || !generation || tag.mutation!=mutationGeneration || tag.sequence<=presenceFloor ||
                    tag.epoch!=PositionSync.ConnectionEpoch || age<0 || age>15f ||
                    current!=tag.room || !(current is RoomField) || StaticData.fieldId!="field_003" ||
                    !System.Object.ReferenceEquals(current.netManager,net) || !FieldWire.Current(out who) ||
                    !NetAPI.SocketIsConnected() || (room!=null && (room!=current || !SameSession(owner,who)))) return;
                if (presenceRoom!=current || presenceEpoch!=tag.epoch || !SameSession(presenceOwner,who))
                {
                    ClearPresence(); presenceRoom=current; presenceOwner=who; presenceEpoch=tag.epoch;
                }
                presenceGeneration=roomGeneration;
                float now=Time.unscaledTime; PrunePresence(now);
                SCPlayerAppear appear=value as SCPlayerAppear;
                if (appear!=null && appear.player!=null)
                {
                    foreach (MovePlayer player in appear.player)
                    {
                        if (player==null || player.info==null || player.info.playerId<=0 || player.info.serverId<=0 ||
                            !PositionValid(player.position) || player.position.playerId!=player.info.playerId) continue;
                        Presence item=PresenceSlot(player.info.playerId,tag.sequence,now); if (item==null) continue;
                        item.serverId=player.info.serverId; item.position=PositionValue(player.position);
                        item.present=item.hasPosition=true; item.sequence=tag.sequence; item.expires=now+PresenceLifetime;
                    }
                }
                SCMovePush move=value as SCMovePush;
                if (move!=null && move.position!=null)
                {
                    foreach (ProtoBuf.Message.Position position in move.position)
                    {
                        Presence item;
                        if (!PositionValid(position) || !presence.TryGetValue(position.playerId,out item) ||
                            !item.present || tag.sequence<=item.sequence) continue;
                        item.position=PositionValue(position); item.hasPosition=true;
                        item.sequence=tag.sequence; item.expires=now+PresenceLifetime;
                    }
                }
                SCPlayerDisappear disappear=value as SCPlayerDisappear;
                if (disappear!=null && disappear.playerId!=null)
                {
                    foreach (long id in disappear.playerId)
                    {
                        Presence item=PresenceSlot(id,tag.sequence,now); if (item==null) continue;
                        item.present=item.hasPosition=false; item.sequence=tag.sequence; item.expires=now+PresenceLifetime;
                    }
                }
            }
            catch (Exception) { ClearPresence(); } // observation never changes the native handler
        }
        // Scalars only. Sampling does not consume responses, change expiry, or
        // latch the tick channel; the view emits this at most once per open.
        public sealed class Diagnostics
        {
            public bool boundRoom,currentRoom,sessionMatch,socketKnown,socket,fieldTarget,selfLoaded,teamMatch,broken;
            public int connectionEpoch,snapshotEpoch,sent,queued,received,members,remainingSeconds,actors,viewMembers,smallMarkers,leases;
            public int breakReason,tickQueued,tickDequeued,tickObserved,pendingAge;
            public bool lastOpcode,lastTracked,lastHandlersKnown,lastHandler,lastBoundHandler,lastManagerKnown,lastCurrentManager;
            static int Bit(bool value) { return value ? 1 : 0; }
            public string StateKey()
            {
                return String.Format(CultureInfo.InvariantCulture,
                    "{0}:{1}:{2}:{3}:{4}:{5}:{6}:{7}:{8}:{9}:{10}:{11}:{12}:{13}:{14}:{15}:{16}:{17}:{18}:{19}:{20}:{21}:{22}:{23}:{24}:{25}:{26}:{27}:{28}:{29}:{30}",
                    Bit(boundRoom),Bit(currentRoom),Bit(sessionMatch),Bit(socketKnown),Bit(socket),Bit(fieldTarget),Bit(selfLoaded),
                    Bit(teamMatch),Bit(broken),connectionEpoch,snapshotEpoch,sent,queued,received,members,Bit(remainingSeconds>0),actors,viewMembers,smallMarkers,leases,
                    breakReason,tickQueued,tickDequeued,tickObserved,Bit(lastOpcode),Bit(lastTracked),Bit(lastHandlersKnown),Bit(lastHandler),Bit(lastBoundHandler),Bit(lastManagerKnown),Bit(lastCurrentManager));
            }
            public override string ToString()
            {
                return String.Format(CultureInfo.InvariantCulture,
                    "bound={0} current={1} session={2} socketKnown={3} socket={4} field={5} self={6} teamMatch={7} broken={8} epoch={9} snapshotEpoch={10} sent={11} queued={12} received={13} members={14} remainingSeconds={15} actors={16} viewMembers={17} smallMarkers={18} leases={19} breakReason={20} tickQueued={21} tickDequeued={22} tickObserved={23} pendingAge={24} lastOpcode={25} lastTracked={26} handlersKnown={27} handler={28} boundHandler={29} managerKnown={30} currentManager={31}",
                    Bit(boundRoom),Bit(currentRoom),Bit(sessionMatch),Bit(socketKnown),Bit(socket),Bit(fieldTarget),Bit(selfLoaded),
                    Bit(teamMatch),Bit(broken),connectionEpoch,snapshotEpoch,sent,queued,received,members,remainingSeconds,actors,viewMembers,smallMarkers,leases,
                    breakReason,tickQueued,tickDequeued,tickObserved,pendingAge,Bit(lastOpcode),Bit(lastTracked),Bit(lastHandlersKnown),Bit(lastHandler),Bit(lastBoundHandler),Bit(lastManagerKnown),Bit(lastCurrentManager));
            }
        }
        public sealed class DiagnosticGate
        {
            string previous;
            int count;
            public bool Full { get { return count>=12; } }
            public bool Allow(Diagnostics state)
            {
                if (state==null || Full) return false;
                string key=state.StateKey();
                if (key==previous) return false;
                previous=key; count++; return true;
            }
        }
        public static Diagnostics DiagnosticState()
        {
            var result=new Diagnostics();
            try
            {
                result.boundRoom=room!=null;
                result.currentRoom=room!=null && room==RoomManager.currentRoom;
                result.fieldTarget=StaticData.fieldId=="field_003" && room is RoomField;
                FieldWire.Session current;
                result.sessionMatch=FieldWire.Current(out current) && SameSession(owner,current);
                // NetAPI.SocketIsConnected() calls a lazy singleton getter.
                // Inspect the existing controller without creating one.
                try
                {
                    result.socketKnown=NativeSocket!=null;
                    if (NativeSocket!=null)
                    {
                        object value=NativeSocket.GetValue(null);
                        NetWork.SocketController socket=value as NetWork.SocketController;
                        if (value!=null && socket==null) result.socketKnown=false;
                        else result.socket=socket!=null && socket.IsConnected();
                    }
                }
                catch (Exception) { result.socketKnown=false; result.socket=false; }
                result.connectionEpoch=PositionSync.ConnectionEpoch; result.snapshotEpoch=snapshotEpoch;
                lock (queueGate)
                {
                    result.sent=sentTicks.Count; result.queued=queued.Count; result.received=receivedTicks; result.broken=tickPairingBroken;
                    result.breakReason=breakReason; result.tickQueued=tickQueued; result.tickDequeued=tickDequeued; result.tickObserved=tickObserved;
                    result.lastOpcode=lastOpcode; result.lastTracked=lastTracked; result.lastHandlersKnown=lastHandlersKnown; result.lastHandler=lastHandler;
                    result.lastBoundHandler=lastBoundHandler; result.lastManagerKnown=lastManagerKnown; result.lastCurrentManager=lastCurrentManager;
                    if (sentTicks.Count>0)
                    {
                        float age=Time.unscaledTime-pendingEarliest;
                        if (!Single.IsNaN(age) && !Single.IsInfinity(age) && age>0) result.pendingAge=(int)Math.Min(3600.0,Math.Ceiling(age));
                    }
                }
                result.members=members.Count;
                float left=expires-Time.unscaledTime;
                if (!Single.IsNaN(left) && !Single.IsInfinity(left) && left>0)
                    result.remainingSeconds=(int)Math.Min(30.0,Math.Ceiling(left));
                result.teamMatch=StaticData.playerData!=null && !String.IsNullOrEmpty(teamId) && StaticData.playerData.teamId==teamId;
                if (room==null || room.charactorManager==null || owner==null) return result;
                RoomCharactor self=room.charactorManager.GetSelfCharactor(); result.selfLoaded=self!=null;
                foreach (Member item in members)
                {
                    if (item.key.playerId==owner.playerId || item.state!=1) continue;
                    RoomCharactor actor=room.charactorManager.GetCharactor(item.actorKey);
                    if (actor==null || actor==self || (int)actor.type!=2 || !actor.gameObject.activeInHierarchy) continue;
                    RCUIPlayer player=actor.ui as RCUIPlayer;
                    if (player!=null && player.serverID==item.key.serverId) result.actors++;
                }
            }
            catch (Exception) { } // diagnostics never disrupt the native map
            return result;
        }
        static int CompareMembers(Member a,Member b)
        {
            int server=a.key.serverId.CompareTo(b.key.serverId);
            return server!=0 ? server : a.key.playerId.CompareTo(b.key.playerId);
        }
        static bool Prepare(Team team,long self,out List<Member> next)
        {
            next=null;
            if (team==null || String.IsNullOrEmpty(team.teamId) || team.memberList==null ||
                team.memberList.Count<1 || team.memberList.Count>5) return false;
            var list=new List<Member>(); var ids=new HashSet<long>(); int selfServer=0; bool selfSeen=false;
            foreach (TeamMember item in team.memberList)
            {
                if (item==null || item.playerId<=0 || item.serverId<=0 || !ids.Add(item.playerId) ||
                    (item.state!=0 && item.state!=1)) return false;
                if (item.playerId==self) { selfSeen=true; selfServer=item.serverId; if (item.state!=1) return false; }
                list.Add(new Member { key=new Key(item.serverId,item.playerId),state=item.state,actorKey=item.playerId.ToString() });
            }
            if (!selfSeen) return false;
            foreach (Member item in list) if (item.key.serverId!=selfServer) return false;
            list.Sort(CompareMembers); next=list; return true;
        }
        static void Apply(Team team,List<Member> next,float lifetime)
        {
            if (teamId!=team.teamId) colors.Clear();
            float now=Time.unscaledTime; PrunePresence(now);
            foreach (Member current in next)
            {
                Presence item;
                if (current.state==1 && presence.TryGetValue(current.key.playerId,out item) &&
                    item.present && item.hasPosition && item.serverId==current.key.serverId)
                    item.expires=now+PresenceLifetime;
            }
            var present=new HashSet<Key>();
            foreach (Member item in next) if (item.key.playerId!=owner.playerId) present.Add(item.key);
            var removed=new List<Key>();
            foreach (Key key in colors.Keys) if (!present.Contains(key)) removed.Add(key);
            foreach (Key key in removed) colors.Remove(key);
            foreach (Member item in next)
            {
                if (item.key.playerId==owner.playerId || colors.ContainsKey(item.key)) continue;
                for (int slot=0;slot<4;slot++)
                {
                    bool occupied=false;
                    foreach (int used in colors.Values) if (used==slot) { occupied=true; break; }
                    if (!occupied) { colors.Add(item.key,slot); break; }
                }
            }
            members.Clear(); members.AddRange(next); teamId=team.teamId;
            snapshotEpoch=PositionSync.ConnectionEpoch; expires=Time.unscaledTime+lifetime;
        }

        // Prefixes observe the original complete responses. The original
        // handler still runs and remains the owner of its cache and UI.
        public static void Observe(object net,object value)
        {
            try
            {
                if (value is SCQueryTeamTick) lock (queueGate) { Count(ref tickObserved); }
                // The native multicast may still contain an older room's
                // delegate. Only the currently bound handler consumes tags.
                if (room==null || !System.Object.ReferenceEquals(room.netManager,net)) return;
                Queued packet;
                if (!TakePacket(value,out packet))
                {
                    if (value is SCQueryTeamTick) { lock (queueGate) { BreakTick(12,true); } ClearSnapshot(false); }
                    return;
                }
                if (packet.epoch!=PositionSync.ConnectionEpoch) return;
                bool isTick=value is SCQueryTeamTick;
                if (isTick && !TakeTick(packet,net)) { ClearSnapshot(false); return; }
                if (packet.discard || packet.roomGeneration!=roomGeneration || !CurrentRoom(net)) return;
                if (value is SCJoinTeam || value is SCTeamCreateAndInvite || value is SCExitTeam || value is SCTeamKick || value is SCDisbandTeam)
                    lock (queueGate) { unchecked { mutationGeneration++; } }
                Team team=null; float lifetime=9f;
                if (value is SCQueryTeam) team=((SCQueryTeam)value).team;
                else if (value is SCQueryTeamTick)
                {
                    SCQueryTeamTick tick=(SCQueryTeamTick)value; team=tick.team;
                    if (tick.seconds>0) lifetime=Mathf.Clamp(tick.seconds*3f,9f,30f);
                }
                else if (value is SCJoinTeam) team=((SCJoinTeam)value).team;
                else if (value is SCTeamCreateAndInvite) team=((SCTeamCreateAndInvite)value).team;
                else if (value is SCExitTeam) team=((SCExitTeam)value).team;
                else if (value is SCTeamKick) team=((SCTeamKick)value).team;
                else if (value is SCDisbandTeam) { ClearSnapshot(true); return; }
                if (isTick)
                {
                    // A response may match any still-possible send in this
                    // batch. Its lifetime starts at the earliest candidate.
                    float age=Time.unscaledTime-packet.earliest;
                    if (Single.IsNaN(age) || Single.IsInfinity(age) || age<0 || age>=lifetime) { ClearSnapshot(false); return; }
                    lifetime-=age;
                }
                List<Member> next;
                if (!Prepare(team,owner.playerId,out next)) { ClearSnapshot(true); return; }
                Apply(team,next,lifetime);
            }
            catch (Exception) { ClearSnapshot(true); }
        }

        // This runs at the existing local marker cadence, bounded to four
        // actors. A stale team state hides dots until another native reply.
        public static void Collect(RoomBase expected,List<Visible> result)
        {
            result.Clear();
            try
            {
                if (expected==null || room!=expected || !CurrentRoom(expected.netManager) || !TickChannelValid() ||
                    snapshotEpoch!=PositionSync.ConnectionEpoch || expires<=Time.unscaledTime ||
                    StaticData.playerData==null || String.IsNullOrEmpty(teamId) ||
                    StaticData.playerData.teamId!=teamId) return;
                bool positionSafe; lock (queueGate) { positionSafe=!positionInvalid; }
                positionSafe=positionSafe && presenceRoom==expected && presenceEpoch==PositionSync.ConnectionEpoch &&
                    presenceGeneration==roomGeneration && SameSession(presenceOwner,owner);
                PrunePresence(Time.unscaledTime);
                RoomCharactor self=expected.charactorManager.GetSelfCharactor();
                foreach (Member item in members)
                {
                    int color;
                    if (item.key.playerId==owner.playerId || item.state!=1 || !colors.TryGetValue(item.key,out color)) continue;
                    RoomCharactor actor=expected.charactorManager.GetCharactor(item.actorKey);
                    Vector3 position;
                    if (actor!=null && actor.gameObject.activeInHierarchy)
                    {
                        if (actor==self || (int)actor.type!=2) continue;
                        RCUIPlayer player=actor.ui as RCUIPlayer;
                        if (player==null || player.serverID!=item.key.serverId) continue;
                        position=actor.transform.localPosition;
                    }
                    else
                    {
                        Presence cached;
                        if (!positionSafe || !presence.TryGetValue(item.key.playerId,out cached) ||
                            !cached.present || !cached.hasPosition || cached.serverId!=item.key.serverId) continue;
                        actor=null; position=cached.position;
                    }
                    result.Add(new Visible { key=item.key,color=color,actor=actor,actorKey=item.actorKey,position=position });
                }
            }
            catch (Exception) { result.Clear(); }
        }
    }
}
