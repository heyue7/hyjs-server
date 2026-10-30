using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Reflection;
using System.Reflection.Metadata;
using System.Reflection.PortableExecutable;
using System.Security.Cryptography;
using System.Text;
using LitJson;

// Executes the actual PvpBridge source with the game's LitJson.dll. Unity,
// transport and protobuf are controlled fakes; no external state is touched.
namespace UnityEngine
{
    public class Coroutine { }
    public class MonoBehaviour
    {
        protected static void DontDestroyOnLoad(object value) { }
        protected Coroutine StartCoroutine(IEnumerator value) { return new Coroutine(); }
    }
    public class GameObject
    {
        public GameObject(string name) { }
        public T AddComponent<T>() where T : new() { return new T(); }
    }
    public class WWW : IDisposable
    {
        public WWW(string url,byte[] body,Dictionary<string,string> headers) { }
        public bool isDone { get { return true; } }
        public void Dispose() { }
    }
    public static class PlayerPrefs
    {
        static readonly Dictionary<string,string> values=new Dictionary<string,string>();
        public static string GetString(string key,string fallback) { string value; return values.TryGetValue(key,out value) ? value : fallback; }
        public static void SetString(string key,string value) { values[key]=value; }
        public static void DeleteKey(string key) { values.Remove(key); }
        public static bool HasKey(string key) { return values.ContainsKey(key); }
        public static void Save() { }
        public static void Reset() { values.Clear(); }
    }
    public static class JsonUtility
    {
        public static string ToJson(object value)
        {
            var node=new JsonData();
            foreach (FieldInfo f in value.GetType().GetFields())
            {
                if (f.FieldType==typeof(int)) node[f.Name]=(int)f.GetValue(value);
                else if (f.FieldType==typeof(long)) node[f.Name]=(long)f.GetValue(value);
                else node[f.Name]=(string)f.GetValue(value);
            }
            return JsonMapper.ToJson(node);
        }
        public static T FromJson<T>(string text)
        {
            var value=(T)Activator.CreateInstance(typeof(T),true); JsonData node=JsonMapper.ToObject(text);
            foreach (FieldInfo f in typeof(T).GetFields())
            {
                object v=f.FieldType==typeof(int) ? (object)Int32.Parse(node[f.Name].ToString(),CultureInfo.InvariantCulture) :
                    f.FieldType==typeof(long) ? (object)Int64.Parse(node[f.Name].ToString(),CultureInfo.InvariantCulture) : (object)(string)node[f.Name];
                f.SetValue(value,v);
            }
            return value;
        }
    }
}
public static class StaticData
{
    public static string fieldId="field_003";
    public static PlayerData playerData=new PlayerData();
    public class PlayerData { public int ZoneId=1; }
}
public static class BoxManager
{
    public static readonly List<string> Notices=new List<string>();
    public static void CreatePopupTextBox(string text,float delay) { Notices.Add(text); }
}
public static class NetAPI { public static bool SocketIsConnected() { return true; } }
namespace UI
{
    public class UIFieldRunMap { }
    public class PageManager
    {
        public static PageManager Instance=new PageManager();
        public object currentPage=new UIFieldRunMap();
    }
}
namespace Room
{
    public static class RoomManager { public static object currentRoom=new RoomField(); }
    public class RoomField { public TestNetManager netManager=new TestNetManager(); }
    public class TestNetManager
    {
        public int states,results;
        public void SCFieldPlayerState(ProtoBuf.Message.SCFieldPlayerState value) { states++; }
        public void SCSidePvpFightResult(ProtoBuf.Message.SCSidePvpFightResult value) { results++; }
    }
}
namespace NetWork.Packets.Factory
{
    public static class PacketFactory
    {
        public static bool WebPB2Bytes(int opcode,object value,out byte[] packet) { packet=null; return false; }
    }
}
namespace ProtoBuf.Message
{
    public class SCSocketLogin { public int result; }
    public class CSSidePvpFightResult { public long playerId; public int fightType; }
    public class SCSidePvpFightResult
    {
        public int code; public long attPlayerId,defPlayerId,winPlayerId; public string fightId;
    }
    public class PlayerSide { public long playerId; }
    public class SCFieldPlayerState { public List<PlayerSide> player; }
    public static class PBConvert
    {
        public static byte[] toBytes(int opcode,object value) { return Encoding.UTF8.GetBytes("request"); }
        public static object toObject(int opcode,byte[] value)
        {
            string key=Encoding.UTF8.GetString(value);
            if (opcode==30045 && key=="request") return new CSSidePvpFightResult { playerId=3,fightType=0 };
            if (opcode==30046 && key!="invalid")
            {
                var response=new SCSidePvpFightResult { code=1,attPlayerId=2,defPlayerId=3,winPlayerId=2,fightId=PvpBridgeRecoveryChecks.Fight };
                if (key=="attacker") response.attPlayerId=4;
                if (key=="defender") response.defPlayerId=4;
                if (key=="winner") response.winPlayerId=4;
                if (key=="fight") response.fightId="not-a-uuid";
                if (key=="code") response.code=0;
                if (key=="reject") response.code=2001;
                return response;
            }
            if (opcode==30042 && (key=="state" || key=="one" || key=="participants"))
            {
                var state=new SCFieldPlayerState { player=new List<PlayerSide>() };
                state.player.Add(new PlayerSide { playerId=2 });
                if (key!="one") state.player.Add(new PlayerSide { playerId=key=="participants" ? 4 : 3 });
                return state;
            }
            return null;
        }
    }
}
namespace HYJS.DesertV4
{
    public class FieldWireResult { public bool uncertain; public long code; public JsonData data; }
    public class FieldWire : UnityEngine.MonoBehaviour
    {
        public class Session
        {
            public readonly long playerId; public readonly string sessionHex,origin;
            public Session(long player,string key,string server) { playerId=player; sessionHex=key; origin=server; }
        }
        public static Session Live;
        public FieldWireResult Result;
        public int DelayMs;
        public Action BeforeCallback;
        public static bool Current(out Session value) { value=Live; return value!=null; }
        public void PvpStatus(Session owner,long stamp,string body,Action<FieldWireResult> callback)
        {
            if (DelayMs>0) System.Threading.Thread.Sleep(DelayMs);
            if (BeforeCallback!=null) BeforeCallback();
            callback(Result);
        }
    }
    public class FieldRuntime
    {
        public static readonly FieldRuntime Instance=new FieldRuntime();
        public int snapshots;
        public static FieldRuntime Require() { return Instance; }
        public bool CanAttackPvp() { return true; }
        public bool PvpRejected(int code) { return false; }
        public void RequestSnapshot() { snapshots++; }
    }
}

