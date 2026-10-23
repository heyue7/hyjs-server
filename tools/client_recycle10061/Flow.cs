using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using UnityEngine;
using UI;
using NetWork.Packets;
using ProtoBuf.Message;
using HYJS.Recycle;

namespace HYJS.Recycle61
{
    // Long-lived state is limited to the one unresolved original protobuf transaction.
    // The native global array and native picker cache remain the selection source of truth.
    public static class Flow
    {
        const BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        sealed class Submission
        {
            public long Owner;
            public string Session;
            public int Kind;
            public string[] Guids;
            public bool Applied;
            public RecycleBreakupPanel Panel;
        }
        static Submission unresolved;

        internal static string Session()
        {
            return StaticData.sessionKey == null ? "" : BitConverter.ToString(StaticData.sessionKey).Replace("-", "").ToLowerInvariant();
        }
        internal static bool Owner(long owner, string session)
        {
            return StaticData.playerData != null && StaticData.playerData.playerId == owner && Session() == session;
        }
        internal static bool Pending()
        {
            if (unresolved != null && !Owner(unresolved.Owner, unresolved.Session)) unresolved = null;
            return unresolved != null;
        }
        public static bool AllowSelection()
        {
            if (!Pending()) return true;
            BoxManager.CreatePopupTextBox("分解结果确认中，请使用网络提示中的重试", 0f);
            return false;
        }
        public static bool AllowPanel(RecycleSelectPanel panel)
        {
            if (!(panel is RecycleBreakupSelectHeroPanel) && !(panel is RecycleBreakupSelectPetPanel)) return true;
            return AllowSelection() && !SelectionControl.IsReading(panel);
        }
        public static void Limit()
        {
            BoxManager.CreatePopupTextBox("每次最多选择100件", 0f);
        }
        internal static int Kind(RecycleSelectPanel panel)
        {
            return panel is RecycleBreakupSelectEquipPanel ? 1 : panel is RecycleBreakupSelectPetPanel ? 2 : 0;
        }
        internal static string[] Cache(RecycleSelectPanel panel)
        {
            Type type = panel is RecycleBreakupSelectPetPanel ? typeof(RecycleBreakupSelectPetPanel) : typeof(RecycleBreakupSelectHeroPanel);
            return (string[])type.GetField("selectCache", Flags).GetValue(panel);
        }
        internal static List<string> Selected(string[] slots)
        {
            var result = new List<string>();
            var unique = new HashSet<string>();
            if (slots == null || slots.Length != SelectionControl.Maximum) throw new InvalidOperationException("Recycle capacity differs");
            foreach (string guid in slots)
            {
                if (String.IsNullOrEmpty(guid)) continue;
                if (guid.Trim() != guid || !unique.Add(guid)) throw new InvalidOperationException("Invalid recycle selection");
                result.Add(guid);
            }
            return result;
        }
        internal static bool AcceptedCallback(bool applied, bool success, bool sameOwner)
        {
            return !applied && success && sameOwner;
        }
        internal static object Message(int kind, List<string> selected)
        {
            if (kind == 1) { var body = new CSEquipDecompose(); body.guid.AddRange(selected); return body; }
            if (kind == 0) { var body = new CSWarriorDecompose(); body.guid.AddRange(selected); return body; }
            if (kind == 2) { var body = new CSPetDecompose(); body.guids.AddRange(selected); return body; }
            throw new InvalidOperationException("Invalid recycle type");
        }
        static void Restore(Submission request)
        {
            if (!Owner(request.Owner, request.Session)) return;
            Array.Clear(StaticData.recycleBreakupSelect, 0, StaticData.recycleBreakupSelect.Length);
            Array.Copy(request.Guids, StaticData.recycleBreakupSelect, request.Guids.Length);
            StaticData.curRecycleBreakSelectType = request.Kind;
        }
        // Native close, type changes and successful callbacks all call this same method.
        // Closing an unresolved request retains its exact GUID set until the original retry resolves it.
        public static void ClearGlobal()
        {
            if (Pending()) { Restore(unresolved); return; }
            if (StaticData.recycleBreakupSelect != null)
                Array.Clear(StaticData.recycleBreakupSelect, 0, StaticData.recycleBreakupSelect.Length);
        }
        public static void QuickOpen(RecycleBreakupPanel panel, int kind)
        {
            if (!AllowSelection() || panel == null) return;
            if (StaticData.playerData == null || Session().Length != 32) return;
            if ((int)StaticData.curRecycleBreakSelectType != kind) StaticData.ClearRecycleBreakupSelect();
            StaticData.curRecycleBreakSelectType = kind;
            // The covered main page may be disabled by PageManager while its picker opens.
            QuickSelection control = PageManager.Instance.GetComponent<QuickSelection>();
            if (control == null) control = PageManager.Instance.gameObject.AddComponent<QuickSelection>();
            PageManager.Instance.OpenPage("RecycleBreakupSelectPage", "type=" + kind);
            control.Begin(kind, StaticData.playerData.playerId, Session());
        }
        public static void Submit(RecycleBreakupPanel panel)
        {
            if (!AllowSelection() || panel == null || StaticData.playerData == null) return;
            List<string> selected;
            try { selected = Selected(StaticData.recycleBreakupSelect); }
            catch (Exception) { BoxManager.CreatePopupTextBox("选择已变化，请重新选择材料", 0f); return; }
            int kind = (int)StaticData.curRecycleBreakSelectType;
            if (selected.Count == 0 || kind < 0 || kind > 2) return;
            var request = new Submission { Owner = StaticData.playerData.playerId, Session = Session(), Kind = kind, Guids = selected.ToArray(), Panel = panel };
            if (request.Session.Length != 32) return;
            object message = Message(kind, selected);
            int opcode = kind == 1 ? 21009 : kind == 0 ? 21021 : 21517;
            unresolved = request;
            bool sent = NetAPI.SendHttp(opcode, message, delegate(Packet packet, bool success) { Complete(request, packet, success); });
            if (!sent && System.Object.ReferenceEquals(unresolved, request)) unresolved = null;
            DrawMain(panel);
        }
        static void Complete(Submission request, Packet packet, bool success)
        {
            if (!Owner(request.Owner, request.Session) || request.Applied || !System.Object.ReferenceEquals(unresolved, request)) return;
            if (!success)
            {
                Restore(request);
                // The native transport may report a malformed/unknown result using the original request packet.
                // Only a real server ErrorMessage proves that the transaction was rejected.
                if (packet != null && packet.kBody is ErrorMessage) unresolved = null;
                DrawMain(request.Panel);
                return;
            }
            if (!AcceptedCallback(request.Applied, success, Owner(request.Owner, request.Session))) return;
            object body = packet == null ? null : packet.kBody;
            if (request.Kind == 1 && !(body is SCEquipDecompose) || request.Kind == 0 && !(body is SCWarriorDecompose) || request.Kind == 2 && !(body is SCPetDecompose)) return;
            request.Applied = true;
            unresolved = null;
            // Restore the submitted type before invoking the untouched original cost/reward/animation callback.
            Restore(request);
            if (request.Panel != null && request.Panel.gameObject.activeInHierarchy)
            {
                string name = request.Kind == 1 ? "<ComfirmBreakup>m__1AA" : request.Kind == 0 ? "<ComfirmBreakup>m__1AB" : "<ComfirmBreakup>m__1AC";
                typeof(RecycleBreakupPanel).GetMethod(name, Flags).Invoke(request.Panel, new object[] { packet, true });
            }
            else
            {
                // A page may have closed while the original timeout dialog was open.
                // Apply the same original data functions without starting an animation on an inactive object.
                if (body is SCEquipDecompose) { var reply = (SCEquipDecompose)body; StaticData.playerData.Delete(reply.cost); StaticData.playerData.AddResultObj(reply.get); }
                else if (body is SCWarriorDecompose) { var reply = (SCWarriorDecompose)body; StaticData.playerData.Delete(reply.cost); StaticData.playerData.AddResultObj(reply.get); }
                else { var reply = (SCPetDecompose)body; StaticData.playerData.Delete(reply.cost); StaticData.playerData.AddResultObj(reply.get); }
                StaticData.ClearRecycleBreakupSelect();
            }
        }
        public static void DrawMain(RecycleBreakupPanel panel)
        {
            if (panel == null) return;
            SelectionCounter counter = panel.GetComponent<SelectionCounter>();
            if (counter == null) counter = panel.gameObject.AddComponent<SelectionCounter>();
            counter.Main = panel;
            counter.Refresh();
        }
        public static void DrawPanel(RecycleSelectPanel panel)
        {
            if (!(panel is RecycleBreakupSelectHeroPanel) && !(panel is RecycleBreakupSelectPetPanel)) return;
            SelectionCounter counter = panel.GetComponent<SelectionCounter>();
            if (counter == null) counter = panel.gameObject.AddComponent<SelectionCounter>();
            counter.Picker = panel;
            counter.Refresh();
        }
    }

