using System.Collections.Generic;
using System.Reflection;
using ProtoBuf.Message;
using UI;

namespace HYJS.ArenaState
{
    /// <summary>
    /// Narrow client-state bridge for the original arena panels.
    /// The helper never builds battle data, rewards or targets; it only keeps the
    /// original 18-slot cache consistent with the server frame that the original
    /// UI just rendered, hides the prefab placeholders before the first response,
    /// and reports a successful empty rival list.
    /// </summary>
    public static class ArenaStateSync
    {
        static FieldInfo grabListField;

        // UI.GrabPVPPanel.grabList is private; resolve it once and reuse the same list.
        static List<GrabPositionModel> GrabList(GrabPVPPanel panel)
        {
            if (grabListField == null)
            {
                grabListField = typeof(GrabPVPPanel).GetField(
                    "grabList", BindingFlags.Instance | BindingFlags.NonPublic);
            }
            if (grabListField == null) return null;
            return grabListField.GetValue(panel) as List<GrabPositionModel>;
        }

        /// <summary>
        /// Runs after the original GrabPVPItem.SetItem rendered one server frame.
        /// Both the occupation reply and the give-up reply pass through SetItem, so
        /// updating the position entry here keeps OnTabClick's cache correct.
        /// </summary>
        public static void SyncGrabCache(GrabPVPPanel panel, GrabPositionModel data)
        {
            if (panel == null || data == null) return;
            List<GrabPositionModel> cache = GrabList(panel);
            if (cache != null)
            {
                for (int index = 0; index < cache.Count; index++)
                {
                    GrabPositionModel entry = cache[index];
                    if (entry == null || entry.position != data.position) continue;
                    // OnTabClick enumerates this list and passes the same object back;
                    // replacing it would change List.version during the enumeration.
                    if (!object.ReferenceEquals(entry, data)) cache[index] = data;
                    break;
                }
            }
            // Recompute the original "I hold one point" flag from the real cache.
            // An empty cache means the original Refresh is still in flight, so keep
            // the callback's current flag instead of clearing it without evidence.
            if (cache == null || cache.Count == 0) return;
            bool self = false;
            long playerId = 0;
            if (StaticData.playerData != null) playerId = StaticData.playerData.playerId;
            for (int index = 0; index < cache.Count; index++)
            {
                GrabPositionModel entry = cache[index];
                if (entry != null && entry.playerId > 0 && entry.playerId == playerId)
                {
                    self = true;
                    break;
                }
            }
            panel.isSelfGrabed = self;
        }

        /// <summary>
        /// Hides the prefab target slots at PlunderSelectPage.DoOpen entry. The
        /// original 0.5s delay and CSTreasureBattleList request are untouched; the
        /// original callback shows only the real infoList entries.
        /// </summary>
        public static void ClearPlunderTargets(PlunderSelectPage page)
        {
            if (page == null || page.targetItems == null) return;
            for (int index = 0; index < page.targetItems.Length; index++)
            {
                PlunderTargetItem item = page.targetItems[index];
                if (item == null) continue;
                item.Clear();
                item.gameObject.SetActive(false);
            }
        }

        /// <summary>
        /// Injected on the success fall-through of GetRivalCallback only. The
        /// failure branch jumps straight to the original ret and never reaches it.
        /// </summary>
        public static void ShowEmptyRivals(PlunderSelectPage page, SCTreasureBattleList response)
        {
            if (page == null) return;
            if (!page.gameObject.activeInHierarchy) return;
            if (response == null || response.infoList == null || response.infoList.Count != 0) return;
            BoxManager.CreatePopupTextBox("当前没有可夺取该碎片的对手，请换个碎片或稍后刷新。", 0f);
        }
    }
}
