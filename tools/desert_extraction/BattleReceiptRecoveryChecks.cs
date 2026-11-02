// Execute real battle receipt reconciliation and real journal validation with isolated stubs.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using HYJS.DesertV4;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace UnityEngine
{
    public sealed class Coroutine { }
    public class MonoBehaviour
    {
        protected static void DontDestroyOnLoad(object value) { }
        protected Coroutine StartCoroutine(IEnumerator value) { return new Coroutine(); }
    }
    public sealed class GameObject
    {
        public GameObject(string name) { }
        public T AddComponent<T>() where T:new() { return new T(); }
    }
    public sealed class WaitForSeconds { public WaitForSeconds(float value) { } }
    public struct Vector3
    {
        public float x,y,z;
        public Vector3(float a,float b,float c) { x=a; y=b; z=c; }
        public static Vector3 operator -(Vector3 a,Vector3 b) { return new Vector3(a.x-b.x,a.y-b.y,a.z-b.z); }
        public float sqrMagnitude { get { return x*x+y*y+z*z; } }
    }
    public static class Debug
    {
        public static void Log(string value) { }
        public static void LogWarning(string value) { }
    }
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
        static readonly System.Text.Json.JsonSerializerOptions options=new System.Text.Json.JsonSerializerOptions { IncludeFields=true };
        public static string ToJson(object value) { return System.Text.Json.JsonSerializer.Serialize(value,value.GetType(),options); }
        public static T FromJson<T>(string value) { return System.Text.Json.JsonSerializer.Deserialize<T>(value,options); }
    }
}
namespace LitJson
{
    public sealed class JsonData : Dictionary<string,JsonData>
    {
        readonly object scalar;
        public JsonData() { }
        public JsonData(object value) { scalar=value; }
        public bool IsBoolean { get { return scalar is bool; } }
        public static explicit operator bool(JsonData value) { return (bool)value.scalar; }
        public static explicit operator string(JsonData value) { return (string)value.scalar; }
        public override string ToString() { return scalar==null ? "" : scalar.ToString(); }
    }
}
namespace ProtoBuf.Message
{
    public sealed class SCSocketLogin { public int result; }
    public sealed class SCSidePveFightStart { public int code; public string fightId; }
    public sealed class SCSidePveBossFightStart { public int code; public string fightId; }
    public sealed class CSSidePveFightEnd { public string fightId,payload; }
    public sealed class CSSidePveBossFightEnd { public string fightId,payload,uuid; }
    public sealed class SCSidePveFightEnd { public int code; }
    public sealed class SCSidePveBossFightEnd { public int code; }
    public static class PBConvert
    {
        public static byte[] toBytes(int opcode,object value)
        {
            if (opcode==30049 && value is CSSidePveFightEnd)
            { CSSidePveFightEnd v=(CSSidePveFightEnd)value; return Encoding.UTF8.GetBytes(v.fightId+"|"+v.payload); }
            if (opcode==30085 && value is CSSidePveBossFightEnd)
            { CSSidePveBossFightEnd v=(CSSidePveBossFightEnd)value; return Encoding.UTF8.GetBytes(v.fightId+"|"+v.payload+"|"+v.uuid); }
            return null;
        }
        public static object toObject(int opcode,byte[] value)
        {
            string text=Encoding.UTF8.GetString(value); string[] parts=text.Split('|');
            if (opcode==30049 && parts.Length==2) return new CSSidePveFightEnd { fightId=parts[0],payload=parts[1] };
            if (opcode==30085 && parts.Length==3) return new CSSidePveBossFightEnd { fightId=parts[0],payload=parts[1],uuid=parts[2] };
            if (opcode==30050 && text=="native-receipt") return new SCSidePveFightEnd { code=1 };
            if (opcode==30086 && text=="native-receipt") return new SCSidePveBossFightEnd { code=1 };
            return null;
        }
    }
}
namespace Battle
{
    public sealed class BattleManager
    {
        public static BattleManager Instance;
        public bool enabled; public object battleEngine;
    }
    public sealed class FieldPVEBattleOver
    {
        public int calls; public SCSidePveFightEnd last;
        void FieldPVEFightResultHandler(SCSidePveFightEnd value)
        {
            if (BattleRecovery.InterceptEnd(6,this,value)) throw new Exception("Native delivery was intercepted recursively");
            calls++; last=value;
        }
    }
    public sealed class FieldPVEBossBattleOver
    {
        public int calls; public SCSidePveBossFightEnd last;
        void FieldPVEBossFightResultHandler(SCSidePveBossFightEnd value)
        {
            if (BattleRecovery.InterceptEnd(12,this,value)) throw new Exception("Native delivery was intercepted recursively");
            calls++; last=value;
        }
    }
}
namespace Room
{
    public class RoomBase { public object netManager; public CharactorManager charactorManager; }
    public sealed class RoomField : RoomBase { }
    public sealed class RoomNetField { }
    public sealed class RoomCharactor { }
    public sealed class CharactorManager { public RoomCharactor GetSelfCharactor() { return null; } }
    public static class RoomManager { public static RoomBase currentRoom; }
}
namespace UI
{
    public sealed class UIFieldRunMap { }
    public sealed class PageManager { public static PageManager instance; public object currentPage; }
}
public sealed class BattleData { public int battleType; public string fightId; }
public static class StaticData { public static string fieldId; public static BattleData curBattleData; }
public sealed class ResultObject { }
public static class BoxManager
{
    public static int notices;
    public static void CreatePopupTextBox(string text,float delay) { notices++; }
}
public static class NetAPI
{
    public static bool SocketIsConnected() { return true; }
    public static void SendSocket(int opcode,object value) { throw new Exception("Unexpected socket resend"); }
}
namespace HYJS.DesertV4
{
    public sealed class FieldWireResult { public bool uncertain; public long code; public JsonData data; }
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public sealed class Call { public Session owner; public int type; public string fight; public Action<FieldWireResult> callback; }
        public static Session current;
        public static readonly List<Call> calls=new List<Call>();
        public static bool Current(out Session value) { value=current; return value!=null; }
        public void FightReceipt(Session owner,int type,string fight,Action<FieldWireResult> callback)
        { calls.Add(new Call { owner=owner,type=type,fight=fight,callback=callback }); }
        public void FightActive(Session owner,int type,Action<FieldWireResult> callback) { throw new Exception("Unexpected active query"); }
        public void PositionStatus(Session owner,Action<FieldWireResult> callback) { throw new Exception("Unexpected position query"); }
    }
    public static class ColdBattleRecovery { public static bool Pending { get { return false; } } }
    public static class PositionSync
    {
        public static int ConnectionEpoch;
        public static void ReleaseIfOwned(long value) { }
        public static bool OwnsLock(long value) { return false; }
        public static bool LockAfterTeleport(RoomCharactor actor,out long value) { value=0; return false; }
        public static void RequestCurrent(object net,Action<Vector3> callback) { }
    }
    public sealed class FieldRuntime
    {
        public static int snapshots;
        public static FieldRuntime Require() { return new FieldRuntime(); }
        public void RequestSnapshot() { snapshots++; }
    }
    public sealed class MonsterRewardView
    {
        public static MonsterRewardView FromReceipt(string fight,JsonData data) { return new MonsterRewardView(); }
        public ResultObject Project(string fight,ResultObject original) { return original; }
    }
    public static class TradePanel { public static void RegisterIcons() { } }
}

