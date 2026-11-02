using System;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // A reconnect can restore town while a story run remains active. A map
    // choice offers an explicit exit; only its confirmed result releases the
    // original transfer. Nothing here changes a battle, item or saved run.
    public sealed class CopyExitRecovery : MonoBehaviour
    {
        static CopyExitRecovery instance;
        static bool bypass;
        FieldWire wire;
        FieldWire.Session owner;
        RoomBase room;
        int epoch, generation;
        bool busy;
        object transfer;

        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static void Notice(string message) { BoxManager.CreatePopupTextBox(message,0f); }
        static CopyExitRecovery Require()
        {
            if (instance!=null) return instance;
            GameObject go=new GameObject("HYJSCopyExitRecovery");
            DontDestroyOnLoad(go); instance=go.AddComponent<CopyExitRecovery>();
            instance.wire=go.AddComponent<FieldWire>(); return instance;
        }
        bool Current(int request)
        {
            FieldWire.Session live;
            return busy && request==generation && FieldWire.Current(out live) && Same(owner,live) &&
                epoch==PositionSync.ConnectionEpoch && room==RoomManager.currentRoom && room is RoomTown && NetAPI.SocketIsConnected();
        }
        void Reset() { busy=false; generation++; owner=null; room=null; transfer=null; }
        void Update() { if (busy && !Current(generation)) Reset(); }
        static bool Token(string value)
        {
            if (value==null || value.Length!=64) return false;
            foreach (char c in value) if (!(c>='0' && c<='9' || c>='a' && c<='f')) return false;
            return true;
        }
        public static bool Send(int opcode,object message)
        {
            if (bypass || opcode!=30032 || !(RoomManager.currentRoom is RoomTown) || !(message is CSTransferScene)) return false;
            CopyExitRecovery core=Require();
            if (core.busy && !core.Current(core.generation)) core.Reset();
            if (core.busy) { Notice("正在确认副本状态，请稍候"); return true; }
            FieldWire.Session session;
            if (!FieldWire.Current(out session) || !NetAPI.SocketIsConnected()) return false;
            try
            {
                byte[] raw=PBConvert.toBytes(opcode,message);
                if (raw==null || raw.Length==0 || raw.Length>65536) return false;
                object frozen=PBConvert.toObject(opcode,raw);
                if (!(frozen is CSTransferScene) || ((CSTransferScene)frozen).target!=((CSTransferScene)message).target) return false;
                core.owner=session; core.room=RoomManager.currentRoom; core.epoch=PositionSync.ConnectionEpoch;
                core.transfer=frozen; core.busy=true; int request=++core.generation;
                core.wire.CopyRunStatus(session,delegate(FieldWireResult result) { core.Status(request,result); });
            }
            catch (Exception) { core.Reset(); Notice("副本状态暂未确认，请稍后重试"); }
            return true;
        }
        void Status(int request,FieldWireResult result)
        {
            if (!Current(request)) return;
            try
            {
                if (result==null || result.uncertain || result.code!=0 || result.data==null || !result.data["active"].IsBoolean)
                    throw new InvalidOperationException();
                if (!(bool)result.data["active"]) { SendOriginal(request); return; }
                string token=(string)result.data["runToken"];
                if (!Token(token) || String.IsNullOrEmpty((string)result.data["chapterId"]) || String.IsNullOrEmpty((string)result.data["missionId"]))
                    throw new InvalidOperationException();
                BoxManager.CreateMessageBoxWithCofirmAndCancelButton("上次副本尚未结束，退出后继续前往？",delegate(bool yes)
                {
                    if (!Current(request)) return;
                    if (!yes) { Reset(); return; }
                    try { wire.CopyRunLeave(owner,token,delegate(FieldWireResult leave) { Left(request,leave); }); }
                    catch (Exception) { Reset(); Notice("退出结果待确认，请再次选择目的地"); }
                });
            }
            catch (Exception) { Reset(); Notice("副本状态暂未确认，请稍后重试"); }
        }
        void Left(int request,FieldWireResult result)
        {
            if (!Current(request)) return;
            try
            {
                if (result==null || result.uncertain || result.code!=0 || result.data==null ||
                    !result.data["active"].IsBoolean || (bool)result.data["active"]) throw new InvalidOperationException();
                SendOriginal(request);
            }
            catch (Exception) { Reset(); Notice("退出结果待确认，请再次选择目的地"); }
        }
        void SendOriginal(int request)
        {
            if (!Current(request)) return;
            object native=transfer; Reset(); bypass=true;
            try { NetAPI.SendSocket(30032,native); }
            finally { bypass=false; }
        }
    }
}
