using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input helper output");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
var copy = module.GetTypes().Single(t => t.FullName == "UI.UICopy");
var sweep = module.GetTypes().Single(t => t.FullName == "UI.UICopySweep");
var originalCheck = copy.Methods.Single(m => m.Name == "_CheckLuxuryEquipmentActivity");
var levelCheck = copy.Methods.Single(m => m.Name == "_CheckLevelUp");
var sweepClose = sweep.Methods.Single(m => m.Name == "DoClose");
var entry = helper.GetTypes().Single(t => t.FullName == "HYJS.LuxuryDragon.Runtime").Methods.Single(m => m.Name == "Schedule");
if (!originalCheck.HasBody || originalCheck.IsStatic || originalCheck.MethodSig.Params.Count != 0 ||
    originalCheck.MethodSig.RetType.ElementType != ElementType.Void || !levelCheck.HasBody || levelCheck.IsStatic ||
    levelCheck.MethodSig.Params.Count != 0 || levelCheck.MethodSig.RetType.ElementType != ElementType.Void || !sweepClose.HasBody || sweepClose.IsStatic ||
    sweepClose.MethodSig.Params.Count != 0 || sweepClose.MethodSig.RetType.ElementType != ElementType.Void ||
    !entry.IsStatic || entry.MethodSig.Params.Count != 0 || entry.MethodSig.RetType.ElementType != ElementType.Void)
    throw new InvalidOperationException("Unexpected original UI or helper signature");
var call = new Importer(module, ImporterOptions.TryToUseTypeDefs).Import(entry);
originalCheck.Body = new CilBody();
originalCheck.Body.Instructions.Add(Instruction.Create(OpCodes.Call, call));
originalCheck.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
originalCheck.Body.MaxStack = 1;
foreach (var method in new[] { levelCheck, sweepClose }) {
    var returns = method.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray();
    if (returns.Length != 1 || method.Body.Instructions.Last() != returns[0] ||
        method.Body.ExceptionHandlers.Any(h => h.TryEnd == null || h.HandlerEnd == null ||
            method.Body.Instructions.IndexOf(h.TryEnd) > method.Body.Instructions.IndexOf(returns[0]) ||
            method.Body.Instructions.IndexOf(h.HandlerEnd) > method.Body.Instructions.IndexOf(returns[0])))
        throw new InvalidOperationException("Unexpected original control flow: " + method.FullName);
    returns[0].OpCode = OpCodes.Call;
    returns[0].Operand = call;
    method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
    method.Body.UpdateInstructionOffsets();
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
