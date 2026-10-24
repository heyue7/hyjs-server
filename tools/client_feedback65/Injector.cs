using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

using var main = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
MethodDef Method(uint token) => (MethodDef)main.ResolveToken(token);
var start = Method(0x06003bb8);
if (start.DeclaringType.FullName != "PlaybackKit" || start.Name != "Start" || start.Body.Instructions.Count != 5 || start.Body.Instructions[2].OpCode != OpCodes.Ldc_I4_1)
    throw new Exception("Playback start shape drift");
start.Body.Instructions[2].OpCode = OpCodes.Ldc_I4_0;
foreach (var spec in new[] { (Token: 0x06005e8bu, Count: 8), (Token: 0x06005e71u, Count: 7), (Token: 0x06005e66u, Count: 170) })
{
    var method = Method(spec.Token);
    if (method.Body.ExceptionHandlers.Count != 0 || method.Body.Instructions[spec.Count - 1].OpCode != OpCodes.Ret)
        throw new Exception("Guard shape drift");
    for (int i = 0; i < spec.Count; i++) method.Body.Instructions.RemoveAt(0);
}
var open = Method(0x06005284);
if (open.DeclaringType.FullName != "UI.SelectReward" || open.Name != "DoOpen" || open.Body.ExceptionHandlers.Count != 0)
    throw new Exception("Selection target drift");
var hook = helper.GetTypes().Single(t => t.FullName == "HYJS.Feedback65.Runtime").Methods.Single(m => m.Name == "ResetSelection");
if (hook.MethodSig.Params.Count != 1 || hook.MethodSig.Params[0].ElementType != ElementType.Object) throw new Exception("Hook ABI drift");
open.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
open.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Call, new Importer(main).Import(hook)));
open.Body.MaxStack = Math.Max(open.Body.MaxStack, (ushort)1);
void AddPrefix(uint token, string name, int mode, int attackType = 0)
{
    var target = Method(token);
    var original = target.Body.Instructions[0];
    if (target.Body.ExceptionHandlers.Count != 0) throw new Exception("Unexpected Wanted EH");
    var prefix = new System.Collections.Generic.List<Instruction> { Instruction.Create(OpCodes.Ldarg_0) };
    if (mode == 1 || mode == 2) prefix.Add(Instruction.Create(OpCodes.Ldarg_1));
    if (mode == 2) prefix.Add(Instruction.Create(OpCodes.Ldarg_2));
    if (mode == 3) prefix.Add(Instruction.Create(OpCodes.Ldc_I4, attackType));
    var entry = helper.GetTypes().Single(t => t.FullName == "HYJS.Feedback65.Runtime").Methods.Single(m => m.Name == name);
    prefix.Add(Instruction.Create(OpCodes.Call, new Importer(main).Import(entry)));
    if (name == "WantedResult" || name == "WantedResume" || name == "WantedStart")
    {
        prefix.Add(Instruction.Create(OpCodes.Brfalse, original));
        prefix.Add(Instruction.Create(OpCodes.Ret));
    }
    for (int i = 0; i < prefix.Count; i++) target.Body.Instructions.Insert(i, prefix[i]);
    target.Body.MaxStack = Math.Max(target.Body.MaxStack, (ushort)3);
}
AddPrefix(0x0600504e, "WantedRequest", 1);
AddPrefix(0x0600504f, "WantedStart", 2);
AddPrefix(0x06002b6b, "WantedSettlement", 0);
AddPrefix(0x06002b6d, "WantedResult", 2);
AddPrefix(0x0600504d, "WantedResume", 3, 1);
AddPrefix(0x06005050, "WantedResume", 3, 2);
foreach (var method in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody)) method.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
