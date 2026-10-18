using System;
using System.Collections;
using System.Reflection;
using UI;
using UnityEngine;

namespace HYJS.WeaponRefresh
{
    // Rebuild the existing visible inventory from its current ownership model.
    // Conversion equips the same GUID; no item is created, removed or charged.
    public static class WeaponRefresh
    {
        internal const BindingFlags Members = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;

        public static void AfterClose()
        {
            try
            {
                GameObject temporary = new GameObject("SoulFlowerWeaponRefresh");
                temporary.hideFlags = HideFlags.HideAndDontSave;
                temporary.AddComponent<RefreshAfterRemodel>();
            }
            catch (Exception error) { Debug.LogWarning("Weapon inventory refresh: " + error.GetType().FullName); }
        }

        internal static void RefreshVisible()
        {
            foreach (UnityEngine.Object value in UnityEngine.Object.FindObjectsOfType(typeof(EquipTab)))
            {
                EquipTab tab = value as EquipTab;
                if (tab == null || !tab.gameObject.activeInHierarchy) continue;
                FieldInfo ownerField = typeof(EquipTab).GetField("arrayPage", Members);
                ArrayPage page = ownerField == null ? null : ownerField.GetValue(tab) as ArrayPage;
                FieldInfo otherField = typeof(ArrayPage).GetField("isOtherPlayer", Members);
                if (page == null || otherField == null || (bool)otherField.GetValue(page)) continue;
                // UpdateTab keeps curType, so the selected equipment category stays put.
                typeof(EquipTab).GetMethod("UpdateTab", Members).Invoke(tab, null);
            }

            foreach (UnityEngine.Object value in UnityEngine.Object.FindObjectsOfType(typeof(BagCenterPanel)))
            {
                BagCenterPanel panel = value as BagCenterPanel;
                if (panel == null || !panel.gameObject.activeInHierarchy) continue;
                BagPage page = typeof(BagCenterPanel).GetField("bagPage", Members).GetValue(panel) as BagPage;
                if (page == null) continue;
                int kind = (int)typeof(BagPage).GetField("curType", Members).GetValue(page);
                if (kind != 3) continue;
                // Keep curType/curEquipInnerTab; clear only cached icons and rebuild
                // through the original coroutine, including its worn-item labels.
                typeof(BagCenterPanel).GetField("curEquipCount", Members).SetValue(panel, 0);
                typeof(BagCenterPanel).GetField("curTreasureCount", Members).SetValue(panel, 0);
                panel.StopCoroutine("UpdateScrollView");
                typeof(BagCenterPanel).GetMethod("ClearLastTypeIcons", Members).Invoke(panel, new object[] { kind, kind });
                IEnumerator refresh = (IEnumerator)typeof(BagCenterPanel).GetMethod("UpdateScrollView", Members).Invoke(panel, new object[] { kind });
                panel.StartCoroutine(refresh);
            }
        }
    }

    public sealed class RefreshAfterRemodel : MonoBehaviour
    {
        IEnumerator Start()
        {
            // Let PageManager restore the uncovered page before selecting active
            // containers; keep the original conversion animation and close order.
            yield return null;
            yield return null;
            try { WeaponRefresh.RefreshVisible(); }
            catch (Exception error) { Debug.LogWarning("Weapon inventory refresh: " + error.GetType().FullName); }
            finally { UnityEngine.Object.Destroy(gameObject); }
        }
    }
}
