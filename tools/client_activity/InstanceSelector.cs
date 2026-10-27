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
            if ((type == ActivityType.PrizeTask || type == ActivityType.PrizeBoss || type == ActivityType.AincradStars) && template is UIActivityBattleRace)
            {
                BossMerged merged = template.GetComponent<BossMerged>();
                if (merged == null) merged = template.gameObject.AddComponent<BossMerged>();
                merged.StartMerge((UIActivityBattleRace)template, list, id);
                return;
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
    public static class RechargeDisplay
    {
        public static void Apply(AcRechargeSingleItem row, ActivityItem item)
        {
            if (row == null || item == null || row.type != ActivityType.RechargeTotal) return;
            decimal value;
            if (!Decimal.TryParse(item.value, System.Globalization.NumberStyles.Integer, System.Globalization.CultureInfo.InvariantCulture, out value)) return;
            row.labTitle.text = String.Format(TableTextBlo.GetText("ChargeTotal_award_rmb_num"), (value / 100m).ToString("0.##", System.Globalization.CultureInfo.InvariantCulture));
            row.labLeftTimes.text = (item.left / 100m).ToString("0.##", System.Globalization.CultureInfo.InvariantCulture) + "/" + (item.total / 100m).ToString("0.##", System.Globalization.CultureInfo.InvariantCulture);
        }
    }

    public sealed class BossMerged : MonoBehaviour
    {
        const BindingFlags F = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        UIActivityBattleRace view; readonly List<string> ids = new List<string>();
        readonly Dictionary<string, List<ActivityItem>> data = new Dictionary<string, List<ActivityItem>>();
        readonly Dictionary<string, SCActivityInfo> infos = new Dictionary<string, SCActivityInfo>();
        int generation; bool running;
        static object Get(object o, string n) { return o.GetType().GetField(n, F).GetValue(o); }
        bool Live(int stamp) { return stamp == generation && view != null && view.gameObject.activeInHierarchy && Get(view, "activity") != null; }
        public void StartMerge(UIActivityBattleRace v, List<ActivityTab> tabs, string initial)
        {
            if (running) return;
            bool retry = view == v && ids.Count > 0;
            view = v; ids.Clear(); data.Clear(); infos.Clear();
            foreach (ActivityTab tab in tabs) if (!ids.Contains(tab.activityId)) ids.Add(tab.activityId);
            generation++; running = true;
            // Initial Show has already sent its request; an in-place retry must send its own.
            if (retry) Request(ids[0], generation);
        }
        void OnDisable() { generation++; running = false; data.Clear(); ids.Clear(); infos.Clear(); }
        public static bool Intercept(UIActivityBattleRace v, NetWork.Packets.Packet p, bool ok)
        {
            BossMerged m = v == null ? null : v.GetComponent<BossMerged>();
            if (m == null || !m.running) return false;
            m.Receive(p, ok, m.generation); return true;
        }
        void Receive(NetWork.Packets.Packet packet, bool ok, int stamp)
        {
            if (!Live(stamp)) return;
            SCActivityInfo info = ok && packet != null ? packet.kBody as SCActivityInfo : null;
            if (info == null || !ids.Contains(info.activityId)) { running = false; BoxManager.CreatePopupTextBox("活动加载失败，请重新打开", 0f); return; }
            data[info.activityId] = info.itemList; infos[info.activityId] = info;
            foreach (string id in ids) if (!data.ContainsKey(id)) { Request(id, stamp); return; }
            Draw(stamp);
        }
        void Request(string id, int stamp)
        {
            CSActivityInfo request = new CSActivityInfo(); request.activityId = id;
            if (!NetAPI.SendHttp(57007, request, delegate(NetWork.Packets.Packet p, bool ok) { Receive(p, ok, stamp); }))
            { running = false; BoxManager.CreatePopupTextBox("活动加载失败，请重新打开", 0f); }
        }
        void Refresh(List<ActivityItem> ignored)
        {
            if (!Live(generation)) return;
            data.Clear(); infos.Clear(); generation++; running = true; Request(ids[0], generation);
        }
        void Draw(int stamp)
        {
            if (!Live(stamp)) return;
            long earliest = Int64.MaxValue, latest = Int64.MinValue;
            float offset = 0f; bool firstHeading = true;
            foreach (string id in ids)
            {
                SCActivityInfo info = infos[id];
                if (info.startTime > 0 && info.endTime > info.startTime)
                { earliest = Math.Min(earliest, info.startTime); latest = Math.Max(latest, info.endTime); }
            }
            UILabel activityTime = (UILabel)Get(view, "activityTime");
            activityTime.text = earliest == Int64.MaxValue ? "" :
                ServerTimerTool.Java2CSTimeLocale(earliest).ToString("M月d日HH:mm:ss") + "至" +
                ServerTimerTool.Java2CSTimeLocale(latest).ToString("M月d日HH:mm:ss");
            SimpleGrid grid = (SimpleGrid)Get(view, "simpleGrid");
            GameObject single = (GameObject)Get(view, "vsGetItem"), multi = (GameObject)Get(view, "multiItem");
            UIScrollView scroll = (UIScrollView)Get(view, "scrollView");
            foreach (GameObject child in new List<GameObject>(grid.childs)) UnityEngine.Object.Destroy(child);
            grid.childs.Clear();
            typeof(UIActivityBattleRace).GetMethod("ResetCanGetNum", F).Invoke(view, null);
            foreach (string id in ids)
            {
                SCActivityInfo info = infos[id];
                if (view.type != ActivityType.AincradStars && info.endTime > info.startTime)
                {
                    DateTime start = ServerTimerTool.Java2CSTimeLocale(info.startTime);
                    DateTime end = ServerTimerTool.Java2CSTimeLocale(info.endTime);
                    long duration = info.endTime - info.startTime;
                    string name = duration <= 86400000L ? "每日奖励  " : duration <= 604800000L && duration >= 518400000L ? "每周奖励  " : "";
                    UILabel style = (UILabel)Get(view, "topTips");
                    GameObject heading = (GameObject)UnityEngine.Object.Instantiate(style.gameObject);
                    heading.name = "HYJSBossGroupHeading";
                    foreach (MonoBehaviour component in heading.GetComponents<MonoBehaviour>())
                        if (component != null && component.GetType().Name == "TextKit") UnityEngine.Object.DestroyImmediate(component);
                    UILabel label = heading.GetComponent<UILabel>();
                    label.leftAnchor.target = null; label.rightAnchor.target = null; label.topAnchor.target = null; label.bottomAnchor.target = null;
                    string window = start.ToString("M月d日 HH:mm:ss") + " - " + end.ToString("M月d日 HH:mm:ss");
                    if (info.param != "1")
                    {
                        DateTime deadline = ServerTimerTool.Java2CSTimeLocale(info.rewardEndTime);
                        window = String.Format(TableTextBlo.GetText("getReward_DeadTime"), deadline.ToString("M月d日 HH:mm:ss"));
                    }
                    label.text = name + window;
                    label.height = 32; label.maxLineCount = 1; label.overflowMethod = UILabel.Overflow.ShrinkContent;
                    heading.SetActive(true); grid.AddChild(heading, 1f);
                    // The native row origin is near its bottom; retain the first header's
                    // original cell baseline, compress later header gaps to 32 units.
                    if (!firstHeading) offset -= Math.Max(0f, grid.cellHeight - 32f);
                    heading.transform.localPosition = new Vector3(0f, -offset, 0f); offset += grid.cellHeight;
                    firstHeading = false;
                }
                foreach (ActivityItem item in data[id])
                {
                    GameObject go = (GameObject)UnityEngine.Object.Instantiate(item.rewardList.Count > 1 ? multi : single);
                    go.SetActive(true); grid.AddChild(go, 1f);
                    go.transform.localPosition = new Vector3(0f, -offset, 0f); offset += grid.cellHeight;
                    VsGetItem row = go.GetComponent<VsGetItem>(); row.type = view.type;
                    row.m_update = Refresh;
                    row.m_addNum = delegate { typeof(UIActivityBattleRace).GetMethod("AddCanGetNum", F).Invoke(view, null); };
                    if (item.rewardList.Count > 1) row.SetActivityMultiItem(item, id); else row.SetActivityItem(item, id);
                }
            }
            typeof(UIActivityBattleRace).GetMethod("SetRedByNum", F).Invoke(view, null);
            single.SetActive(false); multi.SetActive(false); scroll.enabled = grid.childs.Count >= 3; scroll.ResetPosition();
        }
    }

}
