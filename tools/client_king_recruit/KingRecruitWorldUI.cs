using System;
using System.Collections;
using System.Collections.Generic;
using System.Text;
using System.Reflection;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.KingRecruit
{
    public static class KingRecruitWorldUI
    {
        public static void Ensure(TabKingsRecruit tab)
        {
            if (tab == null) return;
            KingRecruitWorldControl control = tab.GetComponent<KingRecruitWorldControl>();
            if (control == null) control = tab.gameObject.AddComponent<KingRecruitWorldControl>();
            control.Initialize(tab);
        }
        public static bool CanAct(TabKingsRecruit tab)
        {
            KingRecruitWorldControl control = tab == null ? null : tab.GetComponent<KingRecruitWorldControl>();
            return control != null && control.Ready;
        }
        public static void Received(TabKingsRecruit tab)
        {
            KingRecruitWorldControl control = tab == null ? null : tab.GetComponent<KingRecruitWorldControl>();
            if (control != null) control.Received();
        }
        public static void BeginDraw(TabKingsRecruit tab)
        {
            KingRecruitWorldControl control = tab == null ? null : tab.GetComponent<KingRecruitWorldControl>();
            if (control != null) control.BeginDraw();
        }
        public static void EndDraw(TabKingsRecruit tab)
        {
            KingRecruitWorldControl control = tab == null ? null : tab.GetComponent<KingRecruitWorldControl>();
            if (control != null) control.EndDraw();
        }
        public static string OriginalCardPath(string path)
        {
            if (path == "hero2d/hero_2d_069") return "heropainter/hero_card_069";
            if (path == "hero2d/hero_2d_070") return "heropainter/hero_card_070";
            if (path == "hero2d/hero_2d_073") return "heropainter/hero_card_073";
            return path;
        }
        public static void BindCard(RecruitResultCardItem item, string buyId)
        {
            if (item == null || item.heroCard == null) return;
            string id = new UIHero(buyId).heroId;
            OriginalRecruitPortrait control = item.heroCard.GetComponent<OriginalRecruitPortrait>();
            bool special = id == "hero_069" || id == "hero_070" || id == "hero_073";
            if (control == null && special) control = item.heroCard.gameObject.AddComponent<OriginalRecruitPortrait>();
            if (control != null) control.Bind(special ? id.Substring(5) : null);
        }
    }

    // The original painter is loaded through the normal bundle lifecycle.
    public sealed class OriginalRecruitPortrait : MonoBehaviour
    {
        string id;
        Texture last;
        public void Bind(string value) { id = value; last = null; }
        void LateUpdate()
        {
            if (id == null) return;
            UITexture texture = GetComponent<UITexture>();
            if (texture == null || texture.mainTexture == null || texture.mainTexture == last) return;
            last = texture.mainTexture;
            HYJS.HeroArt.HeroArtFallback.AssignRequested(texture, last, "hero2d/hero_2d_" + id);
        }
    }

    public sealed class KingRecruitWorldControl : MonoBehaviour
    {
        sealed class World { public string Pool; public string Label; public int Space; }
        TabKingsRecruit tab;
        readonly List<World> worlds = new List<World>();
        readonly List<GameObject> buttons = new List<GameObject>();
        string selected = "", version = "", ownerSession = "";
        long owner;
        bool busy, initialized, ready, drawing;
        WWW pending;
        float nextRefresh;
        int selectedSpace;
        bool visible;
        static readonly FieldInfo CurrentShopTab = typeof(ShopPage).GetField("curTab", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        static readonly FieldInfo CurrentShopType = typeof(ShopPage).GetField("curType", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        bool ContentVisible()
        {
            if (tab == null || tab.backGround == null || !tab.backGround.enabled || !tab.backGround.gameObject.activeInHierarchy) return false;
            ShopPage shop = tab.GetComponentInParent<ShopPage>();
            return shop != null && CurrentShopTab != null && CurrentShopType != null &&
                CurrentShopTab.GetValue(shop) as GameObject == tab.gameObject && Convert.ToInt32(CurrentShopType.GetValue(shop)) == 1;
        }
        public bool Ready { get { return ready && !busy && !drawing && CurrentOwner() && ContentVisible(); } }
        bool CurrentOwner()
        {
            return StaticData.playerData != null && StaticData.playerData.playerId == owner &&
                Hex(StaticData.sessionKey) == ownerSession;
        }
        static string Hex(byte[] bytes)
        {
            if (bytes == null) return "";
            StringBuilder value = new StringBuilder(bytes.Length * 2);
            foreach (byte b in bytes) value.Append(b.ToString("x2"));
            return value.ToString();
        }
        public void Initialize(TabKingsRecruit value)
        {
            tab = value;
            if (initialized && CurrentOwner()) return;
            StopAllCoroutines();
            if (pending != null) { pending.Dispose(); pending = null; }
            busy = false; drawing = false; initialized = true; ready = false;
            if (StaticData.playerData == null || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return;
            owner = StaticData.playerData.playerId; ownerSession = Hex(StaticData.sessionKey);
            visible = ContentVisible();
            if (!busy && visible) StartCoroutine(Request(""));
        }
        public void BeginDraw() { drawing = true; ready = false; }
        public void EndDraw() { drawing = false; ready = false; nextRefresh = 0; }
        public void Received()
        {
            // Only a reply for the selected world releases original draw actions.
            PropertyInfo property = typeof(TabKingsRecruit).GetProperty("recruitInfo", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            ProtoBuf.Message.WarriorRecruit info = property == null ? null : property.GetValue(tab, null) as ProtoBuf.Message.WarriorRecruit;
            ready = CurrentOwner() && ContentVisible() && !busy && selected != "" && info != null && info.on == 1 &&
                (selectedSpace == 0 || info.space == selectedSpace);
        }
        void Suspend()
        {
            ready = false; busy = false; visible = false;
            StopAllCoroutines();
            if (pending != null) { pending.Dispose(); pending = null; }
            foreach (GameObject button in buttons) if (button != null) button.SetActive(false);
        }
        void OnDisable() { Suspend(); }
        void OnDestroy() { Suspend(); ClearButtons(); }
        void OnEnable() { visible = false; nextRefresh = 0; }
        void LateUpdate()
        {
            if (tab == null || !gameObject.activeInHierarchy) return;
            if (!ContentVisible()) { Suspend(); return; }
            if (!visible) { visible = true; ready = false; nextRefresh = 0; }
            if (!CurrentOwner()) { ready = false; initialized = false; ClearButtons(); Initialize(tab); return; }
            if (!busy && !drawing && Time.realtimeSinceStartup >= nextRefresh) StartCoroutine(Request(""));
            PositionButtons();
        }
        void ClearButtons()
        {
            foreach (GameObject button in buttons) if (button != null) UnityEngine.Object.Destroy(button);
            buttons.Clear();
        }
        void CreateButtons(string failure)
        {
            ClearButtons();
            UILabel theme = tab.labCostOne;
            if (theme == null) return;
            int count = failure == null ? worlds.Count : 1;
            for (int i = 0; i < count; i++)
            {
                World world = failure == null ? worlds[i] : null;
                GameObject button = new GameObject("KingWorld" + i);
                button.layer = tab.gameObject.layer;
                button.transform.parent = tab.transform;
                button.transform.localScale = Vector3.one;
                button.SetActive(false);
                UILabel text = button.AddComponent<UILabel>();
                text.bitmapFont = theme.bitmapFont; text.trueTypeFont = theme.trueTypeFont;
                text.fontSize = 24; text.width = failure == null ? 122 : 260; text.height = 48;
                text.alignment = NGUIText.Alignment.Center; text.pivot = UIWidget.Pivot.Center;
                text.color = world != null && world.Pool == selected ? Color.yellow : Color.white;
                text.effectStyle = UILabel.Effect.Outline; text.effectColor = Color.black;
                text.depth = theme.depth + 150;
                text.text = failure ?? ((world.Pool == selected ? "● " : "") + world.Label);
                BoxCollider collider = button.AddComponent<BoxCollider>(); collider.size = new Vector3(text.width, 54f, 1f);
                World captured = world;
                UIEventListener.Get(button).onClick = delegate(GameObject ignored) {
                    if (busy || drawing || !CurrentOwner() || !ContentVisible()) return;
                    ready = false;
                    StartCoroutine(Request(captured == null ? "" : captured.Pool));
                };
                buttons.Add(button);
            }
            PositionButtons();
        }
        void PositionButtons()
        {
            // Anchor to the original king content, which occupies only part of the shop.
            // Screen coordinates can land behind the left menu on wide devices.
            if (!ContentVisible() || buttons.Count == 0) return;
            Vector3[] corners = tab.backGround.worldCorners;
            Vector3 left = tab.transform.InverseTransformPoint(corners[1]);
            Vector3 right = tab.transform.InverseTransformPoint(corners[2]);
            Vector3 bottom = tab.transform.InverseTransformPoint(corners[0]);
            Vector3 axis = right - left;
            float width = axis.magnitude;
            if (width <= 0f) return;
            axis /= width;
            Vector3 center = left + (right - left) * .49f + (bottom - left) * .055f;
            float cell = Mathf.Min(buttons.Count == 1 ? 260f : 112f, width * .40f / buttons.Count);
            for (int i = 0; i < buttons.Count; i++)
            {
                GameObject button = buttons[i]; if (button == null) continue;
                UILabel text = button.GetComponent<UILabel>();
                text.width = Mathf.Max(1, (int)cell - 4);
                button.GetComponent<BoxCollider>().size = new Vector3(text.width, 54f, 1f);
                button.transform.localPosition = center + axis * ((i - (buttons.Count - 1) * .5f) * cell);
                button.SetActive(true);
            }
        }
        IEnumerator Request(string pool)
        {
            if (busy || drawing || !CurrentOwner() || !ContentVisible()) yield break;
            busy = true; ready = false; nextRefresh = Time.realtimeSinceStartup + 20f;
            string origin = null;
            try { origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); } catch (Exception) { }
            if (origin == null) { busy = false; CreateButtons("世界加载失败，点击重试"); yield break; }
            string body = "{\"playerId\":" + owner + ",\"sessionHex\":\"" + ownerSession + "\"";
            if (pool != "") body += ",\"poolId\":" + JsonMapper.ToJson(pool) + ",\"version\":" + JsonMapper.ToJson(version);
            body += "}";
            Dictionary<string,string> headers = new Dictionary<string,string>(); headers.Add("Content-Type", "application/json");
            WWW request = new WWW(origin + (pool == "" ? "/king-recruit/worlds" : "/king-recruit/select"), Encoding.UTF8.GetBytes(body), headers);
            pending = request;
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool success = false;
            try
            {
                if (request.isDone && String.IsNullOrEmpty(request.error) && CurrentOwner() && ContentVisible())
                {
                    JsonData response = JsonMapper.ToObject(request.text);
                    if ((int)response["code"] == 0)
                    {
                        JsonData data = response["data"];
                        selected = (string)data["selectedPoolId"]; version = (string)data["version"];
                        worlds.Clear(); selectedSpace = 0;
                        foreach (JsonData row in data["worlds"])
                        {
                            World world = new World(); world.Pool = (string)row["poolId"]; world.Label = (string)row["label"]; world.Space = (int)row["world"];
                            worlds.Add(world); if (world.Pool == selected) selectedSpace = world.Space;
                        }
                        success = true;
                    }
                }
            }
            catch (Exception) { }
            request.Dispose(); pending = null; busy = false;
            if (!CurrentOwner() || !ContentVisible()) yield break;
            if (!success) { CreateButtons("世界加载失败，点击重试"); yield break; }
            CreateButtons(worlds.Count == 0 ? "当前没有开放世界" : null);
            if (selected != "") tab.UpdateAll(true);
        }
    }
}
