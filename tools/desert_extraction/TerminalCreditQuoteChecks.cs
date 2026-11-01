using System;
using HYJS.DesertV4;

static class TerminalCreditQuoteChecks
{
    static void Check(bool condition,string reason) { if (!condition) throw new Exception(reason); }
    public static void Main()
    {
        var prices=new long[] {3,9,30,90,300,900};
        var counts=new long[] {0,1,0,0,0,0};
        var guarded=new long[] {0,1,0,0,0,0};
        string text=TerminalCreditQuote.Build(counts,guarded,prices,true,15);
        Check(text.Contains("绿色芯片 1×9（死亡不掉落 1）") && text.Contains("已有信用点 15，本次预计 +9"),"one green");
        counts[1]=guarded[1]=3;
        text=TerminalCreditQuote.Build(counts,guarded,prices,true,15);
        Check(text.Contains("绿色芯片 3×9") && text.Contains("已有信用点 15，本次预计 +27"),"three green");
        counts[0]=2; counts[1]=1; guarded[1]=1;
        Check(TerminalCreditQuote.Build(counts,guarded,prices,true,15).Contains("本次预计 +15"),"mixed chips");
        Check(TerminalCreditQuote.Build(counts,guarded,prices,false,15).Contains("本次金额待确认"),"old server");
        counts[0]=Int64.MaxValue;
        Check(TerminalCreditQuote.Build(counts,guarded,prices,true,15).Contains("本次金额待确认"),"overflow");
        Console.WriteLine("PASS terminal quote: one, three, mixed, old server, overflow");
    }
}
