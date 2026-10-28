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
        a=New(); a.DefinitelyRejected(); Check(a.Journal.phase==GatherPhase.Finished,"definitive 7204 releases start");
        a=New(); a.Expire(2000); a.DefinitelyRejected(); Check(a.Journal.phase==GatherPhase.Cancelling,"late rejection cannot bypass cancellation");
        Console.WriteLine("GatherAttempt checks passed");
    }
}
