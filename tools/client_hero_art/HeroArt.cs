using System;
using System.Collections.Generic;
using UnityEngine;
using Dict.DataModel;

namespace HYJS.HeroArt
{
    // One component keeps the prefab's original UVs intact when an item is reused.
    public sealed class OriginalHeroCrop : MonoBehaviour
    {
        public Rect Original;
        public Vector4 OriginalRegion;
        public bool Applied;
        public string CardId;
    }

    public static class HeroArtFallback
    {
        // The asset boundary only aliases the exact six missing paths. Crops are
        // applied once by the widget setters below, bound to the request path, so
        // a 1024 painting is never cropped twice on its way to a widget.
        public static UnityEngine.Object MapRequestedObject(string requestedPath, UnityEngine.Object asset)
        {
            return asset;
        }

        // Authored copy for the three originals whose keys are absent from the
        // original/GM/Re text tables. The single source is hero_copy.json; the
        // world, weapon and skill mechanics come from this package's tables.
        public static void AdjustSkillCast(TableSkillCast row) { HeroCopyTable.Adjust(row); }

        public static string MissingOriginalText(string key)
        {
            string activityText = OpeningPeriodText(key);
            if (activityText != null) return activityText;
            return HeroCopyTable.Get(key);
        }

        // Exact half-month source keys only. Amounts and task thresholds retain
        // the original table values; eligibility remains the server's state.
        static string OpeningPeriodText(string key)
        {
            switch (key)
            {
                case "sevenday_login_send_14": return "活动期间登录领取角色碎片，可合成角色【三选一】";
                case "sevendayTaskTitle_137":
                case "sevendayTaskTitle_152":
                case "sevendayTaskTitle_167":
                case "sevendayTaskTitle_186":
                case "sevendayTaskTitle_205":
                case "sevendayTaskTitle_222": return "活动期间登陆赠送";
                case "sevendayTaskTitle_138": return "活动期间单笔充值6元";
                case "sevendayTaskTitle_153": return "活动期间单笔充值30元";
                case "sevendayTaskTitle_139":
                case "sevendayTaskTitle_154": return "活动期间参与夺宝5次";
                case "sevendayTaskTitle_140":
                case "sevendayTaskTitle_155": return "活动期间参与夺宝15次";
                case "sevendayTaskTitle_141":
                case "sevendayTaskTitle_156": return "活动期间参与夺宝25次";
                case "sevendayTaskTitle_142":
                case "sevendayTaskTitle_157": return "活动期间参与夺宝40次";
                case "sevendayTaskTitle_143":
                case "sevendayTaskTitle_158": return "活动期间参与夺宝60次";
                case "sevendayTaskTitle_144":
                case "sevendayTaskTitle_159": return "活动期间参与夺宝80次";
                case "sevendayTaskTitle_145":
                case "sevendayTaskTitle_160": return "活动期间参与夺宝110次";
            }
            return null;
        }

        public static void RefreshOpeningIndicators()
        {
            // Resolve the actual cached 7/14-day page, then use the original
            // request/callback chain that updates every level of red points.
            // A delayed successful purchase must not resurrect a closed page.
            try
            {
                UI.SevenDayPage page = UI.PageManager.Instance.GetDictPage("SevenDayPage") as UI.SevenDayPage;
                if (page != null && page.gameObject.activeInHierarchy && page.topLeft != null)
                    page.topLeft.ReFreshDayAct();
            }
            catch (Exception)
            {
                Debug.LogWarning("SoulFlower opening indicators could not refresh after purchase");
            }
        }

        static bool Same(string a, string b)
        {
            return String.Equals(a, b, StringComparison.OrdinalIgnoreCase);
        }

        static string AliasId(string path)
        {
            if (Same(path, "heroheader/hero_icon_069") || Same(path, "hero2d/hero_2d_069")) return "069";
            if (Same(path, "heroheader/hero_icon_070") || Same(path, "hero2d/hero_2d_070")) return "070";
            if (Same(path, "heroheader/hero_icon_073") || Same(path, "hero2d/hero_2d_073")) return "073";
            return null;
        }

        public static string MapBundle(string path)
        {
            string id = AliasId(path);
            return id == null ? path : "heropainter/hero_card_" + id;
        }

        public static string MapAsset(string bundle, string asset)
        {
            string id = AliasId(bundle);
            if (id != null && (Same(asset, "hero_icon_" + id) || Same(asset, "hero_2d_" + id)))
                return "hero_card_" + id;
            return asset;
        }

