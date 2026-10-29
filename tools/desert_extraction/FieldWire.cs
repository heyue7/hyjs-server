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
        public void PositionStatus(Session session,Action<FieldWireResult> callback)
        { Post(session,"/ggo-field/position/status",Base(session),callback); }
        public void LeavePrepare(Session session,string requestId,string mode,string targetScene,int targetLine,Action<FieldWireResult> callback)
        {
            CheckId(requestId); JsonData body=Base(session); body["requestId"]=requestId; body["mode"]=mode;
            if (mode=="scene" && !String.IsNullOrEmpty(targetScene) && targetLine==0) body["targetScene"]=targetScene;
            else if (mode=="line" && String.IsNullOrEmpty(targetScene) && targetLine>0) body["targetLine"]=targetLine;
            else throw new ArgumentException("Invalid leave target");
            Post(session,"/ggo-field/leave/prepare",body,callback);
        }
        public void LeaveStatus(Session session,string requestId,Action<FieldWireResult> callback)
        {
            CheckId(requestId); JsonData body=Base(session); body["leaveRequestId"]=requestId;
            Post(session,"/ggo-field/leave/status",body,callback);
        }
        public void TradeCurrent(Session session,Action<FieldWireResult> callback)
        { Post(session,"/ggo-field/trade/current",Base(session),callback); }
        public void TradeInvite(Session session,string requestId,long target,Action<FieldWireResult> callback)
        {
            CheckId(requestId); if (target<=0 || target==session.playerId) throw new ArgumentException("Invalid trade target");
            JsonData body=Base(session); body["requestId"]=requestId; body["targetPlayerId"]=target;
            Post(session,"/ggo-field/trade/invite",body,callback);
        }
        public void TradeAction(Session session,string action,string requestId,string tradeId,long initiator,long receiver,long version,long[] offer,Action<FieldWireResult> callback)
        {
            CheckId(requestId); CheckId(tradeId);
            if (requestId==tradeId || initiator<=0 || receiver<=0 || initiator==receiver ||
                (session.playerId!=initiator && session.playerId!=receiver) ||
                (action!="accept" && action!="offer" && action!="confirm" && action!="cancel") || (action!="cancel" && version<1))
                throw new ArgumentException("Invalid trade action");
            JsonData body=Base(session); body["requestId"]=requestId; body["tradeId"]=tradeId;
            body["initiatorId"]=initiator; body["receiverId"]=receiver; body["version"]=version;
            if (action=="offer")
            {
                if (offer==null || offer.Length!=6) throw new ArgumentException("Six chip quantities required");
                JsonData values=new JsonData(); values.SetJsonType(JsonType.Array);
                foreach (long count in offer) { if (count<0) throw new ArgumentException("Negative chip offer"); values.Add(count); }
                body["offer"]=values;
            }
            else if (offer!=null) throw new ArgumentException("Unexpected offer");
            Post(session,"/ggo-field/trade/"+action,body,callback);
        }
        public void TradeStatus(Session session,string tradeId,long initiator,long receiver,Action<FieldWireResult> callback)
        {
            CheckId(tradeId);
            if (initiator<=0 || receiver<=0 || initiator==receiver || (session.playerId!=initiator && session.playerId!=receiver)) throw new ArgumentException("Invalid trade owner");
            JsonData body=Base(session); body["tradeId"]=tradeId; body["initiatorId"]=initiator; body["receiverId"]=receiver;
            Post(session,"/ggo-field/trade/status",body,callback);
        }
        public void PvpStatus(Session session,long stamp,string nativeRequest,Action<FieldWireResult> callback)
        {
            if (stamp<=0 || String.IsNullOrEmpty(nativeRequest)) throw new ArgumentException("Invalid pending PK");
            JsonData body=Base(session); body["stamp"]=stamp; body["nativeRequest"]=nativeRequest;
            Post(session,"/ggo-field/pvp/status",body,callback);
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
                    string text = request.text;
                    if (path.StartsWith("/ggo-field/trade/", StringComparison.Ordinal)) text = NestedArrayJson.Normalize(text);
                    JsonData root = JsonMapper.ToObject(text);
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
