using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length != 2) throw new ArgumentException("input and output managed DLL required");
using var module = ModuleDefMD.Load(args[0]);
MethodDef Method(string owner, string name) => module.GetTypes().Single(t => t.FullName == owner).Methods.Single(m => m.Name == name);
var request = Method("ProtoBuf.Message.CSRedPoint", ".ctor");
var send = Method("NetAPI", "SendSocket");
if(send.MethodSig.Params.Count != 2 || send.MethodSig.RetType.ElementType != ElementType.Void)
    throw new InvalidOperationException("Original one-way redpoint socket ABI differs");
foreach(string name in new[]{"GetAllCallback", "RemoveAndUpdate"}) {
    var method = Method("UI.RewardCenterPage", name);
    var instructions = method.Body.Instructions;
    if(method.Body.ExceptionHandlers.Count != 0 || instructions.Count(i => i.OpCode == OpCodes.Ret) != 1)
        throw new InvalidOperationException("Original reward callback control flow differs");
    var exit = instructions.Single(i => i.OpCode == OpCodes.Ret);
    // Preserve the original return instruction as every rejected callback's
    // branch destination. Successful fallthrough queries after local clearing.
    int index = instructions.IndexOf(exit);
    instructions.Insert(index++, Instruction.Create(OpCodes.Ldc_I4, 20117));
    instructions.Insert(index++, Instruction.Create(OpCodes.Newobj, request));
    instructions.Insert(index, Instruction.Create(OpCodes.Call, send));
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)2);
    method.Body.KeepOldMaxStack = true;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Original reward callbacks query authoritative redpoints after local cleanup; failures unchanged");
