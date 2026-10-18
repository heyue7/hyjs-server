using System;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.Loader;
using System.Globalization;

internal static class Program
{
    static int Main(string[] args)
    {
        if (args.Length != 2) throw new ArgumentException("runtime directory and exact case directory required");
        string root = Path.GetFullPath(args[0]);
        string cases = Path.GetFullPath(args[1]);
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
            foreach (string file in Directory.GetFiles(cases, "*.pb").OrderBy(x => x, StringComparer.Ordinal))
            {
                phase = "original-547-run:" + Path.GetFileName(file);
                run.Invoke(null, new object[] { file, false, false });
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
