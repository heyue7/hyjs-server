using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input and output main DLL required");
using var module = ModuleDefMD.Load(args[0]);
int repaired = 0;
foreach (var type in module.GetTypeRefs())
{
    if (type.ResolutionScope is not ModuleRef scope || scope.Name != "HYJS.NewbieSign.dll") continue;
    if (!module.GetTypes().Any(t => t.FullName == type.FullName))
        throw new InvalidOperationException("Unexpected external helper type: " + type.FullName);
    // These types are defined in the main module. A helper module scope prevents old Mono from JIT compiling the caller.
    type.ResolutionScope = module;
    repaired++;
}
if (repaired == 0) throw new InvalidOperationException("Expected affected self references");
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Repaired original main type scopes: " + repaired);
