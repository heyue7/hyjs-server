using System;

namespace HYJS.DesertV4
{
    // Pure timing/receipt state. The caller persists Journal after every transition
    // before issuing the corresponding POST. Server acknowledgement is the only
    // way to release a node; timeout or process shutdown is never acknowledgement.
    public enum GatherPhase { Starting, Active, Cancelling, Claiming, RecoverReceipt, Finished, PendingDelivery }
    [Serializable]
    public sealed class GatherJournal
    {
        public string startRequestId, cancelRequestId, claimRequestId;
        public string pointId, attemptToken;
        public long generation, acceptedAtMs, readyAtMs;
        public int fatigueCost;
        public GatherPhase phase;
    }
    public sealed class GatherAttempt
    {
        public readonly GatherJournal Journal;
        readonly long deadlineTicks;
        public GatherAttempt(string pointId, long generation, long triggerTicks, long ticksPerSecond)
        {
            if (String.IsNullOrEmpty(pointId) || generation < 1 || ticksPerSecond <= 0 || triggerTicks < 0 || triggerTicks > Int64.MaxValue - ticksPerSecond)
                throw new ArgumentException("Invalid gather start");
            deadlineTicks = triggerTicks + ticksPerSecond;
            Journal = new GatherJournal(); Journal.pointId = pointId; Journal.generation = generation;
            Journal.startRequestId = Guid.NewGuid().ToString("D");
            Journal.cancelRequestId = Guid.NewGuid().ToString("D");
            Journal.claimRequestId = Guid.NewGuid().ToString("D");
            Journal.attemptToken = ""; Journal.phase = GatherPhase.Starting;
        }
        GatherAttempt(GatherJournal journal)
        {
            if (journal == null || String.IsNullOrEmpty(journal.pointId) || journal.generation < 1 ||
                !ValidId(journal.startRequestId) || !ValidId(journal.cancelRequestId) || !ValidId(journal.claimRequestId) ||
                journal.startRequestId == journal.cancelRequestId || journal.startRequestId == journal.claimRequestId || journal.cancelRequestId == journal.claimRequestId ||
                !Enum.IsDefined(typeof(GatherPhase), journal.phase))
                throw new ArgumentException("Invalid persisted gather journal");
            if (journal.phase == GatherPhase.Finished) throw new ArgumentException("Completed journal cannot be resumed");
            if ((journal.phase == GatherPhase.Active || journal.phase == GatherPhase.Claiming || journal.phase == GatherPhase.PendingDelivery) && String.IsNullOrEmpty(journal.attemptToken))
                throw new ArgumentException("Missing attempt token");
            // No saved monotonic deadline survives a process restart. A previously
            // started but unclaimed action must be cancelled before new attempts.
            if (journal.phase == GatherPhase.Starting || journal.phase == GatherPhase.Active) journal.phase = GatherPhase.Cancelling;
            Journal = journal; deadlineTicks = 0;
        }
        public static GatherAttempt Restore(GatherJournal journal) { return new GatherAttempt(journal); }
        static bool ValidId(string value)
        {
            if (String.IsNullOrEmpty(value)) return false;
            try { return new Guid(value).ToString("D") == value; }
            catch (Exception) { return false; }
        }
        public bool Expire(long nowTicks)
        {
            if (Journal.phase != GatherPhase.Starting || nowTicks < deadlineTicks) return false;
            Journal.phase = GatherPhase.Cancelling; return true;
        }
        // true permits the caller to start the actual native action and progress.
        // The caller must call Cancel if that rendering could not start before the
        // same deadline. A late response may carry a token but cannot start work.
        public bool Accept(string requestId, string pointId, long generation, string token, long acceptedAtMs, long readyAtMs, int fatigueCost, long nowTicks)
        {
            if (requestId != Journal.startRequestId || pointId != Journal.pointId || generation != Journal.generation ||
                String.IsNullOrEmpty(token) || acceptedAtMs <= 0 || readyAtMs <= acceptedAtMs || fatigueCost < 0)
                throw new ArgumentException("Start receipt does not match attempt");
            if (Journal.phase == GatherPhase.Finished || Journal.phase == GatherPhase.Claiming) return false;
            if (!String.IsNullOrEmpty(Journal.attemptToken) &&
                (Journal.attemptToken != token || Journal.acceptedAtMs != acceptedAtMs || Journal.readyAtMs != readyAtMs || Journal.fatigueCost != fatigueCost))
                throw new InvalidOperationException("Frozen start receipt changed");
            Journal.attemptToken = token; Journal.acceptedAtMs = acceptedAtMs; Journal.readyAtMs = readyAtMs; Journal.fatigueCost = fatigueCost;
            // Duplicates cannot restart the animation. RecoverReceipt retains a
            // token arriving after the server reported a completed action.
            if (Journal.phase == GatherPhase.Active || Journal.phase == GatherPhase.Cancelling || Journal.phase == GatherPhase.RecoverReceipt || Journal.phase == GatherPhase.PendingDelivery) return false;
            if (nowTicks >= deadlineTicks) { Journal.phase = GatherPhase.Cancelling; return false; }
            Journal.phase = GatherPhase.Active; return true;
        }
        public void Cancel()
        {
            if (Journal.phase == GatherPhase.Starting || Journal.phase == GatherPhase.Active) Journal.phase = GatherPhase.Cancelling;
        }
        public bool RenderingStartedInTime(long nowTicks)
        {
            if (Journal.phase != GatherPhase.Active) return false;
            if (nowTicks < deadlineTicks) return true;
            Journal.phase = GatherPhase.Cancelling; return false;
        }
        public void Claim()
        {
            if (Journal.phase != GatherPhase.Active || String.IsNullOrEmpty(Journal.attemptToken)) throw new InvalidOperationException("No accepted active action");
            Journal.phase = GatherPhase.Claiming;
        }
        public void DefinitelyRejected()
        {
            if (Journal.phase == GatherPhase.Starting) Journal.phase = GatherPhase.Finished;
        }
        public void CancelReceipt(string requestId, string startRequestId, string status)
        {
            if (Journal.phase != GatherPhase.Cancelling || requestId != Journal.cancelRequestId || startRequestId != Journal.startRequestId)
                throw new InvalidOperationException("Unexpected cancel receipt");
            if (status == "cancelled" || status == "already_cancelled") Journal.phase = GatherPhase.Finished;
            else if (status == "settled") Journal.phase = GatherPhase.RecoverReceipt;
            else throw new InvalidOperationException("Unknown cancellation status");
        }
        // Recovery responses are identified by start/token and carry the original
        // server claim receipt, whose request ID can predate this process.
        public void RecoveredReceipt(string startRequestId, string attemptToken)
        {
            if (Journal.phase != GatherPhase.RecoverReceipt || startRequestId != Journal.startRequestId || String.IsNullOrEmpty(attemptToken) ||
                (!String.IsNullOrEmpty(Journal.attemptToken) && Journal.attemptToken != attemptToken))
                throw new InvalidOperationException("Unexpected recovered receipt");
            Journal.attemptToken = attemptToken; Journal.phase = GatherPhase.Finished;
        }
        public void PendingDelivery(string startRequestId, string attemptToken)
        {
            if ((Journal.phase != GatherPhase.Claiming && Journal.phase != GatherPhase.Cancelling && Journal.phase != GatherPhase.RecoverReceipt) ||
                startRequestId != Journal.startRequestId || String.IsNullOrEmpty(attemptToken) ||
                (!String.IsNullOrEmpty(Journal.attemptToken) && Journal.attemptToken != attemptToken))
                throw new InvalidOperationException("Unexpected pending-delivery response");
            Journal.attemptToken=attemptToken; Journal.phase=GatherPhase.PendingDelivery;
        }
        public void ResumeDelivery()
        {
            if (Journal.phase!=GatherPhase.PendingDelivery || String.IsNullOrEmpty(Journal.attemptToken)) throw new InvalidOperationException("No pending delivery");
            Journal.phase=GatherPhase.Claiming;
        }
        public void ClaimReceipt(string requestId, string startRequestId, string attemptToken)
        {
            if ((Journal.phase != GatherPhase.Claiming && Journal.phase != GatherPhase.RecoverReceipt) || requestId != Journal.claimRequestId ||
                startRequestId != Journal.startRequestId || attemptToken != Journal.attemptToken || String.IsNullOrEmpty(attemptToken))
                throw new InvalidOperationException("Unexpected claim receipt");
            Journal.phase = GatherPhase.Finished;
        }
    }
}
