using System;
using System.Collections;
using System.Reflection;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using UI;
using UnityEngine;

namespace HYJS.DesertV4
{
    // The native selection view ignores SendHttp's false result. If the global
    // HTTP channel is busy, retry this read after it becomes available.
    public static class ExpeditionHeroList
    {
        static readonly FieldInfo Mission=typeof(SelectPlayerPanel).GetField("missionSerial",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
        static long generation;
        sealed class Request
        {
            internal SelectPlayerPanel panel;
            internal object player;
            internal byte[] session;
            internal string mission;
            internal int opcode;
            internal object body;
            internal HttpHandler callback;
            internal long sequence;
        }
        static bool SessionEquals(byte[] left,byte[] right)
        {
            if (left==null || right==null || left.Length!=right.Length) return false;
            for (int i=0;i<left.Length;i++) if (left[i]!=right[i]) return false;
            return true;
        }
        static bool Current(Request r)
        {
            try
            {
                return r!=null && r.sequence==generation && r.panel!=null && r.panel.gameObject!=null && r.panel.gameObject.activeInHierarchy &&
                    r.player!=null && System.Object.ReferenceEquals(r.player,StaticData.playerData) && SessionEquals(r.session,StaticData.sessionKey) &&
                    Mission!=null && (Mission.GetValue(r.panel) as string)==r.mission;
            }
            catch (Exception) { return false; }
        }
        public static bool Send(int opcode,object body,HttpHandler callback)
        {
            var query=body as CSGetExpeditionIdleHeros;
            var panel=callback==null ? null : callback.Target as SelectPlayerPanel;
            if (opcode!=57041 || query==null || panel==null || callback==null || Mission==null || StaticData.sessionKey==null)
                return NetAPI.SendHttp(opcode,body,callback);
            long serial=++generation;
            if (NetAPI.SendHttp(opcode,body,callback)) return true;
            var request=new Request { panel=panel,player=StaticData.playerData,session=(byte[])StaticData.sessionKey.Clone(),
                mission=query.missionSerial,opcode=opcode,body=body,callback=callback,sequence=serial };
            try
            {
                if (!Current(request)) return false;
                panel.StartCoroutine(Retry(request));
                return true;
            }
            catch (Exception) { return false; }
        }
        static IEnumerator Retry(Request r)
        {
            float deadline=Time.realtimeSinceStartup+15f;
            float nextAttempt=Time.realtimeSinceStartup;
            while (Current(r) && Time.realtimeSinceStartup<deadline)
            {
                yield return null;
                if (!Current(r)) yield break;
                if (NetAPI.IsSendingHttp() || Time.realtimeSinceStartup<nextAttempt) continue;
                nextAttempt=Time.realtimeSinceStartup+.5f;
                bool sent=false;
                try
                {
                    HttpHandler guarded=delegate(Packet packet,bool success) { if (Current(r)) r.callback(packet,success); };
                    sent=NetAPI.SendHttp(r.opcode,r.body,guarded);
                }
                catch (Exception) { }
                if (sent) yield break;
            }
            if (Current(r)) BoxManager.CreatePopupTextBox("佣兵列表加载失败，请重新打开任务",0f);
        }
    }
}
