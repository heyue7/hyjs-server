using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input managed, helper, output managed");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string type, string method) => T(type).Methods.Single(m => m.Name == method);
var importer = new Importer(module);
var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.GGO.GGOShop");
IMethod H(string name) => importer.Import(helperType.Methods.Single(m => m.Name == name));
void Check(MethodDef method) {
 if (method.Body.Instructions.Any(i => i.Operand is IMethod m && m.DeclaringType.FullName == "HYJS.GGO.GGOShop")) throw new InvalidOperationException("already hooked " + method.FullName);
 method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)8); method.Body.KeepOldMaxStack = true;
}
var update = M("UI.ShopPage", "UpdateTabBtns"); Check(update);
var ret = update.Body.Instructions.Last();
if (ret.OpCode != OpCodes.Ret || update.Body.Instructions.Count(i => i.OpCode == OpCodes.Ret) != 1) throw new Exception("Unexpected UpdateTabBtns returns");
ret.OpCode = OpCodes.Ldarg_0; ret.Operand = null;
update.Body.Instructions.Add(Instruction.Create(OpCodes.Call, H("Ensure")));
update.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var select = M("UI.ShopPage", "OnTabBtn"); Check(select);
var first = select.Body.Instructions[0];
var prefix = new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call,H("TrySelect")), Instruction.Create(OpCodes.Brfalse,first), Instruction.Create(OpCodes.Ret) };
for (int n = 0; n < prefix.Length; n++) select.Body.Instructions.Insert(n,prefix[n]);
var inventory = M("UI.UIPlayerData", "AddSimpleItem"); Check(inventory);
var items = new[] { Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call,H("InventoryItems")), Instruction.Create(OpCodes.Starg, inventory.Parameters[1]) };
for (int n = 0; n < items.Length; n++) inventory.Body.Instructions.Insert(n,items[n]);
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
Console.WriteLine("Three scoped shop/inventory hooks; original bodies and exception regions retained");
