using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using System;
using System.Linq;

using var main = ModuleDefMD.Load(args[0]);
var manager = main.GetTypes().Single(t => t.FullName == "AssetLoad.AssetBundleManager");
var scope = main.GetAssemblyRefs().Single(a => a.Name == "SoulFlowerGGOShop");
var helper = new TypeRefUser(main, "HYJS.GGO", "PetPortraitAssets75", scope);
var ensure = new MemberRefUser(main, "Ensure", MethodSig.CreateStatic(main.CorLibTypes.Void, main.CorLibTypes.Object), helper);
var exist = manager.Methods.Single(m => m.MDToken.Raw == 0x0600320a);
exist.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
exist.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Call, ensure));

var load = manager.Methods.Single(m => m.MDToken.Raw == 0x060031f8);
var dependency = load.Body.Instructions.Single(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m && m.Name == "GetAllDependencies" && m.DeclaringType.FullName == "UnityEngine.AssetBundleManifest");
var original = (IMethod)dependency.Operand;
var dependencies = new MemberRefUser(main, "Dependencies", MethodSig.CreateStatic(original.MethodSig.RetType, original.DeclaringType.ToTypeSig(), main.CorLibTypes.String), helper);
dependency.OpCode = OpCodes.Call;
dependency.Operand = dependencies;

var path = manager.Methods.Single(m => m.MDToken.Raw == 0x06003206);
var tryPath = new MemberRefUser(main, "TryPath", MethodSig.CreateStatic(main.CorLibTypes.Boolean, main.CorLibTypes.String, new ByRefSig(main.CorLibTypes.Boolean), new ByRefSig(main.CorLibTypes.String)), helper);
var result = new Local(main.CorLibTypes.String);
path.Body.Variables.Add(result);
var start = path.Body.Instructions[0];
var prefix = new[] {
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2),
    Instruction.Create(OpCodes.Ldloca, result), Instruction.Create(OpCodes.Call, tryPath),
    Instruction.Create(OpCodes.Brfalse, start), Instruction.Create(OpCodes.Ldloc, result),
    Instruction.Create(OpCodes.Ret)
};
for (int index = 0; index < prefix.Length; index++) path.Body.Instructions.Insert(index, prefix[index]);
foreach (var method in new[] { exist, load, path }) method.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[1], options);
