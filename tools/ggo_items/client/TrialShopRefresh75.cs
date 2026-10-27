using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.GGO
{
    // A read-only companion to the original four-tab trial shop. The native
    // purchase handlers, tabs, catalogue and currency continue to own gameplay.
    public sealed class TrialShopRefresh75 : MonoBehaviour
    {
        static readonly BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        static readonly FieldInfo CurrentType = typeof(ShopPage).GetField("curType", Flags);
        static readonly FieldInfo CurrentTab = typeof(ShopPage).GetField("curTab", Flags);
        static readonly FieldInfo CurrentGroup = typeof(TabPointShop).GetField("curTabIndex", Flags);
        ShopPage shop;
        TabPointShop tab;
        UILabel label;
        long owner, serverTime;
        string session = "", origin = "";
        int generation;
        float checkAt, nextRead, receivedAt;
        bool busy, ready, failed;
        WWW pending;
        readonly long[] resets = new long[5];
        readonly string[] periods = new string[5];
        readonly bool[] available = new bool[5];

        public static void Ensure(ShopPage page)
        {
            if (page == null) return;
            TrialShopRefresh75 value = page.GetComponent<TrialShopRefresh75>();
            if (value == null) value = page.gameObject.AddComponent<TrialShopRefresh75>();
            value.shop = page;
        }
        static bool Current(out long player, out string key)
        {
            player = StaticData.playerData == null ? 0 : StaticData.playerData.playerId;
            key = "";
            if (player < 1 || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return false;
            StringBuilder bytes = new StringBuilder(32);
            foreach (byte b in StaticData.sessionKey) bytes.Append(b.ToString("x2"));
            key = bytes.ToString(); return true;
        }
        bool Owns(int expected)
        {
            long id; string key;
            return expected == generation && tab != null && tab.gameObject.activeInHierarchy && shop != null && shop.isOpen &&
                Current(out id, out key) && id == owner && key == session;
        }
        void Clear()
        {
            generation++; StopAllCoroutines();
            if (pending != null) { pending.Dispose(); pending = null; }
            if (label != null) UnityEngine.Object.Destroy(label.gameObject);
            label = null; tab = null; owner = 0; session = origin = "";
            busy = ready = failed = false; nextRead = 0f;
        }
        void OnDisable() { Clear(); }
        void OnDestroy() { Clear(); }
        void Update()
        {
            if (Time.realtimeSinceStartup < checkAt) return;
            checkAt = Time.realtimeSinceStartup + .2f;
            GameObject current = null;
            if (shop != null && shop.isOpen && shop.gameObject.activeInHierarchy && CurrentType != null && Convert.ToInt32(CurrentType.GetValue(shop)) == 5)
                current = CurrentTab == null ? null : CurrentTab.GetValue(shop) as GameObject;
            TabPointShop page = current == null || !current.activeInHierarchy ? null : current.GetComponent<TabPointShop>();
            long player; string key;
            if (page == null || !Current(out player, out key)) { if (tab != null) Clear(); return; }
            if (page != tab || owner != player || session != key)
            {
                Clear(); tab = page; owner = player; session = key;
                try { origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); }
                catch (Exception) { origin = ""; }
                CreateLabel();
            }
            if (label == null) return;
            // The trial catalogue is a one-time reward shop. Do not show a
            // daily or weekly reset timer for purchases that never reset.
            label.text = "每件限购一次";
        }
        void CreateLabel()
        {
            UILabel theme = null;
            if (tab.shopItemPrefab != null)
                foreach (UILabel value in tab.shopItemPrefab.GetComponentsInChildren<UILabel>(true))
                    if (value.gameObject.name == "Name") { theme = value; break; }
            if (theme == null) return;
            GameObject go = (GameObject)UnityEngine.Object.Instantiate(theme.gameObject);
            go.name = "TrialShopRefreshTime"; go.layer = tab.gameObject.layer;
            go.transform.parent = tab.transform; go.transform.localScale = Vector3.one; go.transform.localRotation = Quaternion.identity;
            Transform tabs = tab.transform.Find("Tabs");
            Vector3 anchor = tabs == null ? new Vector3(0f, 238f, 0f) : tabs.localPosition;
            go.transform.localPosition = anchor + new Vector3(-378f, -37f, 0f);
            label = go.GetComponent<UILabel>();
            foreach (MonoBehaviour component in go.GetComponents<MonoBehaviour>())
                if (component != null && component.GetType().Name == "TextKit") UnityEngine.Object.DestroyImmediate(component);
            foreach (Collider collider in go.GetComponentsInChildren<Collider>(true)) UnityEngine.Object.DestroyImmediate(collider);
            label.pivot = UIWidget.Pivot.Left; label.alignment = NGUIText.Alignment.Left;
            label.fontSize = 18; label.width = 440; label.height = 24; label.maxLineCount = 1;
            label.overflowMethod = UILabel.Overflow.ShrinkContent; label.depth = theme.depth + 2;
            label.text = "每件限购一次"; go.SetActive(true);
        }
        void Paint()
        {
            if (label == null) return;
            if (failed) { label.text = "刷新时间读取失败，点击重试"; return; }
            if (!ready) { label.text = "正在读取刷新时间…"; return; }
            int group = CurrentGroup == null ? 0 : Convert.ToInt32(CurrentGroup.GetValue(tab)) + 1;
            if (group < 1 || group > 4) { label.text = "刷新时间暂不可用"; return; }
            if (!available[group] || resets[group] <= 0) { label.text = "本期未开放"; return; }
            long now = serverTime + (long)((Time.realtimeSinceStartup - receivedAt) * 1000f);
            long seconds = Math.Max(0L, (resets[group] - now + 999L) / 1000L);
            if (seconds == 0) { label.text = "正在更新本期商品…"; nextRead = Math.Min(nextRead, Time.realtimeSinceStartup + 2f); return; }
            string time = (seconds / 3600L).ToString("D2") + ":" + ((seconds / 60L) % 60L).ToString("D2") + ":" + (seconds % 60L).ToString("D2");
            label.text = (periods[group] == "week" ? "每周刷新 · " : "每日刷新 · ") + time;
        }
        IEnumerator Read(int expected)
        {
            busy = true; failed = false; nextRead = Time.realtimeSinceStartup + 30f;
            string body = "{\"playerId\":" + owner + ",\"sessionHex\":\"" + session + "\",\"shopType\":5}";
            Dictionary<string,string> headers = new Dictionary<string,string>(); headers.Add("Content-Type", "application/json");
            WWW request = new WWW(origin + "/shop-refresh/status", Encoding.UTF8.GetBytes(body), headers); pending = request;
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            if (Owns(expected))
            {
                try
                {
                    if (!request.isDone || !String.IsNullOrEmpty(request.error) || request.text.Length > 32768) throw new Exception("refresh status unavailable");
                    JsonData response = JsonMapper.ToObject(request.text);
                    if ((int)response["code"] != 0) throw new Exception("refresh status declined");
                    JsonData data = response["data"];
                    long clock = Int64.Parse(data["serverTime"].ToString()); if (clock < 1) throw new Exception("invalid server clock");
                    bool[] seen = new bool[5]; long[] deadlines = new long[5]; string[] kinds = new string[5]; bool[] open = new bool[5];
                    foreach (JsonData row in data["menus"])
                    {
                        int group = (int)row["group"]; if (group < 1 || group > 4 || seen[group]) throw new Exception("invalid refresh group");
                        seen[group] = true; kinds[group] = (string)row["period"]; if (kinds[group] != "day" && kinds[group] != "week") throw new Exception("invalid refresh period");
                        open[group] = (bool)row["available"];
                        if (row["resetAt"] != null) deadlines[group] = Int64.Parse(row["resetAt"].ToString());
                        if (open[group] && deadlines[group] < 1) throw new Exception("invalid refresh deadline");
                    }
                    for (int group=1;group<=4;group++) if (!seen[group]) throw new Exception("missing refresh group");
                    Array.Copy(deadlines,resets,5); Array.Copy(kinds,periods,5); Array.Copy(open,available,5);
                    serverTime = clock; receivedAt = Time.realtimeSinceStartup; ready = true; failed = false;
                }
                catch (Exception) { failed = true; }
                busy = false; Paint();
            }
            request.Dispose(); if (pending == request) pending = null;
        }
    }
}