    public sealed class QuickSelection : MonoBehaviour
    {
        int generation;
        internal void Begin(int kind, long owner, string session)
        {
            generation++; StopAllCoroutines(); StartCoroutine(Open(generation, kind, owner, session));
        }
        IEnumerator Open(int current, int kind, long owner, string session)
        {
            yield return null;
            float deadline = Time.realtimeSinceStartup + 3f;
            RecycleSelectPanel panel = null;
            while (Time.realtimeSinceStartup < deadline)
            {
                if (current != generation || !Flow.Owner(owner, session)) yield break;
                RecycleBreakupSelectPage page = PageManager.Instance.currentPage as RecycleBreakupSelectPage;
                if (page != null && page.gameObject.activeInHierarchy)
                {
                    panel = kind == 1 ? page.equipPanel : kind == 2 ? (RecycleSelectPanel)page.petPanel : page.heroPanel;
                    if (panel != null && panel.gameObject.activeInHierarchy) break;
                }
                panel = null;
                yield return null;
            }
            if (panel == null) yield break;
            UIToggle toggle = panel is RecycleBreakupSelectPetPanel ? ((RecycleBreakupSelectPetPanel)panel).checkbox : ((RecycleBreakupSelectHeroPanel)panel).checkbox;
            toggle.value = true;
            SelectionControl.Select(panel);
        }
        void OnDisable() { generation++; StopAllCoroutines(); }
    }

