using System;
using System.Collections;
using System.Collections.Generic;
using System.Text;
using UnityEngine;

namespace HYJS.Notices
{
    public sealed class NoticeEntry
    {
        public string Title;
        public string Content;
        public NoticeEntry(string title, string content) { Title = title; Content = content; }
    }

    public static class OriginalNotices
    {
        public static void ShowAnnouncement(string url, int top, int left, int bottom, int right, int full)
        {
            UI.AnnouncePage current = UI.PageManager.Instance.GetDictPage("AnnouncePage") as UI.AnnouncePage;
            if (current != null) { Open(current, current.bg, url); return; }
            UI.AnnouncePage[] pages = Resources.FindObjectsOfTypeAll<UI.AnnouncePage>();
            for (int i = 0; i < pages.Length; i++)
                if (pages[i] != null && pages[i].gameObject.activeInHierarchy)
                { Open(pages[i], pages[i].bg, url); return; }
        }

        public static void ShowSystem(string url, int top, int left, int bottom, int right, int full, SystemAnnouncePage owner)
        {
            if (owner != null) Open(owner, owner.bg, url);
        }

        static void Open(MonoBehaviour owner, UISprite background, string url)
        {
            NoticeContent content = owner.GetComponent<NoticeContent>();
            if (content == null) content = owner.gameObject.AddComponent<NoticeContent>();
            content.Open(background, url);
        }

        public static void HideAnnouncement(UI.AnnouncePage owner) { Hide(owner); }
        public static void HideSystem(SystemAnnouncePage owner) { Hide(owner); }
        static void Hide(MonoBehaviour owner)
        {
            NoticeContent content = owner.GetComponent<NoticeContent>();
            if (content != null) content.Close();
        }

        // The existing public endpoint emits escaped, plain title/content inside
        // article/h2/p tags. We consume those values as text; no HTML is executed.
        public static List<NoticeEntry> Parse(string html)
        {
            List<NoticeEntry> entries = new List<NoticeEntry>();
            int offset = 0;
            while (offset < html.Length)
            {
                int start = html.IndexOf("<article>", offset, StringComparison.OrdinalIgnoreCase);
                if (start < 0) break;
                int end = html.IndexOf("</article>", start, StringComparison.OrdinalIgnoreCase);
                if (end < 0) throw new FormatException("Incomplete announcement article");
                string block = html.Substring(start, end - start);
                string title = Element(block, "h2");
                string body = Element(block, "p");
                entries.Add(new NoticeEntry(Decode(title), Decode(body)));
                offset = end + 10;
            }
            if (entries.Count == 0 && html.IndexOf("暂无公告", StringComparison.Ordinal) < 0)
                throw new FormatException("Unknown announcement response");
            return entries;
        }

        static string Element(string block, string tag)
        {
            int start = block.IndexOf("<" + tag + ">", StringComparison.OrdinalIgnoreCase);
            int end = start < 0 ? -1 : block.IndexOf("</" + tag + ">", start, StringComparison.OrdinalIgnoreCase);
            if (start < 0 || end < 0) throw new FormatException("Missing announcement text");
            return block.Substring(start + tag.Length + 2, end - start - tag.Length - 2);
        }

        public static string Decode(string text)
        {
            StringBuilder result = new StringBuilder(text.Length);
            for (int i = 0; i < text.Length; i++)
            {
                if (text[i] != '&') { result.Append(text[i]); continue; }
                int end = text.IndexOf(';', i + 1);
                if (end < 0 || end - i > 12) { result.Append(text[i]); continue; }
                string entity = text.Substring(i + 1, end - i - 1);
                string decoded = null;
                if (entity == "amp") decoded = "&";
                else if (entity == "lt") decoded = "<";
                else if (entity == "gt") decoded = ">";
                else if (entity == "quot") decoded = "\"";
                else if (entity == "apos" || entity == "#39") decoded = "'";
                else if (entity == "nbsp") decoded = " ";
                else if (entity.StartsWith("#"))
                {
                    try
                    {
                        bool hex = entity.StartsWith("#x", StringComparison.OrdinalIgnoreCase);
                        int value = Convert.ToInt32(entity.Substring(hex ? 2 : 1), hex ? 16 : 10);
                        if (value >= 0 && value <= 0x10ffff && (value < 0xd800 || value > 0xdfff))
                            decoded = Char.ConvertFromUtf32(value);
                    }
                    catch (Exception) { }
                }
                if (decoded == null) result.Append(text[i]);
                else { result.Append(decoded); i = end; }
            }
            return result.ToString();
        }
    }

    // The original game uses NGUI bitmap fonts. Copy the exact original label
    // font references; never substitute an Android or Unity built-in font.
    public sealed class NoticeContent : MonoBehaviour
    {
        int diagnosticLines;
        void Trace(string text)
        {
            if (diagnosticLines >= 12) return;
            try
            {
                string path = Application.persistentDataPath + "/notice-layout.txt";
                if (diagnosticLines++ == 0) System.IO.File.WriteAllText(path, text + "\n");
                else System.IO.File.AppendAllText(path, text + "\n");
            }
            catch (Exception) { }
        }
        UISprite background;
        UILabel template;
        WWW request;
        List<NoticeEntry> entries = new List<NoticeEntry>();
        GameObject root;
        GameObject bodyRoot;
        UIScrollView rightScroll;
        bool visible;
        string status;
        int baseDepth;
        float rightWidth;
        float viewHeight;
        int lastWidth;
        int lastHeight;

