using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input managed DLL, output managed DLL required");
using var module = ModuleDefMD.Load(args[0]);
var quality = module.GetTypes().Single(t => t.FullName == "UI.QualityBar");
var refresh = quality.Methods.Single(m => m.Name == "Refresh");
var attach = module.GetMemberRefs().Single(m =>
    m.DeclaringType.FullName == "HYJS.Display.DisplaySettings" && m.Name == "Attach");
if (refresh.Body.ExceptionHandlers.Count != 0 || refresh.Body.Instructions.Count(i => i.OpCode == OpCodes.Ret) != 1)
    throw new Exception("Review original quality Refresh control flow");
if (refresh.Body.Instructions.Any(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m &&
    m.DeclaringType.FullName == "HYJS.Display.DisplaySettings" && m.Name == "Attach"))
    throw new Exception("Quality Refresh is already repaired");
var end = refresh.Body.Instructions.Last();
if (end.OpCode != OpCodes.Ret) throw new Exception("Expected original terminal return");
// Existing switch/branch operands refer to this same instruction object. Keep
// that join point and turn it into the common post-refresh initialization path.
end.OpCode = OpCodes.Ldarg_0;
refresh.Body.Instructions.Add(Instruction.Create(OpCodes.Call, attach));
refresh.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Original quality Refresh branches retained; Attach runs at the real player-info open entry");
