using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input and output assembly required");
using var module = ModuleDefMD.Load(args[0]);
var callback = module.ResolveToken(0x06006c61) as MethodDef
    ?? throw new InvalidOperationException("copy callback missing");
if (callback.DeclaringType.Name != "<CopyToBattleScene>c__AnonStorey163" || callback.Name != "<>m__37" ||
    !callback.HasBody || callback.Body.ExceptionHandlers.Count != 0 || callback.Body.Variables.Count != 2 ||
    callback.MethodSig.Params.Count != 2 || callback.MethodSig.Params[1].ElementType != ElementType.Boolean)
    throw new InvalidOperationException("copy callback identity, ABI, locals or EH differs");

var packetBody = module.ResolveToken(0x040037ef) as FieldDef
    ?? throw new InvalidOperationException("packet body field missing");
var errorType = module.ResolveToken(0x020000dd) as TypeDef
    ?? throw new InvalidOperationException("error message type missing");
var errorCode = module.ResolveToken(0x06000b82) as MethodDef
    ?? throw new InvalidOperationException("error code getter missing");
var tryParse = module.ResolveToken(0x0a0007e1) as MemberRef
    ?? throw new InvalidOperationException("integer parser reference missing");
if (packetBody.DeclaringType.FullName != "NetWork.Packets.Packet" || packetBody.Name != "kBody" ||
    errorType.FullName != "ProtoBuf.Message.ErrorMessage" || errorCode.DeclaringType != errorType ||
    errorCode.Name != "get_code" || tryParse.DeclaringType.FullName != "System.Int32" || tryParse.Name != "TryParse" ||
    tryParse.MethodSig.Params.Count != 2 || tryParse.MethodSig.Params[0].ElementType != ElementType.String ||
    tryParse.MethodSig.Params[1].ElementType != ElementType.ByRef)
    throw new InvalidOperationException("packet/error/parser binding differs");

var original = callback.Body.Instructions.Single(i => i.OpCode == OpCodes.Ldc_I4 && (int)i.Operand == 1047);
var originalIndex = callback.Body.Instructions.IndexOf(original);
if (originalIndex == 0 || callback.Body.Instructions[originalIndex - 1].OpCode != OpCodes.Callvirt ||
    ((IMethod)callback.Body.Instructions[originalIndex - 1].Operand).Name != "HideMask" ||
    callback.Body.Instructions[^1].OpCode != OpCodes.Ret)
    throw new InvalidOperationException("original cleanup/relogin branch differs");

// HTTPManager.Tick has already shown ordinary ErrorMessage text before invoking
// this callback. Keep code 21 and unknown/non-error/transport paths on the
// original relogin branch; never suppress a malformed or unrecognized code.
var code = new Local(module.CorLibTypes.Int32);
callback.Body.Variables.Add(code);
var originalReturn = callback.Body.Instructions[^1];
var notError = Instruction.Create(OpCodes.Pop);
var guard = new List<Instruction> {
    Instruction.Create(OpCodes.Ldarg_1),
    Instruction.Create(OpCodes.Brfalse, original),
    Instruction.Create(OpCodes.Ldarg_1),
    Instruction.Create(OpCodes.Ldfld, packetBody),
    Instruction.Create(OpCodes.Isinst, errorType),
    Instruction.Create(OpCodes.Dup),
    Instruction.Create(OpCodes.Brfalse, notError),
    Instruction.Create(OpCodes.Callvirt, errorCode),
    Instruction.Create(OpCodes.Ldloca_S, code),
    Instruction.Create(OpCodes.Call, tryParse),
    Instruction.Create(OpCodes.Brfalse, original),
    Instruction.Create(OpCodes.Ldloc_S, code),
    Instruction.Create(OpCodes.Ldc_I4_S, (sbyte)21),
    Instruction.Create(OpCodes.Beq, original),
    Instruction.Create(OpCodes.Br, originalReturn),
    notError,
};
for (var i = 0; i < guard.Count; i++) callback.Body.Instructions.Insert(originalIndex + i, guard[i]);
callback.Body.MaxStack = 2;
callback.Body.KeepOldMaxStack = true;
callback.Body.UpdateInstructionOffsets();
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Inserted error-message-only copy failure guard");