        public void Open(UISprite originalBackground, string url)
        {
            Close();
            diagnosticLines = 0;
            Trace("open background=" + (originalBackground != null));
            background = originalBackground;
            visible = true;
            status = "公告加载中…";
            entries.Clear();
            Build();
            StartCoroutine(Load(url));
        }

        public void Close()
        {
            visible = false;
            StopAllCoroutines();
            if (request != null) { request.Dispose(); request = null; }
            if (root != null) { root.SetActive(false); Destroy(root); root = null; }
            bodyRoot = null;
        }

        void OnDisable() { Close(); }
        void OnDestroy() { Close(); }

        IEnumerator Load(string url)
        {
            bool failed = false;
            try
            {
                Uri address = new Uri(url, UriKind.Absolute);
                if ((address.Scheme != "http" && address.Scheme != "https") || address.AbsolutePath != "/notices")
                    throw new FormatException("Unsupported notice URL");
                request = new WWW(url);
            }
            catch (Exception) { failed = true; }
            if (failed) { status = "公告暂时无法加载，请稍后重新打开"; Build(); yield break; }
            float start = Time.realtimeSinceStartup;
            while (!request.isDone && Time.realtimeSinceStartup - start < 12f) yield return null;
            if (!request.isDone || !String.IsNullOrEmpty(request.error)) failed = true;
            if (!failed)
            {
                try
                {
                    string html = request.text;
                    if (html.Length > 524288) throw new FormatException("Announcement response too large");
                    entries = OriginalNotices.Parse(html);
                }
                catch (Exception) { failed = true; }
            }
            request.Dispose(); request = null;
            status = failed ? "公告暂时无法加载，请稍后重新打开" : entries.Count == 0 ? "暂无公告" : null;
            Build();
        }

        GameObject Child(string name, Transform parent, Vector3 position)
        {
            GameObject go = new GameObject(name);
            go.layer = background.gameObject.layer;
            go.transform.parent = parent;
            go.transform.localScale = Vector3.one;
            go.transform.localRotation = Quaternion.identity;
            go.transform.localPosition = position;
            return go;
        }

        UILabel FontTemplate()
        {
            UILabel[] labels = GetComponentsInChildren<UILabel>(true);
            for (int i = 0; i < labels.Length; i++)
                if (labels[i] != null && labels[i].gameObject.activeInHierarchy &&
                    labels[i].GetComponentInParent<NoticeContent>() == this &&
                    (labels[i].bitmapFont != null || labels[i].trueTypeFont != null)) return labels[i];
            labels = Resources.FindObjectsOfTypeAll<UILabel>();
            for (int i = 0; i < labels.Length; i++)
                if (labels[i] != null && labels[i].gameObject.activeInHierarchy && labels[i].bitmapFont != null) return labels[i];
            return null;
        }

        UILabel Label(string name, Transform parent, float x, float top, float width, string text, bool heading, UIScrollView scroll)
        {
            GameObject go = Child(name, parent, new Vector3(x, top, 0));
            UILabel label = go.AddComponent<UILabel>();
            // NGUI's trueTypeFont getter can expose bitmapFont.dynamicFont;
            // assigning it afterwards clears bitmapFont and loses the game's UIFont.
            if (template.bitmapFont != null) label.bitmapFont = template.bitmapFont;
            else label.trueTypeFont = template.trueTypeFont;
            label.material = template.material;
            label.fontStyle = template.fontStyle;
            label.fontSize = heading ? 24 : 22;
            label.effectStyle = template.effectStyle;
            label.effectColor = template.effectColor;
            label.effectDistance = template.effectDistance;
            label.depth = baseDepth + 4;
            // rawPivot retains the explicit top-left layout coordinates. The
            // normal pivot setter moves the transform to preserve old corners.
            label.rawPivot = UIWidget.Pivot.TopLeft;
            label.alignment = NGUIText.Alignment.Left;
            label.supportEncoding = false;
            label.maxLineCount = 0;
            label.overflowMethod = UILabel.Overflow.ResizeHeight;
            label.width = Mathf.Max(40, Mathf.RoundToInt(width));
            label.height = 24;
            label.color = heading ? new Color(.13f, .36f, .48f, 1f) : new Color(.20f, .23f, .25f, 1f);
            label.text = text;
            Vector2 measured = label.printedSize;
            label.height = Mathf.Max(label.fontSize + 4, Mathf.CeilToInt(measured.y));
            BoxCollider hit = go.AddComponent<BoxCollider>();
            hit.center = new Vector3(label.width / 2f, -label.height / 2f, 0);
            hit.size = new Vector3(label.width, label.height, 1f);
            if (scroll != null) go.AddComponent<UIDragScrollView>().scrollView = scroll;
            return label;
        }

