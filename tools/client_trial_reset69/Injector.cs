using System;
using System.Linq;
using System.IO;
using System.Security.Cryptography;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if (args.Length != 4) throw new ArgumentException("Main, helper, output and pinned input SHA required");
byte[] raw = File.ReadAllBytes(args[0]);
if (Convert.ToHexString(SHA256.HashData(raw)).ToLowerInvariant() != args[3]) throw new Exception("Input Main changed");
using var main = ModuleDefMD.Load(raw);
using var helper = ModuleDefMD.Load(args[1]);
var target = main.GetTypes().Single(t => t.FullName == "UI.RushPage").Methods.Single(m => m.Name == "Refresh");
if (target.Body.ExceptionHandlers.Count != 0 || target.MethodSig.Params.Count != 1 || target.MethodSig.Params[0].FullName != "ProtoBuf.Message.SCGetRushInfo") throw new Exception("Refresh ABI changed");
var hooks = helper.GetTypes().Single(t => t.FullName == "HYJS.TrialReset69.Controls");
var source = hooks.Methods.Single(m => m.Name == "AfterRefresh");
if (!source.IsStatic || source.MethodSig.Params.Count != 2 || source.MethodSig.Params.Any(p => p.FullName != "System.Object")) throw new Exception("Hook ABI changed");
var hook = new MemberRefUser(main, source.Name, MethodSig.CreateStatic(main.CorLibTypes.Void, main.CorLibTypes.Object, main.CorLibTypes.Object), new Importer(main).Import(hooks));
var exits = target.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray();
if (exits.Length != 1) throw new Exception("Unexpected Refresh exits");
var hookStart = Instruction.Create(OpCodes.Ldarg_0);
// Original conditional exits must pass the post-refresh hook as well.
foreach (var instruction in target.Body.Instructions.ToArray())
{
    if (ReferenceEquals(instruction.Operand, exits[0])) instruction.Operand = hookStart;
    else if (instruction.Operand is Instruction[] targets)
        for (int i = 0; i < targets.Length; i++) if (ReferenceEquals(targets[i], exits[0])) targets[i] = hookStart;
}
int at = target.Body.Instructions.IndexOf(exits[0]);
target.Body.Instructions.Insert(at++, hookStart);
target.Body.Instructions.Insert(at++, Instruction.Create(OpCodes.Ldarg_1));
target.Body.Instructions.Insert(at, Instruction.Create(OpCodes.Call, hook));
foreach (var m in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody)) m.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
Console.WriteLine("Original RushPage.Refresh retained with one post-refresh native-button hook");
