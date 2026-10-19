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
        // Six bounded, process-lifetime textures match the original resource
        // sizes. Full detail paintings continue sharing the original bundle.
        static readonly Dictionary<string, Texture2D> Icons = new Dictionary<string, Texture2D>();

        public static UnityEngine.Object MapRequestedObject(string requestedPath, UnityEngine.Object asset)
        {
            string id = AliasId(requestedPath);
            Texture source = asset as Texture;
            if (id == null || PortraitId(source) != id) return asset;
            bool head = Same(requestedPath, "heroheader/hero_icon_" + id);
            string key = (head ? "head:" : "card:") + id;
            Texture2D result;
            if (Icons.TryGetValue(key, out result) && result != null) return result;
            int size = head ? 128 : 512;
            int rim = head ? 8 : 0;
            int inner = size - rim * 2;
            Rect crop = head ? Face(id) : Card(id);
            RenderTexture previous = RenderTexture.active;
            RenderTexture target = null;
            Texture2D readback = null;
            result = null;
            try
            {
                target = RenderTexture.GetTemporary(source.width, source.height, 0, RenderTextureFormat.ARGB32);
                // Original ASTC textures are not CPU-readable. Copy only this
                // small UI crop through the GPU, leaving bundle bytes untouched.
                Graphics.Blit(source, target);
                RenderTexture.active = target;
                int cropSize = Mathf.RoundToInt(crop.width * source.width);
                readback = new Texture2D(cropSize, cropSize, TextureFormat.RGBA32, false);
                readback.ReadPixels(new Rect(Mathf.RoundToInt(crop.x * source.width), Mathf.RoundToInt(crop.y * source.height), cropSize, cropSize), 0, 0);
                Color32[] sourcePixels = readback.GetPixels32();
                Color32[] pixels = new Color32[size * size];
                // Resample in managed memory after one bounded readback. This
                // works with the original Unity 5.3 API, which has no Blit UV
                // scale/offset overload, and preserves the source alpha.
                for (int row = 0; row < inner; row++)
                {
                    float sy = Mathf.Clamp((row + .5f) * cropSize / inner - .5f, 0, cropSize - 1);
                    int y0 = (int)sy, y1 = Math.Min(y0 + 1, cropSize - 1);
                    for (int column = 0; column < inner; column++)
                    {
                        float sx = Mathf.Clamp((column + .5f) * cropSize / inner - .5f, 0, cropSize - 1);
                        int x0 = (int)sx, x1 = Math.Min(x0 + 1, cropSize - 1);
                        Color32 a = sourcePixels[y0 * cropSize + x0], b = sourcePixels[y0 * cropSize + x1];
                        Color32 c = sourcePixels[y1 * cropSize + x0], d = sourcePixels[y1 * cropSize + x1];
                        float fx = sx - x0, fy = sy - y0;
                        pixels[(row + rim) * size + column + rim] = new Color32(Blend(a.r,b.r,c.r,d.r,fx,fy), Blend(a.g,b.g,c.g,d.g,fx,fy), Blend(a.b,b.b,c.b,d.b,fx,fy), Blend(a.a,b.a,c.a,d.a,fx,fy));
                    }
                }
                result = new Texture2D(size, size, TextureFormat.RGBA32, false);
                result.name = "soulflower_" + (head ? "icon_" : "card_") + id;
                result.hideFlags = HideFlags.DontSave;
                result.wrapMode = TextureWrapMode.Clamp;
                result.filterMode = FilterMode.Bilinear;
                result.SetPixels32(pixels);
                result.Apply(false, true);
                Icons[key] = result;
                return result;
            }
            catch (Exception)
            {
                // Retain the prior bounded UV fallback if GPU readback is not
                // available; a missing portrait must not abort the original UI.
                Debug.LogWarning("SoulFlower original portrait crop could not be prepared");
                if (result != null) UnityEngine.Object.Destroy(result);
                return asset;
            }
            finally
            {
                RenderTexture.active = previous;
                if (target != null) RenderTexture.ReleaseTemporary(target);
                if (readback != null) UnityEngine.Object.Destroy(readback);
            }
        }

        static byte Blend(byte a, byte b, byte c, byte d, float x, float y)
        {
            return (byte)Mathf.RoundToInt((a + (b - a) * x) * (1 - y) + (c + (d - c) * x) * y);
        }

        // These exact keys are absent from all three original/GM/Re text tables.
        // A source-status label replaces blank text without inventing lore,
        // skill names, effects, probabilities or balance values.
        static readonly string[] MissingIDs = { "069", "070", "073" };
        public static string MissingOriginalText(string key)
        {
            string activityText = OpeningPeriodText(key);
            if (activityText != null) return activityText;
            if (String.IsNullOrEmpty(key) || !(key.StartsWith("desc_hero_", StringComparison.Ordinal) || key.StartsWith("name_skill_", StringComparison.Ordinal) || key.StartsWith("desc_skill_", StringComparison.Ordinal))) return null;
            foreach (string id in MissingIDs)
            {
                if (key == "desc_hero_" + id) return "原客户端未提供该角色简介。";
                for (int slot = 1; slot <= 3; slot++)
                {
                    string skill = id + "_" + slot;
                    if (key == "desc_skill_" + skill)
                    {
                        // Read the same current cast sum used by SkillItem's
                        // native formatter. Only ultimate descriptions retain
                        // its {0} placeholder for the real current-level value.
                        var casts = Dict.DictDataManager.Instance.tableSkillCastDao.GetByOneIndex(skill);
                        var data = Dict.DictDataManager.Instance.tableSkillDao.GetById(skill);
                        double factor = Dict.Blo.DictSkillCastBlo.GetSkillExportFactor(skill);
                        string basis = factor.ToString("0.##", System.Globalization.CultureInfo.InvariantCulture);
                        string lead = "按原配置：" + casts.Count + "段攻击，攻击系数合计";
                        if (slot == 3 && data.LevelGrowFactor > 0)
                            return lead + "{0}%（基础" + basis + "%，每级增加" + data.LevelGrowFactor + "个百分点）。";
                        return lead + basis + "%。";
                    }
                    if (key == "name_skill_" + skill) return slot == 3 ? "必杀技（名称待补）" : "普通技能" + slot + "（名称待补）";
                }
            }
            return null;
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
