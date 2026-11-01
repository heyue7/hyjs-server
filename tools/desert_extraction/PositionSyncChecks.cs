// Isolated replay of the original packet and fallback ordering. No Unity,
// device, transport, server or database is involved.
using System;
using HYJS.DesertV4;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace UnityEngine
{
    public partial struct Vector3
    {
        public float x,y,z;
        public Vector3(float a,float b,float c) { x=a; y=b; z=c; }
    }
}
namespace NetWork.Packets { public sealed class Packet { public object kBody; } }
namespace ProtoBuf.Message
{
    public sealed class SCSocketLogin { public int result; }
    public sealed class PointNew { public float x,y,z; }
    public sealed class MovePlayerInfo { public long playerId; }
    public sealed class Position { public PointNew current; public string scene="field_003"; }
    public sealed class MovePlayer { public MovePlayerInfo info; public Position position; }
    public sealed class SCCurrentPosition { public MovePlayer self; }
}
namespace Room
{
    public class RoomCharactor
    {
        public bool moving=true;
        public void StopMove() { moving=false; }
        public bool IsMoving() { return moving; }
    }
    public class RoomCharactorManager
    {
        public RoomCharactor self;
        public RoomCharactor GetSelfCharactor() { return self; }
    }
    public class RoomBase { public RoomCharactorManager charactorManager; public RoomNetBase netManager; }
    public static class RoomManager { public static RoomBase currentRoom; }
    public class RoomNetBase
    {
        public virtual void SendCSCurrentPosition(object ignored) { PositionSync.TrackSend(30024); }
    }
    public sealed class RoomNetField : RoomNetBase { }
}
public sealed class EasyTouchTool
{
    public static EasyTouchTool Instance=new EasyTouchTool();
    public bool Enable=true;
    public void Hide() { Enable=false; }
    public void ShowJoystick() { Enable=true; }
}
public static partial class NetAPI
{
    public static bool connected=true;
    public static bool SocketIsConnected() { return connected; }
}
namespace HYJS.DesertV4
{
    public sealed partial class FieldWire
    {
        public sealed class Session
        {
            public long playerId;
            public string sessionHex,origin;
        }
        public static Session session;
        public static bool Current(out Session result) { result=session; return result!=null; }
    }
    public static partial class FieldRuntime
    {
        public static int readyCalls;
        public static object readyNet;
        public static SCCurrentPosition readyFrame;
        public static void NativePositionReady(object net,SCCurrentPosition frame)
        { readyCalls++; readyNet=net; readyFrame=frame; }
    }
}
public static class PositionSyncChecks
{
    static int checks;
    static RoomCharactor actor;
    static RoomNetField field;
    static void Check(bool condition,string message)
    { checks++; if (!condition) throw new Exception(message); }
    static void Reset()
    {
        PositionSync.Release();
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        NetAPI.connected=true; EasyTouchTool.Instance.Enable=true;
        FieldRuntime.readyCalls=0; FieldRuntime.readyNet=null; FieldRuntime.readyFrame=null;
        actor=new RoomCharactor(); field=new RoomNetField();
        RoomManager.currentRoom=new RoomBase { charactorManager=new RoomCharactorManager { self=actor },netManager=field };
        PositionSync.SetFieldActive(true); PositionSync.ConnectionStarting();
        PositionSync.LoginResponse(new SCSocketLogin { result=100 });
    }
    static SCCurrentPosition Position(long player,float x)
    {
        return new SCCurrentPosition { self=new MovePlayer { info=new MovePlayerInfo { playerId=player },
            position=new Position { current=new PointNew { x=x,y=0,z=2 } } } };
    }
    static Packet Queue(SCCurrentPosition value)
    { Packet packet=new Packet { kBody=value }; PositionSync.TrackPacket(packet); return packet; }
    static void Deliver(Packet packet)
    {
        Check(!PositionSync.DropPacket(packet),"current-epoch packet was dropped");
        PositionSync.TrackBaseResponse(field,packet.kBody);
        PositionSync.TrackFieldResponse(packet.kBody);
    }
    static void OriginalFallbackThenLateReply()
    {
        Reset(); PositionSync.TrackSend(30024);
        // RoomNetBase.Tick builds this body locally after its 10-second wait.
        PositionSync.TrackFieldResponse(Position(42,2));
        Check(!PositionSync.ChannelReady,"local fallback consumed the outstanding native request");
        Deliver(Queue(Position(42,12)));
        Check(PositionSync.ChannelReady,"the late real reply did not restore the native channel");
        Check(PositionSync.TryLock(actor),"entry could not lock after the late real reply");
        Check(!actor.moving && !EasyTouchTool.Instance.Enable,"entry did not stop the actor and hide the joystick");
        PositionSync.Release(); Check(EasyTouchTool.Instance.Enable,"release did not restore the joystick");
    }
    static void NormalAndOverlappingPackets()
    {
        Reset(); PositionSync.TrackSend(30024); PositionSync.TrackSend(30024);
        Packet first=Queue(Position(42,8));
        Check(!PositionSync.DropPacket(first),"first reply was dropped");
        // An unrelated local fallback cannot take the current dispatch marker.
        PositionSync.TrackFieldResponse(Position(42,2));
        PositionSync.TrackFieldResponse(first.kBody);
        Check(!PositionSync.ChannelReady,"one reply incorrectly satisfied two sends");
        Deliver(Queue(Position(42,9)));
        Check(PositionSync.ChannelReady,"two real replies did not drain two sends");
        PositionSync.TrackFieldResponse(first.kBody);
        Check(PositionSync.ChannelReady,"a duplicate handler call invalidated the confirmed channel");
        Check(PositionSync.TryLock(actor),"normal channel could not lock");
        Check(!PositionSync.TryLock(actor),"a second operation took an existing lock");
        int callbacks=0; float observed=0;
        PositionSync.RequestCurrent(field,delegate(Vector3 p) { callbacks++; observed=p.x; });
        PositionSync.TrackFieldResponse(Position(42,2));
        Check(callbacks==0,"local fallback completed the post-teleport request");
        Deliver(Queue(Position(42,31)));
        Check(callbacks==1 && observed==31,"the real requested position did not complete once");
        Check(PositionSync.Locked,"position reply released the lock before server revision validation");
        PositionSync.Release();
    }
    static void ReconnectRejectsOldPackets()
    {
        Reset(); PositionSync.TrackSend(30024); Packet stale=Queue(Position(42,11));
        int before=PositionSync.ConnectionEpoch; PositionSync.ConnectionStarting();
        Check(PositionSync.ConnectionEpoch!=before,"connection epoch did not advance");
        PositionSync.LoginResponse(new SCSocketLogin { result=100 }); PositionSync.TrackSend(30024);
        Check(PositionSync.DropPacket(stale),"old socket reply survived reconnect");
        PositionSync.TrackFieldResponse(stale.kBody);
        Check(!PositionSync.ChannelReady,"old socket reply completed the new request");
        Deliver(Queue(Position(42,40)));
        Check(PositionSync.ChannelReady,"new authenticated connection did not become ready");
    }
    static void IdentityAndTransportRemainRequired()
    {
        Reset(); PositionSync.TrackSend(30024); Deliver(Queue(Position(99,1)));
        Check(!PositionSync.ChannelReady,"another player's real reply enabled entry");
        PositionSync.TrackSend(30024); Deliver(Queue(Position(42,1)));
        NetAPI.connected=false; Check(!PositionSync.ChannelReady,"disconnected socket enabled entry");
        NetAPI.connected=true; Check(PositionSync.ChannelReady,"socket readiness did not recover");
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-2",origin="https://example.test" };
        Check(!PositionSync.ChannelReady,"a changed session reused the prior authentication");
        PositionSync.TrackSend(30024); Deliver(Queue(Position(42,1)));
        Check(!PositionSync.ChannelReady,"a changed session became ready without login");
        PositionSync.ConnectionStarting(); PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        PositionSync.TrackSend(30024); Deliver(Queue(Position(42,1)));
        Check(PositionSync.ChannelReady,"fresh session did not become ready after login and reply");
    }
    static void OriginalNonFieldExitHandshake()
    {
        Reset(); PositionSync.SetFieldActive(false); RoomNetBase town=new RoomNetBase();
        PositionSync.TrackSend(30024);
        PositionSync.TrackBaseResponse(town,Position(42,2));
        Check(!PositionSync.ChannelReady,"town fallback consumed the native exit handshake");
        Packet packet=Queue(Position(42,5));
        Check(!PositionSync.DropPacket(packet),"town reply was dropped");
        PositionSync.TrackBaseResponse(town,packet.kBody);
        Check(PositionSync.ChannelReady,"town native exit handshake did not complete");
        PositionSync.TrackBaseResponse(town,packet.kBody);
        Check(PositionSync.ChannelReady,"duplicate town callback damaged readiness");
        PositionSync.ConnectionStarting(); PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        PositionSync.TrackSend(30024);
        Check(!PositionSync.DropPacket(packet),"non-field old frame changed native dispatch behavior");
        PositionSync.TrackBaseResponse(town,packet.kBody);
        Check(!PositionSync.ChannelReady,"an untracked old town frame satisfied the new handshake");
    }
    static void OldLeaseCannotReleaseNewOwner()
    {
        Reset(); PositionSync.TrackSend(30024); Deliver(Queue(Position(42,4)));
        long oldLease,newLease;
        Check(PositionSync.TryLock(actor,out oldLease) && oldLease>0,"first lease was not acquired");
        Check(PositionSync.OwnsLock(oldLease),"first lease is not recognized");
        PositionSync.Release();
        Check(!PositionSync.OwnsLock(oldLease),"released lease still owns the lock");
        Check(PositionSync.TryLock(actor,out newLease) && newLease>0 && newLease!=oldLease,"new owner reused an old lease");
        Check(!PositionSync.ReleaseIfOwned(oldLease),"stale callback released a new owner's lock");
        Check(PositionSync.OwnsLock(newLease) && PositionSync.Locked,"new owner lost the lock");
        Check(PositionSync.ReleaseIfOwned(newLease) && !PositionSync.Locked,"current owner could not release");
        Check(!PositionSync.ReleaseIfOwned(newLease),"released lease could be reused");
        long failed=123; Check(!PositionSync.TryLock(null,out failed) && failed==0,"failed acquisition exposed a lease");
    }
    static void LateOwnerKeepsTrackedBaseline()
    {
        Reset(); PositionSync.ConnectionStarting();
        typeof(PositionSync).GetField("owner",System.Reflection.BindingFlags.Static|System.Reflection.BindingFlags.NonPublic).SetValue(null,null);
        FieldWire.session=null;
        PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        // The original request is sent before the player snapshot exists.
        PositionSync.TrackSend(30024); PositionSync.TrackSend(30024);
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.TrackFieldResponse(Position(42,4));
        Check(!PositionSync.ChannelReady,"a fabricated fallback established a baseline");
        Deliver(Queue(Position(42,7)));
        Check(!PositionSync.ChannelReady,"one early response incorrectly satisfied two requests");
        Deliver(Queue(Position(42,7)));
        Check(PositionSync.ChannelReady,"a real initial frame failed to restore the tracked baseline");
        Check(PositionSync.TryLock(actor),"the recovered baseline did not allow the requested action");
        int callbacks=0;
        PositionSync.RequestCurrent(field,delegate(Vector3 p) { callbacks++; });
        PositionSync.TrackFieldResponse(Position(42,7));
        Check(callbacks==0 && PositionSync.Locked,"a local fallback satisfied a locked relocation");
        Deliver(Queue(Position(42,8)));
        Check(callbacks==1 && PositionSync.Locked,"the numbered relocation did not await its real frame");
        PositionSync.Release();
    }
    static void MissingBaselineRequestsFreshNativeFrame()
    {
        Reset();
        Check(!PositionSync.ChannelReady,"reset started with a confirmed baseline");
        PositionSync.Tick();
        Check(!PositionSync.ChannelReady,"sending a request alone established a baseline");
        PositionSync.TrackFieldResponse(Position(42,4));
        Check(!PositionSync.ChannelReady,"the local fallback satisfied the retry");
        Deliver(Queue(Position(42,9)));
        Check(PositionSync.ChannelReady,"the real retry did not restore the channel");
    }
    static void InvalidUntrackedFrameCannotEstablishBaseline()
    {
        Reset(); Deliver(Queue(Position(99,1)));
        Check(!PositionSync.ChannelReady,"another player's untracked frame established a baseline");
        Deliver(Queue(Position(42,Single.NaN)));
        Check(!PositionSync.ChannelReady,"nonfinite position established a baseline");
        Deliver(Queue(Position(42,2)));
        Check(!PositionSync.ChannelReady,"an unsolicited valid frame established a baseline");
        PositionSync.ConnectionStarting(); PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        Packet stale=Queue(Position(42,3)); PositionSync.ConnectionStarting();
        PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        Check(PositionSync.DropPacket(stale),"old epoch frame was not dropped");
        PositionSync.TrackFieldResponse(stale.kBody);
        Check(!PositionSync.ChannelReady,"old epoch established an untracked baseline");
    }
    static void DeliveredBeforeOwnerDrainsInOrder()
    {
        Reset(); PositionSync.ConnectionStarting();
        typeof(PositionSync).GetField("owner",System.Reflection.BindingFlags.Static|System.Reflection.BindingFlags.NonPublic).SetValue(null,null);
        FieldWire.session=null; PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        PositionSync.TrackSend(30024); PositionSync.TrackSend(30024);
        SCCurrentPosition first=Position(42,10),second=Position(42,20);
        PositionSync.TrackFieldResponse(Position(42,30)); // Unmarked fallback cannot be stored.
        Deliver(Queue(first)); Deliver(Queue(second));
        Check(!PositionSync.ChannelReady,"frames before owner enabled the channel");
        RoomBase saved=RoomManager.currentRoom; RoomManager.currentRoom=null;
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.Tick();
        Check(!PositionSync.ChannelReady && FieldRuntime.readyCalls==0,"frames drained before the native room was ready");
        RoomManager.currentRoom=saved; PositionSync.Tick();
        Check(PositionSync.ChannelReady,"real frames were lost before the owner was ready");
        Check(FieldRuntime.readyCalls==1 && FieldRuntime.readyNet==field && FieldRuntime.readyFrame==second,
            "readiness did not use the last real frame in request order");
    }
    static void DeferredLockedReplyPreservesLease()
    {
        Reset(); PositionSync.TrackSend(30024); Deliver(Queue(Position(42,2)));
        Check(PositionSync.TryLock(actor),"could not prepare locked relocation");
        int callbacks=0; float observed=0;
        PositionSync.RequestCurrent(field,delegate(Vector3 p) { callbacks++; observed=p.x; });
        FieldWire.session=null; Deliver(Queue(Position(42,23)));
        Check(callbacks==0 && PositionSync.Locked,"early frame bypassed owner validation");
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.Tick();
        Check(callbacks==1 && observed==23 && PositionSync.Locked,"deferred reply did not preserve the numbered relocation and lease");
        PositionSync.Tick(); Check(callbacks==1,"deferred callback ran more than once");
        PositionSync.Release();
    }
    static void DeferredThenLiveFramesStayOrdered()
    {
        Reset(); PositionSync.TrackSend(30024); PositionSync.TrackSend(30024);
        FieldWire.session=null; Deliver(Queue(Position(42,11)));
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        SCCurrentPosition later=Position(42,19); Deliver(Queue(later));
        Check(!PositionSync.ChannelReady,"a later live reply overtook the deferred response");
        PositionSync.Tick();
        Check(PositionSync.ChannelReady && FieldRuntime.readyCalls==1 && FieldRuntime.readyFrame==later,
            "deferred and live frames did not drain in original order");
    }
    static void DeferredRepliesDiscardedOnEpochOrOwnerChange()
    {
        Reset(); PositionSync.TrackSend(30024); FieldWire.session=null;
        Packet old=Queue(Position(42,4)); Deliver(old);
        PositionSync.ConnectionStarting();
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.LoginResponse(new SCSocketLogin { result=100 }); PositionSync.Tick();
        Check(!PositionSync.ChannelReady && FieldRuntime.readyCalls==0,"a buffered old connection established readiness");
        Check(PositionSync.DropPacket(old),"a duplicate old frame was accepted after reconnect");
        Deliver(Queue(Position(42,5))); Check(PositionSync.ChannelReady,"new connection could not establish a fresh baseline");
        Reset(); PositionSync.TrackSend(30024); FieldWire.session=null; Deliver(Queue(Position(42,8)));
        FieldWire.session=new FieldWire.Session { playerId=99,sessionHex="session-2",origin="https://example.test" };
        PositionSync.Tick();
        Check(!PositionSync.ChannelReady && FieldRuntime.readyCalls==0,"a prior role's buffered frame survived an owner change");
    }
    static void DeferredQueueIsBoundedByRequests()
    {
        Reset(); FieldWire.session=null;
        for (int i=0;i<64;i++) PositionSync.TrackSend(30024);
        for (int i=0;i<65;i++) Deliver(Queue(Position(42,i)));
        var pending=(System.Collections.ICollection)typeof(PositionSync).GetField("deferred",System.Reflection.BindingFlags.Static|System.Reflection.BindingFlags.NonPublic).GetValue(null);
        Check(pending.Count==64,"unsolicited frames exceeded the request-bounded queue");
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.Tick();
        Check(PositionSync.ChannelReady && FieldRuntime.readyCalls==1 && FieldRuntime.readyFrame.self.position.current.x==63,
            "bounded real frames did not drain in order");
    }
    static void DeferredIdentityAndCallbackFailuresStayClosed()
    {
        Reset(); PositionSync.TrackSend(30024); FieldWire.session=null; Deliver(Queue(Position(99,2)));
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.Tick();
        Check(!PositionSync.ChannelReady && FieldRuntime.readyCalls==0,"deferred frame for another player was accepted");
        Deliver(Queue(Position(42,3))); Check(PositionSync.ChannelReady,"valid retry did not recover after identity mismatch");
        Reset(); PositionSync.TrackSend(30024); Deliver(Queue(Position(42,2)));
        Check(PositionSync.TryLock(actor),"could not prepare callback failure test");
        PositionSync.RequestCurrent(field,delegate(Vector3 p) { throw new Exception("fixture callback failure"); });
        FieldWire.session=null; Deliver(Queue(Position(42,5)));
        FieldWire.session=new FieldWire.Session { playerId=42,sessionHex="session-1",origin="https://example.test" };
        PositionSync.Tick();
        Check(!PositionSync.ChannelReady && PositionSync.Locked,"callback failure escaped or released a protected relocation");
        PositionSync.Release();
    }
    public static void Main()
    {
        OriginalFallbackThenLateReply(); NormalAndOverlappingPackets(); ReconnectRejectsOldPackets();
        IdentityAndTransportRemainRequired(); OriginalNonFieldExitHandshake(); OldLeaseCannotReleaseNewOwner();
        LateOwnerKeepsTrackedBaseline(); MissingBaselineRequestsFreshNativeFrame(); InvalidUntrackedFrameCannotEstablishBaseline();
        DeliveredBeforeOwnerDrainsInOrder(); DeferredLockedReplyPreservesLease(); DeferredThenLiveFramesStayOrdered();
        DeferredRepliesDiscardedOnEpochOrOwnerChange(); DeferredQueueIsBoundedByRequests(); DeferredIdentityAndCallbackFailuresStayClosed();
        Console.WriteLine("PositionSync replay passed: "+checks+" assertions");
    }
}
