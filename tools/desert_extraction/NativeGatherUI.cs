using System;
using System.Reflection;
using Room;
using UI;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Wraps the original FSM and field progress widget. It never issues a
    // request or awards items; the caller owns receipt and cancellation logic.
    public sealed class NativeGatherUI
    {
        static readonly MethodInfo Hide=typeof(UIFieldReadState).GetMethod("Hide",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
        readonly RoomCharactor actor;
        readonly RCAIActor ai;
        readonly GatherPreload preload;
        UIFieldReadState progress;
        string kind;
        bool active;
        public NativeGatherUI(RoomCharactor self,GatherPreload ready)
        {
            if (self==null || ready==null) throw new ArgumentNullException("Native gather owner");
            actor=self; ai=self.ai as RCAIActor; preload=ready;
            if (ai==null || Hide==null) throw new InvalidOperationException("Native field UI anchors unavailable");
        }
        // Call only after a matching authoritative start receipt. A false return
        // means the caller must cancel that same reservation and retain its ID.
        public bool Begin(FieldResources.Point point,GatherJournal journal,Action onComplete)
        {
            if (active || point==null || journal==null || journal.phase!=GatherPhase.Active || onComplete==null ||
                point.id!=journal.pointId || point.generation!=journal.generation || !preload.Ready(point.kind) ||
                !(ai.GetCurrState() is PlayerAIStateNormal) || PageManager.instance==null) return false;
            UIFieldRunMap page=PageManager.instance.currentPage as UIFieldRunMap;
            if (page==null || page.readState==null || actor.lod==null || actor.lod.Model==null) return false;
            long duration=journal.readyAtMs-journal.acceptedAtMs;
            if (duration<=0 || duration>300000 || journal.fatigueCost<0) return false;
            kind=point.kind; progress=page.readState;
            try
            {
                StaticData.resId=point.groupId; StaticData.resPosId=point.nativePosId; StaticData.tirednessCost=journal.fatigueCost;
                ai.SwitchState(kind=="fish" ? 3 : 4);
                if ((kind=="fish" && !(ai.GetCurrState() is PlayerAIStateFishing)) || (kind=="mine" && !(ai.GetCurrState() is PlayerAIStateMining))) { Stop(); return false; }
                active=true;
                UIFieldRunMap.OpenGetResourceWaiting(new OkHander(delegate() { if (active) onComplete(); }));
                if (progress==null || !progress.gameObject.activeInHierarchy) { Stop(); return false; }
                // From receipt arrival use the complete frozen duration. This may
                // wait longer than ReadyAt, and can never complete before it.
                float seconds=duration/1000f; progress.usedTime=seconds; progress.fullTime=seconds;
                return true;
            }
            catch (Exception) { Stop(); return false; }
        }
        // Check on the frame after Begin as well as its monotonic 1-second gate.
        // On-device tracing still needs to measure the actual displayed frame.
        public bool IsAnimating()
        {
            if (!active || progress==null || !progress.gameObject.activeInHierarchy || actor==null || actor.lod==null || actor.lod.Model==null) return false;
            if (kind=="fish" && !(ai.GetCurrState() is PlayerAIStateFishing)) return false;
            if (kind=="mine" && !(ai.GetCurrState() is PlayerAIStateMining)) return false;
            LegacyAnimation native=actor.lod.Model.GetComponent<LegacyAnimation>();
            return native!=null && native.IsPlaying(kind=="fish" ? "diaoyu" : "wakuang");
        }
        // Called inside the original read-state completion callback. That native
        // method hides its widget after our callback returns.
        public void CompleteFromNative()
        {
            active=false;
            if (ai!=null && (ai.GetCurrState() is PlayerAIStateFishing || ai.GetCurrState() is PlayerAIStateMining)) ai.SwitchState(0);
            progress=null; preload.RecheckAfterAction();
        }
        public void Stop()
        {
            active=false;
            if (ai!=null && (ai.GetCurrState() is PlayerAIStateFishing || ai.GetCurrState() is PlayerAIStateMining)) ai.SwitchState(0);
            if (progress!=null && progress.gameObject.activeSelf) Hide.Invoke(progress,null);
            progress=null; preload.RecheckAfterAction();
        }
    }
}
