// Adds two hooks to the original type12 field-boss settlement path.
//
//   1. After NetAPI.SendSocket(30085, request) in
//      Battle.FieldPVEBossBattleOver.AskFieldPVEFightResult, call
//      LocalFieldSettlement.Observe(owner, request) so the exact request that
//      was just sent can be frozen and re-sent with a bounded wait.
//   2. At the first instruction of
//      Battle.FieldPVEBossBattleOver.FieldPVEBossFightResultHandler, call
//      LocalFieldSettlement.Completed(owner, response) so a late or retried
//      success stops the wait before the original handler runs.
//
// The original SendSocket, handler registration, handler body, callback and
// End method stay untouched. No other method is edited.
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input, bridge, output required");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
var type = module.GetTypes().Single(t => t.FullName == "Battle.FieldPVEBossBattleOver");
var impl = bridge.GetTypes().Single(t => t.Name == "LocalFieldSettlement");
if (module.GetTypes().Any(t => t.FullName == "LocalFieldSettlement"))
    throw new InvalidOperationException("Local field settlement helper already exists in the input module");

var observe = impl.Methods.Single(m => m.Name == "Observe");
var completed = impl.Methods.Single(m => m.Name == "Completed");
foreach (var method in new[] { observe, completed })
{
    if (!method.IsStatic || method.MethodSig.Params.Count != 2 || method.MethodSig.RetType.ElementType != ElementType.Void ||
        method.MethodSig.Params[0].ElementType != ElementType.Object || method.MethodSig.Params[1].ElementType != ElementType.Object)
        throw new InvalidOperationException("Helper hook signature changed");
}
if (impl.Methods.Count(m => m.Name == "Observe") != 1 || impl.Methods.Count(m => m.Name == "Completed") != 1)
    throw new InvalidOperationException("Helper hook shape changed");

var ask = type.Methods.Single(m => m.Name == "AskFieldPVEFightResult");
if (ask.Body == null || ask.MethodSig.Params.Count != 0 || ask.MethodSig.RetType.ElementType != ElementType.Void)
    throw new InvalidOperationException("Original type12 AskFieldPVEFightResult shape changed");
if (ask.Body.Variables.Count < 1 || ask.Body.Variables[0].Type.FullName != "ProtoBuf.Message.CSSidePveBossFightEnd")
    throw new InvalidOperationException("Original type12 request local changed");
var instructions = ask.Body.Instructions;
var send = -1;
for (int i = 0; i < instructions.Count; i++)
{
    if (instructions[i].Operand is IMethod called && called.Name == "SendSocket" &&
        called.DeclaringType.FullName == "NetAPI")
        send = i;
}
if (send < 3 || send + 2 != instructions.Count)
    throw new InvalidOperationException("Original type12 SendSocket site changed");
if (instructions[send - 2].OpCode.Code != Code.Ldc_I4 || (int)instructions[send - 2].Operand != 30085 ||
    instructions[send - 1].OpCode.Code != Code.Ldloc_0 || instructions[send + 1].OpCode.Code != Code.Ret)
    throw new InvalidOperationException("Original type12 SendSocket request/opcode shape changed");

var handler = type.Methods.Single(m => m.Name == "FieldPVEBossFightResultHandler");
if (handler.Body == null || handler.MethodSig.Params.Count != 1 ||
    handler.MethodSig.Params[0].ElementType != ElementType.Object ||
    handler.MethodSig.RetType.ElementType != ElementType.Void || handler.Body.ExceptionHandlers.Count != 0)
    throw new InvalidOperationException("Original type12 result handler shape changed");
if (instructions.Concat(handler.Body.Instructions).Any(i =>
        i.Operand is IMethod m && m.DeclaringType.FullName == "LocalFieldSettlement"))
    throw new InvalidOperationException("Local field settlement hooks were already injected");

// Keep the original request local and owner as the hook arguments. The hook
// runs after the original send, so the original registration is still in place.
instructions.Insert(send + 1, Instruction.Create(OpCodes.Ldarg_0));
instructions.Insert(send + 2, Instruction.Create(OpCodes.Ldloc_0));
instructions.Insert(send + 3, Instruction.Create(OpCodes.Call, module.Import(observe)));
ask.Body.KeepOldMaxStack = false;

handler.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
handler.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Ldarg_1));
handler.Body.Instructions.Insert(2, Instruction.Create(OpCodes.Call, module.Import(completed)));
handler.Body.KeepOldMaxStack = false;

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Local type12 field boss settlement hooks injected; original send/handler/callback preserved");