public static class PvpBridgeRecoveryChecks
{
    public const string Fight="123e4567-e89b-12d3-a456-426614174000";
    const long ServerTime=1793350000000L;
    static readonly BindingFlags Private=BindingFlags.NonPublic|BindingFlags.Instance|BindingFlags.Static;
    static readonly Type Bridge=typeof(HYJS.DesertV4.PvpBridge);
    static readonly string Request=Convert.ToBase64String(Encoding.UTF8.GetBytes("request"));
    static int count;
    static HYJS.DesertV4.PvpBridge bridge;
    static HYJS.DesertV4.FieldWire wire;
    static HYJS.DesertV4.FieldWire.Session owner;
    static Room.TestNetManager net;
    static string journalKey;
    static string Base64(string text) { return Convert.ToBase64String(Encoding.UTF8.GetBytes(text)); }
    static void Check(bool condition,string message)
    { count++; if (!condition) throw new Exception(message); }
    static object Call(string name,params object[] args) { return Bridge.GetMethod(name,Private).Invoke(null,args); }
    static void Set(string field,object value) { Bridge.GetField(field,Private).SetValue(bridge,value); }
    static object Get(string field) { return Bridge.GetField(field,Private).GetValue(bridge); }
    static string Digest(byte[] value)
    {
        var b=new StringBuilder();
        using (SHA256 sha=SHA256.Create()) foreach (byte x in sha.ComputeHash(value)) b.Append(x.ToString("x2",CultureInfo.InvariantCulture));
        return b.ToString();
    }
    static void Setup(bool resetPrefs)
    {
        if (resetPrefs) UnityEngine.PlayerPrefs.Reset();
        BoxManager.Notices.Clear(); HYJS.DesertV4.FieldRuntime.Instance.snapshots=0;
        Room.RoomManager.currentRoom=new Room.RoomField(); net=((Room.RoomField)Room.RoomManager.currentRoom).netManager;
        UI.PageManager.Instance=new UI.PageManager();
        owner=new HYJS.DesertV4.FieldWire.Session(2,"0123456789abcdef0123456789abcdef","http://127.0.0.1:28979");
        HYJS.DesertV4.FieldWire.Live=owner; wire=new HYJS.DesertV4.FieldWire(); bridge=new HYJS.DesertV4.PvpBridge();
        var record=new HYJS.DesertV4.PvpPending { version=1,playerId=2,opponentId=3,stamp=123,origin=owner.origin,
            body=Request,bodyHash=Digest(Encoding.UTF8.GetBytes("request")) };
        Call("Save",owner,record); journalKey=(string)Call("Key",owner,"pending");
        Set("session",owner); Set("pending",record); Set("wire",wire); Bridge.GetField("instance",Private).SetValue(null,bridge);
    }
    static string Receipt(string extra)
    {
        return "{\"stamp\":123,\"opponentId\":3,\"requestHash\":\""+new string('a',64)+"\",\"status\":\"settled\","+
            "\"nativeResponse\":\""+Base64("receipt")+"\",\"fightId\":\""+Fight+"\",\"winnerId\":2,\"serverTimeMs\":"+
            ServerTime.ToString(CultureInfo.InvariantCulture)+","+extra+"}";
    }
    static string Current(string until,string state)
    {
        string fields="\"presentationCurrent\":true";
        if (until!=null) fields+=",\"presentationUntilMs\":"+until;
        if (state!=null) fields+=",\"nativeState\":"+state;
        return Receipt(fields);
    }
    static string Future() { return (ServerTime+5000).ToString(CultureInfo.InvariantCulture); }
    static string Native(string key) { return "\""+Base64(key)+"\""; }
    static void Query(string raw)
    {
        wire.Result=new HYJS.DesertV4.FieldWireResult { code=0,data=JsonMapper.ToObject(raw) };
        Bridge.GetMethod("Query",Private).Invoke(bridge,null);
    }
    static void Preserved(string name,string raw)
    {
        Setup(true); Query(raw);
        Check(Get("pending")!=null && UnityEngine.PlayerPrefs.HasKey(journalKey),name+": journal must remain");
        Check(net.states==0 && net.results==0 && HYJS.DesertV4.FieldRuntime.Instance.snapshots==0,name+": no presentation or snapshot");
        Check(BoxManager.Notices.Contains("PK结果需要确认，请勿重复挑战"),name+": confirmation warning");
    }
    static void Synced(string name)
    {
        Check(Get("pending")==null && !UnityEngine.PlayerPrefs.HasKey(journalKey),name+": journal cleared");
        Check(net.results==0 && HYJS.DesertV4.FieldRuntime.Instance.snapshots==1,name+": snapshot and no replay");
        Check(BoxManager.Notices.Contains("PK结果已同步"),name+": sync notice");
    }
    static void Run()
    {
        Setup(true); Query(Receipt("\"presentationCurrent\":false")); Synced("false without until or state");
        Setup(true); Query(Receipt("\"presentationCurrent\":false,\"presentationUntilMs\":-1,\"nativeState\":42")); Synced("false ignores presentation fields");
        Preserved("true missing until",Current(null,Native("state")));
        Preserved("true zero until",Current("0",Native("state")));
        Preserved("true negative until",Current("-1",Native("state")));
        Preserved("true string until",Current("\""+Future()+"\"",Native("state")));
        Preserved("true fractional until",Current("1.5",Native("state")));
        Preserved("true missing state",Current(Future(),null));
        Preserved("true non-string state",Current(Future(),"42"));
        Preserved("true invalid base64",Current(Future(),"\"!\""));
        Preserved("true empty state",Current(Future(),"\"\""));
        Preserved("true wrong player count",Current(Future(),Native("one")));
        Preserved("true wrong players",Current(Future(),Native("participants")));
        Preserved("expired true missing state",Current(ServerTime.ToString(CultureInfo.InvariantCulture),null));
        Preserved("false missing server time",Receipt("\"presentationCurrent\":false").Replace("\"serverTimeMs\":"+ServerTime+",",""));
        Preserved("false zero server time",Receipt("\"presentationCurrent\":false").Replace("\"serverTimeMs\":"+ServerTime,"\"serverTimeMs\":0"));
        Preserved("false string server time",Receipt("\"presentationCurrent\":false").Replace("\"serverTimeMs\":"+ServerTime,"\"serverTimeMs\":\""+ServerTime+"\""));
        string settled=Receipt("\"presentationCurrent\":false");
        Preserved("missing presentation flag",settled.Replace(",\"presentationCurrent\":false",""));
        Preserved("non-boolean presentation flag",settled.Replace("\"presentationCurrent\":false","\"presentationCurrent\":0"));
        Preserved("wrong stamp",settled.Replace("\"stamp\":123","\"stamp\":124"));
        Preserved("wrong opponent",settled.Replace("\"opponentId\":3","\"opponentId\":4"));
        Preserved("invalid hash",settled.Replace(new string('a',64),new string('g',64)));
        Preserved("wrong fight ID",settled.Replace(Fight,"bad-id"));
        Preserved("wrong winner",settled.Replace("\"winnerId\":2","\"winnerId\":3"));
        foreach (string key in new string[]{"invalid","attacker","defender","winner","fight","code"})
            Preserved("invalid native response "+key,settled.Replace(Base64("receipt"),Base64(key)));

        string unknown=settled.Replace("\"status\":\"settled\"","\"status\":\"unknown\"");
        Preserved("unknown invalid server time",unknown.Replace("\"serverTimeMs\":"+ServerTime,"\"serverTimeMs\":0"));
        Setup(true); Query(unknown);
        Check(Get("pending")!=null && UnityEngine.PlayerPrefs.HasKey(journalKey) && BoxManager.Notices.Count==0,"valid unknown retains journal without warning");
        string rejected=settled.Replace("\"status\":\"settled\"","\"status\":\"rejected\"").Replace(Base64("receipt"),Base64("reject"));
        Preserved("rejected invalid server time",rejected.Replace("\"serverTimeMs\":"+ServerTime,"\"serverTimeMs\":0"));
        Setup(true); Query(rejected);
        Check(Get("pending")==null && !UnityEngine.PlayerPrefs.HasKey(journalKey) && net.results==1 &&
            HYJS.DesertV4.FieldRuntime.Instance.snapshots==1,"valid rejected clears and forwards original response");

        Setup(true);
        var newer=new HYJS.DesertV4.PvpPending { version=1,playerId=2,opponentId=3,stamp=124,origin=owner.origin,
            body=Request,bodyHash=Digest(Encoding.UTF8.GetBytes("request")) };
        Call("Save",owner,newer); string newerJournal=UnityEngine.PlayerPrefs.GetString(journalKey,"");
        Query(settled);
        Check(Get("pending")!=null && UnityEngine.PlayerPrefs.GetString(journalKey,"")==newerJournal &&
            HYJS.DesertV4.FieldRuntime.Instance.snapshots==0,"different persisted journal is preserved");
        Check(BoxManager.Notices.Contains("PK结果需要确认，请勿重复挑战"),"changed journal warns");
        Setup(true); string corrupt=UnityEngine.PlayerPrefs.GetString(journalKey,"").Replace(Digest(Encoding.UTF8.GetBytes("request")),new string('a',64));
        UnityEngine.PlayerPrefs.SetString(journalKey,corrupt); Query(settled);
        Check(Get("pending")!=null && UnityEngine.PlayerPrefs.GetString(journalKey,"")==corrupt &&
            HYJS.DesertV4.FieldRuntime.Instance.snapshots==0,"corrupt request hash preserves journal");
        Setup(true); wire.BeforeCallback=delegate { Set("epoch",1); }; Query(settled);
        Check(Get("pending")!=null && UnityEngine.PlayerPrefs.HasKey(journalKey) &&
            HYJS.DesertV4.FieldRuntime.Instance.snapshots==0 && BoxManager.Notices.Count==0,"stale callback leaves journal untouched");

        Setup(true); wire.DelayMs=50; Query(Current((ServerTime+10).ToString(CultureInfo.InvariantCulture),Native("state"))); wire.DelayMs=0;
        Check(Get("pending")!=null && UnityEngine.PlayerPrefs.HasKey(journalKey),"expired true waits for authoritative status");
        Check((long)Get("next")==0 && net.states==0 && net.results==0,"expired true immediately rechecks without replay");
        Query(settled); Synced("expired lease followed by false");

        Setup(true); Query(Current(Future(),Native("state")));
        Check(Get("pending")==null && !UnityEngine.PlayerPrefs.HasKey(journalKey),"current clears after presentation");
        Check(net.states==1 && net.results==1 && HYJS.DesertV4.FieldRuntime.Instance.snapshots==1,"current presents once and refreshes");
        Setup(false); Query(Current(Future(),Native("state")));
        Check(Get("pending")==null && net.states==1 && net.results==0 && HYJS.DesertV4.FieldRuntime.Instance.snapshots==1,"shown fight does not replay");
        Console.WriteLine("PASS "+count+" actual PvpBridge Query assertions with original LitJson (Unity/transport/protobuf fakes)");
    }
    static string TypeName(MetadataReader metadata,EntityHandle handle)
    {
        if (handle.Kind==HandleKind.TypeReference)
        {
            TypeReference type=metadata.GetTypeReference((TypeReferenceHandle)handle);
            return metadata.GetString(type.Namespace)+"."+metadata.GetString(type.Name);
        }
        if (handle.Kind==HandleKind.TypeDefinition)
        {
            TypeDefinition type=metadata.GetTypeDefinition((TypeDefinitionHandle)handle);
            return metadata.GetString(type.Namespace)+"."+metadata.GetString(type.Name);
        }
        return handle.Kind.ToString();
    }
    static SortedDictionary<string,string> Abi(string path)
    {
        var entries=new SortedDictionary<string,string>();
        using (FileStream file=File.OpenRead(path)) using (var pe=new PEReader(file))
        {
            MetadataReader md=pe.GetMetadataReader();
            foreach (TypeDefinitionHandle handle in md.TypeDefinitions)
            {
                TypeDefinition type=md.GetTypeDefinition(handle); string name=md.GetString(type.Name);
                if (md.GetString(type.Namespace)!="HYJS.DesertV4" || name!="PvpBridge" && name!="PvpPending") continue;
                entries.Add(name,(int)type.Attributes+":"+TypeName(md,type.BaseType));
                foreach (MethodDefinitionHandle mh in type.GetMethods())
                {
                    MethodDefinition method=md.GetMethodDefinition(mh);
                    if ((method.Attributes&MethodAttributes.MemberAccessMask)!=MethodAttributes.Public) continue;
                    string signature=BitConverter.ToString(md.GetBlobBytes(method.Signature));
                    entries.Add(name+".method."+md.GetString(method.Name)+":"+signature,((int)method.Attributes).ToString());
                }
                foreach (FieldDefinitionHandle fh in type.GetFields())
                {
                    FieldDefinition field=md.GetFieldDefinition(fh);
                    if ((field.Attributes&FieldAttributes.FieldAccessMask)!=FieldAttributes.Public) continue;
                    entries.Add(name+".field."+md.GetString(field.Name),((int)field.Attributes)+":"+
                        BitConverter.ToString(md.GetBlobBytes(field.Signature)));
                }
            }
        }
        return entries;
    }
    static void CompareAbi(string before,string after)
    {
        SortedDictionary<string,string> a=Abi(before),b=Abi(after);
        if (!a.ContainsKey("PvpBridge") || !a.ContainsKey("PvpPending") || a.Count!=b.Count)
            throw new Exception("PK ABI type or member count changed");
        foreach (KeyValuePair<string,string> x in a)
        {
            string value;
            if (!b.TryGetValue(x.Key,out value) || x.Value!=value) throw new Exception("PK ABI changed: "+x.Key);
        }
        Console.WriteLine("PASS "+a.Count+" PvpBridge/PvpPending type, public method and persisted-field ABI entries match baseline");
    }
    public static int Main(string[] args)
    {
        try
        {
            if (args.Length==3 && args[0]=="--abi") CompareAbi(args[1],args[2]);
            else if (args.Length==0) Run();
            else throw new ArgumentException("Expected no arguments or --abi <baseline> <candidate>");
            return 0;
        }
        catch (Exception error) { Console.Error.WriteLine(error); return 1; }
    }
}
