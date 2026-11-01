using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.Text;
using Dict.Blo;
using Dict.DataModel;
using LitJson;
using UI;

namespace HYJS.DesertV4
{
    public interface ITerminalView
    {
        void Notice(string text);
        void RequestSnapshot();
    }
    // Original NGUI confirmation and server-authoritative absolute credit.
    // Replayed receipt deltas are only displayed, never re-applied locally.
    public sealed class TerminalController
    {
        readonly FieldWire wire;
        readonly FieldWire.Session session;
        readonly ITerminalView view;
        readonly PendingRequest pending;
        readonly long[] chips=new long[6], protectedChips=new long[6], exchangeValues=new long[6];
        long terminalRevision=-1, credit;
        bool safe, ready, busy, suspended, hasExchangeValues;
        static readonly string[] Colors={"白","绿","蓝","紫","橙","红"};
        public TerminalController(FieldWire transport,FieldWire.Session owner,ITerminalView sink)
        {
            if (transport==null || owner==null || sink==null) throw new ArgumentNullException("Terminal dependencies");
            wire=transport; session=owner; view=sink; pending=new PendingRequest(owner,"settlement");
        }
        static long Number(JsonData d) { return Int64.Parse(d.ToString(),CultureInfo.InvariantCulture); }
        bool OwnerCurrent()
        {
            FieldWire.Session current;
            return !suspended && FieldWire.Current(out current) && current.playerId==session.playerId && current.sessionHex==session.sessionHex && current.origin==session.origin;
        }
        public void Suspend() { suspended=true; }
        public void ApplySnapshot(JsonData snapshot)
        {
            if (!OwnerCurrent()) throw new InvalidOperationException("Terminal owner changed");
            JsonData t=snapshot["terminal"];
            if (t==null || t["chips"].Count!=6 || t["protected"].Count!=6) throw new InvalidOperationException("Incomplete terminal state");
            long rev=Number(t["revision"]), balance=Number(t["creditBalance"]);
            if (rev<terminalRevision || balance<0) throw new InvalidOperationException("Invalid terminal revision or balance");
            var next=new long[6]; var guarded=new long[6];
            for (int i=0;i<6;i++)
            {
                next[i]=Number(t["chips"][i]); guarded[i]=Number(t["protected"][i]);
                if (next[i]<0 || guarded[i]<0 || guarded[i]>next[i]) throw new InvalidOperationException("Invalid terminal chips");
                if (rev==terminalRevision && (next[i]!=chips[i] || guarded[i]!=protectedChips[i])) throw new InvalidOperationException("Changed terminal at same revision");
            }
            if (!snapshot["safe"].IsBoolean) throw new InvalidOperationException("Missing safe-zone state");
            var prices=new long[6]; bool hasPrices=false;
            // Older servers have no optional quote; settlement remains available.
            try
            {
                if (((IDictionary)t).Contains("exchangeValues") && t["exchangeValues"]!=null && t["exchangeValues"].IsArray && t["exchangeValues"].Count==6)
                {
                    hasPrices=true;
                    for (int i=0;i<6;i++) { prices[i]=Number(t["exchangeValues"][i]); if (prices[i]<1) hasPrices=false; }
                }
            }
            catch (Exception) { hasPrices=false; }
            for (int i=0;i<6;i++) { chips[i]=next[i]; protectedChips[i]=guarded[i]; }
            for (int i=0;i<6;i++) exchangeValues[i]=prices[i];
            hasExchangeValues=hasPrices;
            terminalRevision=rev; credit=balance; safe=(bool)snapshot["safe"]; ready=true;
            SetCredit(balance);
        }
        static void SetCredit(long amount)
        {
            if (amount<0 || StaticData.playerData==null) throw new InvalidOperationException("Credit unavailable");
            Dictionary<string,UISimpleItem> inventory=StaticData.playerData.SimpleGoods;
            if (inventory==null) throw new InvalidOperationException("Native inventory unavailable");
            UISimpleItem current;
            if (!inventory.TryGetValue("item_9710",out current) || current==null)
            {
                TableItem table=DictItemBlo.GetTableItem("item_9710");
                if (table==null) throw new InvalidOperationException("Original GGO currency table missing");
                current=new UISimpleItem(table); inventory["item_9710"]=current;
            }
            current.num=amount;
        }
        public void Open()
        {
            if (!OwnerCurrent() || busy) return;
            if (pending.Pending)
            {
                BoxManager.CreateMessageBoxWithCofirmAndCancelButton("上次终端结算尚待确认，重试查询结果？",delegate(bool confirmed)
                { if (confirmed && OwnerCurrent()) Send(); });
                return;
            }
            if (!ready) { view.Notice("终端正在同步，请稍候"); view.RequestSnapshot(); return; }
            if (!safe) { view.Notice("请回到安全区结算"); return; }
            string text=TerminalCreditQuote.Build(chips,protectedChips,exchangeValues,hasExchangeValues,credit);
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton(text,delegate(bool confirmed)
            {
                if (!confirmed || !OwnerCurrent() || busy) return;
                try { pending.Start(); Send(); } catch (Exception) { view.Notice("结算记录保存失败，请重试"); }
            });
        }
        // Called after reconnect only for a previously confirmed operation.
        public void RetryPending() { if (OwnerCurrent() && pending.Pending && !busy) Send(); }
        void Send()
        {
            string id=pending.Id; if (String.IsNullOrEmpty(id) || !OwnerCurrent() || busy) return;
            busy=true;
            try
            {
                wire.Settlement(session,id,delegate(FieldWireResult result)
                {
                    busy=false; if (!OwnerCurrent() || pending.Id!=id) return;
                    if (result.uncertain || result.code==7002 || result.code==7207)
                    { view.Notice("结算结果待确认，请再次点击终端重试"); return; }
                    if (result.code!=0)
                    {
                        try { pending.Finish(id); } catch (Exception) { view.Notice("结算记录待确认，请重试"); return; }
                        if (result.code==7212) view.Notice("回收信用点已达上限，芯片仍保留");
                        else if (result.code==7202) view.Notice("请靠近安全区终端结算员");
                        else view.Notice("当前无法结算，请稍后重试");
                        view.RequestSnapshot(); return;
                    }
                    try
                    {
                        JsonData receipt=result.data;
                        if (receipt==null || (string)receipt["requestId"]!=id || receipt["chips"].Count!=6) throw new InvalidOperationException("Invalid settlement receipt");
                        for (int i=0;i<6;i++) if (Number(receipt["chips"][i])<0) throw new InvalidOperationException("Invalid settled chips");
                        long added=Number(receipt["creditAdded"]), balance=Number(receipt["creditBalance"]), revision=Number(receipt["terminalRevision"]);
                        if (added<0 || balance<0 || revision<0) throw new InvalidOperationException("Invalid settlement values");
                        SetCredit(balance); pending.Finish(id);
                        view.Notice("结算完成，获得 "+added.ToString(CultureInfo.InvariantCulture)+" 回收信用点");
                        view.RequestSnapshot();
                    }
                    catch (Exception) { view.Notice("结算结果待确认，请再次点击终端重试"); }
                });
            }
            catch (Exception) { busy=false; view.Notice("结算结果待确认，请再次点击终端重试"); }
        }
    }
}
