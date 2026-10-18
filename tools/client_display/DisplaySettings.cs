using System;
using System.Reflection;
using UI;
using UnityEngine;

// A display-only bridge compiled against the original Unity 5.3 / .NET profile.
// The saved value is device-wide and never touches account/server state.
namespace HYJS.Display
{
    public static class DisplaySettings
    {
        const string FPSKey = "soulflower_target_fps";
        const BindingFlags InstanceFields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        const BindingFlags StaticFields = BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic;
        public static int TargetFPS { get { return PlayerPrefs.GetInt(FPSKey, 60) == 120 ? 120 : 60; } }

        static void SetIntField(Type type, object owner, string name, int value, BindingFlags flags)
        {
            FieldInfo field = type.GetField(name, flags);
            if (field != null && field.FieldType == typeof(int)) field.SetValue(owner, value);
        }

        public static void Apply()
        {
            int cap = TargetFPS;
            QualitySettings.vSyncCount = 0;
            Application.targetFrameRate = cap;
            SetIntField(typeof(RenderManager), null, "lowMaxFPS", cap, StaticFields);
            SetIntField(typeof(RenderManager), null, "middleMaxFPS", cap, StaticFields);
            SetIntField(typeof(RenderManager), null, "highMaxFPS", cap, StaticFields);
        }

        static void ApplyFixer(FPSFixer fixer)
        {
            if (fixer == null) return;
            foreach (string name in new string[] { "InitFrameRate", "MinFrameRate", "MaxFrameRate", "targetFps" })
                SetIntField(typeof(FPSFixer), fixer, name, TargetFPS, InstanceFields);
        }

        public static void SetTargetFPS(int fps)
        {
            if (fps != 60 && fps != 120) throw new ArgumentOutOfRangeException("fps");
            PlayerPrefs.SetInt(FPSKey, fps);
            PlayerPrefs.Save();
            Apply();
            foreach (UnityEngine.Object current in UnityEngine.Object.FindObjectsOfType(typeof(FPSFixer)))
                ApplyFixer(current as FPSFixer);
        }

        public static void StartFPS(FPSFixer fixer)
        {
            ApplyFixer(fixer);
            Apply();
            QualitySettings.masterTextureLimit = 0;
            QualitySettings.maximumLODLevel = 0;
            if (fixer != null && fixer.GetComponent<DisplayFocusPolicy>() == null)
                fixer.gameObject.AddComponent<DisplayFocusPolicy>();
        }

        public static void Attach(QualityBar quality)
        {
            if (quality == null || quality.GetComponent<DisplayFrameOptions>() != null) return;
            DisplayFrameOptions controls = null;
            try { controls = quality.gameObject.AddComponent<DisplayFrameOptions>(); controls.Initialize(quality); }
            catch (Exception error)
            {
                if (controls != null) UnityEngine.Object.Destroy(controls);
                Debug.LogWarning("SoulFlower display options: " + error.GetType().Name);
            }
        }
    }

    public sealed class DisplayFocusPolicy : MonoBehaviour
    {
        void OnApplicationFocus(bool focus) { if (focus) DisplaySettings.Apply(); }
        void OnApplicationPause(bool paused) { if (!paused) DisplaySettings.Apply(); }
    }

    public sealed class DisplayFrameOptions : MonoBehaviour
    {
        GameObject row;
        Transform qualityRow, musicRow;
        Vector3 oldQualityPosition, oldMusicPosition;
        UISprite sixty, oneTwenty;
        UILabel sixtyText, oneTwentyText;
        Color textColor;

