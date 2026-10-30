using System;
using System.Collections;
using System.Diagnostics;
using System.Reflection;
using LitJson;
using ProtoBuf.Message;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Retains a live native battle during a socket reconnect, and reconciles
    // its original end request against the server before displaying a receipt.
    // A killed process cannot reconstruct a still-running engine here.
    public sealed class BattleRecovery : MonoBehaviour
    {
        static BattleRecovery singleton;
        FieldWire wire;
        FieldWire.Session session;
        string liveFightId="",inputHash="";
        int liveType;
        object nativeOwner;
        bool query,delivering,loginReady,holdWarning;
        int connectionEpoch;
        object unrecordedMessage,unrecordedOwner;
        int unrecordedOpcode;
        string unrecordedFight;
        FieldWire.Session unrecordedSession;
        long nextQuery;
        FightEndRecord pending;
        string resumedFight="",resumedHash="";
        int resumedType;
        static MonsterRewardView monsterRewardView;
        static FieldWire.Session monsterRewardOwner;
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        public static ResultObject MonsterRewardDisplay(ResultObject original)
        {
            FieldWire.Session owner;
            if (!Target() || monsterRewardView==null || !FieldWire.Current(out owner) || !SameActor(owner,monsterRewardOwner) ||
                StaticData.curBattleData==null || (int)StaticData.curBattleData.battleType!=6) return original;
            ResultObject projected=monsterRewardView.Project(StaticData.curBattleData.fightId,original);
            if (System.Object.ReferenceEquals(projected,original)) return original;
            try { TradePanel.RegisterIcons(); return projected; }
            catch (Exception) { try { Notice("芯片已存入临时终端"); } catch (Exception) { } return original; }
        }
        static bool Target() { return StaticData.fieldId=="field_003"; }
        static bool Hex(string value,int length)
        {
            if (value==null || value.Length!=length) return false;
            foreach(char c in value) if (!((c>='0'&&c<='9')||(c>='a'&&c<='f'))) return false;
            return true;
        }
        static string Optional(JsonData value,string key)
        {
            var d=value as IDictionary;
            return d!=null && d.Contains(key) && value[key]!=null ? value[key].ToString() : "";
        }
        static int TypeFromOpcode(int op) { return op==30049 ? 6 : op==30085 ? 12 : 0; }
        static int NativeResponseOpcode(int type) { return type==6 ? 30050 : 30086; }
        public static BattleRecovery Require()
        {
            if (singleton!=null) return singleton;
            var host=new GameObject("HYJS.DesertV4.BattleRecovery"); DontDestroyOnLoad(host);
            singleton=host.AddComponent<BattleRecovery>(); singleton.wire=host.AddComponent<FieldWire>();
            return singleton;
        }
        void OnDestroy() { if (singleton==this) singleton=null; }
        static void Notice(string message) { BoxManager.CreatePopupTextBox(message,0f); }
        void Warning(string message) { if (!holdWarning) { holdWarning=true; Notice(message); } }
        void ResetWarning() { holdWarning=false; }
        static bool CurrentBattleMatches(int type,string fightId)
        {
            return Target() && StaticData.curBattleData!=null && (int)StaticData.curBattleData.battleType==type &&
                StaticData.curBattleData.fightId==fightId && Battle.BattleManager.Instance!=null &&
                Battle.BattleManager.Instance.enabled && Battle.BattleManager.Instance.battleEngine!=null;
        }
        bool AdoptSession()
        {
            FieldWire.Session now; if (!FieldWire.Current(out now)) return false;
            if (!SameSession(now,session))
            {
                if (!SameActor(now,session))
                {
                    liveFightId=""; inputHash=""; liveType=0; nativeOwner=null; loginReady=false;
                    resumedFight=resumedHash=""; resumedType=0;
                }
                query=false; nextQuery=0; session=now;
                try { pending=FightEndJournal.Load(now); }
                catch (Exception) { pending=null; Warning("战斗记录需要恢复，请勿开始新战斗"); return false; }
            }
            return true;
        }
        public static bool HasEndJournal()
        {
            if (!Target()) return false;
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return true;
            if (singleton!=null && singleton.unrecordedMessage!=null && SameActor(singleton.unrecordedSession,owner)) return true;
            try { return FightEndJournal.Load(owner)!=null; }
            catch (Exception) { return true; }
        }
        public static bool HasUnsettled() { return HasEndJournal() || ColdBattleRecovery.Pending; }
        public static bool LivePresent
        { get { return singleton!=null && !String.IsNullOrEmpty(singleton.liveFightId) && CurrentBattleMatches(singleton.liveType,singleton.liveFightId); } }
        public static void AdmitResume(int type,string fight,string hash)
        {
            if ((type!=6 && type!=12) || !Hex(fight,24) || !Hex(hash,64)) throw new InvalidOperationException("Invalid accepted fight");
            BattleRecovery core=Require();
            if (!core.AdoptSession() || core.pending!=null) throw new InvalidOperationException("Earlier original end pending");
            core.resumedType=type; core.resumedFight=fight; core.resumedHash=hash;
        }
        public static bool PreserveBattle()
        {
            if (!Target() || singleton==null || String.IsNullOrEmpty(singleton.liveFightId)) return false;
            return CurrentBattleMatches(singleton.liveType,singleton.liveFightId);
        }
        public static void BeforeTcpLogin()
        { if (singleton!=null) { singleton.connectionEpoch++; singleton.loginReady=false; singleton.query=false; } }
        public static void SceneReady()
        {
            if (!Target() || !NetAPI.SocketIsConnected()) return;
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return;
            try { if (FightEndJournal.Load(owner)==null) return; }
            catch (Exception) { Notice("战斗记录需要恢复，请勿开始新战斗"); return; }
            BattleRecovery core=Require();
            if (core.AdoptSession()) { core.loginReady=true; core.nextQuery=0; }
        }
        public static void LoginResponse(object obj)
        {
            var response=obj as SCSocketLogin;
            if (response==null || response.result!=100) return;
            if (singleton==null)
            {
                FieldWire.Session owner;
                if (!FieldWire.Current(out owner)) return;
                try { if (FightEndJournal.Load(owner)==null) return; }
                catch (Exception) { Notice("战斗记录需要恢复，请勿开始新战斗"); return; }
                Require();
            }
            if (!singleton.AdoptSession()) return;
            singleton.loginReady=true; singleton.query=false; singleton.nextQuery=0;
        }
        public static void StartResponse(int type,object obj)
        {
            if (!Target()) return;
            string id="";
            if (type==6)
            {
                var r=obj as SCSidePveFightStart; if (r==null || r.code!=1) return;
                id=r.fightId;
            }
            else if (type==12)
            {
                var r=obj as SCSidePveBossFightStart; if (r==null || r.code!=1) return;
                id=r.fightId;
            }
            if (!Hex(id,24)) { Notice("战斗身份待确认，请稍候"); return; }
            BattleRecovery core=Require(); if (!core.AdoptSession()) return;
            if (core.pending!=null && (core.pending.fightId!=id || TypeFromOpcode(core.pending.opcode)!=type))
            { core.Warning("上次战斗结果尚待确认"); return; }
            core.liveFightId=id; core.liveType=type;
            core.inputHash=core.resumedType==type && core.resumedFight==id ? core.resumedHash : ""; core.nativeOwner=null;
            core.loginReady=true; core.query=false; core.nextQuery=0; core.ResetWarning();
        }
        public static void SendEnd(int opcode,object message,object over)
        {
            int type=TypeFromOpcode(opcode);
            if (!Target() || type==0) { NetAPI.SendSocket(opcode,message); return; }
            BattleRecovery core=Require();
            try
            {
                if (!core.AdoptSession() || core.liveType!=type || String.IsNullOrEmpty(core.liveFightId) ||
                    StaticData.curBattleData==null || StaticData.curBattleData.fightId!=core.liveFightId)
                    throw new InvalidOperationException("Original fight context differs");
                string actual=type==6 ? (message as CSSidePveFightEnd)==null ? "" : ((CSSidePveFightEnd)message).fightId :
                    (message as CSSidePveBossFightEnd)==null ? "" : ((CSSidePveBossFightEnd)message).fightId;
                if (actual!=core.liveFightId) throw new InvalidOperationException("Original end identity differs");
                core.unrecordedMessage=message; core.unrecordedOwner=over; core.unrecordedOpcode=opcode;
                core.unrecordedFight=core.liveFightId; core.unrecordedSession=core.session;
                core.TryCapture();
                // Reconciliation checks active/receipt before every send. The
                // native callback remains registered while the response is pending.
            }
            catch (Exception) { core.Warning("战斗结果保存失败，请保持当前界面并重试"); }
        }
        void TryCapture()
        {
            if (unrecordedMessage==null || !SameActor(unrecordedSession,session) ||
                unrecordedFight!=liveFightId || TypeFromOpcode(unrecordedOpcode)!=liveType) return;
            try
            {
                pending=FightEndJournal.Capture(session,unrecordedOpcode,unrecordedMessage,inputHash,
                    resumedType==liveType && resumedFight==liveFightId && resumedHash==inputHash);
                if (pending.fightId!=liveFightId) throw new InvalidOperationException("End fight ID differs");
                nativeOwner=unrecordedOwner; unrecordedMessage=null; unrecordedOwner=null; unrecordedSession=null;
                nextQuery=0; ResetWarning();
            }
            catch (Exception) { Warning("战斗结果保存失败，请保持当前界面并重试"); nextQuery=Stopwatch.GetTimestamp()+2L*Stopwatch.Frequency; }
        }
        public static bool InterceptEnd(int type,object owner,object message)
        {
            if (!Target() || singleton==null) return false;
            BattleRecovery core=singleton;
            if (core.delivering) return false;
            if (core.liveType!=type || String.IsNullOrEmpty(core.liveFightId)) return false;
            if (core.pending==null || core.pending.fightId!=core.liveFightId || !CurrentBattleMatches(type,core.liveFightId))
            { core.Warning("战斗结果待服务器确认"); return true; }
            core.nativeOwner=owner; core.nextQuery=0;
            // The native response has no fightId. Use the authoritative receipt
            // for this journal instead of attributing the packet by arrival time.
            return true;
        }
        void Update()
        {
            if (!AdoptSession()) return;
            if (unrecordedMessage!=null && Stopwatch.GetTimestamp()>=nextQuery) TryCapture();
            if (!NetAPI.SocketIsConnected()) loginReady=false;
            if (query || Stopwatch.GetTimestamp()<nextQuery || !loginReady || !NetAPI.SocketIsConnected()) return;
            if (pending!=null && DifferentLiveFight(pending)) return;
            if (pending!=null) QueryReceipt();
            else if (!String.IsNullOrEmpty(liveFightId) && String.IsNullOrEmpty(inputHash)) QueryActive(null);
        }
        static bool DifferentLiveFight(FightEndRecord expected)
        {
            if (expected==null || Battle.BattleManager.Instance==null || !Battle.BattleManager.Instance.enabled ||
                Battle.BattleManager.Instance.battleEngine==null || StaticData.curBattleData==null) return false;
            return StaticData.curBattleData.fightId!=expected.fightId ||
                (int)StaticData.curBattleData.battleType!=TypeFromOpcode(expected.opcode);
        }
        void Later() { query=false; nextQuery=Stopwatch.GetTimestamp()+2L*Stopwatch.Frequency; }
        bool Still(FieldWire.Session owner,FightEndRecord r)
        {
            return SameSession(owner,session) && pending!=null && r!=null &&
                pending.fightId==r.fightId && pending.opcode==r.opcode && pending.payloadHash==r.payloadHash;
        }
        void QueryActive(FightEndRecord expected)
        {
            int type=expected==null ? liveType : TypeFromOpcode(expected.opcode);
            string fight=expected==null ? liveFightId : expected.fightId;
            FieldWire.Session owner=session; int epoch=connectionEpoch; query=true;
            try
            {
                wire.FightActive(owner,type,delegate(FieldWireResult result)
                {
                    if (epoch!=connectionEpoch || !SameSession(owner,session) || (expected!=null && !Still(owner,expected))) return;
                    Later();
                    if (!loginReady || result.uncertain || result.code!=0 || result.data==null) return;
                    try
                    {
                        JsonData d=result.data;
                        if (!d["active"].IsBoolean || !(bool)d["active"]) { if (expected!=null) Warning("战斗状态待确认"); return; }
                        string id=(string)d["fightId"], hash=(string)d["inputHash"];
                        if (id!=fight || !Hex(hash,64)) { Warning("战斗身份不匹配，已暂停恢复"); return; }
                        if (type==12 && expected!=null)
                        {
                            var msg=FightEndJournal.Message(owner,expected) as CSSidePveBossFightEnd;
                            if (msg==null || Optional(d,"eventId")!=msg.uuid) { Warning("首领身份不匹配，已暂停恢复"); return; }
                        }
                        if (expected==null)
                        {
                            if (liveFightId!=id || liveType!=type) return;
                            inputHash=hash; ResetWarning(); return;
                        }
                        pending=FightEndJournal.Bind(owner,expected,hash); inputHash=hash; ResetWarning();
                        if (!loginReady || !NetAPI.SocketIsConnected()) return;
                        object original=FightEndJournal.Message(owner,pending);
                        NetAPI.SendSocket(pending.opcode,original);
                    }
                    catch (Exception) { Warning("战斗状态待确认"); }
                });
            }
            catch (Exception) { Later(); }
        }
        void QueryReceipt()
        {
            FieldWire.Session owner=session; FightEndRecord expected=pending; int type=TypeFromOpcode(expected.opcode), epoch=connectionEpoch; query=true;
            try
            {
                wire.FightReceipt(owner,type,expected.fightId,delegate(FieldWireResult result)
                {
                    if (epoch!=connectionEpoch || !Still(owner,expected)) return;
                    Later();
                    if (!loginReady || DifferentLiveFight(expected) || result.uncertain || result.code!=0 || result.data==null) return;
                    try
                    {
                        JsonData d=result.data; string status=Optional(d,"status");
                        if (String.IsNullOrEmpty(status) && type==12) status=Optional(d,"state");
                        if (status=="active" || status=="unknown") { QueryActive(expected); return; }
                        if (status!="settled") { Warning("战斗结果待确认"); return; }
                        string hash=Optional(d,"inputHash"), endHash=Optional(d,"endRequestHash"), encoded=Optional(d,"nativeEnd");
                        if (!Hex(hash,64) || Optional(d,"fightId")!=expected.fightId ||
                            (!String.IsNullOrEmpty(expected.inputHash) && hash!=expected.inputHash) ||
                            (!expected.resumeApproved && endHash!=expected.payloadHash) || !Hex(endHash,64) || String.IsNullOrEmpty(encoded))
                        { Warning("战斗回执不匹配，已暂停恢复"); return; }
                        pending=FightEndJournal.Bind(owner,expected,hash);
                        byte[] bytes=Convert.FromBase64String(encoded);
                        if (bytes.Length==0 || bytes.Length>262144) throw new InvalidOperationException("Invalid native receipt");
                        object native=PBConvert.toObject(NativeResponseOpcode(type),bytes);
                        if ((type==6 && !(native is SCSidePveFightEnd)) || (type==12 && !(native is SCSidePveBossFightEnd)))
                            throw new InvalidOperationException("Wrong native receipt type");
                        if (type==6)
                        {
                            // The native response stays untouched for its later inventory update.
                            // Receipt data is bound to this owner and fight for display only.
                            monsterRewardView=null; monsterRewardOwner=null;
                            try { monsterRewardView=MonsterRewardView.FromReceipt(expected.fightId,d); monsterRewardOwner=owner; }
                            catch (Exception) { }
                        }
                        if (CurrentBattleMatches(type,expected.fightId))
                        {
                            if (nativeOwner==null || (type==6 && !(nativeOwner is Battle.FieldPVEBattleOver)) ||
                                (type==12 && !(nativeOwner is Battle.FieldPVEBossBattleOver)))
                            { Warning("战斗界面仍在恢复，请稍候"); return; }
                            string name=type==6 ? "FieldPVEFightResultHandler" : "FieldPVEBossFightResultHandler";
                            MethodInfo method=nativeOwner.GetType().GetMethod(name,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
                            if (method==null) throw new InvalidOperationException("Original result handler missing");
                            delivering=true;
                            try { method.Invoke(nativeOwner,new object[]{native}); }
                            finally { delivering=false; }
                        }
                        else Notice("上次战斗结果已确认");
                        FightEndJournal.Clear(owner,pending);
                        pending=null; nativeOwner=null; liveFightId=""; liveType=0; inputHash=""; ResetWarning();
                        resumedFight=resumedHash=""; resumedType=0;
                        FieldRuntime.Require().RequestSnapshot();
                    }
                    catch (Exception) { Warning("战斗结果待确认"); }
                });
            }
            catch (Exception) { Later(); }
        }
    }
}
