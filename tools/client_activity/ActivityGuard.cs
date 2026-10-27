using System;
using System.Collections.Generic;
using System.Reflection;
using UI;
using UnityEngine;
using ProtoBuf.Message;
using NetWork.Packets;

namespace HYJS.ActivitySafety
{
    // Protects directory and original template initialization only. It never
    // substitutes rewards, completion, IDs, template mappings or server requests.
    public static class ActivityGuard
    {
        const BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        const string EmptyName = "SoulFlowerActivityUnavailable";
        static int lifecycleReports;
        static void Trace(string text)
        {
            Debug.LogWarning("SoulFlowerActivity " + text);
        }

        static object Get(object owner, string name)
        {
            if (owner == null) return null;
            FieldInfo field = owner.GetType().GetField(name, Flags);
            return field == null ? null : field.GetValue(owner);
        }
        static void Set(object owner, string name, object value)
        {
            if (owner == null) return;
            FieldInfo field = owner.GetType().GetField(name, Flags);
            if (field != null) field.SetValue(owner, value);
        }
        static void Call(object owner, string name, object[] args)
        {
            if (owner == null) return;
            MethodInfo method = owner.GetType().GetMethod(name, Flags);
            if (method == null) throw new InvalidOperationException("Original activity method missing: " + name);
            method.Invoke(owner, args);
        }
        static UIActivity Owner(UIActivityPanel panel) { return panel == null ? null : panel.GetComponent<UIActivity>(); }
        static UIActivityPanel Panel(UIActivity page) { return Get(page, "panel") as UIActivityPanel; }
        static Transform Module(UIActivityPanel panel) { return Get(panel, "moduleTrans") as Transform; }
        static UIActivityTemplate Current(UIActivityPanel panel) { return Get(panel, "curActivity") as UIActivityTemplate; }
        static bool Known(ActivityType type) { return type != ActivityType.None && Enum.IsDefined(typeof(ActivityType), type); }
        public static bool HasCurrent(UIActivityPanel panel)
        {
            bool has = Current(panel) != null;
            if (lifecycleReports++ < 4) Trace("lifecycle current=" + has);
            return has;
        }

        static void ClearPanel(UIActivityPanel panel)
        {
            if (panel == null) return;
            UIActivity owner = Owner(panel);
            InstanceSelector selector = owner == null ? null : owner.GetComponent<InstanceSelector>();
            if (selector != null) selector.ClearStrip();
            UIActivityTemplate current = Current(panel);
            Set(panel, "curActivity", null); Set(panel, "activedType", ActivityType.None);
            if (current != null) { current.gameObject.SetActive(false); UnityEngine.Object.Destroy(current.gameObject); }
        }
        static void RemoveEmpty(UIActivityPanel panel)
        {
            Transform root = Module(panel);
            Transform empty = root == null ? null : root.Find(EmptyName);
            if (empty != null) UnityEngine.Object.Destroy(empty.gameObject);
        }
        static void Empty(UIActivityPanel panel, string reason)
        {
            Trace("empty " + reason);
            try
            {
                ClearPanel(panel); RemoveEmpty(panel);
                Transform root = Module(panel);
                UIActivity owner = Owner(panel);
                UILabel[] fonts = owner == null ? new UILabel[0] : owner.GetComponentsInChildren<UILabel>(true);
                if (root != null && fonts.Length > 0)
                {
                    int depth = 0;
                    foreach (UIWidget widget in owner.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, widget.depth);
                    GameObject go = new GameObject(EmptyName); go.layer = panel.gameObject.layer;
                    go.transform.parent = root; go.transform.localPosition = Vector3.zero; go.transform.localScale = Vector3.one;
                    UILabel label = go.AddComponent<UILabel>();
                    label.bitmapFont = fonts[0].bitmapFont; label.trueTypeFont = fonts[0].trueTypeFont;
                    label.fontStyle = fonts[0].fontStyle; label.fontSize = 24;
                    label.width = 520; label.height = 100; label.pivot = UIWidget.Pivot.Center;
                    Color color = fonts[0].color; color.a = 1f;
                    label.alignment = NGUIText.Alignment.Center; label.color = color;
                    label.depth = depth + 5; label.text = "暂无开放活动";
                    Trace("empty-label created module=" + root.name + " active=" + root.gameObject.activeInHierarchy);
                }
                else Trace("empty-label unavailable owner=" + (owner != null) + " module=" + (root != null) + " fonts=" + fonts.Length);
                // InitTabs initializes the navigation with alpha-zero tweeners.
                // The successful original Show path starts these animations;
                // the guarded empty path must reveal that same navigation too.
                List<TweenAlpha> alphas = Get(owner, "alphas") as List<TweenAlpha>;
                if (alphas != null && alphas.Count > 0)
                {
                    Set(owner, "showTimes", 0);
                    Call(owner, "PlayTweener", new object[0]);
                    Trace("empty navigation revealed tabs=" + alphas.Count);
                }
                BoxManager.CreatePopupTextBox("当前暂无开放活动", 0f);
            }
            catch (Exception error) { Trace("empty-render failure " + error.ToString()); }
        }
        static void ClearDirectory(UIActivity page, string reason)
        {
            try { Call(page, "InitTabs", new object[] { new List<ActivityType>() }); }
            catch (Exception error) { Debug.LogWarning("SoulFlower activity directory clear: " + error.GetType().Name); }
            Empty(Panel(page), reason);
        }

