using System;
using System.Globalization;
using System.Text;

namespace HYJS.DesertV4
{
    // Pure presentation only: the server still validates and settles chips.
    internal static class TerminalCreditQuote
    {
        static readonly string[] Colors={"白","绿","蓝","紫","橙","红"};
        internal static string Build(long[] chips,long[] guarded,long[] prices,bool hasPrices,long balance)
        {
            if (chips==null || guarded==null || prices==null || chips.Length!=6 || guarded.Length!=6 || prices.Length!=6 || balance<0)
                throw new ArgumentException("Invalid terminal quote");
            long added=0; bool valid=hasPrices;
            try
            {
                checked
                {
                    for (int i=0;i<6;i++)
                    {
                        if (chips[i]<0 || guarded[i]<0 || guarded[i]>chips[i] || prices[i]<1) valid=false;
                        if (valid) added+=chips[i]*prices[i];
                    }
                    if (valid) { long future=balance+added; if (future<balance) valid=false; }
                }
            }
            catch (OverflowException) { valid=false; }
            var text=new StringBuilder("临时终端\n");
            for (int i=0;i<6;i++)
            {
                text.Append(Colors[i]).Append("色芯片 ").Append(chips[i].ToString(CultureInfo.InvariantCulture));
                if (valid) text.Append("×").Append(prices[i].ToString(CultureInfo.InvariantCulture));
                if (guarded[i]>0) text.Append("（死亡不掉落 ").Append(guarded[i].ToString(CultureInfo.InvariantCulture)).Append("）");
                text.Append("\n");
            }
            text.Append("已有信用点 ").Append(balance.ToString(CultureInfo.InvariantCulture));
            if (valid) text.Append("，本次预计 +").Append(added.ToString(CultureInfo.InvariantCulture));
            else text.Append("，本次金额待确认");
            return text.Append("\n确认结算携带芯片？").ToString();
        }
    }
}
