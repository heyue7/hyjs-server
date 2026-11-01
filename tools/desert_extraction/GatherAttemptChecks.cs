using System;
using HYJS.DesertV4;

public static class GatherAttemptChecks
{
    static void Check(bool ok, string name) { if (!ok) throw new Exception(name); }
    static GatherAttempt New() { return new GatherAttempt("O01", 8, 1000, 1000); }
    static bool Accept(GatherAttempt a, long now)
    { return a.Accept(a.Journal.startRequestId,"O01",8,"frozen-token",5000,8200,4,now); }
    static void Main()
    {
        var a=New(); Check(Accept(a,1500),"in-time accepted");
        Check(a.RenderingStartedInTime(1550),"in-time actual start");
        Check(!Accept(a,1600) && a.Journal.phase==GatherPhase.Active,"duplicate receipt does not restart or cancel");
        a.Claim(); a.ClaimReceipt(a.Journal.claimRequestId,a.Journal.startRequestId,"frozen-token");
        Check(a.Journal.phase==GatherPhase.Finished,"authoritative claim finishes");

        // ReadyAt worker may settle before the client's claim, retaining the
        // system UUIDv5 bound to this frozen token in the business receipt.
        a=New(); Check(Accept(a,1500),"system receipt start"); a.Claim();
        string system=GatherAttempt.SystemClaimReceiptId("frozen-token");
        Check(system=="381997e6-1d01-5114-837b-0ad86d4ed1ff","Go UUIDv5 parity");
        try { a.ClaimReceipt(system,Guid.NewGuid().ToString("D"),"frozen-token"); throw new Exception("wrong start accepted"); }
        catch (InvalidOperationException) { }
        try { a.ClaimReceipt(system,a.Journal.startRequestId,"other-token"); throw new Exception("wrong token accepted"); }
        catch (InvalidOperationException) { }
        try { a.ClaimReceipt(Guid.NewGuid().ToString("D"),a.Journal.startRequestId,"frozen-token"); throw new Exception("wrong UUID accepted"); }
        catch (InvalidOperationException) { }
        Check(a.Journal.phase==GatherPhase.Claiming,"mismatches keep claim pending");
        a.ClaimReceipt(system,a.Journal.startRequestId,"frozen-token");
        Check(a.Journal.phase==GatherPhase.Finished,"system business receipt finalizes once");
        try { a.ClaimReceipt(system,a.Journal.startRequestId,"frozen-token"); throw new Exception("duplicate finalization accepted"); }
        catch (InvalidOperationException) { }

        a=New(); Check(!a.Expire(1999) && a.Expire(2000),"deadline transition");
        Check(!Accept(a,2001) && a.Journal.phase==GatherPhase.Cancelling,"late start never animates");
        string cancelId=a.Journal.cancelRequestId;
        try { a.CancelReceipt(Guid.NewGuid().ToString("D"),a.Journal.startRequestId,"cancelled"); throw new Exception("wrong receipt accepted"); }
        catch (InvalidOperationException) { }
        Check(a.Journal.phase==GatherPhase.Cancelling && a.Journal.cancelRequestId==cancelId,"uncertain cancel keeps node blocked");
        a.CancelReceipt(cancelId,a.Journal.startRequestId,"cancelled"); Check(a.Journal.phase==GatherPhase.Finished,"tombstone receipt releases node");

        a=New(); Check(Accept(a,1900),"accepted near deadline");
        Check(!a.RenderingStartedInTime(2001) && a.Journal.phase==GatherPhase.Cancelling,"late render cancels accepted action");
        a=GatherAttempt.Restore(a.Journal); Check(a.Journal.phase==GatherPhase.Cancelling,"restart retains cancellation");

        a=New(); Check(Accept(a,1200),"before restart"); a=GatherAttempt.Restore(a.Journal);
        Check(a.Journal.phase==GatherPhase.Cancelling,"incomplete animation cancels on restart");
        a.CancelReceipt(a.Journal.cancelRequestId,a.Journal.startRequestId,"settled");
        Check(a.Journal.phase==GatherPhase.RecoverReceipt,"settled enters recovery");
        a.RecoveredReceipt(a.Journal.startRequestId,"frozen-token"); Check(a.Journal.phase==GatherPhase.Finished,"recovery uses original server receipt");

        a=New(); Check(a.Expire(2000),"cancel precedes delayed start");
        a.CancelReceipt(a.Journal.cancelRequestId,a.Journal.startRequestId,"settled");
        Check(!Accept(a,2200) && a.Journal.attemptToken=="frozen-token" && a.Journal.phase==GatherPhase.RecoverReceipt,"late token retained for settled recovery");
        a.RecoveredReceipt(a.Journal.startRequestId,"frozen-token");

        a=New(); Check(Accept(a,1200),"delivery start"); a.Claim(); string delivery=a.Journal.claimRequestId;
        a.PendingDelivery(a.Journal.startRequestId,"frozen-token"); Check(a.Journal.phase==GatherPhase.PendingDelivery,"full inventory waits for user");
        a=GatherAttempt.Restore(a.Journal); Check(a.Journal.phase==GatherPhase.PendingDelivery,"full inventory survives restart");
        a.ResumeDelivery(); Check(a.Journal.claimRequestId==delivery,"delivery retry keeps claim UUID");
        a.ClaimReceipt(delivery,a.Journal.startRequestId,"frozen-token");
        a=New(); a.Expire(2000); a.PendingDelivery(a.Journal.startRequestId,"recovered-token");
        Check(a.Journal.phase==GatherPhase.PendingDelivery && a.Journal.attemptToken=="recovered-token","cancel can recover pending delivery");
        a=New(); Check(Accept(a,1200),"claim restart start"); a.Claim(); string claim=a.Journal.claimRequestId;
        a=GatherAttempt.Restore(a.Journal); Check(a.Journal.phase==GatherPhase.Claiming && a.Journal.claimRequestId==claim,"claim UUID survives restart");
        a.ClaimReceipt(claim,a.Journal.startRequestId,"frozen-token");

        a=New(); Check(Accept(a,1200),"checked claim start"); a.Claim();
        string checkedStart=a.Journal.startRequestId, checkedClaim=a.Journal.claimRequestId;
        a.CheckClaimStatus();
        Check(!Accept(a,1300) && a.Journal.phase==GatherPhase.CheckingClaim,"late start cannot replace checking");
        a=GatherAttempt.Restore(a.Journal);
        Check(a.Journal.phase==GatherPhase.CheckingClaim && a.Journal.claimRequestId==checkedClaim,"checking and claim UUID survive restart");
        try { a.StatusCancellation(Guid.NewGuid().ToString("D"),"frozen-token"); throw new Exception("wrong status start accepted"); }
        catch (InvalidOperationException) { }
        try { a.StatusCancellation(checkedStart,"other-token"); throw new Exception("wrong status token accepted"); }
        catch (InvalidOperationException) { }
        try { a.StatusCancellation(checkedStart,""); throw new Exception("empty status token accepted"); }
        catch (InvalidOperationException) { }
        Check(a.Journal.phase==GatherPhase.CheckingClaim,"status mismatches stay blocked");
        a.ResumeCheckedClaim(checkedStart,"frozen-token");
        Check(a.Journal.phase==GatherPhase.Claiming && a.Journal.claimRequestId==checkedClaim,"pending retries original claim");
        a.CheckClaimStatus();
        Check(!a.MatchesClaimReceipt(Guid.NewGuid().ToString("D"),checkedStart,"frozen-token"),"checking rejects unrelated claim UUID");
        Check(a.MatchesClaimReceipt(checkedClaim,checkedStart,"frozen-token"),"checking accepts original claim UUID");
        Check(a.MatchesClaimReceipt(system,checkedStart,"frozen-token"),"checking accepts token system UUID");
        a.StatusCancellation(checkedStart,"frozen-token");
        Check(a.Journal.phase==GatherPhase.Finished,"matching cancellation finishes checking");

        a=New(); Check(Accept(a,1200),"checked delivery start"); a.Claim(); a.CheckClaimStatus();
        a.PendingDelivery(a.Journal.startRequestId,"frozen-token");
        Check(a.Journal.phase==GatherPhase.PendingDelivery,"checking can recover pending delivery");
        a=New(); Check(Accept(a,1200),"checked receipt start"); a.Claim(); a.CheckClaimStatus();
        try { a.RecoveredReceipt(a.Journal.startRequestId,"frozen-token"); throw new Exception("checking recovery without claim UUID accepted"); }
        catch (InvalidOperationException) { }
        a.ClaimReceipt(a.Journal.claimRequestId,a.Journal.startRequestId,"frozen-token");
        Check(a.Journal.phase==GatherPhase.Finished,"checking can recover receipt");
        a=New(); Check(Accept(a,1200),"invalid checking journal start"); a.Claim(); a.CheckClaimStatus(); a.Journal.attemptToken="";
        try { GatherAttempt.Restore(a.Journal); throw new Exception("checking journal without token accepted"); }
        catch (ArgumentException) { }

        a=New(); a.Expire(2000); a.CancelReceipt(a.Journal.cancelRequestId,a.Journal.startRequestId,"settled");
        try { a.StatusCancellation(a.Journal.startRequestId,"recovered-token"); throw new Exception("unknown old token accepted"); }
        catch (InvalidOperationException) { }
        a.RecoveredReceipt(a.Journal.startRequestId,"recovered-token");
        Check(a.Journal.phase==GatherPhase.Finished,"legacy recovery without saved token remains supported");

        a=New(); a.DefinitelyRejected(); Check(a.Journal.phase==GatherPhase.Finished,"definitive 7204 releases start");
        a=New(); a.Expire(2000); a.DefinitelyRejected(); Check(a.Journal.phase==GatherPhase.Cancelling,"late rejection cannot bypass cancellation");
        Console.WriteLine("GatherAttempt checks passed");
    }
}