public static class BattleReceiptRecoveryChecks
{
    const string Fight="1234567890abcdef12345678";
    const string OtherFight="1234567890abcdef12345679";
    static readonly string InputHash=new string('a',64);
    static readonly string OtherHash=new string('b',64);
    static readonly FieldInfo Singleton=typeof(BattleRecovery).GetField("singleton",BindingFlags.Static|BindingFlags.NonPublic);
    static readonly FieldInfo Input=typeof(BattleRecovery).GetField("inputHash",BindingFlags.Instance|BindingFlags.NonPublic);
    static readonly MethodInfo Update=typeof(BattleRecovery).GetMethod("Update",BindingFlags.Instance|BindingFlags.NonPublic);
    static int checks,failures;
    sealed class Case
    {
        public int type; public object owner,message; public FightEndRecord record; public FieldWire.Session session;
        public int Calls { get { return type==6 ? ((Battle.FieldPVEBattleOver)owner).calls : ((Battle.FieldPVEBossBattleOver)owner).calls; } }
    }
    static void Check(bool value,string message) { checks++; if (!value) throw new Exception(message); }
    static void Run(string name,Action test)
    {
        int before=checks;
        try { test(); Console.WriteLine("CASE\t"+name+"\tPASS\t"+(checks-before)); }
        catch (Exception error) { failures++; Console.WriteLine("CASE\t"+name+"\tFAIL\t"+error.Message); }
    }
    static void Reset()
    {
        Check(Singleton!=null && Input!=null && Update!=null,"Private recovery hooks unavailable");
        Singleton.SetValue(null,null); PlayerPrefs.Reset(); FieldWire.calls.Clear(); FieldRuntime.snapshots=0; BoxManager.notices=0;
        FieldWire.current=new FieldWire.Session { playerId=2,sessionHex="session-a",origin="local-qa" };
        StaticData.fieldId="field_003"; StaticData.curBattleData=null;
        RoomManager.currentRoom=new RoomField();
        Battle.BattleManager.Instance=new Battle.BattleManager { enabled=true,battleEngine=new object() };
        UI.PageManager.instance=new UI.PageManager { currentPage=new UI.UIFieldRunMap() };
    }
    static Case Setup(int type)
    {
        Reset(); Case c=new Case { type=type,session=FieldWire.current };
        StaticData.curBattleData=new BattleData { battleType=type,fightId=Fight };
        object start=type==6 ? (object)new SCSidePveFightStart { code=1,fightId=Fight } : new SCSidePveBossFightStart { code=1,fightId=Fight };
        BattleRecovery.StartResponse(type,start);
        object core=Singleton.GetValue(null); Check(core!=null,"Recovery did not capture start");
        Input.SetValue(core,InputHash);
        c.owner=type==6 ? (object)new Battle.FieldPVEBattleOver() : new Battle.FieldPVEBossBattleOver();
        c.message=type==6 ? (object)new CSSidePveFightEnd { fightId=Fight,payload="original-end" } :
            new CSSidePveBossFightEnd { fightId=Fight,payload="original-end",uuid="event-1" };
        int opcode=type==6 ? 30049 : 30085;
        BattleRecovery.SendEnd(opcode,c.message,c.owner);
        c.record=FightEndJournal.Load(c.session);
        Check(c.record!=null && c.record.opcode==opcode && c.record.fightId==Fight && c.record.inputHash==InputHash,
              "Real journal did not persist exact fight and input hash");
        Check(c.record.payloadBase64==Convert.ToBase64String(PBConvert.toBytes(opcode,c.message)) && c.record.payloadHash.Length==64,
              "Real journal did not preserve and hash exact end payload");
        object restored=FightEndJournal.Message(c.session,c.record);
        Check(Convert.ToBase64String(PBConvert.toBytes(opcode,restored))==c.record.payloadBase64,
              "Real journal message did not reproduce payload");
        return c;
    }
    static void Query(Case c)
    {
        Update.Invoke(Singleton.GetValue(null),null);
        Check(FieldWire.calls.Count==1 && FieldWire.calls[0].type==c.type && FieldWire.calls[0].fight==Fight,
              "Receipt query identity differs");
    }
    static FieldWireResult Receipt(Case c,string hash,string endHash)
    {
        JsonData data=new JsonData();
        data["status"]=new JsonData("settled"); data["fightId"]=new JsonData(Fight);
        data["inputHash"]=new JsonData(hash); data["endRequestHash"]=new JsonData(endHash);
        data["nativeEnd"]=new JsonData(Convert.ToBase64String(Encoding.UTF8.GetBytes("native-receipt")));
        return new FieldWireResult { data=data };
    }
    static void MatchedReceipt(int type,bool engineRunning)
    {
        Case c=Setup(type);
        Check(BattleRecovery.LivePresent && BattleRecovery.PreserveBattle(),"Running engine lost strict live guard");
        Battle.BattleManager.Instance.enabled=engineRunning;
        Battle.BattleManager.Instance.battleEngine=engineRunning ? new object() : null;
        Check(BattleRecovery.LivePresent==engineRunning && BattleRecovery.PreserveBattle()==engineRunning,
              "Strict live guard did not follow engine state");
        Query(c); FieldWireResult receipt=Receipt(c,InputHash,c.record.payloadHash);
        FieldWire.calls[0].callback(receipt);
        Check(c.Calls==1,(engineRunning ? "running-engine" : "disabled-engine")+" original result handler not called exactly once");
        Check(FightEndJournal.Load(c.session)==null,"Confirmed receipt did not clear real journal");
        Check(FieldRuntime.snapshots==1,"Confirmed receipt did not request one snapshot");
        FieldWire.calls[0].callback(receipt);
        Check(c.Calls==1 && FieldRuntime.snapshots==1,"Repeated receipt delivered native result twice");
    }
    static void IdentityChanged(int type,int changed,bool engineRunning)
    {
        Case c=Setup(type);
        Battle.BattleManager.Instance.enabled=engineRunning;
        Battle.BattleManager.Instance.battleEngine=engineRunning ? new object() : null;
        Query(c);
        if (changed==0) RoomManager.currentRoom=new RoomField();
        if (changed==1) StaticData.curBattleData=new BattleData { battleType=type,fightId=Fight };
        if (changed==2) StaticData.curBattleData.battleType=type==6 ? 12 : 6;
        if (changed==3) StaticData.curBattleData.fightId=OtherFight;
        FieldWire.calls[0].callback(Receipt(c,InputHash,c.record.payloadHash));
        Check(c.Calls==0,"changed identity delivered receipt to old native owner");
        if (engineRunning && changed<2)
        {
            FightEndRecord still=FightEndJournal.Load(c.session);
            Check(still!=null && still.payloadHash==c.record.payloadHash,
                  "running engine with changed room or battle cleared the original journal");
            Check(BoxManager.notices>0,"running engine with changed room or battle omitted warning");
        }
    }
    static void BadHashes(int type,bool engineRunning)
    {
        for (int changed=0;changed<2;changed++)
        {
            Case c=Setup(type); Battle.BattleManager.Instance.enabled=engineRunning;
            Battle.BattleManager.Instance.battleEngine=engineRunning ? new object() : null;
            Query(c);
            FieldWire.calls[0].callback(Receipt(c,changed==0 ? OtherHash : InputHash,
                                               changed==1 ? OtherHash : c.record.payloadHash));
            FightEndRecord still=FightEndJournal.Load(c.session);
            Check(c.Calls==0,"invalid receipt hash delivered native result");
            Check(still!=null && still.fightId==c.record.fightId && still.payloadHash==c.record.payloadHash &&
                  still.inputHash==c.record.inputHash && still.payloadBase64==c.record.payloadBase64,
                  "invalid receipt hash changed or cleared real journal");
        }
    }
    static void SessionChangeInvalidatesCallback()
    {
        Case c=Setup(12); Battle.BattleManager.Instance.enabled=false; Battle.BattleManager.Instance.battleEngine=null; Query(c);
        FieldWire.current=new FieldWire.Session { playerId=2,sessionHex="session-b",origin="local-qa" };
        Update.Invoke(Singleton.GetValue(null),null);
        FieldWire.calls[0].callback(Receipt(c,InputHash,c.record.payloadHash));
        Check(c.Calls==0 && FightEndJournal.Load(c.session)!=null,"Old session callback delivered or cleared journal");
    }
    public static int Main()
    {
        Run("boss_disabled_engine_receipt",delegate { MatchedReceipt(12,false); });
        Run("monster_disabled_engine_receipt",delegate { MatchedReceipt(6,false); });
        Run("boss_running_engine_receipt",delegate { MatchedReceipt(12,true); });
        Run("monster_running_engine_receipt",delegate { MatchedReceipt(6,true); });
        Run("boss_changed_room_disabled",delegate { IdentityChanged(12,0,false); });
        Run("boss_changed_battle_disabled",delegate { IdentityChanged(12,1,false); });
        Run("boss_changed_type_disabled",delegate { IdentityChanged(12,2,false); });
        Run("boss_changed_fight_disabled",delegate { IdentityChanged(12,3,false); });
        Run("monster_changed_room_disabled",delegate { IdentityChanged(6,0,false); });
        Run("monster_changed_battle_disabled",delegate { IdentityChanged(6,1,false); });
        Run("monster_changed_type_disabled",delegate { IdentityChanged(6,2,false); });
        Run("monster_changed_fight_disabled",delegate { IdentityChanged(6,3,false); });
        Run("boss_changed_room_running",delegate { IdentityChanged(12,0,true); });
        Run("boss_changed_battle_running",delegate { IdentityChanged(12,1,true); });
        Run("boss_changed_type_running",delegate { IdentityChanged(12,2,true); });
        Run("boss_changed_fight_running",delegate { IdentityChanged(12,3,true); });
        Run("monster_changed_room_running",delegate { IdentityChanged(6,0,true); });
        Run("monster_changed_battle_running",delegate { IdentityChanged(6,1,true); });
        Run("monster_changed_type_running",delegate { IdentityChanged(6,2,true); });
        Run("monster_changed_fight_running",delegate { IdentityChanged(6,3,true); });
        Run("boss_bad_hashes_disabled",delegate { BadHashes(12,false); });
        Run("monster_bad_hashes_disabled",delegate { BadHashes(6,false); });
        Run("boss_bad_hashes_running",delegate { BadHashes(12,true); });
        Run("monster_bad_hashes_running",delegate { BadHashes(6,true); });
        Run("session_change_invalidates_callback",SessionChangeInvalidatesCallback);
        Console.WriteLine("TOTAL\t"+checks+"\t"+failures);
        return failures==0 ? 0 : 1;
    }
}
