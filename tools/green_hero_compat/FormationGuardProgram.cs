using System;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.Loader;
using System.Runtime.Serialization;
using dnlib.DotNet;
using dnlib.DotNet.Writer;

if (args.Length == 4 && args[0] == "--probe")
{
    string assemblyPath = Path.GetFullPath(args[1]);
    string references = Path.GetFullPath(args[2]);
    AssemblyLoadContext.Default.Resolving += (_, name) => {
        string path = Path.Combine(references, name.Name + ".dll");
        return File.Exists(path) ? AssemblyLoadContext.Default.LoadFromAssemblyPath(path) : null;
    };
    var game = AssemblyLoadContext.Default.LoadFromAssemblyPath(assemblyPath);
    var type = game.GetType("Battle.View.BattleViewCtrl", true);
    var flags = BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance;
    var can = type.GetMethod("IsCanTransfer", flags);
    var end = type.GetMethod("AskTransEnd", flags);
    var report = new System.Collections.Generic.List<string>();
    foreach (int round in new[] {0,1,2,8})
    foreach (bool fought in new[] {false,true})
    foreach (bool transferred in new[] {false,true})
    {
        var value = FormatterServices.GetUninitializedObject(type);
        type.GetField("roundNum",flags).SetValue(value,round);
        type.GetField("hasRoundFight",flags).SetValue(value,fought);
        type.GetField("isFormationTransfered",flags).SetValue(value,transferred);
        bool actual = (bool)can.Invoke(value,null);
        bool expected = round <= 1 && !fought && !transferred;
        if (actual != expected) throw new InvalidOperationException("Formation entry truth table differs");
        // Rejected submits return before inspecting actors or sending any packet.
        // The uninitialized object deliberately has no scene/engine/actor state.
        if (round > 1 || fought) end.Invoke(value,null);
        report.Add("round="+round+",fought="+fought+",transferred="+transferred+",allowed="+actual);
    }
    File.WriteAllLines(args[3],report);
    Console.WriteLine("PASS sixteen actual managed guard states; twelve rejected submits return without scene or Engine");
    return;
}
if (args.Length != 2) throw new ArgumentException("input/output DLL required");
using var module = ModuleDefMD.Load(args[0]);
FormationGuard.Apply(module);
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("Added first-round-only formation entry and submit guards; Engine unchanged");
