using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input managed, output managed");
using var module = ModuleDefMD.Load(args[0]);
var owner = module.GetTypes().Single(t => t.FullName == "UI.ShopVipItem");
var method = owner.Methods.Single(m => m.Name == "OnIconClick");
if (method.Body.Instructions.Any(i => i.Operand is IMethod m && m.DeclaringType.FullName == "HYJS.VipShop.VipGiftPreview"))
    throw new InvalidOperationException("Preview hook already exists");
var field = owner.Fields.Single(f => f.Name == "shopItemInfo");
var reference = module.GetAssemblyRefs().Single(r => r.Name == "SoulFlowerVipEmpty");
var helper = new TypeRefUser(module, "HYJS.VipShop", "VipGiftPreview", reference);
var call = new MemberRefUser(module, "TryOpen", MethodSig.CreateStatic(module.CorLibTypes.Boolean, field.FieldType), helper);
var original = method.Body.Instructions[0];
var prefix = new[] {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, field),
    Instruction.Create(OpCodes.Call, call), Instruction.Create(OpCodes.Brfalse, original),
    Instruction.Create(OpCodes.Ret)
};
for (int i = 0; i < prefix.Length; i++) method.Body.Instructions.Insert(i, prefix[i]);
method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)8);
method.Body.KeepOldMaxStack = true;
method.Body.SimplifyBranches(); method.Body.OptimizeBranches();
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
