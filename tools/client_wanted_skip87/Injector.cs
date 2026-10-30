using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input main, helper, output main required");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
void Require(bool value, string message) { if (!value) throw new InvalidOperationException(message); }
TypeDef Type(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef Method(string owner, string name) => Type(owner).Methods.Single(m => m.Name == name);
var bridgeRef = new AssemblyRefUser(new AssemblyNameInfo("HYJS.WantedSkip87, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var bridge = new TypeRefUser(module, "HYJS.WantedSkip87", "WantedSkip", bridgeRef);
var before = new MemberRefUser(module, "BeforeSkip", MethodSig.CreateStatic(module.CorLibTypes.Void, Type("Battle.BattleManager").ToTypeSig()), bridge);
var ignore = new MemberRefUser(module, "IgnoreEndedControl", MethodSig.CreateStatic(module.CorLibTypes.Boolean, Type("Battle.BattlePacket").ToTypeSig()), bridge);
var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.WantedSkip87.WantedSkip");
foreach (var expected in new[] { before, ignore }) {
    var actual = helperType.Methods.Single(m => m.Name == expected.Name);
    Require(actual.IsPublic && actual.IsStatic && actual.MethodSig.RetType.FullName == expected.MethodSig.RetType.FullName &&
        actual.MethodSig.Params.Select(p => p.FullName).SequenceEqual(expected.MethodSig.Params.Select(p => p.FullName)), "helper ABI differs");
}
foreach (var reference in helper.GetTypeRefs().Where(t => t.FullName.StartsWith("Battle.") || t.FullName == "StaticData" || t.FullName == "BattleData"))
    Require(reference.ResolutionScope is AssemblyRef a && a.Name == module.Assembly.Name, "helper host identity differs");
var skip = Method("Battle.BattleManager", "Skip");
Require(skip.Body.ExceptionHandlers.Count == 0 && skip.Body.Instructions.Count == 7, "skip body differs");
var set = skip.Body.Instructions[2];
Require(set.OpCode == OpCodes.Stfld && set.Operand is IField f && f.FullName == "System.Boolean Battle.BattleManager::isSkip", "skip flag differs");
skip.Body.Instructions.Insert(3, Instruction.Create(OpCodes.Ldarg_0));
skip.Body.Instructions.Insert(4, Instruction.Create(OpCodes.Call, before));
var receive = Method("Battle.Communication.EngineMsgReceive", "PushMessageToEngine");
Require(receive.Body.ExceptionHandlers.Count == 0, "receiver exception regions differ");
var first = receive.Body.Instructions[0];
var prefix = new[] { Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, ignore), Instruction.Create(OpCodes.Brfalse, first), Instruction.Create(OpCodes.Ret) };
for (int i = 0; i < prefix.Length; i++) receive.Body.Instructions.Insert(i, prefix[i]);
foreach (var method in new[] { skip, receive }) { method.Body.KeepOldMaxStack = true; method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)2); }
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Added two Wanted control boundaries; original engine and result requests preserved");