    public sealed class SelectionCounter : MonoBehaviour
    {
        internal RecycleBreakupPanel Main;
        internal RecycleSelectPanel Picker;
        UILabel countLabel;
        float nextRefresh;
        readonly Dictionary<UIButton, bool> disabled = new Dictionary<UIButton, bool>();
        static void CopyStyle(UILabel source, UILabel label)
        {
            foreach (string name in new string[] { "bitmapFont", "trueTypeFont", "font", "fontSize", "fontStyle", "effectStyle", "effectColor", "effectDistance", "applyGradient", "gradientTop", "gradientBottom", "spacingX", "spacingY", "color", "pivot", "alignment", "overflowMethod" })
            {
                PropertyInfo property = typeof(UILabel).GetProperty(name);
                if (property != null && property.CanRead && property.CanWrite)
                    try { property.SetValue(label, property.GetValue(source, null), null); } catch (Exception) { }
            }
        }
        internal void Refresh()
        {
            if (Main == null && Picker == null) return;
            if (countLabel == null) Create();
            if (countLabel == null) return;
            string[] slots = Main != null ? StaticData.recycleBreakupSelect : Flow.Cache(Picker);
            int count = 0;
            if (slots != null) foreach (string guid in slots) if (!String.IsNullOrEmpty(guid)) count++;
            bool pending = Flow.Pending();
            countLabel.text = "已选 " + count + "/100" + (Main != null ? (pending ? "\n结果确认中" : "\n点击材料区查看") : SelectionControl.IsReading(Picker) ? "  正在选择…" : "");
            // Preserve controls that were already disabled by their original owner.
            foreach (UIButton button in GetComponentsInChildren<UIButton>(true))
            {
                if (button == null || button.name != "ConfirmBtn") continue;
                if (pending && !disabled.ContainsKey(button)) { disabled.Add(button, button.isEnabled); button.isEnabled = false; }
            }
            if (!pending) Release();
        }
        void Release()
        {
            foreach (KeyValuePair<UIButton, bool> pair in disabled) if (pair.Key != null) pair.Key.isEnabled = pair.Value;
            disabled.Clear();
        }
        void Create()
        {
            UILabel source = null;
            UIWidget confirm = null;
            UIWidget backdrop = null;
            if (Picker != null)
            {
                // The native ConfirmBtn caption is a Sprite, so it has no UILabel child.
                // Bind the actual footer label and widgets, never a material row label.
                Transform labelNode = Picker.transform.Find("Main/OneKeyChose/Label");
                Transform buttonNode = Picker.transform.Find("Main/ConfirmBtn");
                Transform backdropNode = Picker.transform.Find("Main/Di");
                if (labelNode == null || buttonNode == null || backdropNode == null) return;
                source = labelNode.GetComponent<UILabel>();
                confirm = buttonNode.GetComponent<UIWidget>();
                backdrop = backdropNode.GetComponent<UIWidget>();
                if (source == null || confirm == null || backdrop == null) return;
            }
            else
                foreach (UILabel candidate in GetComponentsInChildren<UILabel>(true))
                {
                    if (candidate.name == "HYJSRecycleCount61") continue;
                    if (source == null) source = candidate;
                    if (candidate.transform.parent != null && candidate.transform.parent.name == "ConfirmBtn") { source = candidate; break; }
                }
            if (source == null) return;
            var go = new GameObject("HYJSRecycleCount61");
            go.layer = source.gameObject.layer;
            countLabel = go.AddComponent<UILabel>();
            CopyStyle(source, countLabel);
            countLabel.width = 230; countLabel.height = Main != null ? 62 : 36;
            countLabel.depth = source.depth + 3;
            countLabel.fontSize = 20;
            countLabel.applyGradient = false;
            countLabel.pivot = UIWidget.Pivot.Center;
            countLabel.alignment = NGUIText.Alignment.Center;
            go.transform.parent = transform;
            go.transform.localRotation = Quaternion.identity;
            go.transform.localScale = Vector3.one;
            if (Main != null && Main.items != null && Main.items.Count >= 4)
            {
                Vector3 center = Vector3.zero;
                for (int index = 0; index < 4; index++) center += transform.InverseTransformPoint(Main.items[index].transform.position);
                go.transform.localPosition = center / 4f;
                countLabel.color = Color.white;
                countLabel.effectStyle = UILabel.Effect.Outline;
                countLabel.effectColor = new Color(0f, 0f, 0f, 0.85f);
            }
            else
            {
                // Share Main's native tween/alpha coordinate system with the footer button.
                Transform footer = confirm.transform.parent;
                go.transform.parent = footer;
                go.transform.localScale = Vector3.one;
                float buttonRight = Single.MinValue;
                float panelRight = Single.MinValue;
                foreach (Vector3 point in confirm.worldCorners) buttonRight = Mathf.Max(buttonRight, footer.InverseTransformPoint(point).x);
                foreach (Vector3 point in backdrop.worldCorners) panelRight = Mathf.Max(panelRight, footer.InverseTransformPoint(point).x);
                float inset = Mathf.Max(12f, source.fontSize);
                float left = buttonRight + inset;
                float right = panelRight - inset;
                if (right - left < 80f) { Destroy(go); countLabel = null; return; }
                Vector3 anchor = footer.InverseTransformPoint(confirm.transform.position);
                countLabel.width = Mathf.FloorToInt(right - left);
                go.transform.localPosition = new Vector3((left + right) / 2f, anchor.y, anchor.z);
                countLabel.color = new Color(0.14f, 0.14f, 0.14f, 1f);
                countLabel.effectStyle = UILabel.Effect.None;
            }
        }
        void Update() { if (Time.realtimeSinceStartup >= nextRefresh) { nextRefresh = Time.realtimeSinceStartup + 0.15f; Refresh(); } }
        void OnDisable() { Release(); }
    }
}
