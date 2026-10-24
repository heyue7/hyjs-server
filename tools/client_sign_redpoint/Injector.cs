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
foreach(var target in new[]{("UI.UIActivitySignIcon", "cb"), ("UI.UIActivityEveryDaySign", "<OnClickSuper>m__1D6")}) {
    var method = Method(target.Item1, target.Item2);
    var instructions = method.Body.Instructions;
    if(method.Body.ExceptionHandlers.Count != 0 || instructions.Count(i => i.OpCode == OpCodes.Ret) != 1 ||
       instructions[0].OpCode != OpCodes.Ldarg_2 || instructions[1].OpCode != OpCodes.Brfalse)
        throw new InvalidOperationException("Original sign callback control flow differs");
    var exit = instructions.Single(i => i.OpCode == OpCodes.Ret);
    var failure = instructions[1];
    if(failure.Operand != exit) throw new InvalidOperationException("Original failed callback exit differs");
    var query = Instruction.Create(OpCodes.Ldc_I4, 20117);
    // Both callbacks branch to ret after successful local cleanup as well as
    // on HTTP failure. Redirect only successful exits through the query.
    foreach(var instruction in instructions)
        if(instruction != failure && instruction.Operand == exit) instruction.Operand = query;
    int index = instructions.IndexOf(exit);
    instructions.Insert(index++, query);
    instructions.Insert(index++, Instruction.Create(OpCodes.Newobj, request));
    instructions.Insert(index, Instruction.Create(OpCodes.Call, send));
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)2);
    method.Body.KeepOldMaxStack = true;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Original sign callbacks query authoritative redpoints after successful local cleanup; HTTP failures unchanged");
