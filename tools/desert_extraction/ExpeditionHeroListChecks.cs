using System;
using System.Collections;
using System.Collections.Generic;
using HYJS.DesertV4;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using UI;

namespace UnityEngine
{
    public sealed class GameObject { public bool activeInHierarchy=true; }
    public class MonoBehaviour
    {
        public GameObject gameObject=new GameObject();
        public static readonly List<IEnumerator> routines=new List<IEnumerator>();
        public object StartCoroutine(IEnumerator value) { routines.Add(value); return value; }
    }
    public static class Time { public static float realtimeSinceStartup; }
}
namespace NetWork.Packets { public sealed class Packet { } }
namespace NetWork.Layer { public delegate void HttpHandler(Packet packet,bool success); }
namespace ProtoBuf.Message { public sealed class CSGetExpeditionIdleHeros { public string missionSerial; } }
namespace UI
{
    public sealed class SelectPlayerPanel : UnityEngine.MonoBehaviour
    {
        public string missionSerial;
        public int calls;
        public void OnReply(Packet value,bool success) { calls++; }
    }
}
public static class StaticData { public static object playerData=new object(); public static byte[] sessionKey=new byte[] {1,2}; }
public static class BoxManager
{
    public static int notices;
    public static void CreatePopupTextBox(string text,float delay) { notices++; }
}
public static class NetAPI
{
    public static bool busy;
    public static int sends;
    public static HttpHandler callback;
    public static bool IsSendingHttp() { return busy; }
    public static bool SendHttp(int opcode,object body,HttpHandler handler)
    { sends++; if (busy) return false; callback=handler; return true; }
}
static class ExpeditionHeroListChecks
{
    static int checks;
    static void Check(bool condition,string reason) { checks++; if (!condition) throw new Exception(reason); }
    static SelectPlayerPanel Setup()
    {
        UnityEngine.MonoBehaviour.routines.Clear(); UnityEngine.Time.realtimeSinceStartup=0;
        StaticData.playerData=new object(); StaticData.sessionKey=new byte[] {1,2};
        NetAPI.sends=0; NetAPI.busy=false; NetAPI.callback=null; BoxManager.notices=0;
        return new SelectPlayerPanel { missionSerial="one" };
    }
    static bool Send(SelectPlayerPanel panel)
    { return ExpeditionHeroList.Send(57041,new CSGetExpeditionIdleHeros { missionSerial=panel.missionSerial },panel.OnReply); }
    public static void Main()
    {
        var p=Setup(); Check(Send(p) && NetAPI.sends==1 && UnityEngine.MonoBehaviour.routines.Count==0,"immediate original path");
        NetAPI.callback(new Packet(),true); Check(p.calls==1,"immediate callback");
        p=Setup(); NetAPI.busy=true; Check(Send(p) && UnityEngine.MonoBehaviour.routines.Count==1,"busy read queued");
        IEnumerator retry=UnityEngine.MonoBehaviour.routines[0]; Check(retry.MoveNext(),"retry starts without blocking");
        NetAPI.busy=false; Check(!retry.MoveNext() && NetAPI.sends==2,"retry sent when channel free");
        NetAPI.callback(new Packet(),true); Check(p.calls==1,"retry delivers original callback");
        p=Setup(); NetAPI.busy=true; Check(Send(p),"switch fixture"); retry=UnityEngine.MonoBehaviour.routines[0];
        Check(retry.MoveNext(),"switch pending"); p.missionSerial="two"; NetAPI.busy=false;
        Check(!retry.MoveNext() && NetAPI.sends==1,"changed mission cannot receive old read");
        p=Setup(); NetAPI.busy=true; Check(Send(p),"session fixture"); retry=UnityEngine.MonoBehaviour.routines[0];
        retry.MoveNext(); StaticData.sessionKey=new byte[] {1,3}; NetAPI.busy=false;
        Check(!retry.MoveNext() && NetAPI.sends==1,"changed session cancels retry");
        p=Setup(); NetAPI.busy=true; Check(Send(p),"closure fixture"); retry=UnityEngine.MonoBehaviour.routines[0];
        retry.MoveNext(); p.gameObject.activeInHierarchy=false; NetAPI.busy=false;
        Check(!retry.MoveNext() && NetAPI.sends==1,"closed panel cancels retry");
        p=Setup(); NetAPI.busy=true; Check(Send(p),"timeout fixture"); retry=UnityEngine.MonoBehaviour.routines[0];
        retry.MoveNext(); UnityEngine.Time.realtimeSinceStartup=16; Check(!retry.MoveNext() && BoxManager.notices==1,"timeout reports failure");
        p=Setup(); NetAPI.busy=true; Check(Send(p) && Send(p),"repeat fixture");
        IEnumerator old=UnityEngine.MonoBehaviour.routines[0],latest=UnityEngine.MonoBehaviour.routines[1];
        Check(!old.MoveNext() && latest.MoveNext(),"newer request cancels older retry");
        NetAPI.busy=false; latest.MoveNext(); HttpHandler delayed=NetAPI.callback;
        p.missionSerial="two"; delayed(new Packet(),true); Check(p.calls==0,"stale retry callback ignored");
        Console.WriteLine("PASS expedition busy-channel retry: "+checks+" assertions");
    }
}