        // A closed built-in kitchen still has an original navigation tab. Only
        // its current callback may replace the template with the shared empty state.
        public static bool PrepareKitchenResponse(UIActivityAsunaKitchen page, Packet packet, bool success)
        {
            if (page == null || !page.gameObject.activeInHierarchy) return false;
            UIActivityPanel panel = null;
            for (Transform parent = page.transform; parent != null && panel == null; parent = parent.parent)
                panel = parent.GetComponent<UIActivityPanel>();
            if (panel == null || Current(panel) != page) return false;
            SCStaminaRefill response = packet == null ? null : packet.kBody as SCStaminaRefill;
            if (success && response != null && response.dataList != null && response.dataList.Count >= 2 &&
                response.dataList[0] != null && response.dataList[1] != null) return true;
            Empty(panel, "daily kitchen not open or missing two original windows");
            return false;
        }

        public static bool PrepareResponse(UIActivity page, Packet packet, bool success)
        {
            if (page == null || !page.gameObject.activeInHierarchy) return false;
            SCActivityList response = packet == null ? null : packet.kBody as SCActivityList;
            Trace("response success=" + success + " typed=" + (response != null) + " count=" + (response == null || response.activityList == null ? -1 : response.activityList.Count));
            if (!success || response == null || response.activityList == null)
            {
                ClearDirectory(page, "missing or failed directory response"); return false;
            }
            // The original decoder uses prefix-before-underscore as its exact
            // activity category key. Keep the first occurrence in server order;
            // unknown nonempty prefixes still reach the original mapper unchanged.
            InstanceSelector selector = page.GetComponent<InstanceSelector>();
            if (selector == null) selector = page.gameObject.AddComponent<InstanceSelector>();
            selector.Capture(page, response.activityList);
            Dictionary<string, bool> seen = new Dictionary<string, bool>(StringComparer.Ordinal);
            int index = 0;
            while (index < response.activityList.Count)
            {
                ActivityTab item = response.activityList[index];
                string id = item == null ? null : item.activityId;
                if (String.IsNullOrEmpty(id) || id == "warriorWeightImprove") { response.activityList.RemoveAt(index); continue; }
                int split = id.IndexOf('_');
                string prefix = split < 0 ? id : id.Substring(0, split);
                if (String.IsNullOrEmpty(prefix) || seen.ContainsKey(prefix)) { response.activityList.RemoveAt(index); continue; }
                seen.Add(prefix, true); ++index;
            }
            return true;
        }

        public static bool PrepareTabs(UIActivity page, List<ActivityType> tabs)
        {
            Trace("prepare-tabs count=" + (tabs == null ? -1 : tabs.Count));
            if (tabs == null) { ClearDirectory(page, "null directory tabs"); return false; }
            Dictionary<ActivityType, bool> seen = new Dictionary<ActivityType, bool>();
            int index = 0;
            while (index < tabs.Count)
            {
                ActivityType type = tabs[index];
                if (!Known(type) || seen.ContainsKey(type)) { tabs.RemoveAt(index); continue; }
                seen.Add(type, true); ++index;
            }
            if (tabs.Count == 0) { ClearDirectory(page, "empty directory tabs"); return false; }
            RemoveEmpty(Panel(page));
            return true;
        }

