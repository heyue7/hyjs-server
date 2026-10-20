using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
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
            KingRecruitContentControl control = tab.GetComponent<KingRecruitContentControl>();
            if (control == null) control = tab.gameObject.AddComponent<KingRecruitContentControl>();
            control.Enter(tab);
        }

        public static bool CanAct(TabKingsRecruit tab)
        {
            KingRecruitContentControl control = tab == null ? null : tab.GetComponent<KingRecruitContentControl>();
            return control != null && control.Ready;
        }

        public static void Received(TabKingsRecruit tab)
        {
            KingRecruitContentControl control = tab == null ? null : tab.GetComponent<KingRecruitContentControl>();
            if (control != null) control.Received();
        }

        public static void BeginDraw(TabKingsRecruit tab)
        {
            KingRecruitContentControl control = tab == null ? null : tab.GetComponent<KingRecruitContentControl>();
            if (control != null) control.BeginDraw();
        }

        public static void EndDraw(TabKingsRecruit tab)
        {
            KingRecruitContentControl control = tab == null ? null : tab.GetComponent<KingRecruitContentControl>();
            if (control != null) control.EndDraw();
        }

        // Only the exact three missing recruit-card requests use the existing original painting.
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

    // Readiness is owned by the shop entry's accepted world. The page draws no world selector.
    public sealed class KingRecruitContentControl : MonoBehaviour
    {
        static readonly FieldInfo CurTabField = typeof(ShopPage).GetField("curTab", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        static readonly FieldInfo CurTypeField = typeof(ShopPage).GetField("curType", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        static readonly PropertyInfo InfoProperty = typeof(TabKingsRecruit).GetProperty("recruitInfo", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);

        TabKingsRecruit tab;
        ShopWorldMenuControl menu;
        long owner;
        string session = "";
        float entered;
        bool hasInfo;
        ProtoBuf.Message.WarriorRecruit info;
        UILabel hint;

        public bool Ready { get; private set; }

        public void Enter(TabKingsRecruit value)
        {
            tab = value;
            long currentOwner;
            string currentSession;
            if (!KingRecruitSession.Current(out currentOwner, out currentSession))
            {
                Ready = false;
                return;
            }
            if (currentOwner != owner || currentSession != session)
            {
                owner = currentOwner;
                session = currentSession;
                entered = Time.realtimeSinceStartup;
                hasInfo = false;
                info = null;
            }
            ShopPage shop = tab.GetComponentInParent<ShopPage>();
            menu = shop == null ? null : shop.GetComponent<ShopWorldMenuControl>();
            Ready = Compute();
        }

        bool OwnerCurrent()
        {
            long currentOwner;
            string currentSession;
            if (!KingRecruitSession.Current(out currentOwner, out currentSession)) return false;
            return currentOwner == owner && currentSession == session;
        }

        bool ContentVisible()
        {
            if (tab == null || tab.backGround == null || !tab.backGround.enabled || !tab.backGround.gameObject.activeInHierarchy) return false;
            ShopPage shop = tab.GetComponentInParent<ShopPage>();
            return shop != null && CurTabField != null && CurTypeField != null &&
                CurTabField.GetValue(shop) as GameObject == tab.gameObject &&
                Convert.ToInt32(CurTypeField.GetValue(shop)) == 1;
        }

        void Update()
        {
            if (tab == null) return;
            if (!ContentVisible())
            {
                Ready = false;
                HideHint();
                return;
            }
            Ready = Compute();
            SyncHint();
        }

        bool Compute()
        {
            if (tab == null || menu == null || !OwnerCurrent() || !ContentVisible()) return false;
            if (menu.Selecting || menu.DrawInFlight) return false;
            int world = menu.ResolvedWorld;
            if (world < 1 || world > 3 || !menu.HasWorld(world)) return false;
            ProtoBuf.Message.WarriorRecruit current = CurrentInfo();
            if (current == null || current.on != 1 || current.space != world) return false;
            return true;
        }

        ProtoBuf.Message.WarriorRecruit CurrentInfo()
        {
            if (InfoProperty == null) return null;
            try { return InfoProperty.GetValue(tab, null) as ProtoBuf.Message.WarriorRecruit; }
            catch (Exception) { return null; }
        }

        public void Received()
        {
            info = CurrentInfo();
            hasInfo = info != null;
            Ready = Compute();
        }

        public void BeginDraw()
        {
            if (menu != null) { menu.DrawInFlight = true; menu.DrawDeadline = Time.realtimeSinceStartup + 20f; }
            Ready = false;
        }

        public void EndDraw()
        {
            if (menu != null) menu.DrawInFlight = false;
            Ready = false;
            StartCoroutine(Recheck());
        }

        // The original callback can land before the shop catalog on a cold open.
        IEnumerator Recheck()
        {
            float deadline = Time.realtimeSinceStartup + 3f;
            while (Time.realtimeSinceStartup < deadline)
            {
                yield return null;
                if (tab == null) yield break;
                Ready = Compute();
                if (Ready) yield break;
            }
        }

        void OnDisable()
        {
            Ready = false;
            HideHint();
        }

        void OnDestroy()
        {
            Ready = false;
            if (hint != null) UnityEngine.Object.Destroy(hint.gameObject);
            hint = null;
        }

        void SyncHint()
        {
            if (Ready) { HideHint(); return; }
            if (menu == null)
            {
                if (Time.realtimeSinceStartup - entered > 1.5f) ShowHint("王者招募世界未就绪", null);
                return;
            }
            if (!String.IsNullOrEmpty(menu.Notice))
            {
                ShowHint(menu.Notice, menu);
                return;
            }
            if (menu.Selecting) { ShowHint("世界切换中，请稍候", null); return; }
            if (menu.DrawInFlight) { ShowHint("抽卡请求进行中，请稍候", null); return; }
            if (!menu.CatalogLoaded || menu.ResolvedWorld < 1)
            {
                if (Time.realtimeSinceStartup - entered > 1.5f) ShowHint("正在确认王者招募世界…", null);
                return;
            }
            HideHint();
        }

        void ShowHint(string text, ShopWorldMenuControl retry)
        {
            if (hint == null) hint = BuildHint();
            if (hint == null) return;
            hint.text = retry == null ? text : text + "（点击重试）";
            hint.gameObject.SetActive(true);
            if (retry != null) UIEventListener.Get(hint.gameObject).onClick = delegate(GameObject ignored) { retry.Retry(); };
            PlaceHint();
        }

        void HideHint()
        {
            if (hint != null) hint.gameObject.SetActive(false);
        }

        UILabel BuildHint()
        {
            if (tab == null || tab.labCostOne == null) return null;
            GameObject label = new GameObject("KingWorldHint");
            label.layer = tab.gameObject.layer;
            label.transform.parent = tab.transform;
            label.transform.localScale = Vector3.one;
            UILabel text = label.AddComponent<UILabel>();
            if (tab.labCostOne.bitmapFont != null) text.bitmapFont = tab.labCostOne.bitmapFont;
            else text.trueTypeFont = tab.labCostOne.trueTypeFont;
            text.fontSize = 22;
            text.width = 420;
            text.height = 40;
            text.alignment = NGUIText.Alignment.Center;
            text.pivot = UIWidget.Pivot.Center;
            text.color = Color.white;
            text.effectStyle = UILabel.Effect.Outline;
            text.effectColor = Color.black;
            text.depth = tab.labCostOne.depth + 150;
            BoxCollider collider = label.AddComponent<BoxCollider>();
            collider.size = new Vector3(420f, 44f, 1f);
            return text;
        }

        // Anchor on the original king content instead of screen space.
        void PlaceHint()
        {
            if (hint == null || tab == null || tab.backGround == null) return;
            Vector3[] corners = tab.backGround.worldCorners;
            Vector3 left = tab.transform.InverseTransformPoint(corners[1]);
            Vector3 right = tab.transform.InverseTransformPoint(corners[2]);
            Vector3 bottom = tab.transform.InverseTransformPoint(corners[0]);
            hint.transform.localPosition = left + (right - left) * .5f + (bottom - left) * .12f;
        }
    }
}
