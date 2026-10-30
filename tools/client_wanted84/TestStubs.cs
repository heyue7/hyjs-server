using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using System.Text;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using UnityEngine;

namespace ProtoBuf
{
    public static class Serializer
    {
        public static void Serialize<T>(Stream stream, T value)
        {
            BinaryWriter writer = new BinaryWriter(stream, Encoding.UTF8, true);
            SCRebelFightStart start = value as SCRebelFightStart;
            CSRebelFightEnd end = value as CSRebelFightEnd;
            if (start != null) { writer.Write((byte)1); writer.Write(start.fightId); writer.Write(start.seed); }
            else if (end != null) { writer.Write((byte)2); writer.Write(end.fightId); writer.Write(end.result); writer.Write(end.record); }
            else throw new InvalidOperationException("unexpected test protobuf");
        }
        public static T Deserialize<T>(Stream stream)
        {
            BinaryReader reader = new BinaryReader(stream, Encoding.UTF8, true);
            byte kind = reader.ReadByte();
            object result;
            if (kind == 1) result = new SCRebelFightStart { fightId = reader.ReadString(), seed = reader.ReadInt32() };
            else if (kind == 2) result = new CSRebelFightEnd { fightId = reader.ReadString(), result = reader.ReadInt32(), record = reader.ReadString() };
            else throw new InvalidDataException("unknown test protobuf");
            return (T)result;
        }
    }
}

namespace ProtoBuf.Message
{
    public class RebelInfo { public string id; }
    public class CSRebelFightStart { public string rebelId; public int type; }
    public class SCRebelFightStart { public string fightId; public int seed; }
    public class CSRebelFightEnd { public string fightId; public int result; public string record; }
    public class SCRebelFightEnd { public int get, cost; }
    public class SCGetRebels { public List<RebelInfo> rebels = new List<RebelInfo>(); }
    public class ErrorMessage { public string code, msg; }
}

namespace NetWork.Packets
{
    public class Packet
    {
        public int nOpCode;
        public object kBody;
        public Packet(int code, object body) { nOpCode = code; kBody = body; }
    }
}
namespace NetWork.Layer { public delegate void HttpHandler(Packet packet, bool success); }

namespace UnityEngine
{
    public class Object { public static void Destroy(Object value) { } }
    public class GameObject : Object { public bool activeInHierarchy = true; }
    public class Coroutine { }
    public class MonoBehaviour
    {
        public GameObject gameObject = new GameObject();
        public Coroutine StartCoroutine(IEnumerator coroutine) { TestScheduler.Add(coroutine); return new Coroutine(); }
    }
    public static class Time { public static float realtimeSinceStartup; }
    public static class Debug { public static void LogError(string text) { } }
}

public static class TestScheduler
{
    static readonly List<IEnumerator> tasks = new List<IEnumerator>();
    public static void Reset() { tasks.Clear(); UnityEngine.Time.realtimeSinceStartup = 0; }
    public static void Add(IEnumerator task) { if (task.MoveNext()) tasks.Add(task); }
    public static void Tick(float elapsed = 0.05f)
    {
        UnityEngine.Time.realtimeSinceStartup += elapsed;
        IEnumerator[] current = tasks.ToArray();
        foreach (IEnumerator task in current)
            if (tasks.Contains(task) && !task.MoveNext()) tasks.Remove(task);
    }
}

namespace UI
{
    public class UIPlayerData
    {
        public long playerId;
        public int score, attempts, addCalls, deleteCalls;
        public void AddResultObj(int amount) { score += amount; addCalls++; }
        public void Delete(int amount) { attempts -= amount; deleteCalls++; }
    }
    public class WantedMission : MonoBehaviour
    {
        readonly List<RebelInfo> bossList = new List<RebelInfo>();
        int curBossIndex;
        public int showCalls;
        public WantedMission(string instance) { bossList.Add(new RebelInfo { id = instance }); }
        public void SetBoss(string instance) { bossList.Clear(); if (instance != null) bossList.Add(new RebelInfo { id = instance }); curBossIndex = 0; }
        public List<RebelInfo> BossSnapshot() { return new List<RebelInfo>(bossList); }
        public void SendEnterBattle(int type)
        {
            HYJS.Feedback65.Runtime.WantedRequest(this, type);
            CSRebelFightStart request = new CSRebelFightStart { rebelId = bossList[curBossIndex].id, type = type };
            if (!HYJS.Feedback65.Runtime.WantedPrepareStart(this, request)) return;
            HttpHandler handler = HYJS.Feedback65.Runtime.WantedWrapStart(delegate(Packet packet, bool success) {
                if (!HYJS.Feedback65.Runtime.WantedStart(this, packet, success) && success)
                {
                    StaticData.curBattleData.SetData((SCRebelFightStart)packet.kBody);
                    ManagerController.Instance.RoomToBattle(0);
                }
            });
            bool sent = NetAPI.SendHttp(55003, request, handler);
            HYJS.Feedback65.Runtime.WantedStartSent(sent);
        }
        public void Show()
        {
            showCalls++;
            NetAPI.SendHttp(55001, new object(), delegate(Packet packet, bool success) {
                HYJS.Feedback65.Runtime.WantedListReceived(this, packet, success);
            });
            HYJS.Feedback65.Runtime.WantedPageShown(this);
        }
    }
    public class MercenaryTaskPage { public Dictionary<string, string> options = new Dictionary<string, string>(); }
    public class PageManager { public static PageManager Instance = new PageManager(); public object currentPage = new MercenaryTaskPage(); }
}
namespace Battle
{
    public enum BattleType { WANTED = 8 }
    public static class BattleManager { public static bool isBattleUI; }
}
public class BattleData
{
    public string fightId;
    public Battle.BattleType battleType;
    public int seed;
    public void SetData(SCRebelFightStart start) { fightId = start.fightId; seed = start.seed; battleType = Battle.BattleType.WANTED; }
}
public static class StaticData
{
    public static UI.UIPlayerData playerData;
    public static byte[] sessionKey;
    public static BattleData curBattleData = new BattleData();
}
public class ManagerController
{
    public static ManagerController Instance = new ManagerController();
    public UI.WantedMission page;
    public int enterCount, leaveCount;
    public void RoomToBattle(int ignored) { enterCount++; }
    public void BattleToRoom() { leaveCount++; if (page != null) page.Show(); }
}
public static class BoxManager
{
    public static string lastMessage;
    public static void CreateMsgBox(string message) { lastMessage = message; }
}
public static class NetAPI
{
    public sealed class Request { public int opcode; public object body; public HttpHandler handler; }
    public static Request pending, lastEnd;
    public static int ends;
    public static bool failSend;
    public static bool IsSendingHttp() { return pending != null; }
    public static bool SendHttp(int opcode, object body, HttpHandler handler)
    {
        if (pending != null || failSend) return false;
        pending = new Request { opcode = opcode, body = body, handler = handler };
        if (opcode == 55011) { lastEnd = pending; ends++; }
        return true;
    }
    public static void Reply(Packet packet, bool success)
    {
        Request current = pending;
        if (current == null) throw new InvalidOperationException("no request pending");
        pending = null;
        current.handler(packet, success);
    }
    public static void Reset() { pending = lastEnd = null; ends = 0; failSend = false; }
}
