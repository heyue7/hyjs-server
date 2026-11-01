using System;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using System.Diagnostics;
using LitJson;
using ProtoBuf.Message;
using Room;
using UI;
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
        public static bool ResourceNotification(object value)
        {
            if (!Target()) return false;
            SCFieldResourceInfo packet=value as SCFieldResourceInfo;
            if (packet!=null && (packet.resource==null || packet.resource.Count==0)) TradeController.SignalChange();
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
            // The shared position is advanced by FieldMonsters from the
            // authoritative path and clock, never by the old random wander.
        }
        public static bool MonsterMoving(RoomCharactor actor)
        {
            if (actor==null) return false;
            return (Target() && FieldRuntime.PatrolMoving(actor)) || actor.IsMoving();
        }
        public static bool OpenPlayer(RCUIPlayer player)
        {
            if (!Target() || !(RoomManager.currentRoom is RoomField) || player==null) return false;
            return FieldRuntime.Require().OpenPlayer(player);
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
        public static void RoomLoaded(object value)
        { try { FieldRuntime.NativeRoomLoaded(value as RoomBase); } catch (Exception) { } }
        public static void FieldPositionReady(object net,object value)
        { try { FieldRuntime.NativePositionReady(net,value as SCCurrentPosition); } catch (Exception) { } }
        public static void PlayerDataReady(object original)
        {
            try
            {
                SCPlayerLogin login=original as SCPlayerLogin;
                Player player=login==null ? null : login.player;
                if (player!=null && StaticData.playerData!=null && StaticData.playerData.playerId==player.playerId)
                    StaticData.playerData.teamId=player.teamId ?? "";
            }
            catch (Exception) { }
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
        public static void TcpConnecting() { ChipNames.ReleaseScene(); PositionSync.ConnectionStarting(); TeamMap86.ConnectionStarting(); BattleRecovery.BeforeTcpLogin(); PvpBridge.ConnectionStarting(); }
        public static bool TransferSend(int opcode,object value)
        {
            if (opcode==30026 && BattleRecovery.ReturningAfterDefeat) return true;
            if (PvpBridge.Capture(opcode,value)) return true;
            if (Target() && (opcode==30032 || opcode==30061) && PvpBridge.HasPending())
            { BoxManager.CreatePopupTextBox("上次挑战仍在确认",0f); return true; }
            bool intercepted=LeaveController.Send(opcode,value);
            if (!intercepted && Target() && (opcode==30032 || opcode==30061)) ChipNames.ReleaseScene();
            return intercepted;
        }
        public static bool PvpResponse(object value) { return PvpBridge.Intercept(value); }
        public static bool TransferResponse(int opcode,object owner,object value) { return LeaveController.Response(opcode,owner,value); }
        public static void TrackPositionBase(object net,object value) { PositionSync.TrackBaseResponse(net,value); LeaveController.SceneReady(); }
        public static void TrackPositionField(object value) { PositionSync.TrackFieldResponse(value); LeaveController.SceneReady(); }
        public static void TrackPositionPacket(object value) { PositionSync.TrackPacket(value); }
        public static bool DropPositionPacket(object value) { return PositionSync.DropPacket(value); }
        public static bool DropOldSocket(object tool,object value) { return PositionSync.DropOldSocket(tool,value as IAsyncResult); }
        public static bool BlockMove(RoomCharactor actor)
        {
            if (PositionSync.BlocksMove(actor)) return true;
            RoomBase room=RoomManager.currentRoom;
            if (Target() && room!=null && room.charactorManager!=null && room.charactorManager.GetSelfCharactor()==actor)
            {
                if (PvpBridge.HasPending()) return true;
                TravelController.InterruptExtraction();
            }
            return false;
        }
        public static void TcpLoginResponse(object value) { PositionSync.LoginResponse(value); BattleRecovery.LoginResponse(value); PvpBridge.LoginResponse(value); }
        public static void FightEndSend(int opcode,object value,object over) { BattleRecovery.SendEnd(opcode,value,over); }
        public static bool FightEndResponse(int type,object over,object value) { return BattleRecovery.InterceptEnd(type,over,value); }
        public static ResultObject MonsterRewardDisplay(ResultObject original) { return BattleRecovery.MonsterRewardDisplay(original); }
        public static void LegacyBossObserve(object over,object value)
        { if (!Target()) LocalFieldSettlement.Observe(over,value); }
        public static void LegacyBossCompleted(object over,object value)
        { if (!Target()) LocalFieldSettlement.Completed(over,value); }
    }

    // Reuse the field's original explanation page and button for the desert.
    // This state is only kept in memory for the current visit.
    sealed class DesertGuide
    {
        const string Text = "【沙漠玩法】\n\n" +
            "1. 在安全区找到诗乃，随机传送到沙漠。落地后30秒内不会被攻击；采集或战斗时可确认提前结束保护。\n\n" +
            "2. 同一线路的玩家共享小怪、矿和鱼。击败小怪、采矿或钓鱼有机会获得芯片；采集还可获得材料，并消耗疲劳。\n\n" +
            "3. 芯片暂存在临时终端。安全区里点击其他玩家，可以交易芯片。\n\n" +
            "4. 战败时按携带芯片总数的一半向下取整扣除。品质最高的3枚不会丢失；其余芯片不够扣时，只扣其余的。\n\n" +
            "5. 被其他玩家击败时，对方获得你丢失的芯片。战败后返回安全区，剩余芯片仍在终端里。\n\n" +
            "6. 每条线路有一只首领，未被击败时可持续挑战；被击败后1小时刷新。\n\n" +
            "7. 对首领造成伤害可获得参与奖励，伤害排名和最后一击还有额外奖励。野外首领奖励每天共可领取3次。\n\n" +
            "8. 找到撤离点并等待10秒即可返回安全区；移动、受击或行动会打断撤离。\n\n" +
            "9. 回到安全区找柯尔，把芯片兑换成信用点。确认返城或前往其他野外地图时，芯片也会自动结算。";

        bool shown;
        long playerId;
        string origin;

        public void Observe(string fieldId,FieldWire.Session owner,bool inTown)
        {
            if (fieldId!="field_003" || inTown) { shown=false; playerId=0; origin=null; return; }
            if (owner==null) return;
            if (playerId!=owner.playerId || origin!=owner.origin)
            { shown=false; playerId=owner.playerId; origin=owner.origin; }
        }

        void Apply(DescriptionPage page)
        {
            if (page==null || page.text==null) return;
            page.SetString(Text);
            shown=true;
            try { if (page.scroll!=null) page.scroll.ResetPosition(); } catch (Exception) { }
        }

        public void Tick(bool ready)
        {
            if (PageManager.instance==null) return;
            DescriptionPage description=PageManager.instance.currentPage as DescriptionPage;
            if (description!=null && description.text!=null)
            {
                string original=TableTextBlo.GetText("field_des");
                if (original!=null && description.text.text==original.Replace("\\r","\r").Replace("\\n","\n")) Apply(description);
                return;
            }
            if (shown || !ready) return;
            UIFieldRunMap page=PageManager.instance.currentPage as UIFieldRunMap;
            if (page==null || page.uiEvent==null) return;
            page.uiEvent.Explain();
            Apply(PageManager.instance.currentPage as DescriptionPage);
        }
    }

    // Scene binding for native resource, monster and NPC actors. Public
    // movement, travel and extraction remain separate integration gates.
    public sealed class FieldRuntime : MonoBehaviour, IGatherView, ITerminalView
    {
        static FieldRuntime singleton;
        static RoomBase loadedRoom,positionRoom;
        static object positionNet;
        static FieldWire.Session loadedOwner,positionOwner;
        static int loadedEpoch,positionEpoch;
        static readonly FieldInfo LoadingInstance=typeof(LoadingPanel).GetField("instance",BindingFlags.Static|BindingFlags.NonPublic);
        static readonly FieldInfo OpenBoxes=typeof(BoxManager).GetField("m_kOpenBox",BindingFlags.Static|BindingFlags.NonPublic);
        static readonly FieldInfo NativeTeamInfo=typeof(UIFieldRunMap).GetField("teamInfo",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
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
        FieldMonsterPosition monsterPosition;
        FieldBoss boss;
        TerminalController terminal;
        readonly DesertGuide guide=new DesertGuide();
        bool dirty, inFlight, snapshotReady, worldReady, askingProtection, awaitingEncounter, encounterTimedOut, safe;
        bool safeAreaKnown,lastLocalSafe;
        bool dispatchingConfirmedBoss,dispatchingConfirmedMonster;
        string confirmedBossId,confirmedMonsterId;
        bool teamHudChecked;
        double safeMinX,safeMaxX,safeMinZ,safeMaxZ;
        int sceneSerial, requestVersion;
        long nextSnapshot, protectionDeadline, encounterDeadline;
        static readonly MethodInfo SetNativeTired=typeof(UI.UIFieldRunMapHeroInfo).GetMethod("SetHeroTired",BindingFlags.Instance|BindingFlags.NonPublic);
        UI.UIFieldRunMap tiredHudPage;
        UI.UIFieldRunMapHeroInfo tiredHudInfo;
        int tiredHudValue=Int32.MinValue;
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
        public static bool PatrolMoving(RoomCharactor actor)
        { return singleton!=null && singleton.monsters!=null && singleton.monsters.IsPatrolling(actor); }
        public static Dictionary<string,Vector3> ExtractionMapPositions()
        {
            if (singleton==null || !singleton.worldReady || singleton.npcs==null ||
                StaticData.fieldId!="field_003" || singleton.room==null || singleton.room!=RoomManager.currentRoom)
                return new Dictionary<string,Vector3>();
            return singleton.npcs.ExtractionMapPositions();
        }
        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        public static void NativeRoomLoaded(RoomBase value)
        {
            FieldWire.Session current;
            if (!(value is RoomField) || value!=RoomManager.currentRoom || StaticData.fieldId!="field_003" || !FieldWire.Current(out current)) return;
            loadedRoom=value; loadedOwner=current; loadedEpoch=PositionSync.ConnectionEpoch;
        }
        public static void NativePositionReady(object net,SCCurrentPosition value)
        {
            RoomBase current=RoomManager.currentRoom; FieldWire.Session who;
            if (!(current is RoomField) || StaticData.fieldId!="field_003" || !System.Object.ReferenceEquals(current.netManager,net) ||
                !FieldWire.Current(out who) || value==null || value.self==null || value.self.info==null ||
                value.self.info.playerId!=who.playerId || value.self.position==null || value.self.position.scene!="field_003" ||
                value.self.position.current==null || current.charactorManager==null || current.charactorManager.GetSelfCharactor()==null || !PositionSync.ChannelReady) return;
            positionRoom=current; positionNet=net; positionOwner=who; positionEpoch=PositionSync.ConnectionEpoch;
        }
        static bool Active(GameObject value) { return value!=null && value.activeInHierarchy; }
        // Native completion callbacks are observations. Normal encounters also
        // require an authenticated public snapshot; accepted-fight discovery
        // must work while that snapshot is unavailable during a reserved fight.
        public static bool InteractionReady
        {
            get
            {
                try
                {
                    if (BattleRecovery.ReturningAfterDefeat) return false;
                    FieldWire.Session who;
                    if (singleton==null || singleton.room==null || singleton.room!=RoomManager.currentRoom || singleton.actor==null ||
                        !FieldWire.Current(out who) || !Same(singleton.session,who) || loadedRoom!=singleton.room || positionRoom!=singleton.room ||
                        !SameActor(loadedOwner,who) || !Same(positionOwner,who) ||
                        positionEpoch!=PositionSync.ConnectionEpoch || !System.Object.ReferenceEquals(positionNet,singleton.room.netManager) || !PositionSync.ChannelReady) return false;
                    UIFieldRunMap page=PageManager.instance==null ? null : PageManager.instance.currentPage as UIFieldRunMap;
                    LoadingPanel loading=LoadingInstance==null ? null : LoadingInstance.GetValue(null) as LoadingPanel;
                    if (page==null || !page.isOpen || !page.gameObject.activeInHierarchy || loading==null || loading.isLoading || loading.gameObject.activeInHierarchy ||
                        Active(BoxManager.m_kBox) || Active(BoxManager.netMask) || Active(BoxManager.netBox) || Active(BoxManager.socketBox) ||
                        MapViewer86.ViewOpen || PlayerActionPanel86.ViewOpen) return false;
                    var boxes=OpenBoxes==null ? null : OpenBoxes.GetValue(null) as System.Collections.IEnumerable;
                    int count=0;
                    if (boxes!=null) foreach (object item in boxes) { if (++count>32 || Active(item as GameObject)) return false; }
                    return true;
                }
                catch (Exception) { return false; }
            }
        }
        public static int SceneVersion { get { return singleton==null ? -1 : singleton.sceneSerial; } }
        void Update()
        {
            PlayerActionPanel86.TickInput();
            RoomBase current=StaticData.fieldId=="field_003" ? RoomManager.currentRoom : null;
            RoomCharactor self=null;
            try { if (current!=null && current.charactorManager!=null) self=current.charactorManager.GetSelfCharactor(); } catch (Exception) { }
            FieldWire.Session currentSession; FieldWire.Current(out currentSession);
            // The native town transition clears UIFieldRunMap.fieldId, while
            // StaticData.fieldId can retain the previous field until next entry.
            guide.Observe(StaticData.fieldId,currentSession,RoomManager.currentRoom is RoomTown);
            if (room!=null && (current!=room || self!=actor || !Same(currentSession,session)))
                ReleaseScene(current==room && SameActor(currentSession,session));
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
                    var nextMonsterPosition=new FieldMonsterPosition(current.charactorManager,nextMonsters);
                    var nextBoss=new FieldBoss(current.charactorManager,RequestSnapshot);
                    var nextTerminal=new TerminalController(wire,currentSession,this);
                    var nextPreload=new GatherPreload(this,self);
                    var nextNative=new NativeGatherUI(self,nextPreload);
                    var nextProtection=new ProtectionRelease(wire,currentSession,ProtectionNotice);
                    var nextGather=new GatherCoordinator(wire,currentSession,nextResources,self,nextPreload,nextNative,this);
                    room=current; actor=self; session=currentSession; resources=nextResources; preload=nextPreload;
                    protection=nextProtection; gather=nextGather; npcs=nextNpcs; monsters=nextMonsters; monsterPosition=nextMonsterPosition; boss=nextBoss; terminal=nextTerminal;
                    sceneSerial++; dirty=true; requestVersion++; teamHudChecked=false;
                    TeamMap86.BindScene(room);
                    PositionSync.SetFieldActive(true);
                    if (protection.Pending) protection.RetryPending();
                    BattleRecovery.SceneReady(); TravelController.SceneReady(); LeaveController.SceneReady(); TradeController.SceneReady(); PvpBridge.SceneReady();
                    ColdBattleRecovery.SceneReady(room,sceneSerial);
                }
                catch (Exception) { return; } // actor assets/session may not yet be ready
            }
            try { guide.Tick(InteractionReady); } catch (Exception) { }
            try { PositionSync.Tick(); preload.Tick(); gather.Tick(); if (monsters!=null) monsters.Tick(); if (monsterPosition!=null) monsterPosition.Tick(); if (boss!=null) boss.Tick(); } catch (Exception) { Notice("采集状态需要确认，请重新进入"); ReleaseScene(); return; }
            RefreshNativeTeamHud();
            RefreshTiredHud();
            if (worldReady) MapViewer86.SyncMini();
            if (safeAreaKnown)
            {
                bool local=InsideSafe(actor.transform.position,safeMinX,safeMaxX,safeMinZ,safeMaxZ);
                if (local!=lastLocalSafe) { lastLocalSafe=local; snapshotReady=false; RequestSnapshot(); }
            }
            if (awaitingEncounter && !encounterTimedOut && Stopwatch.GetTimestamp()>=encounterDeadline)
            {
                encounterTimedOut=true;
                if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
                Notice("战斗响应尚待确认，请检查连接");
            }
            if (dirty && !inFlight && Stopwatch.GetTimestamp()>=nextSnapshot) StartSnapshot();
        }
        void RefreshNativeTeamHud()
        {
            if (teamHudChecked || !InteractionReady || StaticData.playerData==null || String.IsNullOrEmpty(StaticData.playerData.teamId)) return;
            try
            {
                UIFieldRunMap page=PageManager.instance.currentPage as UIFieldRunMap;
                Component info=page==null || NativeTeamInfo==null ? null : NativeTeamInfo.GetValue(page) as Component;
                if (info==null) return;
                teamHudChecked=true;
                // Early native team replies can fill the cache after ReRefresh
                // hid this component. Restore its original Update/tick owner.
                if (!info.gameObject.activeSelf) UIFieldRunMap.RefreshTeamInfo();
            }
            catch (Exception) { }
        }
        void ReleaseScene() { ReleaseScene(false); }
        void ReleaseScene(bool sameNativeRoom)
        {
            ChipNames.ReleaseScene();
            ColdBattleRecovery.SceneExit(room);
            // Native 30024 recreates the self actor even in an already loaded
            // RoomField. Keep that room's resource-completion observation;
            // position/session/connection proof is checked independently.
            if (!sameNativeRoom)
            {
                if (loadedRoom==room) { loadedRoom=null; loadedOwner=null; }
                if (positionRoom==room) { positionRoom=null; positionOwner=null; positionNet=null; }
            }
            PlayerActionPanel86.CloseLocal();
            TeamMap86.ReleaseScene();
            MapViewer86.ReleaseScene();
            // A scene/session transition cancels only this local pre-start;
            // an already sent native battle keeps its existing recovery path.
            if (boss!=null && boss.Confirming)
            {
                try { if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading(); } catch (Exception) { }
                try { boss.Clear(); } catch (Exception) { }
            }
            if (monsterPosition!=null && monsterPosition.Confirming)
            {
                try { if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading(); } catch (Exception) { }
                try { monsterPosition.Clear(); } catch (Exception) { }
            }
            if (!PositionSync.Locked) PositionSync.SetFieldActive(false);
            sceneSerial++; inFlight=false; dirty=true; nextSnapshot=0; snapshotReady=false; worldReady=false; safeAreaKnown=false;
            awaitingEncounter=false; encounterTimedOut=false; askingProtection=false; protectionDeadline=0;
            tiredHudPage=null; tiredHudInfo=null; tiredHudValue=Int32.MinValue;
            if (gather!=null) try { gather.Suspend(); } catch (Exception) { }
            if (protection!=null) protection.Suspend();
            if (resources!=null) try { resources.Clear(); } catch (Exception) { }
            if (monsters!=null) try { monsters.Clear(); } catch (Exception) { }
            if (boss!=null) try { boss.Clear(); } catch (Exception) { }
            if (npcs!=null) try { npcs.Clear(); } catch (Exception) { }
            if (terminal!=null) terminal.Suspend();
            TradeController.SceneExit();
            if (preload!=null) try { preload.Dispose(); } catch (Exception) { }
            room=null; actor=null; session=null; resources=null; preload=null; gather=null; protection=null;
            npcs=null; monsters=null; monsterPosition=null; boss=null; terminal=null;
        }
        void OnDestroy() { ReleaseScene(); if (singleton==this) singleton=null; }
        public void Trigger(string groupId,string posId)
        {
            if (gather==null) { Notice("沙漠资源准备中，请稍候"); return; }
            if (PvpBridge.HasPending()) { Notice("上次挑战仍在确认"); return; }
            if (LeaveController.Pending) { Notice("离图正在确认"); return; }
            if (TravelController.InterruptExtraction()) { Notice("撤离正在中断，请再次采集"); return; }
            gather.Trigger(groupId,posId);
        }
        public void RequestSnapshot() { dirty=true; requestVersion++; }
        // The original read-state closes and refreshes its HUD before the
        // authoritative HTTP claim returns. Its tiredness setter does not
        // subscribe this HUD; refresh the original hero widget after each
        // actual value change and whenever the field page is recreated.
        void RefreshTiredHud()
        {
            if (!worldReady || StaticData.playerData==null || UI.PageManager.instance==null) return;
            UI.UIFieldRunMap page=UI.PageManager.instance.currentPage as UI.UIFieldRunMap;
            if (page==null || page.heroInfo==null) { tiredHudPage=null; tiredHudInfo=null; return; }
            int value=StaticData.playerData.Tiredness;
            if (page==tiredHudPage && page.heroInfo==tiredHudInfo && value==tiredHudValue) return;
            try
            {
                if (SetNativeTired==null) throw new InvalidOperationException("Native tiredness HUD unavailable");
                SetNativeTired.Invoke(page.heroInfo,null); tiredHudPage=page; tiredHudInfo=page.heroInfo; tiredHudValue=value;
            }
            catch (Exception) { tiredHudPage=null; tiredHudInfo=null; } // native page may still be initializing
        }
        bool SafeReady()
        {
            if (!snapshotReady || !safeAreaKnown || actor==null) return false;
            bool local=InsideSafe(actor.transform.position,safeMinX,safeMaxX,safeMinZ,safeMaxZ);
            if (local==safe) return true;
            lastLocalSafe=local; snapshotReady=false; RequestSnapshot(); return false;
        }
        static double SafeNumber(JsonData value)
        {
            double n=Double.Parse(value.ToString(),CultureInfo.InvariantCulture);
            if (Double.IsNaN(n) || Double.IsInfinity(n) || Math.Abs(n)>100000) throw new InvalidOperationException("Invalid safe area");
            return n;
        }
        static bool InsideSafe(Vector3 p,double minX,double maxX,double minZ,double maxZ)
        { return p.x>=minX && p.x<=maxX && p.z>=minZ && p.z<=maxZ; }
        void StartSnapshot()
        {
            if (session==null || room==null) return;
            int serial=sceneSerial, version=requestVersion; FieldWire.Session owner=session;
            inFlight=true; dirty=false; long snapshotSent=Stopwatch.GetTimestamp();
            try
            {
                wire.Snapshot(owner,delegate(FieldWireResult result)
                {
                    long snapshotReceived=Stopwatch.GetTimestamp();
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
                        if (!result.data["safe"].IsBoolean) throw new InvalidOperationException("Missing safe state");
                        bool nextSafe=(bool)result.data["safe"];
                        JsonData area=result.data["safeArea"];
                        double minX=SafeNumber(area["minX"]), maxX=SafeNumber(area["maxX"]), minZ=SafeNumber(area["minZ"]), maxZ=SafeNumber(area["maxZ"]);
                        if (minX>=maxX || minZ>=maxZ) throw new InvalidOperationException("Invalid safe area bounds");
                        resources.ApplySnapshot(result.data);
                        monsters.ApplySnapshot(result.data,snapshotSent,snapshotReceived);
                        boss.ApplySnapshot(result.data);
                        npcs.ApplySnapshot(result.data);
                        terminal.ApplySnapshot(result.data);
                        ChipNames.ApplySnapshot(result.data);
                        protectionDeadline=Stopwatch.GetTimestamp()+remain*Stopwatch.Frequency/1000;
                        safe=nextSafe; safeMinX=minX; safeMaxX=maxX; safeMinZ=minZ; safeMaxZ=maxZ; safeAreaKnown=true;
                        lastLocalSafe=InsideSafe(actor.transform.position,minX,maxX,minZ,maxZ);
                        worldReady=true; snapshotReady=lastLocalSafe==nextSafe;
                        if (!snapshotReady) { dirty=true; nextSnapshot=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
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
        public bool OpenPlayer(RCUIPlayer player)
        {
            if (!SafeReady()) { Notice("场景正在同步，请稍候"); RequestSnapshot(); return true; }
            if (!safe) return !CanAttackPvp();
            if (PvpBridge.HasPending()) { Notice("上次挑战仍在确认"); return true; }
            long id;
            if (player.owner==null || !Int64.TryParse(player.owner.name,out id) || id<=0 || session==null || id==session.playerId)
            { Notice("无法识别交易对象"); return true; }
            if (TradeController.BusyTrade || LeaveController.Pending || TravelController.PendingExtraction || BattleRecovery.HasUnsettled())
            { Notice("请先完成当前行动"); return true; }
            FieldWire.Session owner=session;
            RoomBase ownerRoom=room;
            RoomCharactor target=player.owner;
            int server=player.serverID;
            Func<bool> valid=delegate
            {
                FieldWire.Session current; FieldWire.Current(out current);
                return this!=null && Same(owner,current) && Same(owner,session) && room==ownerRoom && RoomManager.currentRoom==ownerRoom &&
                    player!=null && player.owner==target && target!=null && player.serverID==server &&
                    ownerRoom!=null && ownerRoom.charactorManager!=null && ownerRoom.charactorManager.GetCharactor(id.ToString())==target &&
                    SafeReady() && safe && !PvpBridge.HasPending() && !TradeController.BusyTrade && !LeaveController.Pending &&
                    !TravelController.PendingExtraction && !BattleRecovery.HasUnsettled();
            };
            PlayerActionPanel86.Show(valid,delegate(bool team)
            {
                if (!valid()) { Notice("对象或场景已变化，请重新选择"); return; }
                if (!team) TradeController.Invite(id);
                else
                {
                    RoomNetField net=ownerRoom.netManager as RoomNetField;
                    if (net==null) { Notice("队伍连接正在准备"); return; }
                    net.SendCSTeamCreateAndInvite(id,server);
                }
            });
            return true;
        }
        public void OpenNpc(RCUINormalNPC npc)
        {
            if (!SafeReady()) { Notice("场景正在同步，请稍候"); RequestSnapshot(); return; }
            if (PvpBridge.HasPending()) { Notice("上次挑战仍在确认"); return; }
            FieldNpcs.Point p=npcs==null || npc==null ? null : npcs.Find(npc.owner);
            if (p==null || actor==null) { Notice("向导正在同步，请稍候"); RequestSnapshot(); return; }
            if ((actor.transform.position-p.interactionPosition).sqrMagnitude>p.radius*p.radius)
            { Notice("请靠近后再交互"); return; }
            if (p.role=="settlement" && terminal!=null) terminal.Open();
            else if (p.role=="teleport") TravelController.Entry();
            else if (p.role=="extraction") TravelController.Extraction(p.pointId);
        }
        public bool CanAttackPvp()
        {
            if (!SafeReady()) { Notice("场景正在同步，请稍候"); RequestSnapshot(); return false; }
            if (safe) { Notice("安全区不能挑战"); return false; }
            if (PvpBridge.HasPending() || LeaveController.Pending || BattleRecovery.HasUnsettled())
            { Notice("请先完成当前行动"); return false; }
            if (TravelController.InterruptExtraction()) { Notice("撤离正在中断，请再次触碰"); return false; }
            if (Stopwatch.GetTimestamp()<protectionDeadline) { AskProtectionForAttack(); return false; }
            return true;
        }
        public bool PvpRejected(int code)
        {
            if (code==2007 && Stopwatch.GetTimestamp()<protectionDeadline) { AskProtectionForAttack(); return true; }
            return false;
        }
        public bool StartEncounter(int type,string uuid)
        {
            if (type==6 && dispatchingConfirmedMonster)
            {
                bool admitted=awaitingEncounter && !encounterTimedOut && confirmedMonsterId==uuid;
                dispatchingConfirmedMonster=false; confirmedMonsterId=null;
                return admitted;
            }
            // A confirmed boss continues through the original SendPveBoss
            // method exactly once; its prefix returns here before any new gate.
            if (type==12 && dispatchingConfirmedBoss)
            {
                bool admitted=awaitingEncounter && confirmedBossId==uuid;
                dispatchingConfirmedBoss=false; confirmedBossId=null;
                return admitted;
            }
            if (!InteractionReady || !ColdBattleRecovery.AllowNewEncounter()) return false;
            if ((type!=6 && type!=12) || String.IsNullOrEmpty(uuid) || actor==null || !worldReady)
            { Notice("场景正在同步，请稍候"); RequestSnapshot(); return false; }
            if (PvpBridge.HasPending()) { Notice("上次挑战仍在确认"); return false; }
            if (LeaveController.Pending) { Notice("离图正在确认"); return false; }
            if (BattleRecovery.HasUnsettled()) { Notice("上次战斗结果尚待确认"); return false; }
            if (TravelController.InterruptExtraction()) { Notice("撤离正在中断，请再次触碰"); return false; }
            if (type==12 && TravelController.Pending) { Notice("上次行程仍在确认"); return false; }
            if (type==12 && (boss==null || !boss.CanInteract(uuid,actor))) return false;
            if (awaitingEncounter) return false;
            if (Stopwatch.GetTimestamp()<protectionDeadline)
            { AskProtectionForAttack(); return false; }
            if (LoadingPanel.Instance==null) { Notice("战斗画面正在准备"); return false; }
            awaitingEncounter=true; encounterTimedOut=false; encounterDeadline=Stopwatch.GetTimestamp()+15L*Stopwatch.Frequency;
            LoadingPanel.Instance.ShowLoadingPre();
            if (type==12)
            {
                FieldBoss target=boss; RoomBase startRoom=room; FieldWire.Session startOwner=session;
                int serial=sceneSerial,epoch=PositionSync.ConnectionEpoch;
                bool started=target.ConfirmPosition(startRoom,actor,wire,startOwner,delegate(bool confirmed,bool cancelled)
                {
                    FieldWire.Session live;
                    bool same=serial==sceneSerial && epoch==PositionSync.ConnectionEpoch && room==startRoom && room==RoomManager.currentRoom &&
                        boss==target && Same(startOwner,session) && FieldWire.Current(out live) && Same(startOwner,live);
                    if (!same)
                    {
                        if (serial==sceneSerial && awaitingEncounter)
                        {
                            awaitingEncounter=false; encounterTimedOut=false;
                            if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
                        }
                        return;
                    }
                    if (!confirmed || !target.CanInteract(uuid,actor))
                    {
                        if (cancelled)
                        {
                            awaitingEncounter=false; encounterTimedOut=false;
                            if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
                            Notice("已取消战斗");
                        }
                        else EndEncounter(0);
                        return;
                    }
                    try
                    {
                        RoomNetField net=startRoom.netManager as RoomNetField;
                        MethodInfo send=typeof(RoomNetField).GetMethod("SendPveBoss",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
                        if (net==null || send==null || !awaitingEncounter) throw new InvalidOperationException("Original boss entry unavailable");
                        confirmedBossId=uuid; dispatchingConfirmedBoss=true;
                        send.Invoke(net,new object[]{uuid});
                        if (dispatchingConfirmedBoss) throw new InvalidOperationException("Original boss entry hook unavailable");
                    }
                    catch (Exception) { EndEncounter(0); }
                    finally { dispatchingConfirmedBoss=false; confirmedBossId=null; }
                });
                if (!started) EndEncounter(0);
                return false;
            }
            FieldMonsterPosition position=monsterPosition; FieldMonsters targets=monsters;
            RoomBase startMonsterRoom=room; FieldWire.Session startMonsterOwner=session;
            int monsterSerial=sceneSerial,monsterEpoch=PositionSync.ConnectionEpoch;
            if (position==null || targets==null || !targets.HasAvailable(uuid)) { EndEncounter(0); return false; }
            bool syncing=position.Begin(startMonsterRoom,actor,wire,startMonsterOwner,uuid,delegate(bool confirmed,bool cancelled)
            {
                FieldWire.Session live;
                bool same=monsterSerial==sceneSerial && monsterEpoch==PositionSync.ConnectionEpoch && room==startMonsterRoom &&
                    room==RoomManager.currentRoom && monsterPosition==position && monsters==targets && Same(startMonsterOwner,session) &&
                    FieldWire.Current(out live) && Same(startMonsterOwner,live);
                if (!same)
                {
                    if (monsterSerial==sceneSerial && awaitingEncounter)
                    {
                        awaitingEncounter=false; encounterTimedOut=false;
                        if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
                    }
                    return;
                }
                if (!confirmed || !targets.HasAvailable(uuid) || !awaitingEncounter || encounterTimedOut)
                {
                    if (cancelled)
                    {
                        awaitingEncounter=false; encounterTimedOut=false;
                        if (LoadingPanel.Instance!=null) LoadingPanel.Instance.HideLoading();
                        Notice("已取消战斗");
                    }
                    else EndEncounter(0);
                    return;
                }
                try
                {
                    RoomNetField net=startMonsterRoom.netManager as RoomNetField;
                    MethodInfo send=typeof(RoomNetField).GetMethod("SendPve",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
                    if (net==null || send==null) throw new InvalidOperationException("Original monster entry unavailable");
                    confirmedMonsterId=uuid; dispatchingConfirmedMonster=true;
                    send.Invoke(net,new object[]{uuid});
                    if (dispatchingConfirmedMonster) throw new InvalidOperationException("Original monster entry hook unavailable");
                }
                catch (Exception) { EndEncounter(0); }
                finally { dispatchingConfirmedMonster=false; confirmedMonsterId=null; }
            });
            if (!syncing) EndEncounter(0);
            return false;
        }
        public static bool ResumeEncounter(int type,object native)
        {
            if (!InteractionReady || singleton==null || singleton.awaitingEncounter) return false;
            MethodInfo method=typeof(RoomNetField).GetMethod(type==6 ? "SCSidePveFightStart" : "SCSidePveBossFightStart",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
            if (method==null || (type!=6 && type!=12)) return false;
            singleton.awaitingEncounter=true; singleton.encounterTimedOut=false;
            singleton.encounterDeadline=Stopwatch.GetTimestamp()+15L*Stopwatch.Frequency;
            RoomBase original=singleton.room;
            string fight=type==6 ? ((SCSidePveFightStart)native).fightId : ((SCSidePveBossFightStart)native).fightId;
            LoadingPanel.Instance.ShowLoadingPre();
            try { method.Invoke(original.netManager,new object[]{native}); return true; }
            catch (Exception)
            {
                // Only undo our own pre-loader when the native handler has
                // demonstrably not admitted this fight or changed its room.
                bool admitted=StaticData.curBattleData!=null && StaticData.curBattleData.fightId==fight;
                if (RoomManager.currentRoom==original && !admitted)
                {
                    singleton.awaitingEncounter=false; singleton.encounterTimedOut=false;
                    LoadingPanel.Instance.HideLoading(); return false;
                }
                throw;
            }
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
        sealed class GatherRewardView
        {
            public readonly List<BaseResource> rewards=new List<BaseResource>();
            public bool chips;
            public string largeChips="";
        }
        static GatherRewardView ParseGatherRewards(JsonData receipt)
        {
            if (receipt==null || receipt["resources"]==null || !receipt["resources"].IsArray ||
                receipt["chips"]==null || !receipt["chips"].IsArray || receipt["chips"].Count!=6)
                throw new InvalidOperationException("Missing gather receipt rewards");
            var view=new GatherRewardView();
            foreach (JsonData row in receipt["resources"])
            {
                string id=(string)row["id"]; long count=Int64.Parse(row["amount"].ToString(),CultureInfo.InvariantCulture);
                if (String.IsNullOrEmpty(id) || (!id.StartsWith("FishItem_",StringComparison.Ordinal) && !id.StartsWith("MineItem_",StringComparison.Ordinal)) || count<0 || count>Int32.MaxValue)
                    throw new InvalidOperationException("Invalid gather material reward");
                if (count>0) view.rewards.Add(new BaseResource { id=id,num=(int)count });
            }
            string[] colors={"白","绿","蓝","紫","橙","红"};
            for (int i=0;i<6;i++)
            {
                long count=Int64.Parse(receipt["chips"][i].ToString(),CultureInfo.InvariantCulture);
                if (count<0) throw new InvalidOperationException("Invalid gather chip reward");
                if (count>Int32.MaxValue) { view.largeChips+=(view.largeChips.Length==0 ? "":"，")+colors[i]+"色芯片 "+count.ToString(CultureInfo.InvariantCulture); continue; }
                if (count>0) { view.chips=true; view.rewards.Add(new BaseResource { id="item_970"+(i+1),num=(int)count }); }
            }
            return view;
        }
        public void ApplyOriginalReceipt(JsonData receipt)
        {
            // The native DisplayGetResource also adds inventory. Use only its
            // display component: the next snapshot supplies absolute balances.
            GatherRewardView view=ParseGatherRewards(receipt);
            RequestSnapshot();
            try
            {
                if (view.rewards.Count>0 && UI.PageManager.instance!=null && UI.PageManager.instance.currentPage is UI.UIFieldRunMap)
                {
                    if (view.chips) TradePanel.RegisterIcons();
                    CommonMethod.ShowGetObjs(view.rewards,String.Empty);
                    if (view.largeChips.Length>0) Notice("获得 "+view.largeChips);
                }
                else Notice(view.largeChips.Length>0 ? "获得 "+view.largeChips : view.rewards.Count>0 ? "采集完成，奖励已到账" : "采集完成");
            }
            catch (Exception) { try { Notice("采集完成，奖励已到账"); } catch (Exception) { } }
        }
    }

    // Reuse the original two-button modal; clicking its mask only closes it.
    public sealed class PlayerActionPanel86 : MonoBehaviour
    {
        static PlayerActionPanel86 instance;
        public static bool ViewOpen { get { return instance!=null && !instance.closing; } }
        static bool pendingInput;
        static RoomBase pendingRoom;
        static Page pendingPage;
        static GameObject pendingBox;
        Func<bool> valid;
        Action<bool> selected;
        RoomBase room;
        Page page;
        GameObject priorBox;
        bool inputLease,closing;
        float nextValidation;

        static void Unlocalize(UILabel label)
        {
            foreach (MonoBehaviour component in label.GetComponents<MonoBehaviour>())
                if (component!=null && component.GetType().Name=="TextKit") DestroyImmediate(component);
        }
        static void Caption(GameObject button,UILabel original,string text)
        {
            Transform art=button.transform.Find("Label");
            if (art==null || art.GetComponent<UIWidget>()==null) throw new InvalidOperationException("native button caption unavailable");
            GameObject clone=(GameObject)Instantiate(original.gameObject);
            clone.name="PlayerActionText86"; clone.transform.parent=button.transform;
            clone.transform.localPosition=art.localPosition; clone.transform.localRotation=Quaternion.identity; clone.transform.localScale=Vector3.one;
            foreach (Transform child in clone.transform) Destroy(child.gameObject);
            UILabel label=clone.GetComponent<UILabel>(); Unlocalize(label);
            label.fontSize=20; label.width=160; label.depth=art.GetComponent<UIWidget>().depth+1;
            label.alignment=NGUIText.Alignment.Center; label.overflowMethod=UILabel.Overflow.ShrinkContent;
            label.color=Color.white; label.text=text; clone.SetActive(true); art.gameObject.SetActive(false);
        }
        public static void Show(Func<bool> valid,Action<bool> selected)
        {
            CloseLocal(); TickInput();
            if (pendingInput || valid==null || !valid() || selected==null) return;
            GameObject root=null;
            try
            {
                GameObject prefab=Resources.Load("UI/MessageBox/MessageBoxWithConfirmAndCancelButton") as GameObject;
                if (prefab==null || BoxManager.Anthor==null) throw new InvalidOperationException("native player interaction unavailable");
                int topDepth=0,topOrder=0;
                foreach (UnityEngine.Object value in FindObjectsOfType(typeof(UIPanel)))
                { UIPanel panel=value as UIPanel; if (panel!=null && panel.gameObject.activeInHierarchy) { topDepth=Math.Max(topDepth,panel.depth); topOrder=Math.Max(topOrder,panel.sortingOrder); } }
                root=(GameObject)Instantiate(prefab); root.name="DesertPlayerActions86";
                root.transform.parent=BoxManager.Anthor; root.transform.localPosition=Vector3.zero;
                root.transform.localRotation=Quaternion.identity; root.transform.localScale=Vector3.one;
                MessageBoxWidth2BtnController native=root.GetComponent<MessageBoxWidth2BtnController>();
                if (native==null || native.m_kMask==null || native.m_kLabel==null || native.titleLable==null || native.m_kOKBtn==null || native.m_kCancelBtn==null)
                    throw new InvalidOperationException("native player interaction fields unavailable");
                native.enabled=false;
                foreach (UIButtonMessage item in root.GetComponentsInChildren<UIButtonMessage>(true)) DestroyImmediate(item);
                foreach (UIEventListener item in root.GetComponentsInChildren<UIEventListener>(true)) DestroyImmediate(item);
                foreach (UIEventTrigger item in root.GetComponentsInChildren<UIEventTrigger>(true)) DestroyImmediate(item);
                foreach (UIButton item in root.GetComponentsInChildren<UIButton>(true)) item.onClick.Clear();
                Unlocalize(native.titleLable); native.titleLable.text="玩家互动"; native.titleLable.gameObject.SetActive(true);
                Unlocalize(native.m_kLabel); native.m_kLabel.text="请选择互动方式\n点击空白处关闭";
                Caption(native.m_kOKBtn,native.m_kLabel,"芯片交易"); Caption(native.m_kCancelBtn,native.m_kLabel,"邀请组队");
                PlayerActionPanel86 menu=root.AddComponent<PlayerActionPanel86>();
                menu.valid=valid; menu.selected=selected; menu.room=RoomManager.currentRoom;
                menu.page=PageManager.instance==null ? null:PageManager.instance.currentPage; menu.priorBox=BoxManager.m_kBox;
                menu.inputLease=TouchKit.GetEnabled(); if (menu.inputLease) TouchKit.SetEnabled(false);
                instance=menu;
                UIEventListener.Get(native.m_kMask).onClick=delegate(GameObject ignored) { menu.Close(); };
                UIEventListener.Get(native.m_kOKBtn).onClick=delegate(GameObject ignored) { menu.Choose(false); };
                UIEventListener.Get(native.m_kCancelBtn).onClick=delegate(GameObject ignored) { menu.Choose(true); };
                UIPanel[] panels=root.GetComponentsInChildren<UIPanel>(true);
                int minDepth=Int32.MaxValue,minOrder=Int32.MaxValue;
                foreach (UIPanel panel in panels) { minDepth=Math.Min(minDepth,panel.depth); minOrder=Math.Min(minOrder,panel.sortingOrder); }
                foreach (UIPanel panel in panels) { panel.depth+=Math.Max(0,topDepth-minDepth+1); panel.sortingOrder+=Math.Max(0,topOrder-minOrder+1); }
                root.SetActive(true);
            }
            catch (Exception error)
            {
                CloseLocal(); if (root!=null) Destroy(root);
                UnityEngine.Debug.LogWarning("Player interaction: "+error.GetType().Name+": "+error.Message);
                BoxManager.CreatePopupTextBox("互动窗口正在准备，请稍后重试",0f);
            }
        }
        void Choose(bool team)
        {
            if (closing) return;
            Action<bool> action=selected; bool current=valid!=null && valid();
            Close(); if (current && action!=null) action(team);
        }
        public static void CloseLocal() { if (instance!=null) instance.Close(); }
        void Close()
        {
            if (closing) return; closing=true;
            if (instance==this) instance=null;
            gameObject.SetActive(false);
            RestoreInput(); Destroy(gameObject);
        }
        public static void TickInput()
        {
            if (!pendingInput) return;
            if (RoomManager.currentRoom!=pendingRoom)
            { pendingInput=false; pendingRoom=null; pendingPage=null; pendingBox=null; return; }
            if (MapViewer86.ViewOpen || PageManager.instance==null || PageManager.instance.currentPage!=pendingPage || BoxManager.m_kBox!=pendingBox) return;
            if (!TouchKit.GetEnabled()) TouchKit.SetEnabled(true);
            pendingInput=false; pendingRoom=null; pendingPage=null; pendingBox=null;
        }
        void RestoreInput()
        {
            if (!inputLease) return;
            pendingInput=true; pendingRoom=room; pendingPage=page; pendingBox=priorBox;
            inputLease=false; TickInput();
        }
        void Update()
        {
            if (PageManager.instance==null || PageManager.instance.currentPage!=page || BoxManager.m_kBox!=priorBox || Input.GetKeyDown(KeyCode.Escape)) { Close(); return; }
            if (Time.unscaledTime<nextValidation) return;
            nextValidation=Time.unscaledTime+.25f;
            if (valid==null || !valid()) Close();
        }
        void OnDestroy() { RestoreInput(); if (instance==this) instance=null; }
    }
}
