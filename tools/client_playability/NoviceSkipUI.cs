using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

// The original guide keeps every level unlock and normal quest/reward rule.
// Only an explicit, server-confirmed click stops the initial teaching overlay.
namespace HYJS.Playability
{
    public static class NoviceSkipUI
    {
        public static void Ensure(GuideManager guide)
        {
            try
            {
                if (guide == null || Application.platform != RuntimePlatform.Android) return;
                NoviceSkipControl control = guide.GetComponent<NoviceSkipControl>();
                if (control == null) control = guide.gameObject.AddComponent<NoviceSkipControl>();
                control.Initialize(guide);
            }
            catch (Exception) { Debug.LogWarning("Initial tutorial skip UI unavailable"); }
        }
    }

    public sealed class NoviceSkipControl : MonoBehaviour
    {
        const BindingFlags Fields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        GuideManager guide;
        GameObject button;
        UILabel caption;
        bool busy;

        internal void Initialize(GuideManager value) { guide = value; }
        object Get(string name) { return typeof(GuideManager).GetField(name, Fields).GetValue(guide); }
        void Set(string name, object value) { typeof(GuideManager).GetField(name, Fields).SetValue(guide, value); }

        bool InitialRunning()
        {
            if (guide == null || StaticData.playerData == null || StaticData.playerData.playerId < 1 ||
                StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return false;
            string current = Get("curGuideId") as string;
            if (!(bool)Get("running") || String.IsNullOrEmpty(current)) return false;
            Dict.DataModel.TableGuide row = Dict.DictDataManager.Instance.tableGuideDao.GetById(current);
            return row != null && row.order >= 0 && row.order <= 12;
        }

        void CreateButton()
        {
            Component mask = Get("guideMask") as Component;
            if (mask == null) return;
            UILabel[] labels = mask.GetComponentsInChildren<UILabel>(true);
            UILabel theme = null;
            foreach (UILabel label in labels)
                if (label.bitmapFont != null || label.trueTypeFont != null) { theme = label; break; }
            if (theme == null) return;
            button = new GameObject("InitialTutorialSkip");
            button.layer = mask.gameObject.layer;
            button.transform.parent = mask.transform;
            button.transform.localScale = Vector3.one;
            caption = button.AddComponent<UILabel>();
            caption.bitmapFont = theme.bitmapFont;
            caption.trueTypeFont = theme.trueTypeFont;
            caption.fontStyle = theme.fontStyle;
            caption.fontSize = 28; caption.width = 164; caption.height = 54;
            caption.pivot = UIWidget.Pivot.Center; caption.alignment = NGUIText.Alignment.Center;
            caption.color = Color.white; caption.effectStyle = UILabel.Effect.Shadow;
            caption.effectColor = Color.black; caption.effectDistance = new Vector2(2f, -2f);
            int depth = 0;
            foreach (UIWidget widget in mask.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, widget.depth);
            caption.depth = depth + 200; caption.text = "跳过 >>";
            BoxCollider collider = button.AddComponent<BoxCollider>();
            collider.center = Vector3.zero; collider.size = new Vector3(180f, 68f, 1f);
            UIEventListener.Get(button).onClick = delegate(GameObject ignored) { Click(); };
        }

        void LateUpdate()
        {
            try
            {
                bool visible = InitialRunning();
                if (button == null && visible) CreateButton();
                if (button == null) return;
                button.SetActive(visible);
                if (!visible) return;
                Camera camera = NGUITools.FindCameraForLayer(button.layer);
                if (camera == null) return;
                // Use the actual NGUI camera projection so wide-screen layout
                // and system cutout-safe Android content dimensions stay aligned.
                float scale = Screen.height / 720f;
                Vector3 screen = camera.WorldToScreenPoint(button.transform.parent.position);
                // Keep the original top-right close controls unobstructed.
                screen.x = Screen.width - 112f * scale; screen.y = Screen.height - 116f * scale;
                button.transform.position = camera.ScreenToWorldPoint(screen);
                Component mask = Get("guideMask") as Component;
                FieldInfo targetField = mask == null ? null : mask.GetType().GetField("target", Fields);
                GameObject target = targetField == null ? null : targetField.GetValue(mask) as GameObject;
                // The guide owns this real target collider. Move the skip action
                // away from it instead of stealing the step's original click.
                if (OverlapsTarget(camera, target, scale))
                {
                    screen.x = Screen.width - 340f * scale;
                    button.transform.position = camera.ScreenToWorldPoint(screen);
                    if (OverlapsTarget(camera, target, scale))
                    {
                        screen.x = Screen.width * .5f;
                        button.transform.position = camera.ScreenToWorldPoint(screen);
                        if (OverlapsTarget(camera, target, scale)) button.SetActive(false);
                    }
                }
            }
            catch (Exception) { if (button != null) button.SetActive(false); }
        }

        static Rect Project(Camera camera, Bounds bounds)
        {
            Vector3 min = bounds.min, max = bounds.max;
            Vector3 a = camera.WorldToScreenPoint(min), b = camera.WorldToScreenPoint(max);
            return Rect.MinMaxRect(Math.Min(a.x, b.x), Math.Min(a.y, b.y), Math.Max(a.x, b.x), Math.Max(a.y, b.y));
        }

        bool OverlapsTarget(Camera camera, GameObject target, float scale)
        {
            if (target == null) return false;
            Rect skip = Project(camera, button.GetComponent<BoxCollider>().bounds);
            foreach (Collider hit in target.GetComponentsInChildren<Collider>(true))
            {
                if (!hit.enabled || !hit.gameObject.activeInHierarchy) continue;
                Rect area = Project(camera, hit.bounds);
                area.xMin -= 12f * scale; area.yMin -= 12f * scale;
                area.xMax += 12f * scale; area.yMax += 12f * scale;
                if (skip.Overlaps(area)) return true;
            }
            return false;
        }

        void Click()
        {
            if (busy || !InitialRunning()) return;
            StartCoroutine(Skip());
        }

        static string Hex(byte[] bytes)
        {
            var text = new StringBuilder(32);
            foreach (byte part in bytes) text.Append(part.ToString("x2"));
            return text.ToString();
        }

        IEnumerator Skip()
        {
            UIPlayerData player = StaticData.playerData;
            byte[] session = (byte[])StaticData.sessionKey.Clone();
            busy = true; if (caption != null) caption.text = "确认中…";
            string origin = null;
            try { origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); }
            catch (Exception) { }
            if (origin == null)
            {
                busy = false; if (caption != null) caption.text = "跳过 >>";
                BoxManager.CreatePopupTextBox("教学跳过尚未保存，请重试", 0f); yield break;
            }
            string body = "{\"playerId\":" + player.playerId + ",\"sessionHex\":\"" + Hex(session) + "\"}";
            var headers = new Dictionary<string, string>(); headers.Add("Content-Type", "application/json");
            WWW request = new WWW(origin + "/game-guide/skip", Encoding.UTF8.GetBytes(body), headers);
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool confirmed = false;
            try
            {
                if (request.isDone && String.IsNullOrEmpty(request.error))
                {
                    JsonData result = JsonMapper.ToObject(request.text);
                    confirmed = (int)result["code"] == 0 && (bool)result["data"]["skipped"] &&
                        (int)result["data"]["noviceGuideSchedule"] >= 12;
                }
            }
            catch (Exception) { }
            request.Dispose();
            busy = false; if (caption != null) caption.text = "跳过 >>";
            if (!System.Object.ReferenceEquals(player, StaticData.playerData) || Hex(session) != Hex(StaticData.sessionKey ?? new byte[0])) yield break;
            if (!confirmed)
            {
                BoxManager.CreatePopupTextBox("教学跳过尚未确认，请重试", 0f); yield break;
            }
            StopInitialGuide(player);
        }

        void StopInitialGuide(UIPlayerData player)
        {
            // Only stop initial teaching. A normal action may have completed it
            // while the network request was pending; leave any later guide alone.
            if (!InitialRunning()) return;
            guide.StopAllCoroutines();
            List<EventDelegate> delegates = Get("lastEventDelegates") as List<EventDelegate>;
            string originalMethod = Get("lastDelegateMethodName") as string;
            if (delegates != null)
            {
                if (!String.IsNullOrEmpty(originalMethod) && delegates.Count > 0)
                    delegates[0].methodName = originalMethod;
                for (int i = delegates.Count - 1; i >= 0; --i)
                    if (delegates[i].target == guide) delegates.RemoveAt(i);
            }
            Set("lastEventDelegates", null); Set("lastDelegateMethodName", null);
            guide.Clear();
            // Retain a real original guide row for subsequent level-based guides.
            player.currentGuideId = "guide_equip_strengthen";
            Set("curGuideId", "guide_equip_strengthen"); Set("isCopyBattleGuide", false);
            BoxManager.CreatePopupTextBox("已跳过新手教学，等级功能指引仍会正常出现", 0f);
        }
    }
}
