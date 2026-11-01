using System;
using System.Diagnostics;
using System.Globalization;
using LitJson;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Implementations render original NGUI messages and refresh absolute
    // server inventory/terminal state after a successful receipt.
    public interface IGatherView
    {
        void Notice(string text);
        void AskToReleaseProtection();
        // Must be replay-safe: display the frozen receipt and fetch current
        // absolute inventory. Do not add receipt deltas to an existing cache.
        void ApplyOriginalReceipt(JsonData receipt);
        void RequestSnapshot();
        void AskToRetryDelivery();
    }
    // Receives the native click after walking into the server-provided stand
    // radius. The host calls Tick once per frame and retains this object while
    // cancellation/receipt reconciliation is outstanding.
    public sealed class GatherCoordinator
    {
        readonly FieldWire wire;
        readonly FieldWire.Session session;
        readonly FieldResources resources;
        readonly RoomCharactor actor;
        readonly GatherPreload preload;
        readonly NativeGatherUI native;
        readonly IGatherView view;
        GatherAttempt attempt;
        bool cancelInFlight, claimInFlight, statusInFlight, frameVerified, suspended;
        long nextRetry;
        int startFrame;
        public GatherCoordinator(FieldWire transport,FieldWire.Session current,FieldResources projected,RoomCharactor self,GatherPreload assets,NativeGatherUI ui,IGatherView sink)
        {
            if (transport==null || current==null || projected==null || self==null || assets==null || ui==null || sink==null) throw new ArgumentNullException("Gather coordinator dependencies");
            wire=transport; session=current; resources=projected; actor=self; preload=assets; native=ui; view=sink;
            attempt=GatherJournalStore.Load(session);
        }
        static long Number(JsonData d) { return Int64.Parse(d.ToString(),CultureInfo.InvariantCulture); }
        static string Text(JsonData d) { return d==null ? "" : d.ToString(); }
        bool OwnerCurrent()
        {
            FieldWire.Session current;
            return !suspended && FieldWire.Current(out current) && current.playerId==session.playerId && current.sessionHex==session.sessionHex && current.origin==session.origin;
        }
        bool SameAttempt(string id) { return attempt!=null && attempt.Journal.startRequestId==id; }
        void Save() { GatherJournalStore.Save(session,attempt); }
        void WaitForRetry() { nextRetry=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        public void Trigger(string groupId,string nativePosId)
        {
            if (!OwnerCurrent()) return;
            if (attempt!=null) { if (attempt.Journal.phase==GatherPhase.PendingDelivery) view.AskToRetryDelivery(); else view.Notice("上次采集仍在确认，请稍候"); return; }
            FieldResources.Point point=resources.FindByNativePosId(nativePosId);
            if (point==null || point.groupId!=groupId || point.remaining<=0) { view.Notice("资源已变化，请重新选择"); view.RequestSnapshot(); return; }
            Vector3 delta=actor.transform.position-point.standPosition;
            if (delta.sqrMagnitude>point.radius*point.radius) { view.Notice("请靠近采集位置"); return; }
            if (!preload.Ready(point.kind)) { view.Notice(preload.Failed ? "采集资源加载失败，请重新进入" : "采集资源准备中，请稍候"); return; }
            attempt=new GatherAttempt(point.id,point.generation,Stopwatch.GetTimestamp(),Stopwatch.Frequency);
            string id=attempt.Journal.startRequestId;
            try { Save(); }
            catch (Exception) { attempt=null; view.Notice("采集记录需要核对，暂不能开始"); return; }
            try { wire.StartGather(session,attempt.Journal,delegate(FieldWireResult result) { OnStart(id,result); }); }
            catch (Exception) { attempt.Cancel(); try { Save(); } catch (Exception) { } view.Notice("采集确认失败，请稍候"); }
        }
        void OnStart(string id,FieldWireResult result)
        {
            if (!SameAttempt(id) || !OwnerCurrent()) return;
            if (result.uncertain) { attempt.Cancel(); Save(); WaitForRetry(); view.Notice("采集结果确认中"); return; }
            if (result.code!=0)
            {
                if (attempt.Journal.phase==GatherPhase.Starting && result.code!=7002 && result.code!=7207)
                {
                    attempt.DefinitelyRejected();
                    try { GatherJournalStore.ClearFinished(session,attempt); attempt=null; }
                    catch (Exception) { view.Notice("采集记录清理失败，请稍候"); return; }
                    if (result.code==7204) view.AskToReleaseProtection();
                    else view.Notice(Error(result.code,result.message));
                    view.RequestSnapshot(); return;
                }
                attempt.Cancel(); Save(); WaitForRetry(); view.Notice("采集结果确认中"); return;
            }
            try
            {
                JsonData data=result.data;
                bool start=attempt.Accept(Text(data["requestId"]),Text(data["pointId"]),Number(data["generation"]),Text(data["attemptToken"]),
                    Number(data["acceptedAtMs"]),Number(data["readyAtMs"]),checked((int)Number(data["fatigueCost"])),Stopwatch.GetTimestamp());
                if (attempt.Journal.phase==GatherPhase.Finished) return;
                Save();
                if (!start) return;
                FieldResources.Point point=resources.FindByNativePosId("ggo_v4_"+attempt.Journal.pointId);
                if (point==null || !native.Begin(point,attempt.Journal,delegate() { OnNativeComplete(id); }))
                { attempt.Cancel(); Save(); view.Notice("采集未能开始，正在取消"); return; }
                startFrame=Time.frameCount; frameVerified=false;
            }
            catch (Exception) { attempt.Cancel(); try { Save(); } catch (Exception) { } view.Notice("采集状态异常，正在确认"); }
        }
        public void Tick()
        {
            if (!OwnerCurrent() || attempt==null) return;
            long now=Stopwatch.GetTimestamp();
            if (attempt.Journal.phase==GatherPhase.Starting && attempt.Expire(now))
            { Save(); view.Notice("采集开始超时，正在取消"); }
            if (attempt.Journal.phase==GatherPhase.Active)
            {
                if (!frameVerified)
                {
                    if (!attempt.RenderingStartedInTime(now)) { native.Stop(); Save(); view.Notice("采集开始超时，正在取消"); }
                    else if (Time.frameCount>startFrame && native.IsAnimating()) frameVerified=true;
                }
                else if (!native.IsAnimating()) { attempt.Cancel(); native.Stop(); Save(); view.Notice("采集已中断，正在取消"); }
            }
            if (now<nextRetry || attempt==null) return;
            if (attempt.Journal.phase==GatherPhase.Finished)
            { try { Finished(); } catch (Exception) { WaitForRetry(); } return; }
            if (attempt.Journal.phase==GatherPhase.Cancelling && !cancelInFlight) SendCancel();
            else if (attempt.Journal.phase==GatherPhase.Claiming && !claimInFlight) SendClaim();
            else if ((attempt.Journal.phase==GatherPhase.RecoverReceipt || attempt.Journal.phase==GatherPhase.CheckingClaim) && !statusInFlight) SendStatus();
        }
        // Leaving, moving or damage interrupts the active action; retries reuse
        // the same cancellation UUID until the server's tombstone is confirmed.
        public void Interrupt()
        {
            if (attempt==null) return;
            attempt.Cancel(); native.Stop();
            if (attempt.Journal.phase!=GatherPhase.Finished) Save();
        }
        void OnNativeComplete(string id)
        {
            if (!SameAttempt(id) || attempt.Journal.phase!=GatherPhase.Active) return;
            if (!frameVerified) { attempt.Cancel(); native.Stop(); Save(); return; }
            attempt.Claim(); Save(); native.CompleteFromNative(); nextRetry=0;
        }
        // Retire callbacks before binding another scene to the same journal.
        public void Suspend()
        {
            if (suspended) return;
            try { Interrupt(); } finally { suspended=true; }
        }
        public void RetryPendingDelivery()
        {
            if (!OwnerCurrent() || attempt==null || attempt.Journal.phase!=GatherPhase.PendingDelivery) return;
            attempt.ResumeDelivery(); Save(); nextRetry=0;
        }
        void SendCancel()
        {
            string id=attempt.Journal.startRequestId; cancelInFlight=true; WaitForRetry();
            wire.CancelGather(session,attempt.Journal,delegate(FieldWireResult result)
            {
                cancelInFlight=false; if (!SameAttempt(id) || !OwnerCurrent() || result.uncertain || result.code!=0) return;
                try
                {
                    JsonData data=result.data; string status=Text(data["status"]);
                    if (status=="pending_delivery")
                    {
                        if (Text(data["requestId"])!=attempt.Journal.cancelRequestId || Text(data["startRequestId"])!=id) throw new InvalidOperationException("Cancel receipt mismatch");
                        attempt.PendingDelivery(id,Text(data["attemptToken"])); Save(); view.Notice("背包已满，整理后重试领取"); view.RequestSnapshot(); return;
                    }
                    attempt.CancelReceipt(Text(data["requestId"]),Text(data["startRequestId"]),status);
                    if (attempt.Journal.phase==GatherPhase.RecoverReceipt)
                    {
                        Save();
                        JsonData receipt=data["claimReceipt"];
                        if (receipt!=null) Recover(Text(data["attemptToken"]),receipt);
                    }
                    else if (attempt.Journal.phase==GatherPhase.Finished) Finished();
                }
                catch (Exception) { WaitForRetry(); }
            });
        }
        void SendClaim()
        {
            string id=attempt.Journal.startRequestId; claimInFlight=true; WaitForRetry();
            wire.ClaimGather(session,attempt.Journal,delegate(FieldWireResult result)
            {
                claimInFlight=false; if (!SameAttempt(id) || !OwnerCurrent() || result.uncertain) return;
                if (result.code==7211)
                {
                    try { attempt.PendingDelivery(id,attempt.Journal.attemptToken); Save(); view.Notice("背包已满，整理后重试领取"); view.RequestSnapshot(); } catch (Exception) { WaitForRetry(); }
                    return;
                }
                if (result.code==7206)
                {
                    try { attempt.CheckClaimStatus(); Save(); } catch (Exception) { WaitForRetry(); }
                    return;
                }
                if (result.code!=0) return;
                try
                {
                    JsonData receipt=result.data;
                    if (!attempt.MatchesClaimReceipt(Text(receipt["requestId"]),Text(receipt["startRequestId"]),Text(receipt["attemptToken"]))) throw new InvalidOperationException("Claim receipt mismatch");
                    view.ApplyOriginalReceipt(receipt);
                    attempt.ClaimReceipt(Text(receipt["requestId"]),Text(receipt["startRequestId"]),Text(receipt["attemptToken"]));
                    Finished();
                }
                catch (Exception) { WaitForRetry(); }
            });
        }
        void SendStatus()
        {
            string id=attempt.Journal.startRequestId; statusInFlight=true; WaitForRetry();
            wire.GatherStatus(session,id,delegate(FieldWireResult result)
            {
                statusInFlight=false; if (!SameAttempt(id) || !OwnerCurrent() || result.uncertain || result.code!=0) return;
                try
                {
                    JsonData data=result.data;
                    if (Text(data["startRequestId"])!=id) return;
                    string status=Text(data["status"]), token=Text(data["attemptToken"]);
                    if (attempt.Journal.phase==GatherPhase.CheckingClaim &&
                        (String.IsNullOrEmpty(token) || token!=attempt.Journal.attemptToken)) return;
                    if (status=="pending_delivery")
                    { attempt.PendingDelivery(id,token); Save(); view.Notice("背包已满，整理后重试领取"); return; }
                    if (status=="cancelled")
                    { attempt.StatusCancellation(id,token); Finished(); view.Notice("采集已取消，请重新选择"); return; }
                    if (status=="pending" && attempt.Journal.phase==GatherPhase.CheckingClaim)
                    { attempt.ResumeCheckedClaim(id,token); Save(); return; }
                    if (status!="settled") return;
                    Recover(token,data["claimReceipt"]);
                }
                catch (Exception) { WaitForRetry(); }
            });
        }
        void Recover(string token,JsonData receipt)
        {
            if ((attempt.Journal.phase!=GatherPhase.RecoverReceipt && attempt.Journal.phase!=GatherPhase.CheckingClaim) ||
                String.IsNullOrEmpty(token) || (!String.IsNullOrEmpty(attempt.Journal.attemptToken) && token!=attempt.Journal.attemptToken) ||
                receipt==null || Text(receipt["startRequestId"])!=attempt.Journal.startRequestId || Text(receipt["attemptToken"])!=token) throw new InvalidOperationException("Recovered receipt mismatch");
            bool checking=attempt.Journal.phase==GatherPhase.CheckingClaim;
            if (checking && !attempt.MatchesClaimReceipt(Text(receipt["requestId"]),Text(receipt["startRequestId"]),token)) throw new InvalidOperationException("Recovered claim request mismatch");
            view.ApplyOriginalReceipt(receipt);
            if (checking) attempt.ClaimReceipt(Text(receipt["requestId"]),Text(receipt["startRequestId"]),token);
            else attempt.RecoveredReceipt(Text(receipt["startRequestId"]),token);
            Finished();
        }
        void Finished()
        {
            GatherJournalStore.ClearFinished(session,attempt); attempt=null; frameVerified=false;
            view.RequestSnapshot();
        }
        internal static string Error(long code,string message)
        {
            if (code==7201) return "沙漠采集暂未开放";
            if (code==7202) return "请靠近可采集位置";
            if (code==7203) return "资源正在被使用";
            if (code==7205) return String.IsNullOrEmpty(message) ? "技能或疲劳不足" : message;
            if (code==7206) return "资源已刷新，请重新选择";
            return "采集未能开始，请稍后重试";
        }
    }
}