        UIScrollView Scroll(string name, Transform parent, float left, float top, float width, float height)
        {
            GameObject go = Child(name, parent, new Vector3(left + width / 2f, top - height / 2f, 0));
            UIPanel panel = go.AddComponent<UIPanel>();
            UIPanel original = background.GetComponentInParent<UIPanel>();
            panel.depth = original == null ? 1 : original.depth + 1;
            // Page panels carry a nonzero renderer sorting order. New scroll
            // panels retain that order so the native frame cannot cover text.
            panel.sortingOrder = original == null ? 0 : original.sortingOrder;
            panel.clipping = UIDrawCall.Clipping.SoftClip;
            panel.baseClipRegion = new Vector4(0, 0, width, height);
            panel.clipSoftness = new Vector2(2f, 2f);
            UIScrollView scroll = go.AddComponent<UIScrollView>();
            scroll.movement = UIScrollView.Movement.Vertical;
            scroll.contentPivot = UIWidget.Pivot.TopLeft;
            scroll.restrictWithinPanel = true;
            scroll.disableDragIfFits = true;
            BoxCollider hit = go.AddComponent<BoxCollider>();
            hit.center = Vector3.zero; hit.size = new Vector3(width, height, 1f);
            go.AddComponent<UIDragScrollView>().scrollView = scroll;
            return scroll;
        }

        void Build()
        {
            if (!visible || background == null) return;
            if (root != null) { root.SetActive(false); Destroy(root); root = null; }
            template = FontTemplate();
            Trace("build template=" + (template != null) + " status=" + status);
            if (template == null) return;
            baseDepth = background.depth;
            lastWidth = background.width; lastHeight = background.height;
            Vector3[] corners = background.localCorners;
            float left = corners[0].x + 22f;
            float top = corners[1].y - 30f;
            float width = background.width - 44f;
            viewHeight = background.height - 56f;
            if (width < 260f || viewHeight < 100f) return;
            Trace("bounds=" + left + "," + top + "," + width + "," + viewHeight + " font=" + (template.bitmapFont == null ? template.trueTypeFont.name : template.bitmapFont.name) + " bgAlpha=" + background.finalAlpha + " templateAlpha=" + template.finalAlpha);
            // The original centre sprite is deliberately translucent. Its alpha
            // must not tint content. Keep the exact local transform as a sibling
            // so text inherits the page's normal opacity and original game font.
            root = Child("SoulFlowerOriginalNotices", background.transform.parent, background.transform.localPosition);
            root.transform.localScale = background.transform.localScale;
            root.transform.localRotation = background.transform.localRotation;
            // Keep the native panel/material path. Text clamps or its own scroll
            // bounds contain it without adding a second font-clipping shader.
            if (status != null)
            {
                UILabel empty = Label("Status", root.transform, left + 16f, top - viewHeight / 2f + 18f, width - 32f, status, false, null);
                empty.overflowMethod = UILabel.Overflow.ClampContent;
                empty.alignment = NGUIText.Alignment.Center;
                empty.height = Mathf.Min(72, Mathf.RoundToInt(viewHeight - 16f));
                empty.fontSize = 22;
                return;
            }
            // Show all published articles in one full-width native scroll view.
            // Titles and paragraphs share a left edge; no side navigation is needed.
            rightWidth = width;
            rightScroll = Scroll("AnnouncementContent", root.transform, left, top, rightWidth, viewHeight);
            bodyRoot = Child("Articles", rightScroll.transform, Vector3.zero);
            float x = -rightWidth / 2f + 12f;
            float y = viewHeight / 2f - 12f;
            float textWidth = rightWidth - 28f;
            for (int i = 0; i < entries.Count; i++)
            {
                UILabel title = Label("Title" + i, bodyRoot.transform, x, y, textWidth, entries[i].Title, true, rightScroll);
                y -= title.height + 20f;
                // Bound each text mesh while retaining the complete article and
                // original line breaks. Native font measurement determines height.
                string text = entries[i].Content;
                int offset = 0;
                int part = 0;
                while (offset < text.Length)
                {
                    int count = Math.Min(2048, text.Length - offset);
                    if (offset + count < text.Length)
                    {
                        int line = text.LastIndexOf('\n', offset + count - 1, count);
                        if (line >= offset + count / 2) count = line - offset + 1;
                        if (count > 0 && Char.IsHighSurrogate(text[offset + count - 1])) count--;
                    }
                    UILabel paragraph = Label("Text" + i + "_" + part++, bodyRoot.transform, x, y, textWidth, text.Substring(offset, count), false, rightScroll);
                    y -= paragraph.height + 4f;
                    offset += count;
                }
                y -= 32f;
            }
            rightScroll.ResetPosition();
        }

        void LateUpdate()
        {
            if (!visible || background == null) return;
            if (root == null || background.width != lastWidth || background.height != lastHeight) Build();
        }
    }
}
