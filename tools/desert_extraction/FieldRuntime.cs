using System;
using System.Collections.Generic;
using System.Globalization;
using System.Diagnostics;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Entry points intended for narrowly scoped 1.0.78 method prefixes.
    // Returning true means the original private resource path must be skipped.
    public static class FieldHooks
    {
        static bool Target() { return StaticData.fieldId=="field_003"; }
        public static bool ResourceChanged()
        {
            if (!Target()) return false;
            FieldRuntime.Require().RequestSnapshot(); return true;
        }
        public static bool ResourceCheck()
        {
            if (!Target()) return false;
            // Native checks expect private resource IDs; public changes use the
            // server notification and a complete snapshot instead.
            FieldRuntime.Require(); return true;
        }
        public static bool Gather(string groupId,string nativePosId)
        {
            if (!Target()) return false;
            FieldRuntime.Require().Trigger(groupId,nativePosId); return true;
        }
        public static bool MonsterChanged()
        {
            if (!Target()) return false;
            FieldRuntime.Require().RequestSnapshot(); return true;
        }
        public static void MonsterPatrolTick(object component)
        {
            if (!Target()) HYJS.Field63.Runtime.PatrolTick(component);
            // Public patrol needs server-authoritative shared movement. The
            // current development preview deliberately keeps models stationary.
        }
        public static bool OpenNpc(RCUINormalNPC npc)
        {
            if (!Target() || npc==null || npc.pageName!=FieldNpcs.PageMarker) return false;
            FieldRuntime.Require().OpenNpc(npc); return true;
        }
        public static bool BeforeEncounter(int type,string uuid)
        {
            if (!Target()) return false;
            return !FieldRuntime.Require().StartEncounter(type,uuid);
        }
        public static void FightStartResponse(int type,object value)
        {
            if (!Target()) return;
            int code=type==6 && value is SCSidePveFightStart ? ((SCSidePveFightStart)value).code :
                type==12 && value is SCSidePveBossFightStart ? ((SCSidePveBossFightStart)value).code : 0;
            FieldRuntime.Require().EndEncounter(code);
            if (code==1) BattleRecovery.StartResponse(type,value);
        }
        public static bool PreserveBattle() { return BattleRecovery.PreserveBattle(); }
        public static void TcpConnecting() { BattleRecovery.BeforeTcpLogin(); }
        public static void TcpLoginResponse(object value) { BattleRecovery.LoginResponse(value); }
        public static void FightEndSend(int opcode,object value,object over) { BattleRecovery.SendEnd(opcode,value,over); }
        public static bool FightEndResponse(int type,object over,object value) { return BattleRecovery.InterceptEnd(type,over,value); }
        public static void LegacyBossObserve(object over,object value)
        { if (!Target()) LocalFieldSettlement.Observe(over,value); }
        public static void LegacyBossCompleted(object over,object value)
        { if (!Target()) LocalFieldSettlement.Completed(over,value); }
    }

    // Scene binding for native resource, monster and NPC actors. Public
    // movement, travel and extraction remain separate integration gates.
    public sealed class FieldRuntime : MonoBehaviour, IGatherView, ITerminalView
    {
        static FieldRuntime singleton;
        RoomBase room;
        RoomCharactor actor;
        FieldWire wire;
        FieldWire.Session session;
        FieldResources resources;
        GatherPreload preload;
        GatherCoordinator gather;
        ProtectionRelease protection;
        FieldNpcs npcs;
        FieldMonsters monsters;
        TerminalController terminal;
        bool dirty, inFlight, snapshotReady, askingProtection, awaitingEncounter, encounterTimedOut;
        int sceneSerial, requestVersion;
        long nextSnapshot, protectionDeadline, encounterDeadline;
        public static FieldRuntime Require()
        {
            if (singleton!=null) return singleton;
            GameObject host=new GameObject("HYJS.DesertV4.FieldRuntime");
            DontDestroyOnLoad(host);
            singleton=host.AddComponent<FieldRuntime>();
            singleton.wire=host.AddComponent<FieldWire>();
            singleton.dirty=true;
            return singleton;
        }
        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        void Update()
        {
            RoomBase current=StaticData.fieldId=="field_003" ? RoomManager.currentRoom : null;
            RoomCharactor self=null;
            try { if (current!=null && current.charactorManager!=null) self=current.charactorManager.GetSelfCharactor(); } catch (Exception) { }
            FieldWire.Session currentSession; FieldWire.Current(out currentSession);
            if (room!=null && (current!=room || self!=actor || !Same(currentSession,session))) ReleaseScene();
            if (current==null || self==null || currentSession==null) return;
            if (room==null)
            {
                try
                {
                    RoomNetField net=current.netManager as RoomNetField;
                    if (net==null) return;
                    var nextResources=new FieldResources(current.charactorManager);
                    var nextNpcs=new FieldNpcs(current.charactorManager);
                    var nextMonsters=new FieldMonsters(net,current.charactorManager);
                    var nextTerminal=new TerminalController(wire,currentSession,this);
                    var nextPreload=new GatherPreload(this,self);
                    var nextNative=new NativeGatherUI(self,nextPreload);
                    var nextProtection=new ProtectionRelease(wire,currentSession,ProtectionNotice);
                    var nextGather=new GatherCoordinator(wire,currentSession,nextResources,self,nextPreload,nextNative,this);
                    room=current; actor=self; session=currentSession; resources=nextResources; preload=nextPreload;
                    protection=nextProtection; gather=nextGather; npcs=nextNpcs; monsters=nextMonsters; terminal=nextTerminal;
                    sceneSerial++; dirty=true; requestVersion++;
                    if (protection.Pending) protection.RetryPending();
                    BattleRecovery.SceneReady();
                }
                catch (Exception) { return; } // actor assets/session may not yet be ready
            }
            try { preload.Tick(); gather.Tick(); } catch (Exception) { Notice("采集状态需要确认，请重新进入"); ReleaseScene(); return; }
            if (awaitingEncounter && !encounterTimedOut && Stopwatch.GetTimestamp()>=encounterDeadline)
            {
                encounterTimedOut=true;
                if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
                Notice("战斗响应尚待确认，请检查连接");
            }
            if (dirty && !inFlight && Stopwatch.GetTimestamp()>=nextSnapshot) StartSnapshot();
        }
        void ReleaseScene()
        {
            sceneSerial++; inFlight=false; dirty=true; nextSnapshot=0; snapshotReady=false;
            awaitingEncounter=false; encounterTimedOut=false; askingProtection=false; protectionDeadline=0;
            if (gather!=null) try { gather.Suspend(); } catch (Exception) { }
            if (protection!=null) protection.Suspend();
            if (resources!=null) try { resources.Clear(); } catch (Exception) { }
            if (monsters!=null) try { monsters.Clear(); } catch (Exception) { }
            if (npcs!=null) try { npcs.Clear(); } catch (Exception) { }
            if (terminal!=null) terminal.Suspend();
            if (preload!=null) try { preload.Dispose(); } catch (Exception) { }
            room=null; actor=null; session=null; resources=null; preload=null; gather=null; protection=null;
            npcs=null; monsters=null; terminal=null;
        }
        void OnDestroy() { ReleaseScene(); if (singleton==this) singleton=null; }
        public void Trigger(string groupId,string posId)
        {
            if (gather==null) { Notice("沙漠资源准备中，请稍候"); return; }
            gather.Trigger(groupId,posId);
        }
        public void RequestSnapshot() { dirty=true; requestVersion++; }
        void StartSnapshot()
        {
            if (session==null || room==null) return;
            int serial=sceneSerial, version=requestVersion; FieldWire.Session owner=session;
            inFlight=true; dirty=false;
            try
            {
                wire.Snapshot(owner,delegate(FieldWireResult result)
                {
                    if (serial!=sceneSerial || !Same(session,owner)) return;
                    inFlight=false;
                    // A later notification/claim invalidates the entire older
                    // response, including private balances with equal revision.
                    if (version!=requestVersion) { dirty=true; return; }
                    if (result.uncertain || result.code!=0 || result.data==null) { dirty=true; nextSnapshot=Stopwatch.GetTimestamp()+2*Stopwatch.Frequency; return; }
                    try
                    {
                        var update=ParseMaterials(result.data);
                        long nowMs=Int64.Parse(result.data["serverTimeMs"].ToString(),CultureInfo.InvariantCulture);
                        long untilMs=Int64.Parse(result.data["protectionUntilMs"].ToString(),CultureInfo.InvariantCulture);
                        if (nowMs<0 || untilMs<0 || untilMs>nowMs+300000) throw new InvalidOperationException("Invalid protection expiry");
                        long remain=Math.Max(0,untilMs-nowMs);
                        resources.ApplySnapshot(result.data);
                        monsters.ApplySnapshot(result.data);
                        npcs.ApplySnapshot(result.data);
                        terminal.ApplySnapshot(result.data);
                        protectionDeadline=Stopwatch.GetTimestamp()+remain*Stopwatch.Frequency/1000;
                        snapshotReady=true;
                        StaticData.playerData.UpdateSimpleItem(update.items);
                        StaticData.playerData.Tiredness=update.fatigue;
                    }
                    catch (Exception) { dirty=true; nextSnapshot=Stopwatch.GetTimestamp()+5*Stopwatch.Frequency; Notice("沙漠资源同步失败，请稍候"); }
                });
            }
            catch (Exception) { inFlight=false; dirty=true; nextSnapshot=Stopwatch.GetTimestamp()+2*Stopwatch.Frequency; }
        }
        sealed class Balances { public List<SimpleItem> items; public int fatigue; }
        static Balances ParseMaterials(JsonData data)
        {
            if (StaticData.playerData==null) throw new InvalidOperationException("Player inventory unavailable");
            long tired=Int64.Parse(data["fatigue"].ToString(),CultureInfo.InvariantCulture);
            if (tired<0 || tired>Int32.MaxValue) throw new InvalidOperationException("Invalid fatigue");
            var result=new Balances(); result.fatigue=(int)tired; result.items=new List<SimpleItem>();
            var seen=new HashSet<string>();
            foreach (JsonData row in data["materials"])
            {
                string id=(string)row["id"]; long count=Int64.Parse(row["amount"].ToString(),CultureInfo.InvariantCulture);
                if (String.IsNullOrEmpty(id) || !seen.Add(id) || count<0 || count>Int32.MaxValue) throw new InvalidOperationException("Invalid material balance");
                if (id.StartsWith("FishItem_",StringComparison.Ordinal))
                { if (!StaticData.playerData.FishItems.ContainsKey(id)) throw new InvalidOperationException("Unknown native fish ID"); }
                else if (id.StartsWith("MineItem_",StringComparison.Ordinal))
                { if (StaticData.playerData.GetMineItem(id)==null) throw new InvalidOperationException("Unknown native mine ID"); }
                else throw new InvalidOperationException("Unexpected material ID");
                var item=new SimpleItem(); item.id=id; item.num=(int)count; result.items.Add(item);
            }
            if (result.items.Count==0) throw new InvalidOperationException("Empty material snapshot");
            return result;
        }
        public void OpenNpc(RCUINormalNPC npc)
        {
            FieldNpcs.Point p=npcs==null || npc==null ? null : npcs.Find(npc.owner);
            if (p==null || actor==null) { Notice("向导正在同步，请稍候"); RequestSnapshot(); return; }
            if ((actor.transform.position-p.interactionPosition).sqrMagnitude>p.radius*p.radius)
            { Notice("请靠近后再交互"); return; }
            if (p.role=="settlement" && terminal!=null) terminal.Open();
            else Notice(p.role=="teleport" ? "传送正在准备" : "撤离正在准备");
        }
        public bool StartEncounter(int type,string uuid)
        {
            if ((type!=6 && type!=12) || String.IsNullOrEmpty(uuid) || actor==null || !snapshotReady)
            { Notice("场景正在同步，请稍候"); RequestSnapshot(); return false; }
            if (BattleRecovery.HasUnsettled()) { Notice("上次战斗结果尚待确认"); return false; }
            if (awaitingEncounter) return false;
            if (Stopwatch.GetTimestamp()<protectionDeadline)
            { AskProtectionForAttack(); return false; }
            if (LoadingPanel.Instance==null) { Notice("战斗画面正在准备"); return false; }
            awaitingEncounter=true; encounterTimedOut=false; encounterDeadline=Stopwatch.GetTimestamp()+15L*Stopwatch.Frequency;
            LoadingPanel.Instance.ShowLoadingPre();
            return true;
        }
        public void EndEncounter(int code)
        {
            if (!awaitingEncounter) return;
            awaitingEncounter=false; encounterTimedOut=false;
            if (code!=1 && LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
            if (code==2007) { RequestSnapshot(); AskProtectionForAttack(); }
            else if (code!=1) { Notice("战斗尚未开始，请重试"); RequestSnapshot(); }
        }
        void ProtectionNotice(string text) { Notice(text); RequestSnapshot(); }
        void AskProtectionForAttack()
        {
            if (askingProtection) return;
            askingProtection=true; int serial=sceneSerial;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("行动将解除保护，确认后请再次触碰目标。",delegate(bool confirmed)
            {
                if (serial!=sceneSerial) return;
                askingProtection=false;
                if (confirmed && protection!=null) protection.Confirm();
            });
        }
        public void Notice(string text) { BoxManager.CreatePopupTextBox(text,0f); }
        public void AskToReleaseProtection()
        {
            int serial=sceneSerial;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("采集将解除保护，确认后请再次点击目标。",delegate(bool confirmed)
            { if (confirmed && serial==sceneSerial && protection!=null) protection.Confirm(); });
        }
        public void AskToRetryDelivery()
        {
            int serial=sceneSerial;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("请先整理背包，确认后重试领取上次采集。",delegate(bool confirmed)
            { if (confirmed && serial==sceneSerial && gather!=null) gather.RetryPendingDelivery(); });
        }
        public void ApplyOriginalReceipt(JsonData receipt)
        {
            // Receipt quantities are display-only deltas. The next snapshot
            // provides the current absolute balances, even on replay/restart.
            if (receipt==null || receipt["resources"]==null) throw new InvalidOperationException("Missing original receipt");
            Notice("采集完成"); RequestSnapshot();
        }
    }
}
