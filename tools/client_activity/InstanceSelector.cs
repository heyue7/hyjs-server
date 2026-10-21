using System;
using System.Collections.Generic;
using System.Reflection;
using UI;
using UnityEngine;
using ProtoBuf.Message;

namespace HYJS.ActivitySafety
{
    // The original enum/prefab remains authoritative; only the selected instance ID changes.
    public sealed class InstanceSelector : MonoBehaviour
    {
        const BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        readonly Dictionary<string, List<ActivityTab>> entries = new Dictionary<string, List<ActivityTab>>(StringComparer.Ordinal);
        readonly Dictionary<string, string> selected = new Dictionary<string, string>(StringComparer.Ordinal);
        UIActivity owner;
        GameObject strip;
        int generation;
        bool switching;
        Transform adjustedTemplate;
        Vector3 originalScale;
        Vector3 originalPosition;
        static string Prefix(string id) { int split = id.IndexOf('_'); return split < 0 ? id : id.Substring(0, split); }
        static bool Personal(ActivityTab row) { return row.param == "personal-growth" || row.activityId.IndexOf("_character_", StringComparison.Ordinal) >= 0; }
        public void Capture(UIActivity page, List<ActivityTab> rows)
        {
            ClearStrip(); generation++; owner = page; entries.Clear();
            foreach (ActivityTab row in rows)
            {
                if (row == null || String.IsNullOrEmpty(row.activityId)) continue;
                string prefix = Prefix(row.activityId);
                List<ActivityTab> list;
                if (!entries.TryGetValue(prefix, out list)) { list = new List<ActivityTab>(); entries.Add(prefix, list); }
                bool duplicate = false;
                foreach (ActivityTab prior in list) if (prior.activityId == row.activityId) duplicate = true;
                if (!duplicate) list.Add(row);
            }
            // Put the personal default first before the inherited category decoder deduplicates.
            foreach (List<ActivityTab> list in entries.Values)
            {
                list.Sort(delegate(ActivityTab a, ActivityTab b) { if (Personal(a) != Personal(b)) return Personal(a) ? -1 : 1; return String.CompareOrdinal(a.activityId, b.activityId); });
                if (list.Count > 0)
                {
                    string prefix = Prefix(list[0].activityId);
                    string saved;
                    if (selected.TryGetValue(prefix, out saved))
                    {
                        for (int i = 0; i < list.Count; i++) if (list[i].activityId == saved) { ActivityTab chosen = list[i]; list.RemoveAt(i); list.Insert(0, chosen); break; }
                    }
                    selected[prefix] = list[0].activityId;
                }
            }
            List<ActivityTab> reordered = new List<ActivityTab>();
            Dictionary<string, bool> seen = new Dictionary<string, bool>(StringComparer.Ordinal);
            foreach (ActivityTab row in rows)
            {
                if (row == null || String.IsNullOrEmpty(row.activityId)) continue;
                string prefix = Prefix(row.activityId);
                if (seen.ContainsKey(prefix)) continue;
                seen.Add(prefix, true); reordered.AddRange(entries[prefix]);
            }
            rows.Clear(); rows.AddRange(reordered);
        }
        public void ClearStrip() { if (strip != null) { strip.SetActive(false); UnityEngine.Object.Destroy(strip); strip = null; } }
        void OnDisable() { generation++; ClearStrip(); entries.Clear(); selected.Clear(); owner = null; }
        void OnDestroy() { OnDisable(); }
        public void Render(ActivityType type, string id, UIActivityTemplate template, Transform module)
        {
            ClearStrip();
            if (adjustedTemplate != null)
            {
                adjustedTemplate.localScale = originalScale;
                adjustedTemplate.localPosition = originalPosition;
            }
            if (owner == null || module == null || template == null || String.IsNullOrEmpty(id)) return;
            List<ActivityTab> list;
            if (!entries.TryGetValue(Prefix(id), out list) || list.Count < 2) return;
            if (adjustedTemplate == template.transform)
            {
                adjustedTemplate.localScale = originalScale;
                adjustedTemplate.localPosition = originalPosition;
            }
            else
            {
                adjustedTemplate = template.transform;
                originalScale = adjustedTemplate.localScale;
                originalPosition = adjustedTemplate.localPosition;
            }
            UILabel[] labels = template.GetComponentsInChildren<UILabel>(true);
            if (labels.Length == 0) labels = owner.GetComponentsInChildren<UILabel>(true);
            if (labels.Length == 0) return;
            int active = 0;
            for (int i = 0; i < list.Count; i++) if (list[i].activityId == id) active = i;
            selected[Prefix(id)] = id;
            // Reserve a header above a uniformly reduced original template, retaining its aspect ratio.
            Bounds bounds = new Bounds(Vector3.zero, Vector3.zero); bool found = false;
            int depth = 0;
            foreach (UIWidget widget in template.GetComponentsInChildren<UIWidget>(true))
            {
                depth = Math.Max(depth, widget.depth);
                foreach (Vector3 corner in widget.worldCorners)
                {
                    Vector3 point = module.InverseTransformPoint(corner);
                    if (!found) { bounds = new Bounds(point, Vector3.zero); found = true; } else bounds.Encapsulate(point);
                }
            }
            if (!found || bounds.size.y < 100f) return;
            float ratio = Math.Max(.70f, 1f - 54f / bounds.size.y);
            Vector3 position = originalPosition;
            template.transform.localScale = originalScale * ratio;
            template.transform.localPosition = new Vector3(position.x, position.y - 27f, position.z);
            strip = new GameObject("HYJSPersonalActivityInstances"); strip.layer = module.gameObject.layer;
            strip.transform.parent = module; strip.transform.localScale = Vector3.one;
            strip.transform.localPosition = new Vector3(bounds.center.x, bounds.max.y - 21f, 0f);
            int stamp = generation;
            ActivityTab current = list[active];
            string category = Personal(current) ? "个人成长" : "限时活动";
            string text = category + "  " + (active + 1) + "/" + list.Count;
            Label(strip.transform, labels[0], text, 0, 260, depth + 10);
            Button(strip.transform, labels[0], "‹ 上一个", -210, depth + 10, delegate { Select(type, list, (active + list.Count - 1) % list.Count, stamp); });
            Button(strip.transform, labels[0], "下一个 ›", 210, depth + 10, delegate { Select(type, list, (active + 1) % list.Count, stamp); });
        }
        static UILabel Label(Transform parent, UILabel source, string text, float x, int width, int depth)
        {
            GameObject go = new GameObject("Label"); go.layer = parent.gameObject.layer; go.transform.parent = parent;
            go.transform.localScale = Vector3.one; go.transform.localPosition = new Vector3(x, 0f, 0f);
            UILabel label = go.AddComponent<UILabel>(); label.bitmapFont = source.bitmapFont; label.trueTypeFont = source.trueTypeFont;
            label.fontSize = 20; label.fontStyle = source.fontStyle; label.width = width; label.height = 38;
            label.color = source.color; label.alignment = NGUIText.Alignment.Center; label.pivot = UIWidget.Pivot.Center;
            label.depth = depth; label.text = text; return label;
        }
        static void Button(Transform parent, UILabel source, string text, float x, int depth, UIEventListener.VoidDelegate click)
        {
            UILabel label = Label(parent, source, text, x, 142, depth);
            BoxCollider box = label.gameObject.AddComponent<BoxCollider>(); box.size = new Vector3(142f, 42f, 1f);
            UIEventListener.Get(label.gameObject).onClick = click;
        }
        void Select(ActivityType type, List<ActivityTab> list, int index, int stamp)
        {
            if (switching || stamp != generation || owner == null || !owner.gameObject.activeInHierarchy) return;
            Dictionary<ActivityType, string> ids = typeof(UIActivity).GetField("activityIds", Flags).GetValue(owner) as Dictionary<ActivityType, string>;
            UIActivityPanel panel = typeof(UIActivity).GetField("panel", Flags).GetValue(owner) as UIActivityPanel;
            if (ids == null || panel == null) return;
            switching = true;
            try { ids[type] = list[index].activityId; ActivityGuard.Show(panel, type); }
            finally { switching = false; }
        }
    }
}
