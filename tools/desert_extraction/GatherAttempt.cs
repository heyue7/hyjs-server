using System;
using System.Security.Cryptography;
using System.Text;

namespace HYJS.DesertV4
{
    // Pure timing/receipt state. The caller persists Journal after every transition
    // before issuing the corresponding POST. Server acknowledgement is the only
    // way to release a node; timeout or process shutdown is never acknowledgement.
    // Values are persisted by Unity. Append new phases without renumbering old journals.
    public enum GatherPhase { Starting, Active, Cancelling, Claiming, RecoverReceipt, Finished, PendingDelivery, CheckingClaim }
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
            if ((journal.phase == GatherPhase.Active || journal.phase == GatherPhase.Claiming || journal.phase == GatherPhase.PendingDelivery || journal.phase == GatherPhase.CheckingClaim) && String.IsNullOrEmpty(journal.attemptToken))
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
            if (Journal.phase == GatherPhase.Finished || Journal.phase == GatherPhase.Claiming || Journal.phase == GatherPhase.CheckingClaim) return false;
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
        public void CheckClaimStatus()
        {
            if (Journal.phase != GatherPhase.Claiming || String.IsNullOrEmpty(Journal.attemptToken)) throw new InvalidOperationException("No claim to check");
            Journal.phase = GatherPhase.CheckingClaim;
        }
        void CheckKnownIdentity(string startRequestId, string attemptToken)
        {
            if (startRequestId != Journal.startRequestId || String.IsNullOrEmpty(Journal.attemptToken) || attemptToken != Journal.attemptToken)
                throw new InvalidOperationException("Unexpected gather status identity");
        }
        public void ResumeCheckedClaim(string startRequestId, string attemptToken)
        {
            if (Journal.phase != GatherPhase.CheckingClaim) throw new InvalidOperationException("Claim is not being checked");
            CheckKnownIdentity(startRequestId, attemptToken);
            Journal.phase = GatherPhase.Claiming;
        }
        public void StatusCancellation(string startRequestId, string attemptToken)
        {
            if (Journal.phase != GatherPhase.CheckingClaim && Journal.phase != GatherPhase.RecoverReceipt) throw new InvalidOperationException("No status cancellation expected");
            CheckKnownIdentity(startRequestId, attemptToken);
            Journal.phase = GatherPhase.Finished;
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
            if ((Journal.phase != GatherPhase.Claiming && Journal.phase != GatherPhase.Cancelling && Journal.phase != GatherPhase.RecoverReceipt && Journal.phase != GatherPhase.CheckingClaim) ||
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
        public static string SystemClaimReceiptId(string token)
        {
            if (String.IsNullOrEmpty(token)) throw new ArgumentException("Missing attempt token");
            // UUIDv5 over the same OID namespace and UTF-8 name as the server.
            byte[] prefix={0x6b,0xa7,0xb8,0x12,0x9d,0xad,0x11,0xd1,0x80,0xb4,0x00,0xc0,0x4f,0xd4,0x30,0xc8};
            byte[] name=Encoding.UTF8.GetBytes("hyjs-ggo:claim:"+token), raw=new byte[prefix.Length+name.Length];
            Buffer.BlockCopy(prefix,0,raw,0,prefix.Length); Buffer.BlockCopy(name,0,raw,prefix.Length,name.Length);
            byte[] hash; using (SHA1 sha=SHA1.Create()) hash=sha.ComputeHash(raw);
            hash[6]=(byte)((hash[6]&15)|80); hash[8]=(byte)((hash[8]&63)|128);
            var value=new StringBuilder(36);
            for (int i=0;i<16;i++) { if (i==4 || i==6 || i==8 || i==10) value.Append('-'); value.Append(hash[i].ToString("x2")); }
            return value.ToString();
        }
        public bool MatchesClaimReceipt(string requestId,string startRequestId,string attemptToken)
        {
            if ((Journal.phase!=GatherPhase.Claiming && Journal.phase!=GatherPhase.RecoverReceipt && Journal.phase!=GatherPhase.CheckingClaim) ||
                startRequestId!=Journal.startRequestId || String.IsNullOrEmpty(attemptToken) || attemptToken!=Journal.attemptToken)
                return false;
            // The worker can settle first; accept only that token's exact system
            // UUID or our own persisted request UUID, never an unrelated receipt.
            return requestId==Journal.claimRequestId || requestId==SystemClaimReceiptId(attemptToken);
        }
        public void ClaimReceipt(string requestId, string startRequestId, string attemptToken)
        {
            if (!MatchesClaimReceipt(requestId,startRequestId,attemptToken)) throw new InvalidOperationException("Unexpected claim receipt");
            Journal.phase = GatherPhase.Finished;
        }
    }
}
