"""Offline component checks only; no Unity/native loader/device claim."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]


def main():
    with tempfile.TemporaryDirectory(prefix='hyjs-local501-check-') as directory:
        work = Path(directory)
        apk, cache = work / 'component.apk', work / 'cache'
        cache.mkdir()
        data = b'owned original bytes test fixture'
        row = ('assets/android/model/scoped', data)
        inventory = ('local-awake501-original-resource-closure-v1\n' + row[0] + '\t' + str(len(data)) +
                     '\t' + hashlib.sha256(data).hexdigest() + '\t' + hashlib.md5(data).hexdigest() + '\n')
        with zipfile.ZipFile(apk, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
            archive.writestr(*row)
        (work / 'proof.txt').write_text(inventory)
        (work / 'Program.cs').write_text(HARNESS.replace('INVENTORY_HASH', hashlib.sha256(inventory.encode()).hexdigest()))
        (work / 'Check.csproj').write_text(
            '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
            '<TargetFramework>net8.0</TargetFramework><AssemblyName>Assembly-CSharp</AssemblyName>'
            '<EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup>'
            '<Compile Include="Program.cs" /><Compile Include="' +
            str(ROOT / 'tools/local_asset_ready/LocalAssetReady.cs') + '" />'
            '<EmbeddedResource Include="proof.txt" LogicalName="local501.inventory" />'
            '</ItemGroup></Project>')
        dotnet = ROOT / '.tools/native-batch/dotnet/dotnet'
        subprocess.run(['rtk', 'proxy', str(dotnet), 'build', str(work / 'Check.csproj'),
                        '--nologo', '--verbosity', 'quiet'], check=True, cwd=ROOT)
        result = subprocess.run(['rtk', 'proxy', str(dotnet), str(work / 'bin/Debug/net8.0/Assembly-CSharp.dll'),
                                 str(apk), str(cache)], check=True, text=True, capture_output=True, cwd=ROOT)
        print(result.stdout.strip())


HARNESS = r'''
using System;
using System.Collections;
using System.IO;
using System.Text;
using System.Threading;
using HYJS.LocalAssets;
public static class RenderManager { public static int Quality { get; set; } = 2; }
public static class StaticData { public static Player playerData = new Player(); }
public sealed class Player { private Inst instPlayerData = new Inst(); public Inst TestInst { get { return instPlayerData; } } }
public sealed class Inst { public Hashtable BattleCamps = new Hashtable(); }
public sealed class Formation { public Hero hero = new Hero(); public Pet pet = new Pet(); }
public sealed class Hero { public string heroId = "hero_001"; }
public sealed class Pet { public string id = "pet_002"; }
public sealed class Cell { public long size; public string hashCode; }
public sealed class FileTool { public Hashtable assetCells = new Hashtable(); }
namespace AssetLoad { public sealed class AssetBundleManager {
    public static AssetBundleManager Instance { get; } = new AssetBundleManager();
    public bool IsInitialized { get; set; } = true;
    public FileTool smallAssetFileTool = new FileTool(), assetFileTool = new FileTool();
} }
namespace Room { public sealed class RCUINormalNPC {
    public string pageName = "ChallengeActivityCopyPage", pageOption = "type=0"; public int pageLevel = 70;
} }
public sealed class TaskInfo { public string copyId = "501"; }
public sealed class Page { public int chapter = 5; public int GetCurrChapterID() { return chapter; } }
public sealed class AwakeDiffItem { private TaskInfo taskInfo = new TaskInfo(); private Page page = new Page();
    public void Copy(string id) { taskInfo.copyId = id; } public void Chapter(int id) { page.chapter = id; }
}
namespace UnityEngine {
    public enum RuntimePlatform { Android }
    public static class Application { public static string dataPath, persistentDataPath; public static RuntimePlatform platform; }
    public static class Debug { public static System.Collections.Generic.List<string> lines = new System.Collections.Generic.List<string>(); public static void LogWarning(object value) { lines.Add(Convert.ToString(value)); } }
    public sealed class AndroidJavaClass : IDisposable {
        public static System.Collections.Generic.List<string> lines = new System.Collections.Generic.List<string>();
        public AndroidJavaClass(string name) { if(name!="android.util.Log") throw new Exception("Unexpected JNI diagnostic target"); }
        public T CallStatic<T>(string method, params object[] args) { if(method!="i" || (string)args[0]!="HYJS501") throw new Exception("Unexpected JNI call"); lines.Add(Convert.ToString(args[1])); return default(T); }
        public void Dispose() { }
    }
}
namespace HYJS.LocalAssets { internal static class LocalResourceProof { internal const string InventorySHA256 = "INVENTORY_HASH"; } }
public static class Program {
    static int checks;
    static void Check(bool value, string name) { if (!value) throw new Exception(name); checks++; }
    static void Reject(Action action, string name) { try { action(); } catch (ResourceValidationException) { checks++; return; } throw new Exception(name); }
    public static void Main(string[] args) {
        UnityEngine.Application.dataPath = args[0]; UnityEngine.Application.persistentDataPath = args[1];
        string text; using (var stream = typeof(Program).Assembly.GetManifestResourceStream("local501.inventory"))
        using (var reader = new StreamReader(stream)) text = reader.ReadToEnd();
        var rows = VerifiedPackage.Parse(text); var source = rows[0];
        Check(VerifiedPackage.Verify(args[0], args[1], rows) == 0, "exact packaged bytes");
        Reject(delegate { VerifiedPackage.Parse(text.Replace(source.SHA256, new string('z',64))); }, "hex proof rejected");
        Reject(delegate { VerifiedPackage.Parse(text + text.Substring(text.IndexOf('\n') + 1)); }, "duplicate proof rejected");
        var bad = new ResourceRow { Entry=source.Entry, Size=source.Size, MD5=source.MD5, SHA256=new string('0',64) };
        Reject(delegate { VerifiedPackage.Verify(args[0], args[1], new System.Collections.Generic.List<ResourceRow> { bad }); }, "changed bytes rejected");
        bad.Entry = "assets/android/missing";
        Reject(delegate { VerifiedPackage.Verify(args[0], args[1], new System.Collections.Generic.List<ResourceRow> { bad }); }, "missing bytes rejected");
        var cell = new Cell { size=source.Size, hashCode=source.MD5 }; var manager=AssetLoad.AssetBundleManager.Instance;
        manager.smallAssetFileTool.assetCells["model/scoped"] = cell; manager.assetFileTool.assetCells["model/scoped"] = cell;
        var formation = new Formation(); StaticData.playerData.TestInst.BattleCamps[0] = formation;
        var npc = new Room.RCUINormalNPC(); var entry = new AwakeDiffItem();
        Check(LocalAssetReady.CatalogueReady(true, null), "original true unaffected");
        Check(!LocalAssetReady.CatalogueReady(false, new object()), "foreign caller closed");
        npc.pageOption="type=1";
        Check(!LocalAssetReady.CatalogueReady(false,npc), "wrong NPC option remains closed before verification");
        Check(UnityEngine.Debug.lines.Exists(x=>x.Contains("catalogue-fields") && x.Contains("pageOption=type=1")), "actual predicate metadata logged");
        Check(!UnityEngine.Debug.lines.Exists(x=>x.Contains("resource verification state=")), "predicate mismatch never starts hashing");
        npc.pageOption="type=0";
        bool ready=false; for(int i=0;i<200 && !ready;i++) { ready=LocalAssetReady.CatalogueReady(false,npc); if(!ready) Thread.Sleep(10); }
        Check(ready, "scoped verified catalogue"); Check(LocalAssetReady.EntryReady(false, entry), "scoped501 entry");
        Check(UnityEngine.Debug.lines.Exists(x=>x.Contains("catalogue-entry") && x.Contains("Room.RCUINormalNPC")), "entry logged before predicate");
        Check(UnityEngine.AndroidJavaClass.lines.Exists(x=>x.Contains("catalogue-entry") && x.Contains("Room.RCUINormalNPC")), "Android entry survives suppressed informational logger");
        entry.Copy("502"); Check(!LocalAssetReady.EntryReady(false,entry), "other copy closed"); entry.Copy("501");
        entry.Chapter(2); Check(!LocalAssetReady.EntryReady(false,entry), "other chapter closed"); entry.Chapter(5);
        RenderManager.Quality=1; Check(!LocalAssetReady.EntryReady(false,entry), "other quality closed"); RenderManager.Quality=2;
        formation.hero.heroId="hero_047"; Check(!LocalAssetReady.EntryReady(false,entry), "uncovered hero closed"); formation.hero.heroId="hero_001";
        formation.pet.id="pet_020"; Check(!LocalAssetReady.EntryReady(false,entry), "uncovered pet closed"); formation.pet.id="pet_002";
        manager.IsInitialized=false; Check(!LocalAssetReady.EntryReady(false,entry), "loader not initialized closed"); manager.IsInitialized=true;
        cell.hashCode=new string('0',32); Check(!LocalAssetReady.EntryReady(false,entry), "loaded cell mismatch closed"); cell.hashCode=source.MD5;
        Check(UnityEngine.Debug.lines.Exists(x=>x.Contains("Local501 cell") && x.Contains("size/hash differs")), "cell rejection reason logged");
        Check(LocalAssetReady.EntryReady(false,entry), "source restored ready");
        Directory.CreateDirectory(Path.Combine(args[1],"model")); File.WriteAllText(Path.Combine(args[1],"model/scoped"),"corrupt");
        Check(!LocalAssetReady.EntryReady(false,entry), "new override invalidates readiness");
        Reject(delegate { VerifiedPackage.Verify(args[0],args[1],rows); }, "corrupt cache rejected");
        for(int i=0;i<100;i++) { npc.pageOption="type="+i; LocalAssetReady.CatalogueReady(false,npc); }
        Check(UnityEngine.Debug.lines.FindAll(x=>x.StartsWith("Local501 ") && !x.StartsWith("Local501 resource verification state=")).Count<=64, "gate diagnostics bounded");
        Check(UnityEngine.AndroidJavaClass.lines.Count<=64, "native diagnostics share the same bounded budget");
        Console.WriteLine("{\"checks\":"+checks+",\"status\":\"PASS\",\"scope\":\"offline component stubs; no Unity loader or device evidence\"}");
    }
}
'''


if __name__ == '__main__':
    main()
