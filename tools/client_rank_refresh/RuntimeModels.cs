using System;
using System.Collections.Generic;
using System.Reflection;
using NetWork.Layer;
using ProtoBuf.Message;
using UnityEngine;

namespace HYJS.Ranks
{
    // The table holds weak owners only. HTTPManager itself owns the initial
    // callback until it completes; an abandoned callback must not be kept alive
    // by this helper's selection tracking.
    internal static class SelectionGenerations
    {
        private sealed class Entry
        {
            internal WeakReference Owner;
            internal object Generation;
        }

        private static readonly List<Entry> Entries = new List<Entry>();

        private static Entry Find(MonoBehaviour owner, bool create)
        {
            Entry found = null;
            for (int i = Entries.Count - 1; i >= 0; --i)
            {
                MonoBehaviour current = Entries[i].Owner.Target as MonoBehaviour;
                if (current == null) Entries.RemoveAt(i);
                else if (System.Object.ReferenceEquals(current, owner)) found = Entries[i];
            }
            if (found == null && create)
            {
                found = new Entry();
                found.Owner = new WeakReference(owner);
                Entries.Add(found);
            }
            return found;
        }

        internal static object Begin(MonoBehaviour owner)
        {
            Entry entry = Find(owner, true);
            entry.Generation = new object();
            return entry.Generation;
        }

        internal static bool Current(MonoBehaviour owner, object generation)
        {
            if (owner == null || generation == null) return false;
            Entry entry = Find(owner, false);
            return entry != null && System.Object.ReferenceEquals(entry.Generation, generation);
        }

        internal static void Invalidate(MonoBehaviour owner, object generation)
        {
            if (owner == null || generation == null) return;
            Entry entry = Find(owner, false);
            if (entry != null && System.Object.ReferenceEquals(entry.Generation, generation))
                entry.Generation = new object();
        }
    }

    internal sealed class RankSelection
    {
        private static readonly BindingFlags InstanceFields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        internal MonoBehaviour Owner;
        internal object Generation;
        internal int Opcode;
        internal object Request;
        internal byte[] RequestBytes;
        internal HttpHandler Handler;
        internal UI.UIPlayerData Player;
        internal long PlayerId;
        internal int ZoneId;
        internal string Account;
        internal byte[] Session;
        internal string Url;
        internal UI.Page Page;
        internal FieldInfo SelectionField;
        internal object SelectionValue;
        internal byte[] LastDisplayed;

        internal static bool Allowed(int opcode, object request)
        {
            switch (opcode)
            {
                case 21057: return request is CSRank;
                case 23023: return request is CSFamous;
                case 23009: return request is CSPvpRank;
                case 82059: return request is CSLimitPvpRank;
                case 57071: return request is CSUnionSearch;
                case 61017: return request is CSUFRankInfo;
                case 24901: return request is CSKFRankInfo;
                case 24605: return request is CSWorldBossRank;
                case 81054: return request is CSBigTurnRank;
                default: return false;
            }
        }

        internal static bool EqualBytes(byte[] a, byte[] b)
        {
            if (a == null || b == null || a.Length != b.Length) return false;
            for (int i = 0; i < a.Length; ++i) if (a[i] != b[i]) return false;
            return true;
        }

        internal static RankSelection Capture(MonoBehaviour owner, object generation, int opcode, object request, HttpHandler handler)
        {
            UI.UIPlayerData player = StaticData.playerData;
            byte[] key = StaticData.sessionKey;
            string url = ServerConfig.GetServerUrl();
            if (player == null || player.playerId <= 0 || key == null || key.Length != 16 || String.IsNullOrEmpty(url)) return null;

            RankSelection selection = new RankSelection();
            selection.Owner = owner;
            selection.Generation = generation;
            selection.Opcode = opcode;
            selection.Request = request;
            selection.RequestBytes = PBConvert.toBytes(opcode, request);
            if (selection.RequestBytes == null) return null;
            selection.Handler = handler;
            selection.Player = player;
            selection.PlayerId = player.playerId;
            selection.ZoneId = player.ZoneId;
            selection.Account = StaticData.UserName;
            selection.Session = (byte[])key.Clone();
            selection.Url = url;
            selection.Page = owner.GetComponentInParent<UI.Page>();

            string ownerType = owner.GetType().FullName;
            if (ownerType == "UI.RankPage")
                selection.SelectionField = owner.GetType().GetField("rankPageType", InstanceFields);
            else if (ownerType == "UI.UnionRankPage")
                selection.SelectionField = owner.GetType().GetField("curPage", InstanceFields);
            if ((ownerType == "UI.RankPage" || ownerType == "UI.UnionRankPage") && selection.SelectionField == null) return null;
            if (selection.SelectionField != null)
                selection.SelectionValue = selection.SelectionField.GetValue(owner);
            if (ownerType == "UI.UnionRankPage" &&
                (!(request is CSUnionSearch) || !System.Object.Equals(selection.SelectionValue, ((CSUnionSearch)request).page))) return null;
            return selection;
        }

        // False means this result belongs to a different account, selection or
        // owner. A covered but otherwise unchanged page is handled separately.
        internal bool Owned()
        {
            if (Owner == null || !Owner.enabled || !Owner.gameObject.activeInHierarchy ||
                !SelectionGenerations.Current(Owner, Generation)) return false;
            UI.UIPlayerData player = StaticData.playerData;
            if (player == null || !System.Object.ReferenceEquals(player, Player) || player.playerId != PlayerId || player.ZoneId != ZoneId ||
                !String.Equals(Account, StaticData.UserName, StringComparison.Ordinal) || !EqualBytes(Session, StaticData.sessionKey) ||
                !String.Equals(Url, ServerConfig.GetServerUrl(), StringComparison.Ordinal)) return false;
            if (SelectionField != null && !System.Object.Equals(SelectionValue, SelectionField.GetValue(Owner))) return false;
            if (Page != null && !System.Object.ReferenceEquals(Owner.GetComponentInParent<UI.Page>(), Page)) return false;
            return true;
        }

        internal bool VisiblePage()
        {
            if (Page == null) return true;
            UI.PageManager manager = UI.PageManager.Instance;
            return manager != null && System.Object.ReferenceEquals(manager.currentPage, Page);
        }

        internal bool RequestUnchanged()
        {
            return EqualBytes(RequestBytes, PBConvert.toBytes(Opcode, Request));
        }
    }
}
