using System;
using System.Collections.Generic;
using System.Reflection;
using HYJS.DesertV4;
using LitJson;
using UnityEngine;

// Exercise the real coordinator and journal store with deterministic transport
// callbacks. The stubbed view never grants inventory; it records receipt display.
namespace LitJson
{
    public sealed class JsonData
    {
        readonly Dictionary<string,JsonData> fields=new Dictionary<string,JsonData>();
        readonly string value;
        public JsonData() { }
        public JsonData(string text) { value=text; }
        public JsonData this[string key] { get { JsonData found; return fields.TryGetValue(key,out found) ? found : null; } set { fields[key]=value; } }
        public override string ToString() { return value; }
    }
}
namespace UnityEngine
{
    public struct Vector3
    {
        public float x,y,z;
        public static Vector3 operator -(Vector3 a,Vector3 b) { return new Vector3 { x=a.x-b.x,y=a.y-b.y,z=a.z-b.z }; }
        public float sqrMagnitude { get { return x*x+y*y+z*z; } }
    }
    public sealed class Transform { public Vector3 position; }
    public static class Time { public static int frameCount; }
    public static class PlayerPrefs
    {
        static readonly Dictionary<string,string> values=new Dictionary<string,string>();
        public static string GetString(string key,string fallback) { string value; return values.TryGetValue(key,out value) ? value : fallback; }
        public static void SetString(string key,string value) { values[key]=value; }
        public static void DeleteKey(string key) { values.Remove(key); }
        public static void Save() { }
        public static void Reset() { values.Clear(); }
    }
    public static class JsonUtility
    {
        static int serial;
        static readonly Dictionary<string,GatherJournal> copies=new Dictionary<string,GatherJournal>();
        static GatherJournal Copy(GatherJournal j)
        {
            return new GatherJournal { startRequestId=j.startRequestId,cancelRequestId=j.cancelRequestId,claimRequestId=j.claimRequestId,
                pointId=j.pointId,attemptToken=j.attemptToken,generation=j.generation,acceptedAtMs=j.acceptedAtMs,
                readyAtMs=j.readyAtMs,fatigueCost=j.fatigueCost,phase=j.phase };
        }
        public static string ToJson(GatherJournal journal) { string id=(++serial).ToString(); copies[id]=Copy(journal); return id; }
        public static T FromJson<T>(string raw) { return (T)(object)Copy(copies[raw]); }
    }
}
namespace Room { public sealed class RoomCharactor { public Transform transform=new Transform(); } }
namespace HYJS.DesertV4
{
    public sealed class FieldWireResult { public bool uncertain; public long code; public string message; public JsonData data; }
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public static Session current;
        public static bool Current(out Session s) { s=current; return s!=null; }
        public Action<FieldWireResult> start,claim,cancel,status;
        public string lastClaimId,lastStatusId;
        public int claims,statuses;
        public void StartGather(Session s,GatherJournal j,Action<FieldWireResult> callback) { start=callback; }
        public void CancelGather(Session s,GatherJournal j,Action<FieldWireResult> callback) { cancel=callback; }
        public void ClaimGather(Session s,GatherJournal j,Action<FieldWireResult> callback) { claim=callback; lastClaimId=j.claimRequestId; claims++; }
        public void GatherStatus(Session s,string id,Action<FieldWireResult> callback) { status=callback; lastStatusId=id; statuses++; }
    }
    public sealed class FieldResources
    {
        public sealed class Point { public string id,groupId,kind; public int remaining; public long generation; public Vector3 standPosition; public float radius; }
        public Point point=new Point { id="O01",groupId="ore",kind="mine",remaining=1,generation=8,radius=2 };
        public Point FindByNativePosId(string id) { return id=="ggo_v4_O01" ? point : null; }
    }
    public sealed class GatherPreload { public bool Failed; public bool Ready(string kind) { return true; } }
    public sealed class NativeGatherUI
    {
        public Action complete; public bool animating;
        public bool Begin(FieldResources.Point p,GatherJournal j,Action callback) { complete=callback; animating=true; return true; }
        public bool IsAnimating() { return animating; }
        public void Stop() { animating=false; }
        public void CompleteFromNative() { animating=false; }
    }
}
public static class GatherCoordinatorChecks
{
    sealed class View : IGatherView
    {
        public int applied,snapshots,retry;
        public void Notice(string text) { }
        public void AskToReleaseProtection() { }
        public void ApplyOriginalReceipt(JsonData receipt) { applied++; }
        public void RequestSnapshot() { snapshots++; }
        public void AskToRetryDelivery() { retry++; }
    }
    sealed class Harness
    {
        public FieldWire wire=new FieldWire();
        public FieldWire.Session session=new FieldWire.Session { playerId=69,sessionHex="session",origin="https://server" };
        public FieldResources resources=new FieldResources();
        public Room.RoomCharactor actor=new Room.RoomCharactor();
        public GatherPreload preload=new GatherPreload();
        public NativeGatherUI native=new NativeGatherUI();
        public View view=new View();
        public GatherCoordinator coordinator;
        public Harness()
        {
            PlayerPrefs.Reset(); FieldWire.current=session; Time.frameCount=1;
            coordinator=NewCoordinator();
        }
        public GatherCoordinator NewCoordinator() { return new GatherCoordinator(wire,session,resources,actor,preload,native,view); }
        public GatherAttempt Attempt()
        { return (GatherAttempt)typeof(GatherCoordinator).GetField("attempt",BindingFlags.NonPublic|BindingFlags.Instance).GetValue(coordinator); }
        public void TickNow()
        { typeof(GatherCoordinator).GetField("nextRetry",BindingFlags.NonPublic|BindingFlags.Instance).SetValue(coordinator,(long)0); coordinator.Tick(); }
        public void StartClaim()
        {
            coordinator.Trigger("ore","ggo_v4_O01");
            GatherJournal j=Attempt().Journal;
            wire.start(Ok(Obj("requestId",j.startRequestId,"pointId",j.pointId,"generation","8","attemptToken","token-69",
                "acceptedAtMs","5000","readyAtMs","8200","fatigueCost","4")));
            Time.frameCount++; coordinator.Tick(); native.complete(); coordinator.Tick();
            Check(Attempt().Journal.phase==GatherPhase.Claiming && wire.claim!=null,"setup reaches claim");
        }
        public void BeginCheck()
        {
            wire.claim(new FieldWireResult { code=7206 });
            Check(Attempt().Journal.phase==GatherPhase.CheckingClaim,"7206 enters checking");
            Check(GatherJournalStore.Load(session).Journal.phase==GatherPhase.CheckingClaim,"checking persisted");
        }
        public void SendStatus() { TickNow(); Check(wire.status!=null,"status requested"); }
        public GatherJournal Stored()
        {
            string key="hyjs.ggo.gather.v4."+Uri.EscapeDataString(session.origin)+"."+session.playerId;
            string raw=PlayerPrefs.GetString(key,"");
            return raw=="" ? null : JsonUtility.FromJson<GatherJournal>(raw);
        }
    }
    static void Check(bool ok,string name) { if (!ok) throw new Exception(name); }
    static JsonData Obj(params string[] fields)
    { JsonData obj=new JsonData(); for (int i=0;i<fields.Length;i+=2) obj[fields[i]]=new JsonData(fields[i+1]); return obj; }
    static FieldWireResult Ok(JsonData data) { return new FieldWireResult { data=data }; }
    static JsonData Status(string status,string start,string token) { return Obj("status",status,"startRequestId",start,"attemptToken",token); }
    static JsonData Receipt(string request,string start,string token) { return Obj("requestId",request,"startRequestId",start,"attemptToken",token); }
    static void Main()
    {
        Check((int)GatherPhase.Starting==0 && (int)GatherPhase.PendingDelivery==6 && (int)GatherPhase.CheckingClaim==7,"persisted enum values remain stable");
        Harness h=new Harness(); h.StartClaim();
        string start=h.Attempt().Journal.startRequestId,claim=h.Attempt().Journal.claimRequestId;
        Action<FieldWireResult> oldClaim=h.wire.claim,oldStart=h.wire.start;
        h.BeginCheck();
        oldStart(Ok(Obj("requestId",start,"pointId","O01","generation","8","attemptToken","token-69","acceptedAtMs","5000","readyAtMs","8200","fatigueCost","4")));
        Check(h.Attempt().Journal.phase==GatherPhase.CheckingClaim,"late start does not replace checking");
        h.coordinator.Suspend(); h.coordinator=h.NewCoordinator();
        Check(h.Attempt().Journal.phase==GatherPhase.CheckingClaim && h.Attempt().Journal.claimRequestId==claim,"cold recovery preserves checking identity");
        h.SendStatus(); h.wire.status(Ok(Status("cancelled",Guid.NewGuid().ToString("D"),"token-69")));
        h.SendStatus(); h.wire.status(Ok(Status("cancelled",start,"wrong-token")));
        h.SendStatus(); h.wire.status(Ok(Status("cancelled",start,"")));
        h.SendStatus(); h.wire.status(Ok(Status("unknown",start,"token-69")));
        h.SendStatus(); h.wire.status(new FieldWireResult { uncertain=true });
        h.SendStatus(); h.wire.status(new FieldWireResult { code=7002 });
        h.SendStatus(); JsonData bad=Status("settled",start,"token-69"); bad["claimReceipt"]=Receipt(Guid.NewGuid().ToString("D"),start,"token-69"); h.wire.status(Ok(bad));
        Check(h.Attempt().Journal.phase==GatherPhase.CheckingClaim && h.view.applied==0 && h.wire.claims==1,"unknown and mismatched results stay blocked without rewards");
        h.SendStatus(); Action<FieldWireResult> oldStatus=h.wire.status; oldStatus(Ok(Status("cancelled",start,"token-69")));
        Check(h.Attempt()==null && GatherJournalStore.Load(h.session)==null && h.view.applied==0,"confirmed cancellation clears journal without reward");
        h.coordinator.Trigger("ore","ggo_v4_O01"); string next=h.Attempt().Journal.startRequestId;
        oldClaim(Ok(Receipt(claim,start,"token-69"))); oldStatus(Ok(Status("cancelled",start,"token-69")));
        Check(h.Attempt().Journal.startRequestId==next && h.Stored().startRequestId==next && h.view.applied==0,"late callbacks do not replace a newer journal");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId; claim=h.Attempt().Journal.claimRequestId; h.BeginCheck();
        h.SendStatus(); h.wire.status(Ok(Status("pending",start,"token-69"))); h.TickNow();
        Check(h.Attempt().Journal.phase==GatherPhase.Claiming && h.wire.claims==2 && h.wire.lastClaimId==claim,"pending resends the same claim UUID");
        h.wire.claim(Ok(Receipt(claim,start,"token-69")));
        Check(h.Attempt()==null && h.view.applied==1,"original claim receipt is applied once");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId; claim=h.Attempt().Journal.claimRequestId;
        oldClaim=h.wire.claim; h.BeginCheck(); h.SendStatus(); oldStatus=h.wire.status;
        oldStatus(Ok(Status("cancelled",start,"token-69")));
        h.coordinator.Trigger("ore","ggo_v4_O01"); next=h.Attempt().Journal.startRequestId;
        oldClaim(Ok(Receipt(claim,start,"token-69"))); oldStatus(Ok(Status("cancelled",start,"token-69")));
        Check(h.Attempt().Journal.startRequestId==next && h.Stored().startRequestId==next && h.view.applied==0,"same coordinator ignores late callbacks after cancellation");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId; claim=h.Attempt().Journal.claimRequestId; h.BeginCheck(); h.SendStatus();
        oldStatus=h.wire.status; oldStatus(Ok(Status("pending",start,"token-69")));
        JsonData late=Status("settled",start,"token-69"); late["claimReceipt"]=Receipt(Guid.NewGuid().ToString("D"),start,"token-69");
        oldStatus(Ok(late));
        Check(h.Attempt().Journal.phase==GatherPhase.Claiming && h.view.applied==0,"late status cannot display a receipt in claim phase");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId; h.BeginCheck(); h.SendStatus();
        JsonData settled=Status("settled",start,"token-69"); settled["claimReceipt"]=Receipt(GatherAttempt.SystemClaimReceiptId("token-69"),start,"token-69");
        h.wire.status(Ok(settled));
        Check(h.Attempt()==null && h.view.applied==1,"matching system settlement is applied once");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId; claim=h.Attempt().Journal.claimRequestId; h.BeginCheck(); h.SendStatus();
        h.wire.status(Ok(Status("pending_delivery",start,"token-69"))); h.coordinator.Trigger("ore","ggo_v4_O01");
        Check(h.Attempt().Journal.phase==GatherPhase.PendingDelivery && h.view.retry==1,"pending delivery remains user controlled");
        h.coordinator.RetryPendingDelivery(); h.TickNow();
        Check(h.wire.lastClaimId==claim && h.wire.claims==2,"delivery retry preserves claim UUID");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId;
        h.Attempt().Journal.phase=GatherPhase.RecoverReceipt; GatherJournalStore.Save(h.session,h.Attempt()); h.SendStatus();
        JsonData wrongOld=Status("settled",start,"wrong-token"); wrongOld["claimReceipt"]=Receipt(Guid.NewGuid().ToString("D"),start,"wrong-token");
        h.wire.status(Ok(wrongOld));
        Check(h.Attempt().Journal.phase==GatherPhase.RecoverReceipt && h.view.applied==0,"old journal rejects different token before displaying receipt");

        h=new Harness(); h.StartClaim(); start=h.Attempt().Journal.startRequestId;
        h.Attempt().Journal.phase=GatherPhase.RecoverReceipt; h.Attempt().Journal.attemptToken=""; GatherJournalStore.Save(h.session,h.Attempt());
        h.coordinator.Suspend(); h.coordinator=h.NewCoordinator(); h.SendStatus();
        JsonData legacy=Status("settled",start,"legacy-token"); legacy["claimReceipt"]=Receipt(Guid.NewGuid().ToString("D"),start,"legacy-token");
        h.wire.status(Ok(legacy));
        Check(h.Attempt()==null && h.view.applied==1,"legacy settled journal without token still recovers");

        h=new Harness(); h.StartClaim(); h.wire.claim(new FieldWireResult { code=7002 });
        Check(h.Attempt().Journal.phase==GatherPhase.Claiming && h.wire.statuses==0,"other errors retain claim retry semantics");
        Console.WriteLine("GatherCoordinator checks passed");
    }
}
