using System;
using System.Collections.Generic;
using System.Reflection;
using UnityEngine;

namespace HYJS.GGO
{
    // Three portraits reconstructed from their own shipped models. Do not migrate or
    // rewrite any user manifest: the native loaders retain every other asset policy.
    public static class PetPortraitAssets75
    {
        const BindingFlags Fields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        static readonly object Gate = new object();
        static readonly string[] NoDependencies = new string[0];
        static Dictionary<string, AssetCell> registeredFull, registeredSmall;
        static AssetFileTool additions;
        static Type managerType;
        static FieldInfo fullField, smallField;
        static bool errorLogged;

        static bool Portrait(string name)
        {
            return name == "pet/pet_icon_076" || name == "pet/pet_icon_077" || name == "pet/pet_icon_078";
        }

        public static void Ensure(object manager)
        {
            if (manager == null) return;
            try
            {
                lock (Gate)
                {
                    Type type = manager.GetType();
                    if (managerType != type)
                    {
                        fullField = type.GetField("assetFileTool", Fields);
                        smallField = type.GetField("smallAssetFileTool", Fields);
                        managerType = type;
                    }
                    AssetFileTool full = fullField == null ? null : fullField.GetValue(manager) as AssetFileTool;
                    AssetFileTool small = smallField == null ? null : smallField.GetValue(manager) as AssetFileTool;
                    if (full == null || small == null || full.assetCells == null || small.assetCells == null) return;
                    if (System.Object.ReferenceEquals(registeredFull, full.assetCells) && System.Object.ReferenceEquals(registeredSmall, small.assetCells)) return;
                    if (additions == null)
                    {
                        additions = AssetFileTool.LoadFromString("1\n" + PetIconManifest75.Text);
                        if (additions == null || additions.assetCells.Count != 3) throw new InvalidOperationException("Portrait manifest must contain exactly three entries");
                        foreach (string key in additions.assetCells.Keys)
                            if (!Portrait(key)) throw new InvalidOperationException("Unexpected portrait key");
                    }
                    foreach (KeyValuePair<string, AssetCell> pair in additions.assetCells)
                    {
                        full.assetCells[pair.Key] = pair.Value;
                        small.assetCells[pair.Key] = pair.Value;
                    }
                    registeredFull = full.assetCells;
                    registeredSmall = small.assetCells;
                }
            }
            catch (Exception error)
            {
                if (!errorLogged) { errorLogged = true; Debug.LogWarning("Pet portrait manifest: " + error.GetType().FullName); }
            }
        }

        public static string[] Dependencies(AssetBundleManifest manifest, string name)
        {
            if (Portrait(name)) return NoDependencies;
            return manifest.GetAllDependencies(name);
        }

        public static bool TryPath(string name, ref bool isFile, out string path)
        {
            path = null;
            if (name == null) return false;
            string lower = name.ToLower(); // Same normalization as the original GetFullPath.
            if (!Portrait(lower)) return false;
            // The native loader uses this flag for AssetBundle.LoadFromFile, which
            // supports the APK's uncompressed !assets path. WWW jar URLs are not used.
            isFile = true;
            path = Application.dataPath + "!assets/android/" + lower;
            return true;
        }
    }
}
