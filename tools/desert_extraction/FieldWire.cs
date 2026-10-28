using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.Text;
using LitJson;
using UnityEngine;

namespace HYJS.DesertV4
{
    public sealed class FieldWireResult
    {
        public bool uncertain;
        public long code;
        public string message;
        public JsonData data;
    }
    // An owner/session-bound transport. A timeout or malformed response has an
    // unknown outcome: its caller must retain and retry the same UUID.
    // This component intentionally does not apply assets or draw scene objects.
    public sealed class FieldWire : MonoBehaviour
    {
        public sealed class Session
        {
            public readonly long playerId;
            public readonly string sessionHex, origin;
            internal Session(long player, string key, string server) { playerId = player; sessionHex = key; origin = server; }
        }
        public static bool Current(out Session value)
        {
            value = null;
            if (StaticData.playerData == null || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return false;
            long player = StaticData.playerData.playerId;
            if (player <= 0) return false;
            string origin;
            try { origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); }
            catch (Exception) { return false; }
            if (String.IsNullOrEmpty(origin)) return false;
            var session = new StringBuilder(32);
            foreach (byte b in StaticData.sessionKey) session.Append(b.ToString("x2"));
            value = new Session(player, session.ToString(), origin);
            return true;
        }
        static bool Same(Session a, Session b)
        { return a != null && b != null && a.playerId == b.playerId && a.sessionHex == b.sessionHex && a.origin == b.origin; }
        static JsonData Base(Session session)
        {
            if (session == null) throw new ArgumentNullException("session");
            JsonData body = new JsonData(); body["playerId"] = session.playerId; body["sessionHex"] = session.sessionHex; return body;
        }
        static void CheckId(string id)
        {
            try { if (String.IsNullOrEmpty(id) || new Guid(id).ToString("D") != id) throw new ArgumentException(); }
            catch (Exception) { throw new ArgumentException("Request ID must be a canonical lowercase UUID"); }
        }
        public void Snapshot(Session session, Action<FieldWireResult> callback)
        { Post(session, "/ggo-field/snapshot", Base(session), callback); }
        public void StartGather(Session session, GatherJournal j, Action<FieldWireResult> callback)
        {
            if (j == null || j.phase != GatherPhase.Starting || j.generation < 1 || String.IsNullOrEmpty(j.pointId)) throw new ArgumentException("Invalid start");
            CheckId(j.startRequestId); JsonData body = Base(session); body["requestId"] = j.startRequestId; body["pointId"] = j.pointId; body["generation"] = j.generation;
            Post(session, "/ggo-field/gather/start", body, callback);
        }
        public void CancelGather(Session session, GatherJournal j, Action<FieldWireResult> callback)
        {
            if (j == null || j.phase != GatherPhase.Cancelling) throw new ArgumentException("Invalid cancellation");
            CheckId(j.cancelRequestId); CheckId(j.startRequestId); JsonData body = Base(session);
            body["requestId"] = j.cancelRequestId; body["startRequestId"] = j.startRequestId;
            // Always cancel by startRequestId. Keeping this body stable matters
            // when a late start reply supplies a token after the first cancel.
            Post(session, "/ggo-field/gather/cancel", body, callback);
        }
        public void ClaimGather(Session session, GatherJournal j, Action<FieldWireResult> callback)
        {
            if (j == null || (j.phase != GatherPhase.Claiming && j.phase != GatherPhase.RecoverReceipt) || String.IsNullOrEmpty(j.attemptToken)) throw new ArgumentException("Invalid claim");
            CheckId(j.claimRequestId); CheckId(j.startRequestId); JsonData body = Base(session);
            body["requestId"] = j.claimRequestId; body["startRequestId"] = j.startRequestId; body["attemptToken"] = j.attemptToken;
            Post(session, "/ggo-field/gather/claim", body, callback);
        }
        public void GatherStatus(Session session, string startRequestId, Action<FieldWireResult> callback)
        {
            CheckId(startRequestId); JsonData body = Base(session); body["startRequestId"] = startRequestId;
            Post(session, "/ggo-field/gather/status", body, callback);
        }
        public void ReleaseProtection(Session session, string requestId, Action<FieldWireResult> callback)
        {
            CheckId(requestId); JsonData body = Base(session); body["requestId"] = requestId;
            Post(session, "/ggo-field/protection/release", body, callback);
        }
        public void Settlement(Session session,string requestId,Action<FieldWireResult> callback)
        {
            CheckId(requestId); JsonData body=Base(session); body["requestId"]=requestId;
            Post(session,"/ggo-field/settlement",body,callback);
        }
        public void Entry(Session session,string requestId,Action<FieldWireResult> callback)
        {
            CheckId(requestId); JsonData body=Base(session); body["requestId"]=requestId;
            Post(session,"/ggo-field/entry",body,callback);
        }
        public void ExtractionStart(Session session,string requestId,string pointId,Action<FieldWireResult> callback)
        {
            CheckId(requestId); if (String.IsNullOrEmpty(pointId)) throw new ArgumentException("Missing exit point");
            JsonData body=Base(session); body["requestId"]=requestId; body["pointId"]=pointId;
            Post(session,"/ggo-field/extraction/start",body,callback);
        }
        public void ExtractionCancel(Session session,string requestId,string startRequestId,Action<FieldWireResult> callback)
        {
            CheckId(requestId); CheckId(startRequestId); JsonData body=Base(session);
            body["requestId"]=requestId; body["startRequestId"]=startRequestId;
            Post(session,"/ggo-field/extraction/cancel",body,callback);
        }
        public void ExtractionStatus(Session session,string startRequestId,Action<FieldWireResult> callback)
        {
            CheckId(startRequestId); JsonData body=Base(session); body["startRequestId"]=startRequestId;
            Post(session,"/ggo-field/extraction/status",body,callback);
        }
        public void FightActive(Session session,int type,Action<FieldWireResult> callback)
        {
            if (type!=6 && type!=12) throw new ArgumentException("Unsupported fight type");
            Post(session,type==6 ? "/ggo-field/monster/active" : "/ggo-field/boss/active",Base(session),callback);
        }
        public void FightReceipt(Session session,int type,string fightId,Action<FieldWireResult> callback)
        {
            if ((type!=6 && type!=12) || String.IsNullOrEmpty(fightId)) throw new ArgumentException("Invalid fight receipt request");
            JsonData body=Base(session); body["fightId"]=fightId;
            Post(session,type==6 ? "/ggo-field/monster/receipt" : "/ggo-field/boss/receipt",body,callback);
        }
        void Post(Session session, string path, JsonData body, Action<FieldWireResult> callback)
        {
            if (callback == null) throw new ArgumentNullException("callback");
            Session live; if (!Current(out live) || !Same(session,live)) { callback(new FieldWireResult { uncertain = true }); return; }
            StartCoroutine(Request(session,path,JsonMapper.ToJson(body),callback));
        }
        IEnumerator Request(Session session, string path, string body, Action<FieldWireResult> callback)
        {
            WWW request = null;
            try
            {
                var headers = new Dictionary<string,string>(); headers.Add("Content-Type", "application/json");
                request = new WWW(session.origin + path, Encoding.UTF8.GetBytes(body), headers);
            }
            catch (Exception) { callback(new FieldWireResult { uncertain = true }); yield break; }
            long deadline = Stopwatch.GetTimestamp() + 15L * Stopwatch.Frequency;
            while (!request.isDone && Stopwatch.GetTimestamp() < deadline) yield return null;
            var result = new FieldWireResult { uncertain = true };
            Session live;
            try
            {
                if (request.isDone && String.IsNullOrEmpty(request.error) && Current(out live) && Same(session,live))
                {
                    JsonData root = JsonMapper.ToObject(request.text);
                    long code = Int64.Parse(root["code"].ToString());
                    result.code = code; result.message = root["msg"] == null ? "" : root["msg"].ToString(); result.data = root["data"];
                    // A system error may have happened after a commit. Preserve
                    // the UUID and reconcile through status/cancel/claim.
                    result.uncertain = code == 7002;
                }
            }
            catch (Exception) { result.uncertain = true; }
            request.Dispose();
            callback(result);
        }
    }
}
