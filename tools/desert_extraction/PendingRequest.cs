using System;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Persist only a request UUID, never a login token or a balance. A corrupt
    // record blocks new operations instead of risking a second transaction.
    public sealed class PendingRequest
    {
        readonly string key;
        string id;
        public PendingRequest(FieldWire.Session owner,string operation)
        {
            if (owner==null || String.IsNullOrEmpty(operation)) throw new ArgumentException("Missing request owner");
            key="hyjs.ggo.v4.pending."+operation+"."+Uri.EscapeDataString(owner.origin)+"."+owner.playerId;
            id=PlayerPrefs.GetString(key,"");
            if (!String.IsNullOrEmpty(id) && !Canonical(id)) throw new InvalidOperationException("Unreadable pending request");
        }
        static bool Canonical(string value)
        { try { return new Guid(value).ToString("D")==value; } catch (Exception) { return false; } }
        public string Id { get { return id; } }
        public bool Pending { get { return !String.IsNullOrEmpty(id); } }
        public string Start()
        {
            if (Pending) return id;
            string next=Guid.NewGuid().ToString("D");
            PlayerPrefs.SetString(key,next); PlayerPrefs.Save(); id=next;
            return id;
        }
        public void Finish(string expected)
        {
            if (!Pending || id!=expected || PlayerPrefs.GetString(key,"")!=id) throw new InvalidOperationException("Pending request changed");
            try { PlayerPrefs.DeleteKey(key); PlayerPrefs.Save(); id=""; }
            catch (Exception) { PlayerPrefs.SetString(key,expected); throw; }
        }
    }
}
