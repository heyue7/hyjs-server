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
            UIActivityTemplate current = Current(panel);
            Set(panel, "curActivity", null); Set(panel, "activedType", ActivityType.None);
            if (current != null) UnityEngine.Object.Destroy(current.gameObject);
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

        public static bool PrepareResponse(UIActivity page, Packet packet, bool success)
        {
            SCActivityList response = packet == null ? null : packet.kBody as SCActivityList;
            Trace("response success=" + success + " typed=" + (response != null) + " count=" + (response == null || response.activityList == null ? -1 : response.activityList.Count));
            if (!success || response == null || response.activityList == null)
            {
                ClearDirectory(page, "missing or failed directory response"); return false;
            }
            // The original decoder uses prefix-before-underscore as its exact
            // activity category key. Keep the first occurrence in server order;
            // unknown nonempty prefixes still reach the original mapper unchanged.
            Dictionary<string, bool> seen = new Dictionary<string, bool>(StringComparer.Ordinal);
            int index = 0;
            while (index < response.activityList.Count)
            {
                ActivityTab item = response.activityList[index];
                string id = item == null ? null : item.activityId;
                if (String.IsNullOrEmpty(id)) { response.activityList.RemoveAt(index); continue; }
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
                (String.IsNullOrEmpty(id) || String.Equals(Get(old, "activityId") as string, id, StringComparison.Ordinal))) return;
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
}
