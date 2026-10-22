using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.InstanceProtection
{
    public static class Protection
    {
        internal const BindingFlags Fields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        static readonly HashSet<string> locked = new HashSet<string>();
        static long owner, revision;
        static string session = "";
        static bool ready, busy;
        static Requests requests;
        internal static string Session() { return StaticData.sessionKey == null ? "" : BitConverter.ToString(StaticData.sessionKey).Replace("-", "").ToLowerInvariant(); }
        internal static bool Same(long id, string key) { return StaticData.playerData != null && StaticData.playerData.playerId == id && key == Session(); }
        internal static bool Ready { get { return ready && Same(owner, session); } }
        internal static bool Busy { get { return busy; } }
        public static bool IsLocked(string guid) { return Ready && guid != null && locked.Contains(guid); }
        public static void ObserveKV(object packet, bool success)
        {
            NetWork.Packets.Packet frame = packet as NetWork.Packets.Packet;
            ProtoBuf.Message.SCKVInfo data = frame == null ? null : frame.kBody as ProtoBuf.Message.SCKVInfo;
            if (!success || data == null || StaticData.playerData == null) return;
            owner = StaticData.playerData.playerId; session = Session(); locked.Clear(); revision = 0;
            foreach (ProtoBuf.Message.KeyValueEntry entry in data.entry)
            {
                if (entry.key == "p5.lockRevision") Int64.TryParse(entry.value, out revision);
                if (entry.key != null && entry.key.StartsWith("p5.lock:") && entry.value == "1") locked.Add(entry.key.Substring(8));
            }
            ready = revision >= 0;
        }
        internal static object Field(object value, string key)
        {
            if (value == null) return null;
            FieldInfo f = value.GetType().GetField(key, Fields);
            return f == null ? null : f.GetValue(value);
        }
        internal static string GuidOf(object value) { return Field(value, "guid") as string; }
        public static List<object> FilterObjects(List<object> source) { return source == null ? new List<object>() : source.FindAll(delegate(object v) { return Ready && v != null && !IsLocked(GuidOf(v)); }); }
        public static List<string> FilterGuids(List<string> source) { return source == null ? new List<string>() : source.FindAll(delegate(string id) { return Ready && !IsLocked(id); }); }
        public static List<UIHero> FilterHeroes(List<UIHero> source) { return source == null ? new List<UIHero>() : source.FindAll(delegate(UIHero v) { return Ready && v != null && !IsLocked(v.guid); }); }
        public static List<UIEquipment> FilterEquips(List<UIEquipment> source) { return source == null ? new List<UIEquipment>() : source.FindAll(delegate(UIEquipment v) { return Ready && v != null && !IsLocked(v.guid); }); }
        public static List<UITreasure> FilterTreasures(List<UITreasure> source) { return source == null ? new List<UITreasure>() : source.FindAll(delegate(UITreasure v) { return Ready && v != null && !IsLocked(v.guid); }); }
        public static string SelectionText(string key) { return key == "no_card_under_blue" ? "没有蓝色及以下的闲置角色可添加" : TableTextBlo.GetText(key); }
        public static void Attach(object value)
        {
            Component page = value as Component;
            if (page == null) return;
            LockControl control = page.GetComponent<LockControl>();
            if (control == null) control = page.gameObject.AddComponent<LockControl>();
            control.Page = page;
        }
        public static void Icon(object value)
        {
            Component icon = value as Component;
            if (icon == null) return;
            LockBadge badge = icon.GetComponent<LockBadge>();
            if (badge == null) badge = icon.gameObject.AddComponent<LockBadge>();
            badge.Icon = icon; badge.Bound = true;
        }
        public static void ClearIcon(object value) { Component icon = value as Component; if (icon == null) return; LockBadge b = icon.GetComponent<LockBadge>(); if (b != null) b.Bound = false; }
        static Requests Runner()
        {
            if (requests == null) { GameObject host = new GameObject("InstanceProtection"); UnityEngine.Object.DontDestroyOnLoad(host); requests = host.AddComponent<Requests>(); }
            return requests;
        }
        internal static void Read() { if (!busy && StaticData.playerData != null) Runner().StartCoroutine(Request(null, false)); }
        internal static void Toggle(string guid) { if (!busy && Ready && !String.IsNullOrEmpty(guid)) Runner().StartCoroutine(Request(guid, !IsLocked(guid))); }
        internal static IEnumerator Request(string guid, bool desired)
        {
            long id = StaticData.playerData.playerId; string key = Session(); long expected = revision;
            string origin = null; try { origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); } catch (Exception) { }
            if (origin == null || key.Length != 32) yield break;
            busy = true;
            string body = "{\"playerId\":" + id + ",\"sessionHex\":\"" + key + "\"";
            if (guid != null) body += ",\"guid\":" + JsonMapper.ToJson(guid) + ",\"locked\":" + (desired ? "true" : "false") + ",\"revision\":" + expected;
            body += "}";
            Dictionary<string, string> headers = new Dictionary<string, string>(); headers.Add("Content-Type", "application/json");
            WWW request = new WWW(origin + "/instance-lock/" + (guid == null ? "state" : "set"), Encoding.UTF8.GetBytes(body), headers);
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool confirmed = false; string error = "状态未确认，请重试";
            try
            {
                if (Same(id, key) && request.isDone && String.IsNullOrEmpty(request.error))
                {
                    JsonData reply = JsonMapper.ToObject(request.text);
                    if ((int)reply["code"] == 0)
                    {
                        JsonData data = reply["data"]; HashSet<string> next = new HashSet<string>();
                        foreach (JsonData item in data["lockedGuids"]) next.Add((string)item);
                        long nextRevision = Convert.ToInt64(data["revision"].ToString());
                        if (nextRevision >= 0 && (guid == null || next.Contains(guid) == desired))
                        {
                            owner = id; session = key; revision = nextRevision; locked.Clear(); foreach (string item in next) locked.Add(item);
                            ready = true; confirmed = true;
                        }
                    }
                    else if (((IDictionary)reply).Contains("msg")) error = (string)reply["msg"];
                }
            }
            catch (Exception) { }
            request.Dispose(); busy = false;
            if (!Same(id, key)) yield break;
            if (!confirmed)
            {
                ready = false; BoxManager.CreatePopupTextBox(error, 0f);
                // An uncertain write is followed by an authoritative read; no local optimistic unlock.
                if (guid != null) Runner().StartCoroutine(Request(null, false));
            }
            else if (guid != null) BoxManager.CreatePopupTextBox(desired ? "已锁定" : "已解锁", 0f);
        }
    }
    public sealed class Requests : MonoBehaviour { }

    public sealed class LockControl : MonoBehaviour
    {
        internal Component Page;
        GameObject button; UILabel caption; string guid;
        GameObject original; Vector3 originalPosition; bool shifted;
        float nextRead;
        static GameObject AsObject(object value) { GameObject go = value as GameObject; Component c = value as Component; return go != null ? go : c == null ? null : c.gameObject; }
        static UILabel Theme(Component page) { foreach (UILabel l in page.GetComponentsInChildren<UILabel>(true)) if (l.bitmapFont != null || l.trueTypeFont != null) return l; return null; }
        GameObject Template()
        {
            BagMemberDetail bag = Page as BagMemberDetail;
            if (bag != null) return bag.publicButton;
            ArrayPage team = Page as ArrayPage;
            if (team != null && team.propertyTab != null) return team.propertyTab.btnSkillSet;
            return AsObject(Protection.Field(Page, "qianghuaBtn"));
        }
        string Current()
        {
            BagMemberDetail bag = Page as BagMemberDetail;
            if (bag != null) return Protection.GuidOf(Protection.Field(bag, "m_hero")) ?? Protection.GuidOf(Protection.Field(bag, "m_equip")) ?? Protection.GuidOf(Protection.Field(bag, "m_treasure"));
            ArrayPage team = Page as ArrayPage;
            if (team != null) { if ((bool)Protection.Field(team, "isOtherPlayer")) return null; return Protection.Field(team, "heroGuid") as string; }
            return Protection.Field(Page, "equipGuid") as string;
        }
        void Make()
        {
            original = Template(); UILabel theme = Theme(Page);
            if (original == null || theme == null) return;
            originalPosition = original.transform.localPosition;
            button = UnityEngine.Object.Instantiate(original) as GameObject;
            button.name = "InstanceLock"; button.transform.parent = original.transform.parent; button.transform.localScale = original.transform.localScale;
            foreach (UIButton b in button.GetComponentsInChildren<UIButton>(true)) b.onClick.Clear();
            foreach (UIEventListener l in button.GetComponentsInChildren<UIEventListener>(true)) l.onClick = null;
            foreach (UILabel l in button.GetComponentsInChildren<UILabel>(true)) l.enabled = false;
            UISprite main = null;
            foreach (UISprite s in button.GetComponentsInChildren<UISprite>(true)) if (main == null || s.width * s.height > main.width * main.height) main = s;
            if (main == null) { UnityEngine.Object.Destroy(button); button = null; return; }
            foreach (UISprite s in button.GetComponentsInChildren<UISprite>(true)) if (s != main) s.enabled = false;
            // Keep the native background dimensions, scale and hit region.
            // The original bag caption is a 44x28 two-character atlas sprite; its glyph size is 22.
            UILabel nativeLabel = original.GetComponentInChildren<UILabel>();
            UISprite nativeGlyph = null;
            foreach (UISprite s in original.GetComponentsInChildren<UISprite>(true))
                if (s.width * s.height < main.width * main.height && s.height >= 12 && s.height <= main.height * .75f && (nativeGlyph == null || s.width > nativeGlyph.width)) nativeGlyph = s;
            foreach (Collider c in button.GetComponentsInChildren<Collider>(true)) c.enabled = false;
            BoxCollider collider = button.GetComponent<BoxCollider>(); if (collider == null) collider = button.AddComponent<BoxCollider>();
            collider.enabled = true;
            BoxCollider nativeCollider = original.GetComponent<BoxCollider>();
            collider.center = nativeCollider == null ? Vector3.zero : nativeCollider.center;
            collider.size = nativeCollider == null ? new Vector3(main.width, main.height, 1f) : nativeCollider.size;
            GameObject text = new GameObject("LockCaption"); text.layer = button.layer; text.transform.parent = button.transform; text.transform.localScale = Vector3.one; text.transform.localPosition = Vector3.zero;
            caption = text.AddComponent<UILabel>(); caption.bitmapFont = theme.bitmapFont; caption.trueTypeFont = theme.trueTypeFont;
            caption.fontSize = nativeLabel != null ? nativeLabel.finalFontSize : nativeGlyph != null ? (Page is BagMemberDetail ? nativeGlyph.width / 2 : nativeGlyph.height) : theme.fontSize;
            caption.width = main.width; caption.height = main.height; caption.pivot = UIWidget.Pivot.Center; caption.alignment = NGUIText.Alignment.Center;
            if (nativeLabel != null) { caption.bitmapFont = nativeLabel.bitmapFont; caption.trueTypeFont = nativeLabel.trueTypeFont; caption.fontStyle = nativeLabel.fontStyle; caption.transform.localScale = nativeLabel.transform.localScale; }
            else if (nativeGlyph != null) caption.transform.localScale = nativeGlyph.transform.localScale;
            caption.depth = main.depth + 2; caption.color = Color.white; caption.effectStyle = theme.effectStyle; caption.effectColor = theme.effectColor;
            UIEventListener.Get(button).onClick = delegate(GameObject ignored) { Protection.Toggle(guid); };
        }
        void LateUpdate()
        {
            if (Page == null || StaticData.playerData == null) return;
            guid = Current(); bool visible = !String.IsNullOrEmpty(guid);
            if (button == null && visible) Make();
            if (button == null) return;
            button.SetActive(visible);
            if (!visible) { if (shifted && original != null) { original.transform.localPosition = originalPosition; shifted = false; } return; }
            if (Page is BagMemberDetail)
            {
                original.transform.localPosition = new Vector3(80f, originalPosition.y, originalPosition.z); shifted = true;
                button.transform.localPosition = new Vector3(-90f, originalPosition.y, originalPosition.z);
            }
            else if (Page is ArrayPage) button.transform.localPosition = originalPosition + new Vector3(-144f, 0f, 0f);
            else button.transform.localPosition = originalPosition + new Vector3(-65f, 345f, 0f);
            bool isLocked = Protection.IsLocked(guid);
            caption.text = Protection.Busy ? "保存中…" : !Protection.Ready ? "读取中…" : isLocked ? "解锁" : "锁定";
            caption.color = isLocked ? new Color(1f, .85f, .25f) : Color.white;
            if (!Protection.Ready && !Protection.Busy && Time.realtimeSinceStartup > nextRead) { nextRead = Time.realtimeSinceStartup + 5f; Protection.Read(); }
        }
        void OnDisable() { if (shifted && original != null) { original.transform.localPosition = originalPosition; shifted = false; } }
    }

    public sealed class LockBadge : MonoBehaviour
    {
        internal Component Icon; internal bool Bound; UILabel caption;
        void LateUpdate()
        {
            if (Icon == null) return;
            HeroIcon hero = Icon as HeroIcon;
            string guid = hero != null ? hero.HeroGuid : Protection.Field(Icon, "guid") as string;
            bool visible = Bound && Protection.IsLocked(guid);
            if (caption == null && visible)
            {
                UILabel theme = null; foreach (UILabel l in Icon.GetComponentsInChildren<UILabel>(true)) if (l.bitmapFont != null || l.trueTypeFont != null) { theme = l; break; }
                if (theme == null) return;
                GameObject go = new GameObject("InstanceLockBadge"); go.layer = Icon.gameObject.layer; go.transform.parent = Icon.transform; go.transform.localScale = Vector3.one; go.transform.localPosition = new Vector3(40f, -8f, -1f);
                caption = go.AddComponent<UILabel>(); caption.bitmapFont = theme.bitmapFont; caption.trueTypeFont = theme.trueTypeFont;
                caption.fontSize = 18; caption.width = 24; caption.height = 24; caption.pivot = UIWidget.Pivot.Center; caption.text = "锁";
                caption.color = new Color(1f, .85f, .25f); caption.effectStyle = UILabel.Effect.Shadow; caption.effectColor = Color.black;
                int depth = 0; foreach (UIWidget w in Icon.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, w.depth); caption.depth = depth + 2;
            }
            if (caption != null) caption.gameObject.SetActive(visible);
        }
    }
}
