using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input and output Assembly-CSharp paths required");
using var module = ModuleDefMD.Load(args[0]);
var scope = module.GetAssemblyRefs().Single(x => x.Name == "SoulFlowerGGOShop");
var helper = new TypeRefUser(module, "HYJS.GGO", "TrialShopRefresh75", scope);
var type = module.GetTypes().Single(x => x.FullName == "UI.TabPointShop");
foreach (var pair in new[] { (Native: "UpdateAll", Hook: "HandleUpdateAll"), (Native: "UpdateProducts", Hook: "HandleProducts") })
{
    var method = type.Methods.Single(x => x.Name == pair.Native);
    var hook = new MemberRefUser(module, pair.Hook, MethodSig.CreateStatic(module.CorLibTypes.Boolean, type.ToTypeSig()), helper);
    var start = method.Body.Instructions[0];
    var prefix = new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, hook), Instruction.Create(OpCodes.Brfalse, start), Instruction.Create(OpCodes.Ret) };
    for (int i = 0; i < prefix.Length; i++) method.Body.Instructions.Insert(i, prefix[i]);
    method.Body.KeepOldMaxStack = true;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
