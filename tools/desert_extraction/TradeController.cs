using System;
using System.Collections;
using System.Diagnostics;
using System.Globalization;
using LitJson;
using UnityEngine;

namespace HYJS.DesertV4
{
    [Serializable] sealed class TradePending
    {
        public int schema;
        public long playerId,target,initiator,receiver,version;
        public string origin,action,requestId,tradeId;
        public long[] offer;
    }
    [Serializable] sealed class TradeLink
    {
        public int schema;
        public long playerId,initiator,receiver;
        public string origin,tradeId;
    }
    public sealed class TradeQuote
    {
        public string id,status,reason;
        public long initiator,receiver,version,serverTime,expires;
        public long[][] offers=new long[][] {new long[6],new long[6]};
        public bool[] confirmed=new bool[2];
        public string[] values=new string[2];
        public long[] balance=new long[6],protectedChips=new long[6],after=new long[6],afterProtected=new long[6];
        public long credit,revision;
        public bool preview,active;
    }
    // All offers, confirmations, balances and settlement come from the server.
    // Mutation UUID + complete original body is saved before the first request.
    public sealed class TradeController : MonoBehaviour
    {
        static TradeController instance;
        FieldWire wire;
        FieldWire.Session session;
        TradePending pending;
        TradeLink link;
        TradeQuote quote;
        bool busy,dirty,warning,faulted,showInvitation;
        long next,refreshAt;
        int generation;
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        static bool UUID(string s)
        { try { return !String.IsNullOrEmpty(s) && new Guid(s).ToString("D")==s; } catch (Exception) { return false; } }
        static long Number(JsonData x) { return Int64.Parse(x.ToString(),CultureInfo.InvariantCulture); }
        static string Key(FieldWire.Session s,string suffix)
        { return "hyjs.ggo.trade.v4."+Uri.EscapeDataString(s.origin)+"."+s.playerId.ToString(CultureInfo.InvariantCulture)+"."+suffix; }
        static void Store(FieldWire.Session s,string suffix,string raw)
        {
            string key=Key(s,suffix),before=PlayerPrefs.GetString(key,"");
            try { if (raw==null) PlayerPrefs.DeleteKey(key); else PlayerPrefs.SetString(key,raw); PlayerPrefs.Save();
                if (PlayerPrefs.GetString(key,"")!=(raw ?? "")) throw new InvalidOperationException("Trade save failed"); }
            catch (Exception) { if (String.IsNullOrEmpty(before)) PlayerPrefs.DeleteKey(key); else PlayerPrefs.SetString(key,before); throw; }
        }
        static bool ValidLink(FieldWire.Session owner,TradeLink p)
        { return p!=null && p.schema==1 && p.playerId==owner.playerId && p.origin==owner.origin && UUID(p.tradeId) && p.initiator>0 && p.receiver>0 && p.initiator!=p.receiver && (p.initiator==owner.playerId || p.receiver==owner.playerId); }
        static TradeLink LoadLink(FieldWire.Session owner)
        {
            string raw=PlayerPrefs.GetString(Key(owner,"link"),""); if (String.IsNullOrEmpty(raw)) return null;
            TradeLink p=JsonUtility.FromJson<TradeLink>(raw); if (!ValidLink(owner,p)) throw new InvalidOperationException("Unreadable trade link"); return p;
        }
        static bool ValidPending(FieldWire.Session owner,TradePending p)
        {
            if (p==null || p.schema!=1 || p.playerId!=owner.playerId || p.origin!=owner.origin || !UUID(p.requestId)) return false;
            if (p.action=="invite") return p.target>0 && p.target!=owner.playerId && p.requestId==p.tradeId;
            if (!UUID(p.tradeId) || p.requestId==p.tradeId || p.initiator<=0 || p.receiver<=0 || p.initiator==p.receiver ||
                (owner.playerId!=p.initiator && owner.playerId!=p.receiver) || p.version<1 ||
                (p.action!="accept" && p.action!="offer" && p.action!="confirm" && p.action!="cancel")) return false;
            if (p.action!="offer") return p.offer==null || p.offer.Length==0;
            if (p.offer==null || p.offer.Length!=6) return false;
            foreach (long x in p.offer) if (x<0) return false;
            return true;
        }
        static TradePending LoadPending(FieldWire.Session owner)
        {
            string raw=PlayerPrefs.GetString(Key(owner,"pending"),""); if (String.IsNullOrEmpty(raw)) return null;
            TradePending p=JsonUtility.FromJson<TradePending>(raw); if (!ValidPending(owner,p)) throw new InvalidOperationException("Unreadable trade request"); return p;
        }
        static void SavePending(FieldWire.Session owner,TradePending p)
        { if (!ValidPending(owner,p)) throw new InvalidOperationException("Invalid trade request"); Store(owner,"pending",JsonUtility.ToJson(p)); }
        static void SaveLink(FieldWire.Session owner,TradeLink p)
        { if (!ValidLink(owner,p)) throw new InvalidOperationException("Invalid trade identity"); Store(owner,"link",JsonUtility.ToJson(p)); }
        static string Optional(JsonData row,string name)
        { return row!=null && ((IDictionary)row).Contains(name) && row[name]!=null ? row[name].ToString() : ""; }
        static long[] Six(JsonData rows)
        {
            if (rows==null || !rows.IsArray || rows.Count!=6) throw new InvalidOperationException("Invalid chip quantities");
            var result=new long[6]; for (int i=0;i<6;i++) { result[i]=Number(rows[i]); if (result[i]<0) throw new InvalidOperationException("Negative chip quantity"); } return result;
        }
        static JsonData NestedArray(JsonData row)
        {
            if (row==null || !row.IsObject || ((IDictionary)row).Count!=1 || !((IDictionary)row).Contains(NestedArrayJson.Key))
                throw new InvalidOperationException("Invalid nested trade array");
            JsonData values=row[NestedArrayJson.Key];
            if (values==null || !values.IsArray) throw new InvalidOperationException("Invalid nested trade values");
            return values;
        }
        static TradeQuote Parse(FieldWire.Session owner,JsonData d)
        {
            if (d==null) throw new InvalidOperationException("Missing trade view");
            TradeQuote q=new TradeQuote(); q.status=(string)d["status"]; q.serverTime=Number(d["serverTimeMs"]);
            if (q.serverTime<=0 || (q.status!="none" && q.status!="invited" && q.status!="open" && q.status!="settled" && q.status!="cancelled" && q.status!="expired")) throw new InvalidOperationException("Invalid trade status");
            if (q.status=="none") return q;
            q.id=(string)d["tradeId"]; q.initiator=Number(d["initiatorId"]); q.receiver=Number(d["receiverId"]); q.version=Number(d["version"]); q.reason=Optional(d,"reason");
            q.expires=Number(d["expiresAtMs"]);
            if (!UUID(q.id) || q.initiator<=0 || q.receiver<=0 || q.initiator==q.receiver || (owner.playerId!=q.initiator && owner.playerId!=q.receiver) || q.version<1 || q.expires<=0 || !d["active"].IsBoolean || !d["previewValid"].IsBoolean) throw new InvalidOperationException("Invalid trade identity");
            q.active=(bool)d["active"]; q.preview=(bool)d["previewValid"];
            if (q.active!=(q.status=="invited" || q.status=="open") || d["offers"].Count!=2 || d["confirmed"].Count!=2 || d["offerCreditValues"].Count!=2) throw new InvalidOperationException("Invalid trade state");
            for (int i=0;i<2;i++) { q.offers[i]=Six(NestedArray(d["offers"][i])); if (!d["confirmed"][i].IsBoolean) throw new InvalidOperationException("Invalid confirmation"); q.confirmed[i]=(bool)d["confirmed"][i]; q.values[i]=d["offerCreditValues"][i].ToString(); }
            JsonData t=d["terminal"]; q.balance=Six(t["chips"]); q.protectedChips=Six(t["protected"]); q.credit=Number(t["creditBalance"]); q.revision=Number(t["revision"]);
            q.after=Six(d["afterChips"]); q.afterProtected=Six(d["afterProtected"]);
            if (q.credit<0 || q.revision<0) throw new InvalidOperationException("Invalid terminal state");
            for (int i=0;i<6;i++) if (q.protectedChips[i]>q.balance[i] || (q.preview && q.afterProtected[i]>q.after[i])) throw new InvalidOperationException("Invalid protection state");
            return q;
        }
        static void Notice(string text) { BoxManager.CreatePopupTextBox(text,0f); }
        void Warn(string text) { if (!warning) { warning=true; Notice(text); } }
        static TradeController Require()
        {
            if (instance!=null) return instance;
            GameObject host=new GameObject("HYJS.DesertV4.Trade"); DontDestroyOnLoad(host);
            instance=host.AddComponent<TradeController>(); instance.wire=host.AddComponent<FieldWire>(); return instance;
        }
        bool Adopt()
        {
            FieldWire.Session now; if (!FieldWire.Current(out now)) return false;
            if (!SameSession(session,now))
            {
                bool same=SameActor(session,now); session=now; busy=false; dirty=true; next=refreshAt=0; generation++;
                if (!same) { quote=null; TradePanel.CloseLocal(); }
                try { pending=LoadPending(now); link=LoadLink(now); faulted=false; }
                catch (Exception) { pending=null; link=null; faulted=true; Warn("交易记录需要恢复，请勿重复操作"); return false; }
            }
            return !faulted;
        }
        public static void SceneReady()
        { if (StaticData.fieldId=="field_003" && Room.RoomManager.currentRoom is Room.RoomField) { TradeController t=Require(); if (t.Adopt()) t.dirty=true; } }
        public static void SignalChange()
        { if (instance!=null) instance.dirty=true; }
        public static void SceneExit()
        { TradePanel.CloseLocal(); if (instance!=null) instance.dirty=true; }
        public static bool BusyTrade { get { return instance!=null && (instance.faulted || instance.pending!=null || instance.quote!=null && instance.quote.active); } }
        public static void Invite(long target)
        {
            TradeController c=Require(); if (!c.Adopt()) return;
            if (c.pending!=null || c.quote!=null && c.quote.active) { Notice("已有交易正在进行"); return; }
            if (LeaveController.Pending || TravelController.PendingExtraction) { Notice("请先完成当前行动"); return; }
            FieldWire.Session owner=c.session;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("向该玩家发起芯片交易或赠送？",delegate(bool yes)
            {
                if (!yes || !c.Adopt() || !SameSession(owner,c.session) || c.pending!=null || c.quote!=null && c.quote.active) return;
                string id=Guid.NewGuid().ToString("D");
                var p=new TradePending {schema=1,playerId=owner.playerId,origin=owner.origin,action="invite",requestId=id,tradeId=id,target=target};
                c.Begin(p);
            });
        }
        void Begin(TradePending p)
        {
            if (busy || pending!=null || !ValidPending(session,p)) return;
            try
            {
                if (LoadPending(session)!=null) throw new InvalidOperationException("Previous trade mutation remains");
                SavePending(session,p); pending=p; next=0; warning=false;
                if (quote!=null && quote.status=="open") TradePanel.Show(quote,session.playerId,true);
            }
            catch (Exception)
            {
                try { pending=LoadPending(session); } catch (Exception) { faulted=true; }
                Warn("交易记录待确认");
            }
        }
        public static void Action(string action,long[] offer)
        {
            TradeController c=instance; if (c==null || !c.Adopt() || c.pending!=null || c.quote==null || !c.quote.active) return;
            TradeQuote q=c.quote;
            if (action=="accept" && (q.status!="invited" || c.session.playerId!=q.receiver) ||
                (action=="offer" || action=="confirm") && q.status!="open") return;
            var p=new TradePending { schema=1,playerId=c.session.playerId,origin=c.session.origin,action=action,requestId=Guid.NewGuid().ToString("D"),tradeId=q.id,initiator=q.initiator,receiver=q.receiver,version=q.version,offer=offer==null ? null:(long[])offer.Clone() };
            c.Begin(p);
        }
        public static void Refresh() { if (instance!=null) { instance.dirty=true; instance.next=0; } }
        void OnDestroy() { if (instance==this) instance=null; }
        void Update()
        {
            if (!Adopt()) return;
            if (busy || Stopwatch.GetTimestamp()<next) return;
            if (pending!=null) { Send(); return; }
            if (dirty || quote!=null && quote.active && Stopwatch.GetTimestamp()>=refreshAt) { Read(); }
        }
        void Later() { busy=false; next=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        bool Still(FieldWire.Session owner,int op) { return SameSession(session,owner) && generation==op; }
        void Send()
        {
            FieldWire.Session owner=session; TradePending current=pending; int op=generation; busy=true;
            Action<FieldWireResult> done=delegate(FieldWireResult r)
            {
                if (!Still(owner,op) || pending==null || pending.requestId!=current.requestId) return; Later();
                if (r.uncertain || r.code==7002 || r.code==7207) { Warn("交易结果待确认"); return; }
                if (r.code!=0)
                {
                    try { Store(owner,"pending",null); pending=null; dirty=true; }
                    catch (Exception) { Warn("交易记录待确认"); return; }
                    Notice(String.IsNullOrEmpty(r.message) ? "当前无法交易" : r.message); return;
                }
                try
                {
                    TradeQuote q=Parse(owner,r.data);
                    if (q.status=="none" || q.id!=current.tradeId || (current.action=="invite" && (q.initiator!=owner.playerId || q.receiver!=current.target)) ||
                        (current.action!="invite" && (q.initiator!=current.initiator || q.receiver!=current.receiver))) throw new InvalidOperationException("Trade receipt identity differs");
                    Apply(q); Store(owner,"pending",null); pending=null; warning=false; dirty=true; next=0;
                }
                catch (Exception) { Warn("交易结果待确认"); }
            };
            try
            {
                if (current.action=="invite") wire.TradeInvite(owner,current.requestId,current.target,done);
                else wire.TradeAction(owner,current.action,current.requestId,current.tradeId,current.initiator,current.receiver,current.version,current.offer,done);
            }
            catch (Exception) { Later(); Warn("交易结果待确认"); }
        }
        void Read()
        {
            FieldWire.Session owner=session; int op=generation; TradeLink expected=link; busy=true; dirty=false;
            Action<FieldWireResult> done=delegate(FieldWireResult r)
            {
                if (!Still(owner,op)) return; Later();
                if (r.uncertain || r.code!=0 || r.data==null) { dirty=true; return; }
                try
                {
                    TradeQuote q=Parse(owner,r.data);
                    if (expected!=null && (q.status=="none" || q.id!=expected.tradeId || q.initiator!=expected.initiator || q.receiver!=expected.receiver)) throw new InvalidOperationException("Trade status identity differs");
                    if (q.status=="none") { quote=null; TradePanel.CloseLocal(); return; }
                    Apply(q); refreshAt=Stopwatch.GetTimestamp()+5L*Stopwatch.Frequency;
                }
                catch (Exception) { dirty=true; Warn("交易状态待确认"); }
            };
            try { if (expected==null) wire.TradeCurrent(owner,done); else wire.TradeStatus(owner,expected.tradeId,expected.initiator,expected.receiver,done); }
            catch (Exception) { Later(); dirty=true; }
        }
        void Apply(TradeQuote q)
        {
            TradeLink nextLink=new TradeLink {schema=1,playerId=session.playerId,origin=session.origin,tradeId=q.id,initiator=q.initiator,receiver=q.receiver};
            if (!ValidLink(session,nextLink) || link!=null && (link.tradeId!=q.id || link.initiator!=q.initiator || link.receiver!=q.receiver)) throw new InvalidOperationException("Trade changed");
            if (quote!=null && quote.id==q.id && q.version<quote.version) throw new InvalidOperationException("Trade version regressed");
            bool newInvitation=quote==null || quote.id!=q.id || quote.status!="invited";
            SaveLink(session,nextLink); link=nextLink; quote=q;
            if (q.status=="invited" && session.playerId==q.initiator && newInvitation) Notice("交易邀请已发送");
            if (q.status=="invited" && session.playerId==q.receiver && !showInvitation)
            {
                showInvitation=true; string id=q.id; FieldWire.Session owner=session;
                BoxManager.CreateMessageBoxWithCofirmAndCancelButton("收到芯片交易邀请，是否接受？",delegate(bool yes)
                { showInvitation=false; if (SameSession(session,owner) && quote!=null && quote.id==id && quote.status=="invited") Action(yes ? "accept":"cancel",null); });
            }
            else if (q.status=="open") TradePanel.Show(q,session.playerId,pending!=null);
            else if (q.status=="settled" || q.status=="cancelled" || q.status=="expired")
            {
                TradePanel.CloseLocal();
                if (StaticData.fieldId=="field_003") FieldRuntime.Require().RequestSnapshot();
                Notice(q.status=="settled" ? "芯片交易完成" : q.status=="expired" ? "交易已超时" : "交易已取消");
                Store(session,"link",null); link=null; quote=null; dirty=true;
            }
        }
    }
}
