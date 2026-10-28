using UnityEngine;
using System.Collections.Generic;

namespace HYJS.Desert53
{
    // Original ray geometry, timing, saved materials and restoration remain.
    // Rebuilt bridge Scene-layer hits and compatible material setters are adapted.
    public static class DesertOcclusion
    {
        public static int ActiveOccluderCount { get; private set; }
        public static float LayerFadeAlpha { get; private set; }
        public static string ExtraBridgeOccluderNames { get; private set; }
        public static RaycastHit[] RaycastAll(Ray ray, float maxDistance, int layerMask)
        {
            GameObject root = GameObject.Find("map_desertfield_001");
            if (root == null)
            {
                ActiveOccluderCount = 0; LayerFadeAlpha = .3f; ExtraBridgeOccluderNames = "";
                return Physics.RaycastAll(ray, maxDistance, layerMask);
            }
            // One original-physics query preserves its returned order. Existing
            // mask hits remain untouched; only this map's Scene-layer bridge is added.
            RaycastHit[] candidates = Physics.RaycastAll(ray, maxDistance, layerMask | (1 << 11));
            List<RaycastHit> accepted = new List<RaycastHit>();
            foreach (RaycastHit hit in candidates)
            {
                Collider collider = hit.collider;
                if (collider == null) continue;
                int bit = 1 << collider.gameObject.layer;
                if ((layerMask & bit) != 0) { accepted.Add(hit); continue; }
                // Exclude only extra Scene-layer contact at the actor's feet.
                // Existing mask hits, including all original layer13 hits, bypass this gate.
                if (collider.gameObject.layer != 11 || hit.distance >= maxDistance - .2f) continue;
                if (IsBridge(collider.transform, root.transform)) accepted.Add(hit);
            }
            // Original Update only consumes RaycastHit.transform, not distance.
            // Preserve its feet results first; append distinct upper-body bridge hits.
            List<Collider> known = new List<Collider>();
            foreach (RaycastHit hit in accepted) if (!known.Contains(hit.collider)) known.Add(hit.collider);
            List<string> extraNames = new List<string>();
            Vector3 camera = ray.GetPoint(maxDistance * .5f);
            Vector3 feet = ray.GetPoint(maxDistance);
            foreach (float height in new float[] { .9f, 1.8f })
            {
                Vector3 delta = feet + Vector3.up * height - camera;
                float length = delta.magnitude;
                foreach (RaycastHit hit in Physics.RaycastAll(new Ray(camera, delta), length, (1 << 11) | (1 << 13)))
                {
                    Collider collider = hit.collider;
                    if (collider == null || hit.distance >= length - .2f || known.Contains(collider) || !IsBridge(collider.transform, root.transform)) continue;
                    known.Add(collider); accepted.Add(hit);
                    string name = collider.gameObject.name;
                    if (extraNames.Count < 12 && !extraNames.Contains(name)) extraNames.Add(name);
                }
            }
            // Thin stairs can fall between the discrete feet/torso/head rays.
            // Sweep the visible body width so every intervening bridge surface
            // joins the original camera cache and restores through its own logic.
            Vector3 bodyDelta = feet + Vector3.up * .9f - camera;
            float bodyLength = bodyDelta.magnitude;
            foreach (RaycastHit hit in Physics.SphereCastAll(new Ray(camera, bodyDelta), .65f, bodyLength - .2f, (1 << 11) | (1 << 13)))
            {
                Collider collider = hit.collider;
                if (collider == null || known.Contains(collider) || !IsBridge(collider.transform, root.transform)) continue;
                known.Add(collider); accepted.Add(hit);
                string name = collider.gameObject.name;
                if (extraNames.Count < 12 && !extraNames.Contains(name)) extraNames.Add(name);
            }
            ExtraBridgeOccluderNames = string.Join(" | ", extraNames.ToArray());
            List<Renderer> renderers = new List<Renderer>();
            foreach (RaycastHit hit in accepted)
            {
                // Match original SetTransRender's same-GameObject renderer lookup.
                Renderer renderer = hit.collider.gameObject.GetComponent<Renderer>();
                if (renderer != null && !renderers.Contains(renderer)) renderers.Add(renderer);
            }
            ActiveOccluderCount = renderers.Count;
            LayerFadeAlpha = 1 - Mathf.Pow(1 - .3f, 1f / Mathf.Max(1, ActiveOccluderCount));
            foreach (Renderer renderer in renderers)
            {
                // sharedMaterials reads existing references without instantiating.
                // Original CameraTool still owns its saved-material cache/revert.
                foreach (Material material in renderer.sharedMaterials)
                    if (material != null && IsOccluded(material.shader))
                    {
                        Color color = material.color; color.a = LayerFadeAlpha; material.color = color;
                    }
            }
            return accepted.ToArray();
        }
        static bool IsBridge(Transform item, Transform root)
        {
            bool bridge = false;
            for (Transform parent = item; parent != null; parent = parent.parent)
            {
                if (parent.name == "Observed bridge") bridge = true;
                if (parent == root) return bridge;
            }
            return false;
        }
        static bool IsOccluded(Shader shader)
        {
            if (shader == null) return false;
            string name = shader.name;
            return name.EndsWith("/Occluded") && Supported(name.Substring(0, name.Length - 9));
        }
        static bool Supported(string name)
        {
            return name == "HYJS/RebuiltSandstone" || name == "HYJS/RockVolume" ||
                name == "HYJS/ReferenceRock" || name == "HYJS/RebuiltCutStone" ||
                name == "HYJS/RuinsStone" || name == "HYJS/VideoMasonry" || name == "HYJS/VideoPaving";
        }
        public static void SetShader(Material material, Shader originalFallback)
        {
            string name = material.shader == null ? "" : material.shader.name;
            if (Supported(name) && GameObject.Find("map_desertfield_001") != null)
            {
                // AssetBundle shaders need their serialized scene reference; Shader.Find
                // alone does not register every inactive holder shader on Android.
                Shader variant = ReferencedVariant(name + "/Occluded");
                if (variant == null) throw new System.InvalidOperationException("Desert occlusion shader missing: " + name);
                material.shader = variant;
            }
            else material.shader = originalFallback;
        }
        static Shader ReferencedVariant(string name)
        {
            GameObject root = GameObject.Find("map_desertfield_001");
            Transform holder = root == null ? null : root.transform.Find("Camera occlusion shader references");
            Renderer renderer = holder == null ? null : holder.GetComponent<Renderer>();
            if (renderer != null)
                foreach (Material reference in renderer.sharedMaterials)
                    if (reference != null && reference.shader != null && reference.shader.name == name) return reference.shader;
            return Shader.Find(name);
        }
        public static void SetColor(Material material, Color originalColor)
        {
            if (IsOccluded(material.shader) && GameObject.Find("map_desertfield_001") != null)
            {
                float alpha = ActiveOccluderCount > 0 ? LayerFadeAlpha : .3f;
                Color color = material.color; color.a = Mathf.Min(originalColor.a, alpha); material.color = color;
            }
            else material.color = originalColor;
        }
    }
}
