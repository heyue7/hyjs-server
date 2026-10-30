using System;
using System.Collections;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Finds an accepted fight without relying on the public field snapshot:
    // a reserved fight deliberately removes public presence until it ends.
    // Resume reads the exact frozen Start under the server's profile lock.
    public sealed class ColdBattleRecovery : MonoBehaviour
    {
        sealed class Accepted
        {
            public int type;
            public string fight,hash,uuid;
        }
        static ColdBattleRecovery instance;
        FieldWire wire;
        RoomBase room;
        FieldWire.Session owner;
        int scene,epoch,generation,attempts;
        bool busy,known,prompted,approved,delivered,warned;
        float nextAttempt;
        Accepted pending;

        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static bool Hex(string value,int length)
        {
            if (value==null || value.Length!=length) return false;
            foreach (char c in value) if (!((c>='0'&&c<='9')||(c>='a'&&c<='f'))) return false;
            return true;
        }
        static string Text(JsonData value,string key)
        { var data=value as IDictionary; return data!=null && data.Contains(key) && value[key]!=null ? value[key].ToString() : ""; }
        static ColdBattleRecovery Require()
        {
            if (instance!=null) return instance;
            GameObject host=new GameObject("HYJS.DesertV4.ColdBattleRecovery"); DontDestroyOnLoad(host);
            instance=host.AddComponent<ColdBattleRecovery>(); instance.wire=host.AddComponent<FieldWire>(); return instance;
        }
        public static void SceneReady(RoomBase value,int version)
        {
            FieldWire.Session who;
            if (!(value is RoomField) || !FieldWire.Current(out who)) return;
            ColdBattleRecovery core=Require();
            core.room=value; core.owner=who; core.scene=version; core.epoch=PositionSync.ConnectionEpoch;
            core.generation++; core.attempts=0; core.busy=core.known=core.prompted=core.approved=core.delivered=core.warned=false;
            core.pending=null; core.nextAttempt=0;
        }
        public static void SceneExit(RoomBase value)
        {
            if (instance==null || instance.room!=value) return;
            instance.generation++; instance.room=null; instance.pending=null; instance.busy=false;
        }
        bool SameScene()
        {
            FieldWire.Session who;
            return room!=null && room==RoomManager.currentRoom && room is RoomField && StaticData.fieldId=="field_003" &&
                scene==FieldRuntime.SceneVersion && FieldWire.Current(out who) && Same(owner,who);
        }
        bool Current() { return SameScene() && epoch==PositionSync.ConnectionEpoch; }
        void RebindConnection()
        {
            if (!SameScene() || epoch==PositionSync.ConnectionEpoch) return;
            // A reconnect can keep the same room and actor. Invalidate old
            // callbacks and discover the new connection's authoritative state.
            epoch=PositionSync.ConnectionEpoch; generation++; attempts=0;
            busy=known=prompted=approved=delivered=warned=false;
            pending=null; nextAttempt=0;
        }
        bool Still(int version)
        { return generation==version && Current(); }
        public static bool Pending
        { get { return instance!=null && instance.SameScene() && (!instance.Current() || !instance.known || instance.pending!=null); } }
        public static bool AllowNewEncounter()
        { return instance!=null && instance.Current() && instance.known && instance.pending==null; }
        void Notice(string message)
        { if (!warned) { warned=true; BoxManager.CreatePopupTextBox(message,0f); } }
        void Failed()
        {
            busy=false; known=false; nextAttempt=Time.realtimeSinceStartup+Math.Min(30,2+2*Math.Min(attempts,14));
            if (attempts>=3) Notice("上次战斗状态待确认，请重新连接后重试");
        }
        static Accepted ParseActive(int type,FieldWireResult result)
        {
            if (result==null || result.uncertain || result.code!=0 || result.data==null ||
                !result.data["active"].IsBoolean) throw new InvalidOperationException("Accepted fight discovery unavailable");
            if (!(bool)result.data["active"]) return null;
            var value=new Accepted { type=type,fight=Text(result.data,"fightId"),hash=Text(result.data,"inputHash"),
                uuid=Text(result.data,type==6 ? "monsterUuid" : "eventId") };
            if (!Hex(value.fight,24) || !Hex(value.hash,64) || String.IsNullOrEmpty(value.uuid) || value.uuid.Length>128)
                throw new InvalidOperationException("Accepted fight binding unavailable");
            return value;
        }
        void Discover()
        {
            busy=true; attempts++; int token=generation; FieldWire.Session who=owner;
            wire.FightActive(who,6,delegate(FieldWireResult first)
            {
                if (!Still(token)) return;
                Accepted monster;
                try { monster=ParseActive(6,first); } catch (Exception) { Failed(); return; }
                wire.FightActive(who,12,delegate(FieldWireResult second)
                {
                    if (!Still(token)) return;
                    try
                    {
                        Accepted boss=ParseActive(12,second);
                        if (monster!=null && boss!=null) throw new InvalidOperationException("Multiple accepted fights");
                        pending=monster ?? boss; known=true; busy=false; warned=false;
                    }
                    catch (Exception) { Failed(); }
                });
            });
        }
        void Prompt()
        {
            int token=generation; Accepted expected=pending; prompted=true;
            BoxManager.CreateMessageBoxWithoutCancelButton("上次战斗未完成。将从本场开头继续，不重复消耗。",true,delegate(bool yes)
            {
                if (!yes || !Still(token) || pending!=expected) return;
                // The native box closes after its callback. Update rechecks
                // the scene and all overlays on a later frame before delivery.
                approved=true;
            },"继续战斗",false);
        }
        void Resume()
        {
            int token=generation; Accepted expected=pending; FieldWire.Session who=owner;
            busy=true; approved=false;
            wire.ResumeFight(who,expected.type,expected.fight,expected.hash,delegate(FieldWireResult result)
            {
                if (!Still(token) || pending!=expected) return;
                busy=false;
                try
                {
                    if (result==null || result.uncertain || result.code!=0 || result.data==null ||
                        Text(result.data,"fightId")!=expected.fight || Text(result.data,"inputHash")!=expected.hash ||
                        Text(result.data,"battleType")!=expected.type.ToString()) throw new InvalidOperationException("Accepted fight changed");
                    string status=Text(result.data,"status");
                    if (status=="settled")
                    {
                        if (expected.type==12 && Text(result.data,"eventId")!=expected.uuid) throw new InvalidOperationException("Wrong settled boss event");
                        if (!Hex(Text(result.data,"endRequestHash"),64)) throw new InvalidOperationException("Missing settled receipt binding");
                        byte[] endBytes=Convert.FromBase64String(Text(result.data,"nativeEnd"));
                        if (endBytes.Length<1 || endBytes.Length>262144) throw new InvalidOperationException("Invalid settled receipt");
                        object end=PBConvert.toObject(expected.type==6 ? 30050 : 30086,endBytes);
                        if ((expected.type==6 && (!(end is SCSidePveFightEnd) || ((SCSidePveFightEnd)end).code!=1)) ||
                            (expected.type==12 && (!(end is SCSidePveBossFightEnd) || ((SCSidePveBossFightEnd)end).code!=1)))
                            throw new InvalidOperationException("Wrong settled receipt");
                        pending=null; known=true; FieldRuntime.Require().RequestSnapshot();
                        BoxManager.CreatePopupTextBox("上次战斗已结算",0f); return;
                    }
                    if (status!="active" || Text(result.data,expected.type==6 ? "monsterUuid" : "eventId")!=expected.uuid)
                        throw new InvalidOperationException("Accepted fight unavailable");
                    byte[] bytes=Convert.FromBase64String(Text(result.data,"nativeStart"));
                    if (bytes.Length<1 || bytes.Length>524288) throw new InvalidOperationException("Invalid original start");
                    object native=PBConvert.toObject(expected.type==6 ? 30048 : 30084,bytes);
                    if (expected.type==6)
                    {
                        SCSidePveFightStart start=native as SCSidePveFightStart;
                        if (start==null || start.code!=1 || start.fightId!=expected.fight) throw new InvalidOperationException("Wrong original start");
                    }
                    else
                    {
                        SCSidePveBossFightStart start=native as SCSidePveBossFightStart;
                        if (start==null || start.code!=1 || start.fightId!=expected.fight) throw new InvalidOperationException("Wrong original boss start");
                    }
                    // A response may finish while another original popup opens.
                    // Keep its accepted identity and ask again after it closes.
                    if (!FieldRuntime.InteractionReady) { prompted=false; return; }
                    BattleRecovery.AdmitResume(expected.type,expected.fight,expected.hash);
                    if (expected.type==12) StaticData.pveBossUUID=expected.uuid;
                    delivered=true;
                    if (!FieldRuntime.ResumeEncounter(expected.type,native)) { delivered=false; prompted=false; }
                }
                catch (Exception)
                {
                    // A native callback may already have scheduled its scene
                    // switch before throwing. Do not start it a second time.
                    if (Current() && (StaticData.curBattleData==null || StaticData.curBattleData.fightId!=expected.fight)) delivered=false;
                    prompted=false; nextAttempt=Time.realtimeSinceStartup+2f;
                    Notice("战斗恢复待确认，请保持连接");
                }
            });
        }
        void Update()
        {
            // Fight discovery is a read-only authenticated query and must be
            // able to finish while the position/UI readiness gate is blocked.
            // Only prompting and resuming require the original UI to be ready.
            RebindConnection();
            if (!Current() || delivered || BattleRecovery.HasEndJournal() || BattleRecovery.LivePresent) return;
            if (busy || Time.realtimeSinceStartup<nextAttempt) return;
            if (!known) { Discover(); return; }
            if (pending==null) return;
            if (!FieldRuntime.InteractionReady) return;
            if (approved) Resume();
            else if (!prompted) Prompt();
        }
        void OnDestroy() { if (instance==this) instance=null; }
    }
}
