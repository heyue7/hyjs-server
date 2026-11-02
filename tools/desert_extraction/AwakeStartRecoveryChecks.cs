// Exercise the real awake-entry wrapper with deterministic native and transport stubs.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using HYJS.DesertV4;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UI;

namespace NetWork.Packets
{
    public sealed class Packet { public object kBody; }
}
namespace NetWork.Layer
{
    public delegate void HttpHandler(Packet packet,bool success);
}
namespace ProtoBuf.Message
{
    public sealed class CSAwakeCopyFightBegin { public string monGuid; }
    public sealed class SCAwakeCopyFightBegin { public string fightId; }
}
namespace Room
{
    public class RoomBase { }
    public static class RoomManager { public static RoomBase currentRoom; }
}
namespace UI
{
    public sealed class ManagerController
    {
        bool enterBattle,receivePveMsg;
        public static ManagerController Instance;
        public void SetFlags(bool enter,bool receive) { enterBattle=enter; receivePveMsg=receive; }
        public bool Enter { get { return enterBattle; } }
        public bool Receive { get { return receivePveMsg; } }
    }
    public sealed class LoadingPanel
    {
        public static LoadingPanel Instance;
        public int hides; public float lastDelay;
        public void HideMask(float delay) { hides++; lastDelay=delay; }
    }
    public sealed class UIChallengeRunMap
    {
        public int ended;
        public void EndIntoBattleEffect() { ended++; }
    }
    public sealed class PageManager
    {
        public static PageManager instance;
        public object currentPage;
    }
}
public static class StaticData
{
    public static object curBattleData;
    public static string awakeCopyFightMonGuid;
}
public static class BoxManager
{
    public static int popupCount,reloginCount;
    public static string lastRelogin;
    public static void CreatePopupTextBox(string message,float delay) { popupCount++; }
    public static void CreateReloginBox(string message) { reloginCount++; lastRelogin=message; }
}
public static class LanguageTool
{
    public static int lastCode;
    public static string GetTextNoUpdate(int code) { lastCode=code; return "native-message-"+code; }
}
public static class NetAPI
{
    public sealed class Call { public int opcode; public object request; public HttpHandler callback; }
    public static readonly List<Call> calls=new List<Call>();
    public static bool sendResult,throwOnSend;
    public static bool SendHttp(int opcode,object request,HttpHandler callback)
    {
        calls.Add(new Call { opcode=opcode,request=request,callback=callback });
        if (throwOnSend) throw new InvalidOperationException("transport failure");
        return sendResult;
    }
}
namespace HYJS.DesertV4
{
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public static Session current;
        public static bool Current(out Session value) { value=current; return value!=null; }
    }
}

public static class AwakeStartRecoveryChecks
{
    sealed class NativeCallback
    {
        public int calls; public bool throwOnCall,signalReceive; public Packet last; public bool success;
        public void Run(Packet packet,bool ok)
        {
            calls++; last=packet; success=ok;
            if (signalReceive) ManagerController.Instance.SetFlags(ManagerController.Instance.Enter,true);
            if (throwOnCall) throw new InvalidOperationException("native callback failure");
        }
    }
    sealed class Sequence : IEnumerator
    {
        readonly object[] values; readonly int throwAt;
        int index=-1;
        public int moves;
        public Sequence(object[] values,int throwAt) { this.values=values; this.throwAt=throwAt; }
        public object Current { get { return values[index]; } }
        public bool MoveNext()
        {
            moves++;
            if (moves==throwAt) throw new InvalidOperationException("iterator resource failure");
            index++; return index<values.Length;
        }
        public void Reset() { throw new NotSupportedException(); }
    }
    // The inspected native iterator yields while waiting. One later MoveNext
    // switches room and returns false without another yield.
    sealed class AwakeLoadSequence : IEnumerator
    {
        readonly RoomBase destination;
        public int moves,switches;
        public AwakeLoadSequence(RoomBase destination) { this.destination=destination; }
        public object Current { get { return 1; } }
        public bool MoveNext()
        {
            moves++;
            if (!ManagerController.Instance.Receive) return true;
            switches++; RoomManager.currentRoom=destination; return false;
        }
        public void Reset() { throw new NotSupportedException(); }
    }
    const BindingFlags HiddenStatic=BindingFlags.Static|BindingFlags.NonPublic;
    static readonly FieldInfo Current=typeof(AwakeStartRecovery).GetField("current",HiddenStatic);
    static int checks;

