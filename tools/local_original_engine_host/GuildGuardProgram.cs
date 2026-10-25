// Independent guard10 host entry: original caller/engine retained; local diagnostic only.
using System;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.Loader;
using System.Globalization;

internal static class GuildGuardProbeProgram
{
    static int Main(string[] args)
    {
        if (args.Length != 3) throw new ArgumentException("runtime directory, exact pve-auto directory and pinned numbered case required");
        string root = Path.GetFullPath(args[0]);
        string cases = Path.GetFullPath(args[1]);
        if (Path.GetFileName(cases) != "pve-auto" || Directory.GetFiles(cases,"*.pb").Length != 10 || !Enumerable.Range(0,10).Select(i=>i.ToString("D3")+".pb").Contains(args[2])) throw new ArgumentException("Only the pinned guard10 pve-auto set is accepted");
        string phase = "assembly-load";
        try
        {
            CultureInfo.DefaultThreadCurrentCulture = CultureInfo.InvariantCulture;
            CultureInfo.DefaultThreadCurrentUICulture = CultureInfo.InvariantCulture;
            Directory.SetCurrentDirectory(root);
            AssemblyLoadContext.Default.Resolving += (_, name) => {
                string path = Path.Combine(root, name.Name + ".dll");
                return File.Exists(path) ? AssemblyLoadContext.Default.LoadFromAssemblyPath(path) : null;
            };
            var game = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root, "Assembly-CSharp.dll"));
            phase = "managed-unity-logger-switch";
            // Original managed property only: no fourth Unity stub, method
            // rewrite, native sink or combat state is introduced. The host
            // keeps the original caller JSONL independent of Unity logging.
            var unity = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root, "UnityEngine.dll"));
            var logger = unity.GetType("UnityEngine.Debug", true).GetProperty("logger").GetValue(null);
            logger.GetType().GetProperty("logEnabled").SetValue(logger, false);
            Console.WriteLine("Original managed Unity logger.logEnabled=false");
            phase = "original-table-init";
            var managerType = game.GetType("Dict.DictDataManager", true);
            var instance = managerType.GetProperty("Instance").GetValue(null);
            managerType.GetMethod("Init").Invoke(instance, new object[] { "tables" });
            Console.WriteLine("Original DictDataManager.Init completed");
            phase = "original-runner-load";
            var runner = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root, "LocalOriginalCaller.dll"));
            var run = runner.GetType("NativeBatchRunner", true).GetMethod("RunCase", BindingFlags.NonPublic | BindingFlags.Static);
            foreach (string file in new[] { Path.Combine(cases,args[2]) })
            {
                phase = "original-547-run:" + Path.GetFileName(file);
                run.Invoke(null, new object[] { file, true, false });
                Console.WriteLine("Original run complete " + Path.GetFileName(file));
            }
            return 0;
        }
        catch (Exception error)
        {
            Console.Error.WriteLine("HOST_PHASE=" + phase);
            Console.Error.WriteLine(error);
            return 1;
        }
    }
}
