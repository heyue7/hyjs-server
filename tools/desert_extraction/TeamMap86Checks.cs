// Isolated behavioral checks for the production TeamMap86 source. These
// stubs do not simulate a real Unity scene, transport, or device.
using System;
using System.Collections.Generic;
using System.Threading;
using HYJS.DesertV4;
using ProtoBuf.Message;
using Room;

namespace UnityEngine
{
    public sealed class GameObject { public bool activeInHierarchy=true; }
    public struct Vector3 { public float x,y,z; public Vector3(float a,float b,float c) { x=a;y=b;z=c; } }
    public sealed class Transform { public Vector3 localPosition; }
    public static class Time { public static float unscaledTime; }
    public static class Mathf { public static float Clamp(float v,float min,float max) { return Math.Max(min,Math.Min(max,v)); } }
}
namespace NetWork.Packets { public sealed class Packet { public int nOpCode; public object kBody; } }
namespace NetWork
{
    public sealed class SocketController
    {
        object eventManager;
        public void SetEventManager(object value) { eventManager=value; }
        public bool IsConnected() { if (NetAPI.socketThrow) throw new InvalidOperationException(); GC.KeepAlive(eventManager); return NetAPI.connected; }
    }
}
namespace ProtoBuf.Message
{
    public sealed class Team { public string teamId; public List<TeamMember> memberList=new List<TeamMember>(); }
    public sealed class TeamMember { public long playerId; public int serverId,state; }
    public sealed class SCQueryTeam { public Team team; }
    public sealed class SCQueryTeamTick { public Team team; public int seconds; }
    public sealed class SCJoinTeam { public Team team; }
    public sealed class SCExitTeam { public Team team; }
    public sealed class SCTeamKick { public Team team; }
    public sealed class SCDisbandTeam { }
    public sealed class SCTeamCreateAndInvite { public Team team; }
    public sealed class ErrorMessage { }
    public sealed class PointNew { public float x,y,z; }
    public sealed class Position { public long playerId; public string scene; public PointNew current; }
    public sealed class MovePlayerInfo { public long playerId; public int serverId; }
    public sealed class MovePlayer { public MovePlayerInfo info; public Position position; }
    public sealed class SCPlayerAppear { public List<MovePlayer> player=new List<MovePlayer>(); }
    public sealed class SCMovePush { public List<Position> position=new List<Position>(); }
    public sealed class SCPlayerDisappear { public List<long> playerId=new List<long>(); }
}
namespace Room
{
    public enum CharactorType { Other=0, Player=2 }
    public class RoomBase { public object netManager; public RoomCharactorManager charactorManager; }
    public sealed class RoomField : RoomBase { }
    public sealed class RoomNetField
    {
        public void SCQueryTeamTick(object value) { }
        public void SCPlayerAppear(object value) { }
        public void SCMovePush(object value) { }
        public void SCPlayerDisappear(object value) { }
    }
    public static class RoomManager { public static RoomBase currentRoom; }
    public sealed class RoomCharactor
    {
        public CharactorType type=CharactorType.Player; public object ui;
        public UnityEngine.GameObject gameObject=new UnityEngine.GameObject();
        public UnityEngine.Transform transform=new UnityEngine.Transform();
    }
    public sealed class RCUIPlayer { public int serverID; }
    public sealed class RoomCharactorManager
    {
        public RoomCharactor self; public Dictionary<string,RoomCharactor> actors=new Dictionary<string,RoomCharactor>();
        public RoomCharactor GetSelfCharactor() { return self; }
        public RoomCharactor GetCharactor(string key) { RoomCharactor value; return actors.TryGetValue(key,out value) ? value : null; }
    }
}
public sealed class PlayerData { public long playerId; public string teamId; }
public static class StaticData { public static string fieldId; public static PlayerData playerData; }
public static class NetAPI
{
    static NetWork.SocketController _socket=new NetWork.SocketController();
    public static bool connected=true,socketThrow; public static int lazyCreates;
    public static bool SocketIsConnected()
    { if (_socket==null) { _socket=new NetWork.SocketController(); lazyCreates++; } return _socket.IsConnected(); }
    public static void DropSocket() { _socket=null; }
    public static void BindManager(object value) { if (_socket==null) _socket=new NetWork.SocketController(); _socket.SetEventManager(value); }
}
public sealed class FakeEventManager
{
    readonly Dictionary<int,Action<object>> msgHandle=new Dictionary<int,Action<object>>();
    public void Register(Action<object> callback)
    {
        int code=callback.Method.Name=="SCQueryTeamTick" ? 22024 : callback.Method.Name=="SCPlayerAppear" ? 30028 :
            callback.Method.Name=="SCMovePush" ? 30027 : 30029;
        msgHandle[code]=callback;
    }
}
namespace HYJS.DesertV4
{
    public static class PositionSync { public static int ConnectionEpoch { get; set; } }
    public static class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public static Session live;
        public static bool Current(out Session value) { value=live; return value!=null; }
    }
}
public static class TeamMap86Checks
{
    static RoomField room;
    static FakeEventManager manager;
    static readonly List<TeamMap86.Visible> visible=new List<TeamMap86.Visible>();
    static void Check(bool value,string message) { if (!value) throw new Exception(message); }
    static Team Team(params long[] ids)
    {
        Team team=new Team { teamId="team-a" };
        foreach (long id in ids) team.memberList.Add(new TeamMember { playerId=id,serverId=1,state=1 });
        return team;
    }
    static void Reset()
    {
        TeamMap86.ReleaseScene(); PositionSync.ConnectionEpoch++; TeamMap86.ConnectionStarting(); UnityEngine.Time.unscaledTime=0; NetAPI.connected=true; NetAPI.socketThrow=false;
        StaticData.fieldId="field_003"; StaticData.playerData=new PlayerData { playerId=1,teamId="team-a" };
        FieldWire.live=new FieldWire.Session { playerId=1,sessionHex="qa-session-SECRET",origin="https://test" };
        room=new RoomField { netManager=new RoomNetField(),charactorManager=new RoomCharactorManager() };
        manager=new FakeEventManager(); var net=(RoomNetField)room.netManager;
        manager.Register(net.SCQueryTeamTick); manager.Register(net.SCPlayerAppear); manager.Register(net.SCMovePush); manager.Register(net.SCPlayerDisappear); NetAPI.BindManager(manager);
        room.charactorManager.self=new RoomCharactor();
        for (long id=2;id<=6;id++) room.charactorManager.actors[id.ToString()]=new RoomCharactor { ui=new RCUIPlayer { serverID=1 } };
        RoomManager.currentRoom=room; TeamMap86.BindScene(room);
    }
    static void Queue(object packet) { TeamMap86.Queue(new NetWork.Packets.Packet { kBody=packet }); }
    static void NetworkQueue(params object[] packets)
    {
        Exception failure=null;
        Thread worker=new Thread(delegate()
        {
            try { foreach (object packet in packets) Queue(packet); }
            catch (Exception error) { failure=error; }
        });
        worker.Start(); Check(worker.Join(5000),"network worker did not complete");
        if (failure!=null) throw new Exception("network worker failed",failure);
    }
    static void Respond(object packet) { if (packet is SCQueryTeamTick) TeamMap86.TickSent(room.netManager); Queue(packet); TeamMap86.Observe(room.netManager,packet); }
    static List<TeamMap86.Visible> Read() { TeamMap86.Collect(room,visible); return visible; }
    static Dictionary<long,int> ColorByPlayer()
    {
        var result=new Dictionary<long,int>(); foreach (TeamMap86.Visible item in Read()) result.Add(item.key.playerId,item.color); return result;
    }
    static Position Point(long id,float x,string scene)
    { return new Position { playerId=id,scene=scene,current=new PointNew { x=x,y=2,z=3 } }; }
    static SCPlayerAppear Appear(long id,int server,float x)
    {
        var packet=new SCPlayerAppear();
        packet.player.Add(new MovePlayer { info=new MovePlayerInfo { playerId=id,serverId=server },position=Point(id,x,"town_side_field_003") });
        return packet;
    }
    static NetWork.Packets.Packet PositionQueue(object value,int opcode)
    {
        var packet=new NetWork.Packets.Packet { nOpCode=opcode,kBody=value };
        TeamMap86.Queue(packet); TeamMap86.InspectDispatch(manager,packet); return packet;
    }
    static void PositionObserve(NetWork.Packets.Packet packet)
    { TeamMap86.ObservePosition(room.netManager,packet.kBody); }
    static void PositionEvent(object value,int opcode) { PositionObserve(PositionQueue(value,opcode)); }
    static SCMovePush Move(long id,float x,string scene)
    { var packet=new SCMovePush(); packet.position.Add(Point(id,x,scene)); return packet; }
    static SCPlayerDisappear Disappear(long id)
    { var packet=new SCPlayerDisappear(); packet.playerId.Add(id); return packet; }
    public static void Main()
    {
        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2,3,4,5),seconds=3 });
        Dictionary<long,int> original=ColorByPlayer(); Check(original.Count==4,"four members visible");
        Check(new HashSet<int>(original.Values).Count==4,"colors are unique");
        Respond(new SCQueryTeamTick { team=Team(5,3,1,4,2),seconds=3 });
        Dictionary<long,int> reordered=ColorByPlayer();
        foreach (long id in original.Keys) Check(original[id]==reordered[id],"member reorder changed color");
        Team some=Team(1,2,3,4,5); some.memberList[2].state=0;
        Respond(new SCQueryTeamTick { team=some,seconds=3 }); Check(!ColorByPlayer().ContainsKey(3),"state zero displayed");
        room.charactorManager.actors["2"].ui=new RCUIPlayer { serverID=2 };
        Check(!ColorByPlayer().ContainsKey(2),"actor server mismatch displayed");
        room.charactorManager.actors["2"].ui=new RCUIPlayer { serverID=1 };
        room.charactorManager.actors.Remove("4"); Check(!ColorByPlayer().ContainsKey(4),"absent actor displayed");
        room.charactorManager.actors["5"].type=CharactorType.Other; Check(!ColorByPlayer().ContainsKey(5),"nonplayer actor displayed");

        Reset(); Respond(new SCTeamCreateAndInvite { team=Team(1,2) }); Check(Read().Count==1,"creator snapshot unavailable");
        Respond(new SCTeamCreateAndInvite { team=Team(2,3) }); Check(Read().Count==0,"unjoined invitation displayed");
        Respond(new SCJoinTeam { team=Team(1,2) }); Check(Read().Count==1,"join snapshot unavailable");
        Respond(new SCExitTeam { team=Team(2) }); Check(Read().Count==0,"self exit retained snapshot");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Respond(new SCTeamKick { team=Team(2) }); Check(Read().Count==0,"self kick retained snapshot");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Respond(new SCDisbandTeam()); Check(Read().Count==0,"disband retained snapshot");

        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        object stale=new SCQueryTeamTick { team=Team(1,3),seconds=3 }; TeamMap86.TickSent(room.netManager); Queue(stale);
        PositionSync.ConnectionEpoch++; TeamMap86.ConnectionStarting(); TeamMap86.Observe(room.netManager,stale);
        Check(Read().Count==0,"old connection packet accepted");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==1,"new connection response rejected");
        stale=new SCDisbandTeam(); Queue(stale); TeamMap86.ReleaseScene(); TeamMap86.BindScene(room);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); TeamMap86.Observe(room.netManager,stale);
        Check(Read().Count==1,"old room packet cleared current snapshot");
        UnityEngine.Time.unscaledTime=10; Check(Read().Count==0,"expired state displayed");

        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        TeamMap86.TickSent(room.netManager); stale=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        Respond(new SCExitTeam { team=Team(1,3) }); Queue(stale); TeamMap86.Observe(room.netManager,stale);
        Check(Read().Count==0,"tick sent before membership change restored old snapshot");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"fresh tick after change rejected");
        TeamMap86.TickSent(room.netManager); Queue(new ErrorMessage()); Check(Read().Count==0,"ambiguous error kept snapshot");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(Read().Count==0,"ambiguous queue was reused");
        Reset(); TeamMap86.TickSent(room.netManager); TeamMap86.ReleaseScene(); TeamMap86.BindScene(room);
        stale=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; Queue(stale); TeamMap86.Observe(room.netManager,stale);
        Check(Read().Count==0,"old room request returned in new room");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"room transition blocked fresh tick");
        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        for (int n=0;n<65;n++) TeamMap86.TickSent(room.netManager);
        Check(Read().Count==0,"overflow queue retained snapshot");

        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Team invalid=Team(1,2,2); Respond(new SCQueryTeamTick { team=invalid,seconds=3 }); Check(Read().Count==0,"duplicate identities accepted");
        invalid=Team(1,2); invalid.memberList[1].serverId=2;
        Respond(new SCQueryTeam { team=invalid }); Check(Read().Count==0,"cross-server member accepted");
        Respond(new SCQueryTeam { team=Team(2,3) }); Check(Read().Count==0,"other team's view accepted");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        StaticData.playerData.teamId="other"; Check(Read().Count==0,"teamId mismatch displayed");
        StaticData.playerData.teamId="team-a"; NetAPI.connected=false; Check(Read().Count==0,"disconnected state displayed");
        NetAPI.connected=true; FieldWire.live=new FieldWire.Session { playerId=1,sessionHex="new-session",origin="https://test" };
        Check(Read().Count==0,"other session displayed");
        Reset();
        object unpaired=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; Queue(unpaired); TeamMap86.Observe(room.netManager,unpaired);
        Check(Read().Count==0,"unpaired response displayed");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==0,"unpaired queue was reused");
        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        TeamMap86.TickSent(room.netManager); UnityEngine.Time.unscaledTime=16;
        Check(!TeamMap86.DiagnosticState().broken,"diagnostic sampling changed timeout state");
        Check(Read().Count==0,"pending tick timeout displayed");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==0,"timed out queue was reused");
        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); TeamMap86.TickSent(room.netManager);
        Queue(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); UnityEngine.Time.unscaledTime=16;
        Check(Read().Count==0 && TeamMap86.DiagnosticState().breakReason==11,"queued response timeout did not fail closed");

        // These separate-thread tests exercise the conservative queue lock.
        // The original AddPacket call itself is in SocketController.Tick.
        Reset(); TeamMap86.TickSent(room.netManager);
        var first=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        NetworkQueue(first,new ErrorMessage());
        var pending=TeamMap86.DiagnosticState();
        Check(!pending.broken && pending.sent==0 && pending.received==1,"success then unrelated error mispaired");
        TeamMap86.Observe(room.netManager,first); Check(Read().Count==1,"queued success was hidden by later error");
        pending=TeamMap86.DiagnosticState(); Check(pending.sent==0 && pending.received==0,"paired counts not consumed");

        Reset(); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        NetworkQueue(new ErrorMessage(),first); TeamMap86.Observe(room.netManager,first);
        Check(TeamMap86.DiagnosticState().broken && Read().Count==0,"error before response did not fail closed");

        Reset(); TeamMap86.TickSent(room.netManager); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        var second=new SCQueryTeamTick { team=Team(1,3),seconds=3 };
        NetworkQueue(first,new ErrorMessage(),second);
        Check(TeamMap86.DiagnosticState().broken,"error with an unanswered tick was accepted");
        TeamMap86.Observe(room.netManager,first); TeamMap86.Observe(room.netManager,second);
        Check(Read().Count==0,"ambiguous batch displayed");

        Reset(); TeamMap86.TickSent(room.netManager); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; second=new SCQueryTeamTick { team=Team(1,3),seconds=3 };
        NetworkQueue(first,second,new ErrorMessage()); pending=TeamMap86.DiagnosticState();
        Check(!pending.broken && pending.sent==0 && pending.received==2,"complete batch mispaired");
        TeamMap86.Observe(room.netManager,first); pending=TeamMap86.DiagnosticState();
        Check(pending.sent==0 && pending.received==1,"first batch response not consumed");
        TeamMap86.Observe(room.netManager,second); Check(ColorByPlayer().ContainsKey(3),"second batch response missing");

        Reset(); TeamMap86.TickSent(room.netManager); UnityEngine.Time.unscaledTime=5;
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first); TeamMap86.Observe(room.netManager,first);
        Check(Read().Count==1,"five-second response rejected"); UnityEngine.Time.unscaledTime=9.1f;
        Check(Read().Count==0,"response age extended snapshot lifetime");

        // Two same-room sends can complete in the opposite order. Keep the
        // earliest possible send time until the whole batch is received.
        Reset(); TeamMap86.TickSent(room.netManager); UnityEngine.Time.unscaledTime=10; TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,3),seconds=3 }; second=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        NetworkQueue(first); TeamMap86.Observe(room.netManager,first); Check(Read().Count==0,"old candidate was assigned a fresh lifetime");
        UnityEngine.Time.unscaledTime=16; NetworkQueue(second); TeamMap86.Observe(room.netManager,second);
        Check(TeamMap86.DiagnosticState().broken && Read().Count==0,"late same-room response escaped timeout");

        Reset(); TeamMap86.TickSent(room.netManager); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; second=new SCQueryTeamTick { team=Team(1,3),seconds=3 };
        ManualResetEvent firstQueued=new ManualResetEvent(false),proceed=new ManualResetEvent(false);
        Exception interleaveFailure=null;
        Thread interleaved=new Thread(delegate()
        {
            try { Queue(first); firstQueued.Set(); if (!proceed.WaitOne(5000)) throw new Exception("main thread timeout"); Queue(second); Queue(new ErrorMessage()); }
            catch (Exception error) { interleaveFailure=error; firstQueued.Set(); }
        });
        interleaved.Start(); Check(firstQueued.WaitOne(5000),"network thread timeout");
        TeamMap86.Observe(room.netManager,first); proceed.Set();
        Check(interleaved.Join(5000) && interleaveFailure==null,"interleaved worker failed");
        firstQueued.Close(); proceed.Close(); TeamMap86.Observe(room.netManager,second);
        Check(ColorByPlayer().ContainsKey(3) && !TeamMap86.DiagnosticState().broken,"interleaved consume mispaired");

        Reset(); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first);
        PositionSync.ConnectionEpoch++; TeamMap86.ConnectionStarting();
        TeamMap86.Observe(room.netManager,first); pending=TeamMap86.DiagnosticState();
        Check(!pending.broken && pending.received==0 && pending.sent==0,"old connection changed new counters");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"new connection did not recover");

        Reset(); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first,first);
        Check(TeamMap86.DiagnosticState().broken,"duplicate packet identity accepted");
        Reset(); first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first);
        Check(TeamMap86.DiagnosticState().broken,"unmatched incoming tick accepted");
        Reset(); TeamMap86.Observe(room.netManager,new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Check(TeamMap86.DiagnosticState().broken,"untracked tick accepted");
        TeamMap86.ReleaseScene(); TeamMap86.BindScene(room);
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(Read().Count==0,"untracked packet reset at room boundary");
        PositionSync.ConnectionEpoch++; TeamMap86.ConnectionStarting();
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"new connection did not clear missing accounting");

        Reset(); var oldManager=new FakeEventManager(); oldManager.Register(((RoomNetField)room.netManager).SCQueryTeamTick); NetAPI.BindManager(oldManager);
        int oldQueued=TeamMap86.DiagnosticState().queued; TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; Queue(first);
        TeamMap86.ReleaseScene(); TeamMap86.BindScene(room);
        Respond(new SCQueryTeam { team=Team(1,3) }); Check(Read().Count==0,"queued old-room candidate exposed a non-tick snapshot");
        TeamMap86.InspectDispatch(oldManager,new NetWork.Packets.Packet { nOpCode=22024,kBody=first });
        TeamMap86.Observe(room.netManager,first); pending=TeamMap86.DiagnosticState();
        Check(!pending.broken && pending.received==0 && pending.queued==oldQueued && Read().Count==0,"queued old-room response survived transition");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"fresh response after old-room dequeue rejected");

        Reset(); TeamMap86.ReleaseScene(); TeamMap86.TickSent(room.netManager); TeamMap86.BindScene(room);
        Respond(new SCQueryTeam { team=Team(1,3) }); Check(Read().Count==0,"unanswered old-room candidate exposed a non-tick snapshot");
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first); TeamMap86.Observe(room.netManager,first);
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"fresh response after old-room candidate rejected");

        // Old and current sends can be answered in either order. Neither
        // mixed response is a safe current-room snapshot, even if FIFO would
        // otherwise select the apparent current ticket for the second one.
        Reset(); TeamMap86.ReleaseScene(); TeamMap86.TickSent(room.netManager); TeamMap86.BindScene(room);
        TeamMap86.TickSent(room.netManager);
        Respond(new SCQueryTeam { team=Team(1,3) }); Check(Read().Count==0,"mixed candidates exposed a non-tick snapshot");
        first=new SCQueryTeamTick { team=Team(1,3),seconds=3 }; second=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        NetworkQueue(first); TeamMap86.Observe(room.netManager,first); Check(Read().Count==0,"mixed new response displayed");
        NetworkQueue(second); TeamMap86.Observe(room.netManager,second); Check(Read().Count==0,"late old response displayed");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"drained mixed batch did not recover");

        // An error before the new binding cannot permanently mark the new
        // room broken; its unanswered old send still remains a tombstone.
        Reset(); TeamMap86.ReleaseScene(); TeamMap86.TickSent(room.netManager); NetworkQueue(new ErrorMessage());
        Check(!TeamMap86.DiagnosticState().broken,"unbound error poisoned the channel");
        TeamMap86.BindScene(room); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,3),seconds=3 }; NetworkQueue(first); TeamMap86.Observe(room.netManager,first);
        Check(Read().Count==0,"uncertain old error displayed a current reply");
        UnityEngine.Time.unscaledTime=16; Check(Read().Count==0 && TeamMap86.DiagnosticState().breakReason==11,"missing old reply did not fail closed");
        TeamMap86.ReleaseScene(); TeamMap86.BindScene(room);
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(Read().Count==0,"hard timeout reset on room change");
        PositionSync.ConnectionEpoch++; TeamMap86.ConnectionStarting();
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"new connection did not clear hard timeout");

        // A current error locks this room but need not poison a later room.
        // Its old send stays in the candidate set until its reply is seen.
        Reset(); TeamMap86.TickSent(room.netManager); NetworkQueue(new ErrorMessage());
        Check(TeamMap86.DiagnosticState().breakReason==1,"current ambiguous error not latched");
        TeamMap86.ReleaseScene(); TeamMap86.BindScene(room); Check(!TeamMap86.DiagnosticState().broken,"old room error poisoned new room");
        TeamMap86.TickSent(room.netManager); first=new SCQueryTeamTick { team=Team(1,3),seconds=3 };
        second=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first,second);
        TeamMap86.Observe(room.netManager,first); TeamMap86.Observe(room.netManager,second);
        Check(Read().Count==0,"mixed replies after room error displayed");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"new room failed after safe drain");

        Reset(); TeamMap86.TickSent(room.netManager);
        Respond(new SCJoinTeam { team=Team(1,3) }); TeamMap86.TickSent(room.netManager);
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; second=new SCQueryTeamTick { team=Team(1,3),seconds=3 };
        NetworkQueue(second,first); TeamMap86.Observe(room.netManager,second); TeamMap86.Observe(room.netManager,first);
        Check(Read().Count==0,"mixed membership generations displayed");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"fresh membership generation did not recover");

        Reset(); TeamMap86.TickSent(room.netManager); room.netManager=new RoomNetField();
        var replacement=new FakeEventManager(); replacement.Register(((RoomNetField)room.netManager).SCQueryTeamTick); NetAPI.BindManager(replacement);
        TeamMap86.TickSent(room.netManager); first=new SCQueryTeamTick { team=Team(1,2),seconds=3 };
        second=new SCQueryTeamTick { team=Team(1,3),seconds=3 }; NetworkQueue(second,first);
        TeamMap86.Observe(room.netManager,second); TeamMap86.Observe(room.netManager,first);
        Check(Read().Count==0,"replaced room network handler accepted mixed replies");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"replacement handler did not recover");

        Reset(); NetAPI.connected=false; TeamMap86.TickSent(room.netManager); NetAPI.connected=true;
        first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; NetworkQueue(first); TeamMap86.Observe(room.netManager,first);
        Check(Read().Count==0,"disconnected send became a valid snapshot");
        Respond(new SCQueryTeamTick { team=Team(1,3),seconds=3 }); Check(ColorByPlayer().ContainsKey(3),"fresh connected send did not recover");

        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        object[] overflow=new object[257];
        for (int n=0;n<overflow.Length;n++) overflow[n]=new SCQueryTeam { team=Team(1,2) };
        NetworkQueue(overflow); pending=TeamMap86.DiagnosticState();
        Check(pending.broken && pending.queued<=256 && Read().Count==0,"packet overflow failed to hide snapshot");

        Reset(); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        pending=TeamMap86.DiagnosticState();
        Check(pending.boundRoom && pending.currentRoom && pending.sessionMatch && pending.socketKnown && pending.socket && pending.fieldTarget &&
            pending.selfLoaded && pending.teamMatch && !pending.broken && pending.members==2 && pending.actors==1 &&
            pending.remainingSeconds==9,"diagnostic scalar facts differ");
        string line=pending.ToString();
        Check(line.IndexOf("team-a")<0 && line.IndexOf("qa-session-SECRET")<0 && line.IndexOf("https://test")<0,
            "diagnostic exposed private data");
        NetAPI.DropSocket(); int creates=NetAPI.lazyCreates; var emptySocket=TeamMap86.DiagnosticState();
        Check(emptySocket.socketKnown && !emptySocket.socket && NetAPI.lazyCreates==creates,"diagnostic created socket");
        NetAPI.SocketIsConnected(); // restore only the test stub for subsequent checks
        NetAPI.socketThrow=true; var badSocket=TeamMap86.DiagnosticState(); NetAPI.socketThrow=false;
        Check(!badSocket.socketKnown && badSocket.members==2 && badSocket.actors==1,"socket failure truncated diagnostics");
        Reset(); var manager=new FakeEventManager(); NetAPI.BindManager(manager);
        manager.Register(((RoomNetField)room.netManager).SCQueryTeamTick);
        TeamMap86.TickSent(room.netManager); first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; Queue(first);
        TeamMap86.InspectDispatch(manager,new NetWork.Packets.Packet { nOpCode=22024,kBody=first });
        pending=TeamMap86.DiagnosticState();
        Check(pending.tickQueued==1 && pending.tickDequeued==1 && pending.tickObserved==0 && pending.lastOpcode &&
            pending.lastTracked && pending.lastHandlersKnown && pending.lastHandler && pending.lastBoundHandler &&
            pending.lastManagerKnown && pending.lastCurrentManager,"registered native dispatch facts differ");
        TeamMap86.Observe(room.netManager,first); pending=TeamMap86.DiagnosticState();
        Check(pending.tickObserved==1 && pending.sent==0 && pending.received==0 && Read().Count==1,"native callback observation failed");

        Reset(); manager=new FakeEventManager(); NetAPI.BindManager(manager);
        int queuedBefore=TeamMap86.DiagnosticState().queued;
        TeamMap86.TickSent(room.netManager); first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; Queue(first);
        TeamMap86.InspectDispatch(manager,new NetWork.Packets.Packet { nOpCode=22024,kBody=first });
        pending=TeamMap86.DiagnosticState();
        Check(!pending.broken && pending.sent==0 && pending.received==0 && pending.queued==queuedBefore &&
            pending.tickQueued==1 && pending.tickDequeued==1 && pending.tickObserved==0 && pending.lastHandlersKnown && !pending.lastHandler,
            "missing native callback left a stale tag");
        manager.Register(((RoomNetField)room.netManager).SCQueryTeamTick);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==1,"fresh response after missing handler was hidden");

        Reset(); manager=new FakeEventManager(); NetAPI.BindManager(manager); manager.Register(new RoomNetField().SCQueryTeamTick);
        TeamMap86.TickSent(room.netManager); first=new SCQueryTeamTick { team=Team(1,2),seconds=3 }; Queue(first);
        TeamMap86.InspectDispatch(manager,new NetWork.Packets.Packet { nOpCode=22024,kBody=first }); pending=TeamMap86.DiagnosticState();
        Check(pending.lastHandler && !pending.lastBoundHandler && pending.lastCurrentManager,"different native callback target missed");
        Reset(); TeamMap86.TickSent(room.netManager); Queue(new ErrorMessage());
        Check(TeamMap86.DiagnosticState().breakReason==1,"ambiguous error cause not recorded");

        var viewGate=new TeamMap86.DiagnosticGate(); var viewState=new TeamMap86.Diagnostics();
        Check(viewGate.Allow(viewState),"first view diagnostic omitted"); viewState.viewMembers=1;
        Check(viewGate.Allow(viewState) && viewState.ToString().IndexOf("viewMembers=1")>=0,"changed view count omitted");
        pending=TeamMap86.DiagnosticState();
        var gate=new TeamMap86.DiagnosticGate(); Check(gate.Allow(pending),"first diagnostic omitted");
        UnityEngine.Time.unscaledTime=1; Check(!gate.Allow(TeamMap86.DiagnosticState()),"countdown alone emitted another diagnostic");
        for (int n=1;n<12;n++) { pending.connectionEpoch++; Check(gate.Allow(pending),"changed diagnostic omitted"); }
        pending.connectionEpoch++; Check(gate.Full && !gate.Allow(pending),"diagnostic process limit exceeded");

        // Presence can arrive before binding or before the first full team.
        Reset(); room.charactorManager.actors.Remove("2"); TeamMap86.ReleaseScene();
        PositionEvent(Appear(2,1,11),30028); TeamMap86.BindScene(room);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Check(Read().Count==1 && Read()[0].actor==null && Read()[0].position.x==11,"early appearance lost");
        PositionEvent(Move(2,12,"town_side_field_003"),30027);
        Check(Read().Count==1 && Read()[0].position.x==12,"far move not projected");
        PositionEvent(Move(2,99,"field_002"),30027); PositionEvent(Move(2,Single.NaN,"field_003"),30027);
        Check(Read().Count==1 && Read()[0].position.x==12,"wrong scene or nonfinite move accepted");
        var near=new RoomCharactor { ui=new RCUIPlayer { serverID=1 } }; near.transform.localPosition=new UnityEngine.Vector3(13,0,0);
        room.charactorManager.actors["2"]=near;
        Check(Read().Count==1 && Read()[0].actor==near && Read()[0].position.x==13,"native actor not preferred");
        room.charactorManager.actors.Remove("2");
        var delayed=PositionQueue(Move(2,14,"field_003"),30027);
        PositionEvent(Disappear(2),30029); PositionObserve(delayed);
        Check(Read().Count==0,"delayed move revived a disappeared player");
        PositionEvent(Appear(2,2,15),30028); Check(Read().Count==0,"wrong server identity displayed");
        PositionEvent(Appear(2,1,16),30028); Check(Read().Count==1 && Read()[0].position.x==16,"fresh same-server appearance missing");
        Respond(new SCExitTeam { team=Team(1,3) }); Check(!ColorByPlayer().ContainsKey(2),"left member retained");
        Respond(new SCJoinTeam { team=Team(1,2) });
        Check(Read().Count==1 && Read()[0].position.x==16,"stationary same-line member missing after rejoin");
        PositionEvent(Move(2,17,"field_003"),30027); Check(Read().Count==1 && Read()[0].position.x==17,"fresh move after rejoin missing");
        delayed=PositionQueue(Move(2,18,"field_003"),30027); TeamMap86.ScopeStarting(30061); PositionObserve(delayed);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==0,"old-line position survived switch");
        PositionEvent(Appear(2,1,19),30028); Check(Read().Count==1,"new-line appearance missing");
        delayed=PositionQueue(Move(2,20,"field_003"),30027); PositionSync.ConnectionEpoch++; TeamMap86.ConnectionStarting(); PositionObserve(delayed);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==0,"old-connection position survived reconnect");

        Reset(); room.charactorManager.actors.Remove("2"); PositionEvent(Appear(2,1,21),30028);
        UnityEngine.Time.unscaledTime=61; Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Check(Read().Count==1 && Read()[0].position.x==21,"stationary same-line candidate lost before invitation");
        PositionEvent(Appear(2,1,22),30028); Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        for (int n=0;n<25;n++) { UnityEngine.Time.unscaledTime+=3; Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); }
        Check(Read().Count==1 && Read()[0].position.x==22,"stationary online member expired despite live team ticks");
        UnityEngine.Time.unscaledTime+=10; Check(Read().Count==0,"expired team snapshot displayed");

        Reset(); room.charactorManager.actors.Remove("2"); PositionEvent(Appear(2,1,23),30028);
        UnityEngine.Time.unscaledTime=61; PositionEvent(Disappear(2),30029);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Check(Read().Count==0,"departed stationary candidate displayed after invitation");

        Reset(); room.charactorManager.actors.Remove("2");
        for (int n=2;n<=22;n++) PositionEvent(Appear(n,1,n),30028);
        Respond(new SCQueryTeamTick { team=Team(1,22),seconds=3 }); Check(Read().Count==0,"presence cap admitted 21st identity");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==1,"bounded first identity missing");
        Reset(); room.charactorManager.actors.Remove("2");
        for (int n=2;n<=21;n++) PositionEvent(Appear(n,1,n),30028);
        delayed=PositionQueue(Appear(2,1,99),30028); PositionEvent(Disappear(2),30029);
        PositionEvent(Appear(22,1,22),30028); PositionObserve(delayed);
        Respond(new SCQueryTeamTick { team=Team(1,22),seconds=3 });
        Check(Read().Count==1 && Read()[0].position.x==22,"departed tombstone blocked new member");
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 });
        Check(Read().Count==0,"evicted tombstone allowed an older packet to revive");
        Reset(); room.charactorManager.actors.Remove("2"); PositionEvent(Move(2,1,"field_003"),30027);
        Respond(new SCQueryTeamTick { team=Team(1,2),seconds=3 }); Check(Read().Count==0,"movement without appearance established identity");
        Console.WriteLine("TeamMap86 behavioral checks passed");
    }
}
