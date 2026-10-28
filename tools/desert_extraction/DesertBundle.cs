using System;
using UnityEngine;

namespace HYJS.Desert53
{
    // Generated constants bind this adapter to one native Android bundle.
    // Logical keys, the original load queue and the scene name stay unchanged.
    public static class DesertBundle
    {
        public const string LogicalName = "map/map_desertfield_001";
        public const string PhysicalName = "hyjs/desert53/__BUNDLE_SHA256__/map_desertfield_001";
        public const long BundleSize = __BUNDLE_SIZE__L;

        public static bool IsTarget(string name)
        {
            return String.Equals(name, LogicalName, StringComparison.OrdinalIgnoreCase);
        }

        public static string ResolvePath(string name, ref bool localFile, bool forWWW)
        {
            if (!IsTarget(name)) return null;
            localFile = !forWWW;
            return forWWW ? Application.streamingAssetsPath + "/android/" + PhysicalName :
                Application.dataPath + "!assets/android/" + PhysicalName;
        }

        public static string[] GetDependencies(AssetBundleManifest manifest, string name)
        {
            return IsTarget(name) ? new string[0] : manifest.GetAllDependencies(name);
        }
    }
}
