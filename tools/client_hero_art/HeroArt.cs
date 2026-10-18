using System;
using UnityEngine;
using Dict.DataModel;

namespace HYJS.HeroArt
{
    // One component keeps the prefab's original UVs intact when an item is reused.
    public sealed class OriginalHeroCrop : MonoBehaviour
    {
        public Rect Original;
        public bool Applied;
        public string CardId;
    }

    public static class HeroArtFallback
    {
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
                state.Applied = false;
            }
        }

        static void Apply(UITexture texture, Rect crop)
        {
            OriginalHeroCrop state = texture.GetComponent<OriginalHeroCrop>();
            if (state == null) state = texture.gameObject.AddComponent<OriginalHeroCrop>();
            if (!state.Applied) state.Original = texture.uvRect;
            texture.uvRect = crop;
            state.Applied = true;
        }

        // Used only by the original head TextureKit and battle portrait setter.
        public static void AssignHead(UITexture texture, Texture asset)
        {
            texture.mainTexture = asset;
            string id = PortraitId(asset);
            if (id == null) Restore(texture);
            else Apply(texture, Face(id));
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
            if (state != null && id != null && state.CardId == id) Apply(texture, Card(id));
            else Restore(texture);
        }
    }
}
