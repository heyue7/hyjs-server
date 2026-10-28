using System;
using UnityEngine;

namespace HYJS.DesertV4
{
    // One in-flight gather per account/origin. A corrupt journal fails closed;
    // it is never discarded to permit a second draw.
    public static class GatherJournalStore
    {
        static string Key(FieldWire.Session owner)
        {
            if (owner == null || owner.playerId <= 0 || String.IsNullOrEmpty(owner.origin)) throw new ArgumentException("Missing owner");
            return "hyjs.ggo.gather.v4." + Uri.EscapeDataString(owner.origin) + "." + owner.playerId;
        }
        public static void Save(FieldWire.Session owner, GatherAttempt attempt)
        {
            if (attempt == null || attempt.Journal.phase == GatherPhase.Finished) throw new ArgumentException("Only pending journal may be saved");
            string key=Key(owner), raw=PlayerPrefs.GetString(key, "");
            if (!String.IsNullOrEmpty(raw))
            {
                GatherJournal existing;
                try { existing=JsonUtility.FromJson<GatherJournal>(raw); }
                catch (Exception error) { throw new InvalidOperationException("Persisted gather journal is unreadable",error); }
                if (existing == null || existing.startRequestId != attempt.Journal.startRequestId) throw new InvalidOperationException("An earlier gather is still pending");
            }
            PlayerPrefs.SetString(key, JsonUtility.ToJson(attempt.Journal)); PlayerPrefs.Save();
        }
        public static GatherAttempt Load(FieldWire.Session owner)
        {
            string key=Key(owner), raw=PlayerPrefs.GetString(key, "");
            if (String.IsNullOrEmpty(raw)) return null;
            try
            {
                GatherJournal journal=JsonUtility.FromJson<GatherJournal>(raw);
                GatherAttempt attempt=GatherAttempt.Restore(journal);
                // Starting/Active are converted to Cancelling at cold recovery.
                Save(owner,attempt);
                return attempt;
            }
            catch (Exception error) { throw new InvalidOperationException("Gather journal cannot be recovered; keep the node blocked", error); }
        }
        public static void ClearFinished(FieldWire.Session owner, GatherAttempt attempt)
        {
            if (attempt == null || attempt.Journal.phase != GatherPhase.Finished) throw new InvalidOperationException("No final receipt");
            string key=Key(owner), raw=PlayerPrefs.GetString(key, "");
            if (String.IsNullOrEmpty(raw)) return;
            GatherJournal recorded;
            try { recorded=JsonUtility.FromJson<GatherJournal>(raw); }
            catch (Exception error) { throw new InvalidOperationException("Persisted gather journal is unreadable",error); }
            if (recorded == null || recorded.startRequestId != attempt.Journal.startRequestId) throw new InvalidOperationException("Persisted gather owner changed");
            try { PlayerPrefs.DeleteKey(key); PlayerPrefs.Save(); }
            catch (Exception) { PlayerPrefs.SetString(key,raw); throw; }
        }
    }
}