        public static void Show(UIActivityPanel panel, ActivityType type)
        {
            if (panel == null) return;
            UIActivity owner = Owner(panel);
            Trace("show type=" + type + " owner=" + (owner != null));
            Dictionary<ActivityType, string> ids = Get(owner, "activityIds") as Dictionary<ActivityType, string>;
            string id = null;
            bool builtin = type == ActivityType.EveryDaySign || type == ActivityType.AsunaKitchen;
            if (!Known(type) || owner == null || (!builtin && (ids == null || !ids.TryGetValue(type, out id) || String.IsNullOrEmpty(id))))
            {
                Empty(panel, "unknown or unpublished original template " + type); return;
            }
            if (builtin && ids != null) ids.TryGetValue(type, out id);
            UIActivityTemplate old = Current(panel);
            object previous = Get(panel, "activedType");
            if (old != null && previous is ActivityType && (ActivityType)previous == type &&
                (String.IsNullOrEmpty(id) || String.Equals(Get(old, "activityId") as string, id, StringComparison.Ordinal)))
            {
                InstanceSelector sameSelector = owner.GetComponent<InstanceSelector>();
                if (sameSelector != null) sameSelector.Render(type, id, old, Module(panel));
                return;
            }
            ClearPanel(panel); RemoveEmpty(panel);
            Transform root = Module(panel);
            List<Transform> existing = new List<Transform>();
            if (root != null) foreach (Transform child in root) existing.Add(child);
            try
            {
                Set(panel, "activedType", type);
                // Preserve the full original prefab mapping, setup, Init and Hide.
                Trace("template-init begin type=" + type);
                Call(panel, "InitActivity", new object[] { type });
                UIActivityTemplate current = Current(panel);
                if (current == null) throw new InvalidOperationException("Original activity prefab/component unavailable");
                if (!String.IsNullOrEmpty(id)) Set(current, "activityId", id);
                Trace("template-show begin type=" + type + " component=" + current.GetType().Name);
                current.Show();
                UIActivityEveryDaySign sign = current as UIActivityEveryDaySign;
                if (sign != null && sign.GetComponent<DailySignPresentation>() == null) sign.gameObject.AddComponent<DailySignPresentation>();
                InstanceSelector selector = owner.GetComponent<InstanceSelector>();
                if (selector != null) selector.Render(type, id, current, root);
                Call(owner, "PlayTweener", new object[0]);
                Trace("template-show complete type=" + type);
            }
            catch (Exception error)
            {
                if (root != null)
                    foreach (Transform child in root) if (!existing.Contains(child)) UnityEngine.Object.Destroy(child.gameObject);
                Exception cause = error is TargetInvocationException && error.InnerException != null ? error.InnerException : error;
                Empty(panel, "original template " + type + " initialization: " + cause.ToString());
            }
        }
    }
    // Keep native requests/rewards; repair only the daily-sign presentation anchors.
    public sealed class DailySignPresentation : MonoBehaviour
    {
        UILabel caption, unavailableTitle, unavailableReward;
        readonly Dictionary<GameObject, bool> advertisement = new Dictionary<GameObject, bool>();
        static readonly MethodInfo RefreshRedPoints = typeof(UIActivityEveryDaySign).GetMethod("ShowOrHideRedPoint", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        static readonly object[] RedPointArguments = new object[] { null };
        void Anchor(GameObject tip, GameObject tabs, string normal, string selected)
        {
            if (tip == null) return;
            if (tabs == null || !tabs.activeInHierarchy || tip.transform.parent == null) { tip.SetActive(false); return; }
            foreach (Transform button in tabs.GetComponentsInChildren<Transform>(true))
            {
                string name = button.name.Trim();
                if (!button.gameObject.activeInHierarchy || (name != normal && name != selected)) continue;
                UIWidget widget = button.GetComponent<UIWidget>();
                if (widget == null) continue;
                // The three selected-tab strips contain their own button artwork.
                // Keep the red point on the permanent bar, outside those switched groups.
                Vector3 point = tip.transform.parent.InverseTransformPoint(widget.worldCorners[2]);
                tip.transform.localPosition = new Vector3(point.x - 8f, point.y - 8f, 0f);
                tip.transform.localScale = Vector3.one;
                return;
            }
            // Eligibility stays in RedPointTip; an unpublished tab has no visible anchor.
            tip.SetActive(false);
        }
        UILabel StatusLabel(Transform parent, string name, UILabel style, string text, int width)
        {
            if (parent == null || style == null) return null;
            GameObject go = new GameObject(name); go.layer = parent.gameObject.layer;
            go.transform.parent = parent; go.transform.localScale = Vector3.one; go.transform.localRotation = Quaternion.identity;
            UILabel label = go.AddComponent<UILabel>();
            if (style.bitmapFont != null) label.bitmapFont = style.bitmapFont; else label.trueTypeFont = style.trueTypeFont;
            label.material = style.material; label.fontSize = style.fontSize; label.fontStyle = style.fontStyle;
            label.effectStyle = style.effectStyle; label.effectColor = style.effectColor; label.effectDistance = style.effectDistance;
            label.color = style.color; label.width = width; label.height = Math.Max(40, style.fontSize * 2);
            label.maxLineCount = 1; label.overflowMethod = UILabel.Overflow.ShrinkContent;
            label.pivot = UIWidget.Pivot.Center; label.alignment = NGUIText.Alignment.Center;
            int depth = style.depth;
            foreach (UIWidget widget in parent.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, widget.depth);
            label.depth = depth + 5; label.text = text;
            return label;
        }
        void LuxuryAvailability(UIActivityEveryDaySign page, bool available)
        {
            Transform body = page.superSign == null ? null : page.superSign.transform;
            Transform banner = body == null ? null : body.Find("tipbk");
            if (banner == null) return;
            if (advertisement.Count == 0)
            {
                // Text is baked into these original sprite words; the Sprite child is the backdrop.
                foreach (string name in new string[] { "0", "1", "2", "num" })
                {
                    Transform word = banner.Find(name);
                    if (word != null) advertisement[word.gameObject] = word.gameObject.activeSelf;
                }
            }
            foreach (KeyValuePair<GameObject, bool> word in advertisement)
                if (word.Key != null) word.Key.SetActive(available && word.Value);
            if (!available && unavailableTitle == null)
            {
                Transform footer = body.Find("Label");
                UILabel rewardStyle = footer == null ? page.needRecharge : footer.GetComponent<UILabel>();
                // The native recharge label may be transparent while no reward is configured.
                // Use the visible native footer font and an explicit banner foreground color.
                unavailableTitle = StatusLabel(banner, "LuxuryUnavailableTitle", rewardStyle, "暂未开放", 700);
                if (unavailableTitle != null)
                {
                    unavailableTitle.color = Color.white;
                    if (page.needRecharge != null && page.needRecharge.fontSize > 0) unavailableTitle.fontSize = page.needRecharge.fontSize;
                    UIWidget background = banner.GetComponent<UIWidget>();
                    Vector3[] corners = background == null ? null : background.localCorners;
                    unavailableTitle.transform.localPosition = corners == null ? new Vector3(386f, 0f, 0f) : (corners[0] + corners[2]) * .5f;
                }
                unavailableReward = StatusLabel(body, "LuxuryUnavailableReward", rewardStyle, "奖励暂未配置", 310);
                if (unavailableReward != null) unavailableReward.transform.localPosition = new Vector3(-100f, -65f, 0f);
            }
            if (unavailableTitle != null) unavailableTitle.gameObject.SetActive(!available);
            if (unavailableReward != null) unavailableReward.gameObject.SetActive(!available);
        }
        void LateUpdate()
        {
            UIActivityEveryDaySign page = GetComponent<UIActivityEveryDaySign>();
            if (page == null) return;
            // Original IL only reads three RedPointTip flags and projects SetActive; no requests/events.
            if (StaticData.redTips != null && RefreshRedPoints != null) RefreshRedPoints.Invoke(page, RedPointArguments);
            GameObject tabs = page.normalSignTab != null && page.normalSignTab.activeInHierarchy ? page.normalSignTab :
                page.superSignTab != null && page.superSignTab.activeInHierarchy ? page.superSignTab : page.limitSignTab;
            Anchor(page.normalTip, tabs, "everyday", "everyday_actived");
            Anchor(page.superTip, tabs, "super", "super_actived");
            Anchor(page.limitSignTip, tabs, "LimitSign", "LimitSign_actived");
            bool available = !String.IsNullOrEmpty(page.superItems);
            if (page.currentTab != 1) return;
            LuxuryAvailability(page, available);
            if (page.btn != null) page.btn.gameObject.SetActive(available);
            if (!available || page.btn == null || page.needRecharge == null) return;
            if (caption == null)
            {
                GameObject go = new GameObject("DailySignButtonCaption"); go.layer = page.btn.gameObject.layer;
                go.transform.parent = page.btn.transform; go.transform.localPosition = Vector3.zero; go.transform.localScale = Vector3.one;
                caption = go.AddComponent<UILabel>();
                if (page.needRecharge.bitmapFont != null) caption.bitmapFont = page.needRecharge.bitmapFont;
                else caption.trueTypeFont = page.needRecharge.trueTypeFont;
                caption.fontSize = page.needRecharge.fontSize; caption.color = Color.white;
                caption.pivot = UIWidget.Pivot.Center; caption.alignment = NGUIText.Alignment.Center;
                caption.width = 180; caption.height = 40; caption.maxLineCount = 1;
                caption.depth = page.needRecharge.depth + 5;
            }
            caption.text = page.superState == 1 ? "领取" : page.superState == 2 ? "已领取" : "充值";
        }
    }

}
