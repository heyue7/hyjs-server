using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input managed, output managed");
using var module = ModuleDefMD.Load(args[0]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string type, string method) => T(type).Methods.Single(m => m.Name == method);
var helperAssembly = module.GetAssemblyRefs().Single(a => a.Name == "SoulFlowerKingRecruit");
var helper = new TypeRefUser(module, "HYJS.KingRecruit", "ShopWorldMenuControl", helperAssembly);
var page = T("UI.ShopPage");
var ensure = new MemberRefUser(module, "Ensure", MethodSig.CreateStatic(module.CorLibTypes.Void, page.ToTypeSig()), helper);

// The batch-26 king hooks stay the only owners of the king page lifecycle. This
// overlay adds one shop-side entry point and never rewrites those bodies.
var existing = new[] {
  "UI.TabKingsRecruit::UpdateAll", "UI.TabKingsRecruit::OnMsgCallback",
  "UI.TabKingsRecruit::RecruitCallback", "UI.TabKingsRecruit::OnRecruitOnce",
  "UI.TabKingsRecruit::OnRecruitTen", "UI.TabKingsRecruit::OnRewardBtn",
  "UI.TabKingsRecruit::OnPreviewBtn", "UI.TabKingsRecruit::OnBattlePreviewBtn",
  "UI.RecruitResultCardItem::SetHeroBuyId" };
foreach (var name in existing) {
  var method = M(name.Substring(0, name.IndexOf("::")), name.Substring(name.IndexOf("::") + 2));
  var called = method.Body.Instructions.Any(i => i.Operand is IMethod m && m.DeclaringType.FullName == "HYJS.KingRecruit.KingRecruitWorldUI");
  if (!called) throw new InvalidOperationException("Compose the batch-26 king overlay first, missing hook: " + name);
}

var update = M("UI.ShopPage", "UpdateTabBtns");
if (update.Body.Instructions.Any(i => i.Operand is IMethod m && m.Name == "Ensure" && m.DeclaringType.Name == "ShopWorldMenuControl"))
  throw new InvalidOperationException("ShopWorldMenu hook already present");
var returns = update.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray();
if (returns.Length != 1) throw new InvalidOperationException("UpdateTabBtns ret count differs");

// Append after the original button list is complete; original branches and
// exception boundaries are untouched because nothing is inserted before the ret.
var ret = returns[0];
ret.OpCode = OpCodes.Ldarg_0;
ret.Operand = null;
update.Body.Instructions.Add(Instruction.Create(OpCodes.Call, ensure));
update.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
update.Body.MaxStack = Math.Max(update.Body.MaxStack, (ushort)8);
update.Body.KeepOldMaxStack = true;

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("One original shop tab body extended with the three king world entries");