    static void Check(bool value,string message)
    { checks++; if (!value) throw new Exception(message); }
    static void Run(string name,Action test)
    {
        int before=checks; test(); Console.WriteLine("CASE\t"+name+"\t"+(checks-before));
    }
    static void Reset()
    {
        Check(Current!=null,"Awake attempt field unavailable"); Current.SetValue(null,null);
        FieldWire.current=new FieldWire.Session { playerId=42,sessionHex="session-42",origin="origin-1" };
        RoomManager.currentRoom=new RoomBase(); StaticData.curBattleData=new object();
        StaticData.awakeCopyFightMonGuid="monster-a";
        ManagerController.Instance=new ManagerController(); ManagerController.Instance.SetFlags(true,true);
        LoadingPanel.Instance=new LoadingPanel(); PageManager.instance=new PageManager();
        PageManager.instance.currentPage=new UIChallengeRunMap();
        BoxManager.popupCount=BoxManager.reloginCount=0; BoxManager.lastRelogin=null; LanguageTool.lastCode=0;
        NetAPI.calls.Clear(); NetAPI.sendResult=true; NetAPI.throwOnSend=false;
    }
    static bool Send(string monster,NativeCallback callback)
    {
        return AwakeStartRecovery.SendHttp(81009,new CSAwakeCopyFightBegin { monGuid=monster },callback.Run);
    }
    static Packet Good(string fight)
    { return new Packet { kBody=new SCAwakeCopyFightBegin { fightId=fight } }; }
    static void Expire()
    {
        object attempt=Current.GetValue(null);
        Check(attempt!=null,"Expected live awake attempt");
        FieldInfo deadline=attempt.GetType().GetField("deadline",BindingFlags.Public|BindingFlags.Instance);
        Check(deadline!=null,"Awake deadline field unavailable"); deadline.SetValue(attempt,0L);
    }
    static void AssertCleaned(string context)
    {
        Check(!ManagerController.Instance.Enter && !ManagerController.Instance.Receive,context+": native wait flags remain");
        Check(LoadingPanel.Instance.hides==1 && LoadingPanel.Instance.lastDelay==.5f,context+": loading mask not hidden once");
        Check(((UIChallengeRunMap)PageManager.instance.currentPage).ended==1,context+": entry effect not ended once");
        Check(BoxManager.reloginCount==1 && LanguageTool.lastCode==1047 && BoxManager.lastRelogin=="native-message-1047",
              context+": original relogin action missing");
        Check(Current.GetValue(null)==null,context+": current attempt remains");
    }
    static void AssertUntouched(string context)
    {
        Check(ManagerController.Instance.Enter && ManagerController.Instance.Receive,context+": native flags changed");
        Check(LoadingPanel.Instance.hides==0 && ((UIChallengeRunMap)PageManager.instance.currentPage).ended==0,
              context+": current scene loading/effect changed");
        Check(BoxManager.reloginCount==0,context+": relogin prompt appeared");
    }
    static void HttpWrapperAndMissingOwner()
    {
        Reset(); NativeCallback callback=new NativeCallback(); object request=new object();
        Check(AwakeStartRecovery.SendHttp(123,request,callback.Run),"Other HTTP request not passed through");
        Check(NetAPI.calls.Count==1 && NetAPI.calls[0].opcode==123 && Object.ReferenceEquals(NetAPI.calls[0].request,request),
              "Other HTTP request identity changed");
        Packet reply=Good("passthrough"); NetAPI.calls[0].callback(reply,true);
        Check(callback.calls==1 && Object.ReferenceEquals(callback.last,reply),"Passthrough callback changed");
        Check(!AwakeStartRecovery.BeforeStart(),"Other opcode created awake attempt");
        Check(AwakeStartRecovery.SendHttp(81009,request,callback.Run),"Non-awake 81009 body was not passed through");
        Check(NetAPI.calls.Count==2 && Object.ReferenceEquals(NetAPI.calls[1].request,request),"Non-awake body changed");
        FieldWire.current=null; int count=NetAPI.calls.Count;
        Check(!Send("monster-a",callback) && NetAPI.calls.Count==count,"Missing owner sent an awake request");
    }
    static void DuplicateStartBlocked()
    {
        Reset(); NativeCallback callback=new NativeCallback();
        Check(Send("monster-a",callback),"Awake request was not accepted");
        Check(NetAPI.calls.Count==1 && NetAPI.calls[0].opcode==81009 && NetAPI.calls[0].request is CSAwakeCopyFightBegin,
              "Original awake request was not sent exactly once");
        Check(AwakeStartRecovery.BeforeStart() && AwakeStartRecovery.BeforeStart(),"Pending start did not block duplicate");
        Check(NetAPI.calls.Count==1 && BoxManager.popupCount==2,"Duplicate guard sent a request or omitted feedback");
        AssertUntouched("duplicate guard");
    }
    static void TimeoutAndLateCallback()
    {
        Reset(); NativeCallback callback=new NativeCallback();
        long started=System.Diagnostics.Stopwatch.GetTimestamp();
        Check(Send("monster-a",callback),"Awake send failed");
        long finished=System.Diagnostics.Stopwatch.GetTimestamp(); object attempt=Current.GetValue(null);
        FieldInfo deadline=attempt.GetType().GetField("deadline",BindingFlags.Public|BindingFlags.Instance);
        Check(deadline!=null,"Awake deadline field unavailable");
        long actual=(long)deadline.GetValue(attempt), window=15L*System.Diagnostics.Stopwatch.Frequency;
        Check(actual>=started+window && actual<=finished+window,"Awake deadline is not the original 15-second window");
        HttpHandler stale=NetAPI.calls[0].callback;
        Sequence source=new Sequence(new object[] { "waiting", "unreachable" },0);
        IEnumerator guard=AwakeStartRecovery.Guard(source);
        Check(guard.MoveNext() && (string)guard.Current=="waiting","Original iterator did not begin");
        Expire(); Check(!guard.MoveNext(),"Timed-out iterator kept waiting");
        AssertCleaned("timeout");
        stale(Good("late-fight"),true); stale(Good("duplicate"),true);
        Check(callback.calls==0 && !AwakeStartRecovery.BeforeStart(),"Late callback resumed timed-out job");
        Check(LoadingPanel.Instance.hides==1 && BoxManager.reloginCount==1,"Late callback repeated timeout cleanup");

        ManagerController.Instance.SetFlags(true,true); LoadingPanel.Instance=new LoadingPanel();
        PageManager.instance.currentPage=new UIChallengeRunMap(); BoxManager.reloginCount=0;
        NativeCallback newer=new NativeCallback(); Check(Send("monster-a",newer),"New request after timeout failed");
        stale(Good("late-after-new"),true);
        Check(callback.calls==0 && newer.calls==0 && AwakeStartRecovery.BeforeStart(),"Late callback changed newer attempt");
        AssertUntouched("late callback against newer attempt");
    }
    static void CallbackDeadlineWithoutIterator()
    {
        Reset(); NativeCallback callback=new NativeCallback(); Check(Send("monster-a",callback),"Awake send failed");
        Expire(); NetAPI.calls[0].callback(Good("too-late"),true);
        Check(callback.calls==0,"Expired HTTP callback reached native code");
        AssertCleaned("expired HTTP callback");
        NetAPI.calls[0].callback(Good("duplicate"),true);
        Check(callback.calls==0 && LoadingPanel.Instance.hides==1,"Expired callback repeated native or cleanup");
    }
    static void FailurePaths()
    {
        for (int variant=0;variant<6;variant++)
        {
            Reset(); NativeCallback callback=new NativeCallback();
            if (variant==3) NetAPI.sendResult=false;
            if (variant==4) NetAPI.throwOnSend=true;
            if (variant==5) callback.throwOnCall=true;
            bool sent=Send("monster-a",callback);
            if (variant==3 || variant==4) Check(!sent,"Immediate transport failure was accepted");
            else
            {
                Check(sent,"Valid transport did not accept request");
                if (variant==0) NetAPI.calls[0].callback(Good("fight"),false);
                if (variant==1) NetAPI.calls[0].callback(null,true);
                if (variant==2) NetAPI.calls[0].callback(new Packet { kBody=new SCAwakeCopyFightBegin { fightId="" } },true);
                if (variant==5) NetAPI.calls[0].callback(Good("fight"),true);
            }
            Check(callback.calls==(variant==5 ? 1 : 0),"Failure invoked unexpected native callback");
            AssertCleaned("failure variant "+variant);
            NetAPI.calls[0].callback(Good("late"),true);
            Check(callback.calls==(variant==5 ? 1 : 0) && BoxManager.reloginCount==1,
                  "Failure variant allowed late callback or duplicate cleanup");
        }
        Reset(); NativeCallback wrong=new NativeCallback(); Check(Send("monster-a",wrong),"Awake send failed");
        NetAPI.calls[0].callback(new Packet { kBody=new object() },true);
        Check(wrong.calls==0,"Unexpected reply type reached native code"); AssertCleaned("wrong response type");
    }
    static void SuccessfulNativeAndIteratorOnce()
    {
        Reset(); ManagerController.Instance.SetFlags(true,false);
        NativeCallback callback=new NativeCallback { signalReceive=true };
        Check(Send("monster-a",callback),"Awake send failed");
        RoomBase destination=new RoomBase(); AwakeLoadSequence source=new AwakeLoadSequence(destination);
        IEnumerator guard=AwakeStartRecovery.Guard(source);
        Check(guard.MoveNext() && (int)guard.Current==1,"First original waiting yield changed");
        Check(guard.MoveNext() && (int)guard.Current==1 && source.switches==0,"Original waiting loop switched early");
        Packet reply=Good("fight-1"); NetAPI.calls[0].callback(reply,true); NetAPI.calls[0].callback(reply,true);
        Check(callback.calls==1 && callback.success && Object.ReferenceEquals(callback.last,reply),"Native callback was not exactly once");
        Check(!guard.MoveNext() && source.moves==3 && source.switches==1 && RoomManager.currentRoom==destination,
              "Original iterator did not switch and finish within one MoveNext");
        Check(Current.GetValue(null)==null && !AwakeStartRecovery.BeforeStart(),"Finished iterator left an attempt");
        NetAPI.calls[0].callback(reply,true);
        Check(callback.calls==1,"Completed native callback ran again"); AssertUntouched("successful iterator");
    }
    static void SuccessfulReplyExternalScopeChange()
    {
        for (int changed=0;changed<3;changed++)
        {
            Reset(); ManagerController.Instance.SetFlags(true,false);
            NativeCallback callback=new NativeCallback { signalReceive=true };
            Check(Send("monster-a",callback),"Awake send failed");
            AwakeLoadSequence source=new AwakeLoadSequence(new RoomBase());
            IEnumerator guard=AwakeStartRecovery.Guard(source);
            Check(guard.MoveNext() && (int)guard.Current==1,"Original waiting iterator did not start");
            HttpHandler stale=NetAPI.calls[0].callback; stale(Good("fight"),true);
            Check(callback.calls==1 && ManagerController.Instance.Receive,"Native success callback was not delivered");
            object battle=StaticData.curBattleData;
            if (changed==0 || changed==2) RoomManager.currentRoom=new RoomBase();
            if (changed==1 || changed==2) StaticData.awakeCopyFightMonGuid="monster-b";
            ManagerController.Instance=new ManagerController(); ManagerController.Instance.SetFlags(true,true);
            LoadingPanel.Instance=new LoadingPanel(); PageManager.instance.currentPage=new UIChallengeRunMap();
            stale(Good("duplicate"),true);
            Check(callback.calls==1,"Old success callback repeated after external scope change");
            Check(Object.ReferenceEquals(battle,StaticData.curBattleData),"Battle identity changed in scope test");
            Check(!guard.MoveNext(),"Old guard continued after external scope change");
            Check(source.moves==1 && source.switches==0,"Old original iterator ran after external scope change");
            AssertUntouched("successful callback with external scope change "+changed);
            Check(Current.GetValue(null)==null && !AwakeStartRecovery.BeforeStart(),"Old job survived external scope change");
        }
    }
    static void IteratorResourceFailure()
    {
        for (int afterReply=0;afterReply<2;afterReply++)
        {
            Reset(); NativeCallback callback=new NativeCallback(); Check(Send("monster-a",callback),"Awake send failed");
            Sequence source=new Sequence(new object[] { "first", "second" },2);
            IEnumerator guard=AwakeStartRecovery.Guard(source);
            Check(guard.MoveNext() && (string)guard.Current=="first","Iterator did not yield before resource failure");
            if (afterReply==1) NetAPI.calls[0].callback(Good("fight"),true);
            Check(!guard.MoveNext(),"Iterator resource exception left guard running");
            AssertCleaned("iterator failure");
            NetAPI.calls[0].callback(Good("late"),true);
            Check(callback.calls==afterReply,"Iterator failure allowed a late callback");
        }
    }
    static void PlaybackWithoutHttp()
    {
        Reset(); Check(!AwakeStartRecovery.BeforeStart(),"Unexpected initial attempt");
        object first=new object(),second=new object(); Sequence source=new Sequence(new object[] { first,second },0);
        IEnumerator guard=AwakeStartRecovery.Guard(source);
        Check(guard.MoveNext() && Object.ReferenceEquals(guard.Current,first),"Playback first yield changed");
        Check(guard.MoveNext() && Object.ReferenceEquals(guard.Current,second),"Playback second yield changed");
        Check(!guard.MoveNext() && source.moves==3 && NetAPI.calls.Count==0,"Playback iterator was incomplete or sent HTTP");
        AssertUntouched("playback");
    }
    static void StaleScopesCannotClearNewRequest()
    {
        for (int changed=0;changed<6;changed++)
        {
            Reset(); NativeCallback old=new NativeCallback(); Check(Send("monster-a",old),"Old request failed");
            HttpHandler stale=NetAPI.calls[0].callback;
            IEnumerator oldGuard=AwakeStartRecovery.Guard(new Sequence(new object[] { "old-1", "old-2" },0));
            Check(oldGuard.MoveNext(),"Old iterator did not start");
            if (changed<3) FieldWire.current=new FieldWire.Session { playerId=changed==0 ? 43 : 42,
                sessionHex=changed==1 ? "other-session" : "session-42",origin=changed==2 ? "other-origin" : "origin-1" };
            if (changed==3) RoomManager.currentRoom=new RoomBase();
            if (changed==4) StaticData.curBattleData=new object();
            if (changed==5) StaticData.awakeCopyFightMonGuid="monster-b";
            stale(Good("old"),true);
            Check(old.calls==0,"Old callback crossed changed scope"); AssertUntouched("stale callback scope "+changed);
            Check(!AwakeStartRecovery.BeforeStart(),"Changed scope blocked a new request");
            NativeCallback newer=new NativeCallback(); Check(Send(StaticData.awakeCopyFightMonGuid,newer),"New scoped request failed");
            Check(!oldGuard.MoveNext(),"Old scoped iterator continued after new request");
            stale(Good("old-again"),true);
            Check(old.calls==0 && newer.calls==0,"Old callback invoked a native handler after replacement");
            AssertUntouched("old guard against new scope "+changed);
            Check(AwakeStartRecovery.BeforeStart(),"Old scope cleared the newer request");
            NetAPI.calls[1].callback(Good("new"),true);
            Check(newer.calls==1,"New scoped callback did not reach native code");
            IEnumerator newGuard=AwakeStartRecovery.Guard(new Sequence(new object[] { "new" },0));
            Check(newGuard.MoveNext() && (string)newGuard.Current=="new" && !newGuard.MoveNext(),
                  "New scoped iterator did not complete");
        }
    }
    public static void Main()
    {
        Run("http_wrapper_and_missing_owner",HttpWrapperAndMissingOwner);
        Run("duplicate_start_blocked",DuplicateStartBlocked);
        Run("timeout_and_late_callback",TimeoutAndLateCallback);
        Run("callback_deadline_without_iterator",CallbackDeadlineWithoutIterator);
        Run("failure_paths",FailurePaths);
        Run("successful_native_and_iterator_once",SuccessfulNativeAndIteratorOnce);
        Run("successful_reply_external_scope_change",SuccessfulReplyExternalScopeChange);
        Run("iterator_resource_failure",IteratorResourceFailure);
        Run("playback_without_http",PlaybackWithoutHttp);
        Run("stale_scopes_cannot_clear_new_request",StaleScopesCannotClearNewRequest);
        Console.WriteLine("PASS\t"+checks);
    }
}
