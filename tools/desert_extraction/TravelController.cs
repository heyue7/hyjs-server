using System;
using System.Diagnostics;
using System.Globalization;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    [Serializable] sealed class TravelRecord
    {
        public int version;
        public long playerId,minimumRevision;
        public string origin,kind,requestId,pointId,cancelId,phase;
    }
    // One durable travel operation per player/server. It never applies the
    // historical HTTP position as a new teleport command.
    public sealed class TravelController : MonoBehaviour
    {
        static TravelController instance;
        FieldWire wire;
        FieldWire.Session session;
        TravelRecord pending;
        bool busy,awaitingOriginal,loggedWarning,faulted,saveFailed;
        int connectionEpoch,operationGeneration;
        long nextAction;
        GameObject extractionPopup;
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        static bool UUID(string value)
        { try { return !String.IsNullOrEmpty(value) && new Guid(value).ToString("D")==value; } catch (Exception) { return false; } }
        static string Key(FieldWire.Session owner)
        { return "hyjs.ggo.travel.v4."+Uri.EscapeDataString(owner.origin)+"."+owner.playerId.ToString(CultureInfo.InvariantCulture); }
        static TravelRecord Load(FieldWire.Session owner)
        {
            string raw=PlayerPrefs.GetString(Key(owner),""); if (String.IsNullOrEmpty(raw)) return null;
            var p=JsonUtility.FromJson<TravelRecord>(raw);
            if (p==null || p.version!=1 || p.playerId!=owner.playerId || p.origin!=owner.origin ||
                (p.kind!="entry" && p.kind!="extraction") || !UUID(p.requestId) ||
                (!String.IsNullOrEmpty(p.cancelId) && !UUID(p.cancelId)) || p.minimumRevision<0 ||
                (p.kind=="extraction" && String.IsNullOrEmpty(p.pointId)) ||
                (p.phase!="start" && p.phase!="extracting" && p.phase!="cancel" && p.phase!="position") ||
                (p.kind=="entry" && p.phase!="start" && p.phase!="position") ||
                (p.phase=="cancel" && !UUID(p.cancelId)) || (p.phase=="position" && p.minimumRevision<1))
                throw new InvalidOperationException("Unreadable pending travel");
            return p;
        }
        static void Save(FieldWire.Session owner,TravelRecord value)
        {
            if (value==null || value.playerId!=owner.playerId || value.origin!=owner.origin || !UUID(value.requestId))
                throw new InvalidOperationException("Invalid travel owner");
            string key=Key(owner),raw=JsonUtility.ToJson(value);
            PlayerPrefs.SetString(key,raw); PlayerPrefs.Save();
            if (PlayerPrefs.GetString(key,"")!=raw) throw new InvalidOperationException("Travel save failed");
        }
        static void Clear(FieldWire.Session owner,TravelRecord value)
        {
            TravelRecord old=Load(owner);
            if (old==null || value==null || old.requestId!=value.requestId || old.kind!=value.kind)
                throw new InvalidOperationException("Pending travel changed");
            string key=Key(owner),raw=PlayerPrefs.GetString(key,"");
            try { PlayerPrefs.DeleteKey(key); PlayerPrefs.Save(); }
            catch (Exception) { PlayerPrefs.SetString(key,raw); throw; }
        }
        static void Notice(string text) { BoxManager.CreatePopupTextBox(text,0f); }
        void Warn(string text) { if (!loggedWarning) { loggedWarning=true; Notice(text); } }
        static long Integer(JsonData d) { return Int64.Parse(d.ToString(),CultureInfo.InvariantCulture); }
        static Vector3 Vector(JsonData d)
        {
            var p=new Vector3((float)Double.Parse(d["x"].ToString(),CultureInfo.InvariantCulture),
                (float)Double.Parse(d["y"].ToString(),CultureInfo.InvariantCulture),
                (float)Double.Parse(d["z"].ToString(),CultureInfo.InvariantCulture));
            if (Single.IsNaN(p.x)||Single.IsNaN(p.y)||Single.IsNaN(p.z)||
                Single.IsInfinity(p.x)||Single.IsInfinity(p.y)||Single.IsInfinity(p.z)) throw new InvalidOperationException("Invalid server position");
            return p;
        }
        static RoomCharactor Actor()
        {
            RoomBase room=RoomManager.currentRoom;
            return room==null || room.charactorManager==null ? null : room.charactorManager.GetSelfCharactor();
        }
        static RoomNetField Net()
        { return RoomManager.currentRoom==null ? null : RoomManager.currentRoom.netManager as RoomNetField; }
        static bool InDesert() { return StaticData.fieldId=="field_003" && RoomManager.currentRoom is RoomField; }
        static TravelController Require()
        {
            if (instance!=null) return instance;
            var host=new GameObject("HYJS.DesertV4.Travel"); DontDestroyOnLoad(host);
            instance=host.AddComponent<TravelController>(); instance.wire=host.AddComponent<FieldWire>();
            return instance;
        }
        public static void SceneReady()
        {
            if (!InDesert()) return;
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return;
            try { if (Load(owner)!=null) Require().Adopt(); }
            catch (Exception) { Notice("行程记录需要恢复，请勿重复操作"); }
        }
        void OnDestroy() { CloseExtractionPopup(); if (instance==this) instance=null; }
        void CloseExtractionPopup()
        {
            if (extractionPopup!=null) Destroy(extractionPopup);
            extractionPopup=null;
        }
        bool Adopt()
        {
            FieldWire.Session now; if (!FieldWire.Current(out now)) return false;
            if (!SameSession(session,now))
            {
                bool same=SameActor(session,now);
                if (!same) { PositionSync.Release(); CloseExtractionPopup(); }
                session=now; busy=false; awaitingOriginal=false; nextAction=0; operationGeneration++;
                if (!(same && saveFailed && pending!=null))
                {
                    try { pending=Load(now); faulted=false; saveFailed=false; }
                    catch (Exception) { pending=null; faulted=true; Warn("行程记录需要恢复，请勿重复操作"); return false; }
                }
            }
            return !faulted;
        }
        bool Still(FieldWire.Session owner,string id)
        { return SameSession(session,owner) && pending!=null && pending.requestId==id; }
        public static bool Pending
        {
            get
            {
                FieldWire.Session owner;
                if (!FieldWire.Current(out owner)) return true;
                if (instance!=null && SameActor(instance.session,owner) &&
                    (instance.pending!=null || instance.busy || instance.awaitingOriginal || instance.saveFailed || instance.faulted)) return true;
                try { return Load(owner)!=null; }
                catch (Exception) { return true; }
            }
        }
        public static bool PendingExtraction
        {
            get { return instance!=null && instance.pending!=null && instance.pending.kind=="extraction"; }
        }
        public static void Entry()
        {
            if (!InDesert()) return;
            if (LeaveController.Pending) { Notice("离图正在确认"); return; }
            var core=Require(); if (!core.Adopt()) return;
            if (core.pending!=null) { Notice("上次行程仍在确认"); return; }
            FieldWire.Session who=core.session;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("随机出发并获得短暂保护？",delegate(bool ok)
            {
                if (!ok || !core.Adopt() || !SameSession(who,core.session) || core.pending!=null || !InDesert()) return;
                if (!PositionSync.TryLock(Actor())) { Notice("位置正在同步，请稍候"); return; }
                try
                {
                    core.pending=new TravelRecord { version=1,playerId=who.playerId,origin=who.origin,kind="entry",requestId=Guid.NewGuid().ToString("D"),phase="start" };
                    Save(who,core.pending); core.nextAction=0;
                }
                catch (Exception) { core.saveFailed=true; Notice("出发记录待确认，请保持当前界面"); }
            });
        }
        public static void Extraction(string pointId)
        {
            if (!InDesert() || String.IsNullOrEmpty(pointId)) return;
            if (LeaveController.Pending) { Notice("离图正在确认"); return; }
            var core=Require(); if (!core.Adopt()) return;
            if (core.pending!=null) { Notice("上次行程仍在确认"); return; }
            FieldWire.Session who=core.session;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("开始撤离？移动、受击或行动会中断。",delegate(bool ok)
            {
                if (!ok || !core.Adopt() || !SameSession(who,core.session) || core.pending!=null || !InDesert()) return;
                try
                {
                    core.pending=new TravelRecord { version=1,playerId=who.playerId,origin=who.origin,kind="extraction",requestId=Guid.NewGuid().ToString("D"),pointId=pointId,phase="start" };
                    Save(who,core.pending); core.nextAction=0;
                }
                catch (Exception) { core.saveFailed=true; Notice("撤离记录待确认，请保持当前界面"); }
            });
        }
        // The interrupted action must be retried after cancellation is confirmed.
        public static bool InterruptExtraction()
        {
            if (!PendingExtraction) return false;
            TravelController core=instance;
            try
            {
                if (!core.Adopt() || core.pending==null) return true;
                if (core.pending.phase=="position") return true;
                core.CloseExtractionPopup();
                if (String.IsNullOrEmpty(core.pending.cancelId)) core.pending.cancelId=Guid.NewGuid().ToString("D");
                core.pending.phase="cancel"; core.operationGeneration++; core.busy=false; core.nextAction=0;
                Save(core.session,core.pending);
            }
            catch (Exception) { core.saveFailed=true; core.Warn("撤离取消记录待确认"); }
            return true;
        }
        void Update()
        {
            if (!Adopt() || pending==null || !InDesert()) { if (!InDesert()) CloseExtractionPopup(); return; }
            if (saveFailed)
            {
                try { Save(session,pending); saveFailed=false; }
                catch (Exception) { Warn("行程记录待确认"); return; }
            }
            PositionSync.Tick();
            int generation=PositionSync.ConnectionEpoch;
            if (generation!=connectionEpoch) { connectionEpoch=generation; operationGeneration++; busy=false; awaitingOriginal=false; nextAction=0; }
            if (busy || awaitingOriginal || Stopwatch.GetTimestamp()<nextAction) return;
            if (pending.phase=="position") { Synchronize(); return; }
            if (pending.kind=="entry") { EntryRequest(); return; }
            if (pending.phase=="cancel") CancelRequest();
            else if (pending.phase=="start") StartRequest();
            else StatusRequest();
        }
        void Later() { busy=false; nextAction=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        void Finish(string notice)
        {
            try { Clear(session,pending); pending=null; busy=false; awaitingOriginal=false; CloseExtractionPopup(); PositionSync.Release(); loggedWarning=false; Notice(notice); FieldRuntime.Require().RequestSnapshot(); }
            catch (Exception) { Warn("行程结果待确认"); }
        }
        void EntryRequest()
        {
            RoomCharactor actor=Actor();
            if (!PositionSync.Locked && !PositionSync.TryLock(actor)) return;
            FieldWire.Session owner=session; string id=pending.requestId; int op=operationGeneration; busy=true;
            wire.Entry(owner,id,delegate(FieldWireResult r)
            {
                if (op!=operationGeneration || !Still(owner,id)) return; Later();
                if (r.uncertain || r.code==7002 || r.code==7207) { Warn("出发结果待确认"); return; }
                if (r.code!=0) { Finish("当前无法出发，请稍后重试"); return; }
                try
                {
                    if (r.data==null || (string)r.data["requestId"]!=id) throw new InvalidOperationException();
                    long rev=Integer(r.data["positionRevision"]); if (rev<=0) throw new InvalidOperationException();
                    pending.minimumRevision=rev; pending.phase="position"; Save(owner,pending); nextAction=0; loggedWarning=false;
                }
                catch (Exception) { Warn("出发位置待确认"); }
            });
        }
        void StartRequest()
        {
            FieldWire.Session owner=session; string id=pending.requestId,point=pending.pointId; int op=operationGeneration; long sent=Stopwatch.GetTimestamp(); busy=true;
            wire.ExtractionStart(owner,id,point,delegate(FieldWireResult r)
            {
                if (op!=operationGeneration || !Still(owner,id)) return; Later();
                if (r.uncertain || r.code==7002 || r.code==7207) { Warn("撤离请求待确认"); return; }
                if (r.code!=0) { Finish("当前无法撤离，请稍后重试"); return; }
                try
                {
                    JsonData d=r.data; long accepted=Integer(d["acceptedAtMs"]),ready=Integer(d["readyAtMs"]);
                    if ((string)d["requestId"]!=id || (string)d["pointId"]!=point || accepted<=0 || ready<=accepted || ready-accepted>300000) throw new InvalidOperationException();
                    pending.phase="extracting"; Save(owner,pending);
                    long rtt=Stopwatch.GetTimestamp()-sent;
                    long duration=Math.Max(0,ready-accepted-(long)((double)rtt*500/Stopwatch.Frequency));
                    // The native one-button countdown invokes the same action on
                    // timeout and on click. Use its two-button form, whose timeout
                    // never invokes the click callback, and keep the native cancel.
                    BoxManager.CreateCountdownBoxTwoButton("撤离中，{0} 秒",true,(float)(duration/1000.0),delegate(bool ok)
                    { if (!ok && Still(owner,id)) InterruptExtraction(); },"");
                    extractionPopup=BoxManager.m_kBox;
                    MessageBoxCountdown box=extractionPopup==null ? null : extractionPopup.GetComponent<MessageBoxCountdown>();
                    if (box==null || box.m_kOKBtn==null || box.m_kCancelBtn==null)
                    { CloseExtractionPopup(); throw new InvalidOperationException("Native extraction countdown unavailable"); }
                    box.m_kOKBtn.SetActive(false);
                    box.m_kCancelBtn.transform.localPosition=new Vector3(0f,-134f,0f);
                    loggedWarning=false; nextAction=0;
                }
                catch (Exception) { Warn("撤离状态待确认"); }
            });
        }
        void CancelRequest()
        {
            FieldWire.Session owner=session; string id=pending.requestId,cancel=pending.cancelId; int op=operationGeneration; busy=true;
            wire.ExtractionCancel(owner,cancel,id,delegate(FieldWireResult r)
            {
                if (op!=operationGeneration || !Still(owner,id)) return; Later();
                if (r.uncertain || r.code!=0 || r.data==null) { Warn("撤离取消待确认"); return; }
                HandleStatus(owner,id,r.data);
            });
        }
        void StatusRequest()
        {
            FieldWire.Session owner=session; string id=pending.requestId; int op=operationGeneration; busy=true;
            wire.ExtractionStatus(owner,id,delegate(FieldWireResult r)
            {
                if (op!=operationGeneration || !Still(owner,id)) return; Later();
                if (r.uncertain || r.code!=0 || r.data==null) { Warn("撤离状态待确认"); return; }
                HandleStatus(owner,id,r.data);
            });
        }
        void HandleStatus(FieldWire.Session owner,string id,JsonData data)
        {
            try
            {
                if ((string)data["startRequestId"]!=id) throw new InvalidOperationException();
                string status=(string)data["status"];
                if (status=="cancelled") { Finish("撤离已中断"); return; }
                if (status=="completed")
                {
                    long rev=Integer(data["positionRevision"]); if (rev<=0) throw new InvalidOperationException();
                    pending.minimumRevision=rev; pending.phase="position"; Save(owner,pending);
                    CloseExtractionPopup(); PositionSync.LockAfterTeleport(Actor()); nextAction=0; loggedWarning=false; return;
                }
                if (status!="pending" && status!="unknown") throw new InvalidOperationException();
                if (status=="unknown" && pending.phase=="extracting") Warn("撤离状态待确认");
            }
            catch (Exception) { Warn("撤离状态待确认"); }
        }
        void Synchronize()
        {
            if (!PositionSync.Locked && !PositionSync.LockAfterTeleport(Actor())) return;
            RoomNetField net=Net(); if (net==null || !NetAPI.SocketIsConnected()) return;
            FieldWire.Session owner=session; string id=pending.requestId; int epoch=PositionSync.ConnectionEpoch;
            try
            {
                awaitingOriginal=true;
                PositionSync.RequestCurrent(net,delegate(Vector3 observed)
                {
                    if (!Still(owner,id) || epoch!=PositionSync.ConnectionEpoch) return;
                    awaitingOriginal=false; busy=true;
                    wire.PositionStatus(owner,delegate(FieldWireResult r)
                    {
                        if (!Still(owner,id) || epoch!=PositionSync.ConnectionEpoch) return;
                        Later();
                        if (r.uncertain || r.code!=0 || r.data==null) { Warn("位置确认中"); return; }
                        try
                        {
                            JsonData d=r.data;
                            if ((string)d["scene"]!="field_003" || !d["loading"].IsBoolean || (bool)d["loading"]) throw new InvalidOperationException();
                            long version=Integer(d["positionRevision"]),confirmed=Integer(d["confirmedRevision"]);
                            Vector3 position=Vector(d["position"]);
                            if (version<pending.minimumRevision || confirmed!=version || (position-observed).sqrMagnitude>.0001f) throw new InvalidOperationException();
                            Finish(pending.kind=="entry" ? "已出发" : "撤离完成，已返回安全区");
                        }
                        catch (Exception) { Warn("位置确认中"); }
                    });
                });
            }
            catch (Exception) { awaitingOriginal=false; nextAction=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        }
    }
}