        public static int AtlasHidden(TableHero hero)
        {
            if (hero.id == "hero_069" || hero.id == "hero_070" || hero.id == "hero_073") return 0;
            return hero.IsShowInCard;
        }

        static string PortraitId(Texture asset)
        {
            if (asset == null) return null;
            if (asset.name == "hero_card_069") return "069";
            if (asset.name == "hero_card_070") return "070";
            if (asset.name == "hero_card_073") return "073";
            return null;
        }

        // Pixel boxes were read from the original 1024-square portraits.
        // Unity UV origin is bottom-left; the inspection image origin is top-left.
        static Rect Box(float x, float top, float size)
        {
            return new Rect(x / 1024f, 1f - (top + size) / 1024f, size / 1024f, size / 1024f);
        }

        static Rect Face(string id)
        {
            if (id == "069") return Box(420, 60, 240);
            if (id == "070") return Box(245, 135, 270);
            return Box(270, 70, 270);
        }

        static Rect Card(string id)
        {
            if (id == "069") return Box(300, 40, 560);
            if (id == "070") return Box(150, 100, 560);
            return Box(130, 30, 560);
        }

        static void Restore(UITexture texture)
        {
            OriginalHeroCrop state = texture.GetComponent<OriginalHeroCrop>();
            if (state != null && state.Applied)
            {
                texture.uvRect = state.Original;
                texture.drawRegion = state.OriginalRegion;
                state.Applied = false;
            }
        }

        static void Apply(UITexture texture, Rect crop, bool head)
        {
            OriginalHeroCrop state = texture.GetComponent<OriginalHeroCrop>();
            if (state == null) state = texture.gameObject.AddComponent<OriginalHeroCrop>();
            if (!state.Applied)
            {
                state.Original = texture.uvRect;
                state.OriginalRegion = texture.drawRegion;
            }
            texture.uvRect = crop;
            // Original icons include a transparent rim under the frame. A crop
            // from a full painting has no such rim; leave it inside that frame.
            texture.drawRegion = head ? new Vector4(.06f, .06f, .94f, .94f) : state.OriginalRegion;
            state.Applied = true;
        }

        // Used only by the original head TextureKit and battle portrait setter.
        public static void AssignHead(UITexture texture, Texture asset)
        {
            texture.mainTexture = asset;
            string id = PortraitId(asset);
            if (id == null) Restore(texture);
            else Apply(texture, Face(id), true);
        }

        // AloneTextureKit also serves full detail paintings. Remember the original
        // requested path so a detail painting never receives a recruit-card crop.
        public static void PrepareCard(AloneTextureKit owner, string requestedPath)
        {
            UITexture texture = owner.GetComponent<UITexture>();
            if (texture == null) return;
            string id = AliasId(requestedPath);
            if (id != null && !Same(requestedPath, "hero2d/hero_2d_" + id)) id = null;
            OriginalHeroCrop state = texture.GetComponent<OriginalHeroCrop>();
            if (state == null && id != null) state = texture.gameObject.AddComponent<OriginalHeroCrop>();
            if (state != null) state.CardId = id;
            Restore(texture);
        }

        public static void AssignCard(UITexture texture, Texture asset)
        {
            texture.mainTexture = asset;
            OriginalHeroCrop state = texture.GetComponent<OriginalHeroCrop>();
            string id = PortraitId(asset);
            if (state != null && id != null && state.CardId == id) Apply(texture, Card(id), false);
            else Restore(texture);
        }

        // AssetLoadHelper is also used by full detail paintings and map images.
        // Inspect its unaliased request; a shared hero_card texture alone does
        // not distinguish those uses from an avatar or a recruit-card fallback.
        public static void AssignRequested(UITexture texture, Texture asset, string requestedPath)
        {
            string id = AliasId(requestedPath);
            if (id != null && PortraitId(asset) == id)
            {
                texture.mainTexture = asset;
                bool head = Same(requestedPath, "heroheader/hero_icon_" + id);
                Apply(texture, head ? Face(id) : Card(id), head);
            }
            else
            {
                Restore(texture);
                texture.mainTexture = asset;
            }
        }

        public static void MakeRequestedPixelPerfect(UITexture texture)
        {
            OriginalHeroCrop state = texture.GetComponent<OriginalHeroCrop>();
            // The aliased painting is 1024px square; retaining the prefab's
            // widget size prevents MakePixelPerfect from expanding an avatar.
            if (state != null && state.Applied && PortraitId(texture.mainTexture) != null) return;
            texture.MakePixelPerfect();
        }
    }
}