        public void Initialize(QualityBar quality)
        {
            // The qualified PlayerInfo prefab has 40-high quality hit targets and
            // 44-high music targets. Give both rows 8 units more breathing room,
            // then place the 24-high FPS row between them without overlapping hits.
            // All original widgets, quality values and event handlers stay intact.
            Transform parent = quality.transform.parent;
            if (parent == null || parent.Find("SoulFlowerFrameRate") != null) return;
            UISprite theme = quality.GetComponent<UISprite>();
            UILabel[] labels = parent.GetComponentsInChildren<UILabel>(true);
            Transform center = parent.parent == null ? null : parent.parent.parent;
            Transform music = center == null ? null : center.Find("MainDown");
            if (theme == null || theme.atlas == null || labels.Length == 0 || parent.name != "GraphicQuality" || music == null)
                throw new InvalidOperationException("Original quality layout unavailable");
            qualityRow = parent; musicRow = music;
            oldQualityPosition = qualityRow.localPosition; oldMusicPosition = musicRow.localPosition;
            qualityRow.localPosition = oldQualityPosition + new Vector3(0f, 8f, 0f);
            musicRow.localPosition = oldMusicPosition - new Vector3(0f, 8f, 0f);
            UILabel font = labels[0];
            int depth = theme.depth;
            foreach (UIWidget widget in parent.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, widget.depth);
            textColor = font.color;
            row = new GameObject("SoulFlowerFrameRate");
            row.layer = quality.gameObject.layer;
            row.transform.parent = parent;
            row.transform.localPosition = new Vector3(0f, -40f, 0f);
            row.transform.localScale = Vector3.one;
            UILabel title = Label("Title", font, row.transform, new Vector3(-147f, 0f, 0f), depth + 4);
            title.text = "帧率上限";
            title.width = 144; title.height = 30; title.fontSize = Math.Min(font.fontSize, 22);
            title.pivot = font.pivot; title.alignment = font.alignment;
            sixty = Option("FPS60", theme, font, new Vector3(20f, 0f, 0f), "60帧", 60, depth, out sixtyText);
            oneTwenty = Option("FPS120", theme, font, new Vector3(150f, 0f, 0f), "120帧", 120, depth, out oneTwentyText);
            Refresh();
        }

        UILabel Label(string name, UILabel font, Transform parent, Vector3 position, int depth)
        {
            GameObject go = new GameObject(name); go.layer = row.layer;
            go.transform.parent = parent; go.transform.localPosition = position; go.transform.localScale = Vector3.one;
            UILabel label = go.AddComponent<UILabel>();
            label.bitmapFont = font.bitmapFont; label.trueTypeFont = font.trueTypeFont;
            label.fontStyle = font.fontStyle; label.fontSize = Math.Min(font.fontSize, 22);
            label.color = font.color; label.depth = depth;
            return label;
        }

        UISprite Option(string name, UISprite theme, UILabel font, Vector3 position, string text, int fps, int depth, out UILabel caption)
        {
            GameObject go = new GameObject(name); go.layer = row.layer;
            go.transform.parent = row.transform; go.transform.localPosition = position; go.transform.localScale = Vector3.one;
            UISprite sprite = go.AddComponent<UISprite>();
            sprite.atlas = theme.atlas; sprite.spriteName = theme.spriteName;
            sprite.type = UIBasicSprite.Type.Sliced; sprite.pivot = UIWidget.Pivot.Center;
            sprite.width = 108; sprite.height = 24; sprite.depth = depth + 4;
            caption = Label("Caption", font, go.transform, Vector3.zero, depth + 5);
            caption.text = text; caption.width = 104; caption.height = 24;
            caption.fontSize = Math.Min(font.fontSize, 20);
            caption.pivot = UIWidget.Pivot.Center; caption.alignment = NGUIText.Alignment.Center;
            BoxCollider hit = go.AddComponent<BoxCollider>(); hit.center = Vector3.zero; hit.size = new Vector3(108f, 24f, 1f);
            UIEventListener.Get(go).onClick = delegate(GameObject ignored)
            {
                DisplaySettings.SetTargetFPS(fps); Refresh();
                BoxManager.CreatePopupTextBox("已切换为" + fps + "帧上限", 0f);
            };
            return sprite;
        }

        void OnEnable() { Refresh(); }
        void Refresh()
        {
            bool fast = DisplaySettings.TargetFPS == 120;
            if (sixty != null) sixty.color = fast ? Color.white : new Color(0.45f, 0.85f, 1f, 1f);
            if (oneTwenty != null) oneTwenty.color = fast ? new Color(0.45f, 0.85f, 1f, 1f) : Color.white;
            if (sixtyText != null) sixtyText.color = fast ? textColor : new Color(0.05f, 0.2f, 0.35f, 1f);
            if (oneTwentyText != null) oneTwentyText.color = fast ? new Color(0.05f, 0.2f, 0.35f, 1f) : textColor;
        }
        void OnDestroy()
        {
            if (row != null) Destroy(row);
            if (qualityRow != null) qualityRow.localPosition = oldQualityPosition;
            if (musicRow != null) musicRow.localPosition = oldMusicPosition;
        }
    }
}
