using System;
using System.Globalization;
using LitJson;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Confirmation authorizes release only. The next gather or attack requires
    // a fresh touch; this class never dispatches a gameplay action.
    public sealed class ProtectionRelease
    {
        readonly FieldWire wire;
        readonly FieldWire.Session session;
        readonly Action<string> notice;
        string requestId;
        bool busy, suspended;
        public ProtectionRelease(FieldWire transport,FieldWire.Session owner,Action<string> report)
        {
            if (transport==null || owner==null || report==null) throw new ArgumentNullException("Protection release dependencies");
            wire=transport; session=owner; notice=report;
            requestId=PlayerPrefs.GetString(Key(),"");
            if (!String.IsNullOrEmpty(requestId))
            {
                try { if (new Guid(requestId).ToString("D")!=requestId) throw new InvalidOperationException(); }
                catch (Exception error) { throw new InvalidOperationException("Protection release journal is unreadable",error); }
            }
        }
        string Key() { return "hyjs.ggo.protection.v4."+Uri.EscapeDataString(session.origin)+"."+session.playerId; }
        bool OwnerCurrent()
        {
            FieldWire.Session current;
            return !suspended && FieldWire.Current(out current) && current.playerId==session.playerId && current.sessionHex==session.sessionHex && current.origin==session.origin;
        }
        public bool Pending { get { return !String.IsNullOrEmpty(requestId); } }
        // Ignore responses owned by an old scene; a new instance reconciles the same UUID.
        public void Suspend() { suspended=true; }
        // Caller invokes this only after an actual confirmation dialog. If a
        // previous request is unresolved, its UUID is reused unchanged.
        public void Confirm()
        {
            if (!OwnerCurrent() || busy) return;
            if (String.IsNullOrEmpty(requestId))
            {
                string next=Guid.NewGuid().ToString("D");
                try { PlayerPrefs.SetString(Key(),next); PlayerPrefs.Save(); requestId=next; }
                catch (Exception) { notice("保护解除记录保存失败，请重试"); return; }
            }
            Send();
        }
        // A previously confirmed request may be reconciled after reconnect.
        public void RetryPending() { if (OwnerCurrent() && !busy && Pending) Send(); }
        void Send()
        {
            string id=requestId; busy=true;
            try
            {
                wire.ReleaseProtection(session,id,delegate(FieldWireResult result)
                {
                    busy=false; if (!OwnerCurrent() || requestId!=id) return;
                    if (result.uncertain || result.code==7002 || result.code==7207)
                    { notice("保护解除结果待确认，请重试"); return; }
                    if (result.code!=0)
                    {
                        Clear(id);
                        notice(result.code==7203 ? "当前动作结束后再解除保护" : "保护解除未完成，请重试");
                        return;
                    }
                    try
                    {
                        JsonData data=result.data;
                        if (data==null || data["requestId"].ToString()!=id || Int64.Parse(data["protectionUntilMs"].ToString(),CultureInfo.InvariantCulture)!=0 || !data["released"].IsBoolean) throw new InvalidOperationException("Invalid release receipt");
                        Clear(id); notice("保护已解除，请再次点击目标");
                    }
                    catch (Exception) { notice("保护解除结果待确认，请重试"); }
                });
            }
            catch (Exception) { busy=false; notice("保护解除结果待确认，请重试"); }
        }
        void Clear(string id)
        {
            if (requestId!=id || PlayerPrefs.GetString(Key(),"")!=id) throw new InvalidOperationException("Protection release journal changed");
            try { PlayerPrefs.DeleteKey(Key()); PlayerPrefs.Save(); requestId=""; }
            catch (Exception) { PlayerPrefs.SetString(Key(),id); throw; }
        }
    }
}
