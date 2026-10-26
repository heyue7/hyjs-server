using System;
using System.Collections.Generic;
using HYJS.GGO;

namespace UnityEngine
{
    public static class Application { public static string dataPath = "/data/app/original/base.apk"; }
    public static class Debug { public static void LogWarning(object value) { throw new Exception("Unexpected warning: " + value); } }
    public class AssetBundleManifest
    {
        public int calls;
        public string[] result = new string[] { "native/dependency" };
        public string[] GetAllDependencies(string name)
        {
            calls++;
            if (name == "native/throws") throw new InvalidOperationException("native dependency failure");
            return result;
        }
    }
}
public class AssetCell { public string path, hashCode; public long size; }
public class AssetFileTool
{
    public static int parses;
    public Dictionary<string, AssetCell> assetCells = new Dictionary<string, AssetCell>();
    public static AssetFileTool LoadFromString(string text)
    {
        parses++;
        string[] lines = text.Split('\n');
        Check.Equal(lines[0], "1", "native manifest version");
        AssetFileTool tool = new AssetFileTool();
        for (int index = 1; index < lines.Length; index++)
        {
            if (lines[index].Length == 0) continue;
            string[] columns = lines[index].Split('\t');
            tool.assetCells.Add(columns[0], new AssetCell { path = columns[0], hashCode = columns[1], size = Int64.Parse(columns[2]) });
        }
        return tool;
    }
}
public class OriginalManager
{
    private AssetFileTool assetFileTool, smallAssetFileTool;
    public OriginalManager(AssetFileTool full, AssetFileTool small) { assetFileTool = full; smallAssetFileTool = small; }
}
public static class Check
{
    static int count;
    public static void Equal<T>(T actual, T expected, string name)
    {
        count++;
        if (!System.Object.Equals(actual, expected)) throw new Exception(name + ": " + actual + " != " + expected);
    }
    public static void Main()
    {
        PetPortraitAssets75.Ensure(null);
        PetPortraitAssets75.Ensure(new OriginalManager(null, null));
        Equal(AssetFileTool.parses, 0, "uninitialized loader does not parse");
        AssetFileTool full = new AssetFileTool(), small = new AssetFileTool();
        AssetCell unknown = new AssetCell { path = "future/user-bundle", hashCode = "unchanged", size = 913 };
        AssetCell existingPet = new AssetCell { path = "pet/pet_icon_002", hashCode = "native", size = 123 };
        full.assetCells.Add(unknown.path, unknown); small.assetCells.Add(existingPet.path, existingPet);
        full.assetCells.Add("pet/pet_icon_076", new AssetCell { path = "pet/pet_icon_076", hashCode = "stale", size = 1 });
        OriginalManager manager = new OriginalManager(full, small);
        PetPortraitAssets75.Ensure(manager);
        Equal(AssetFileTool.parses, 1, "manifest parsed once");
        Equal(full.assetCells.Count, 4, "only three exact keys registered full");
        Equal(small.assetCells.Count, 4, "only three exact keys registered small");
        Equal(full.assetCells[unknown.path], unknown, "unknown identity retained");
        Equal(small.assetCells[existingPet.path], existingPet, "original pet identity retained");
        string[] keys = new string[] { "pet/pet_icon_076", "pet/pet_icon_077", "pet/pet_icon_078" };
        long[] sizes = new long[] { 55992, 45713, 40677 };
        for (int index = 0; index < keys.Length; index++)
        {
            string key = keys[index];
            Equal(full.assetCells[key].size, sizes[index], key + " original-model size");
            Equal(full.assetCells[key], small.assetCells[key], key + " both manifests agree");
            Equal(PetPortraitAssets75.Dependencies(null, key).Length, 0, key + " no old native manifest dependency required");
            foreach (bool before in new bool[] { false, true })
            {
                bool file = before; string path;
                Equal(PetPortraitAssets75.TryPath(key.ToUpperInvariant(), ref file, out path), true, key + " package route");
                Equal(file, true, key + " LoadFromFile flag");
                Equal(path, UnityEngine.Application.dataPath + "!assets/android/" + key, key + " native APK file path");
            }
        }
        for (int index = 0; index < 100; index++) PetPortraitAssets75.Ensure(manager);
        Equal(AssetFileTool.parses, 1, "stable dictionary references avoid reparsing");
        full.assetCells = new Dictionary<string, AssetCell>();
        PetPortraitAssets75.Ensure(manager);
        Equal(full.assetCells.Count, 3, "replacement dictionary registered");
        Equal(AssetFileTool.parses, 1, "replacement dictionaries reuse parser result");
        PetPortraitAssets75.Ensure(new OriginalManager(new AssetFileTool(), new AssetFileTool()));
        Equal(AssetFileTool.parses, 1, "new loader reuses immutable additions");
        UnityEngine.AssetBundleManifest manifest = new UnityEngine.AssetBundleManifest();
        Equal(PetPortraitAssets75.Dependencies(manifest, "pet/pet_icon_002"), manifest.result, "other native dependency array identity");
        Equal(manifest.calls, 1, "other native dependency invocation");
        bool threw = false;
        try { PetPortraitAssets75.Dependencies(manifest, "native/throws"); } catch (InvalidOperationException) { threw = true; }
        Equal(threw, true, "native dependency exception retained");
        threw = false;
        try { PetPortraitAssets75.Dependencies(null, "pet/pet_icon_002"); } catch (NullReferenceException) { threw = true; }
        Equal(threw, true, "other null manifest semantics retained");
        foreach (string key in new string[] { null, "android", "smallassetmanifest", "pet/pet_icon_002", "pet/pet_icon_076-extra", "other/pet_icon_076" })
        {
            bool file = false; string path;
            Equal(PetPortraitAssets75.TryPath(key, ref file, out path), false, "other native path preserved");
            Equal(file, false, "other native flag preserved");
        }
        Console.WriteLine("PASS " + count + " assertions; real helper source, Unity boundary doubles, no phone execution");
    }
}
