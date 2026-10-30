using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input main, helper, output main required");
using var module = ModuleDefMD.Load(args[0]);
using var helperModule = ModuleDefMD.Load(args[1]);
void Require(bool condition, string message) { if (!condition) throw new InvalidOperationException(message); }
TypeDef Type(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef Method(TypeDef owner, string name) => owner.Methods.Single(m => m.Name == name);
FieldDef Field(TypeDef owner, string name) => owner.Fields.Single(f => f.Name == name);
bool Called(Instruction ins, string owner, string name) => ins.Operand is IMethod method &&
    method.DeclaringType.FullName == owner && method.Name == name;
bool FieldOperand(Instruction ins, string owner, string name) => ins.Operand is IField field &&
    field.DeclaringType.FullName == owner && field.Name == name;
bool Incoming(CilBody body, Instruction target) => body.Instructions.Any(i => i.Operand == target ||
    (i.Operand is Instruction[] targets && targets.Contains(target)));

var reference = new AssemblyRefUser(new AssemblyNameInfo("HYJS.FragmentBoxes85, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var bridge = new TypeRefUser(module, "HYJS.FragmentBoxes85", "FragmentBoxes", reference);
var page = Type("UI.Page");
var detail = Type("UI.BagMemberDetail");
var choice = Type("UI.BagChoseReward");
var isEligible = new MemberRefUser(module, "IsEligible", MethodSig.CreateStatic(module.CorLibTypes.Boolean, module.CorLibTypes.String), bridge);
var limit = new MemberRefUser(module, "Limit", MethodSig.CreateStatic(module.CorLibTypes.Int64, module.CorLibTypes.Int64), bridge);
var appendCount = new MemberRefUser(module, "AppendCount", MethodSig.CreateStatic(module.CorLibTypes.String, module.CorLibTypes.String, module.CorLibTypes.Int64, module.CorLibTypes.String), bridge);
var requestedCount = new MemberRefUser(module, "RequestedCount", MethodSig.CreateStatic(module.CorLibTypes.Int32, page.ToTypeSig(), module.CorLibTypes.String), bridge);

// C# compiled helper must reference the actual input Assembly-CSharp types.
var helper = helperModule.GetTypes().Single(t => t.FullName == "HYJS.FragmentBoxes85.FragmentBoxes");
foreach (var pair in new[] { ("IsEligible", isEligible), ("Limit", limit), ("AppendCount", appendCount), ("RequestedCount", requestedCount) })
{
    var actual = helper.Methods.Single(m => m.Name == pair.Item1);
    Require(actual.IsPublic && actual.IsStatic && actual.MethodSig.RetType.FullName == pair.Item2.MethodSig.RetType.FullName &&
        actual.MethodSig.Params.Select(p => p.FullName).SequenceEqual(pair.Item2.MethodSig.Params.Select(p => p.FullName)),
        "Helper ABI mismatch: " + pair.Item1);
}
foreach (var type in helperModule.GetTypeRefs().Where(t => t.FullName == "UI.Page" || t.FullName.StartsWith("Dict.")))
    Require(type.ResolutionScope is AssemblyRef scope && scope.Name == module.Assembly.Name,
        "Helper host type is not bound to Assembly-CSharp: " + type.FullName);
Require(helperModule.GetTypeRefs().Any(t => t.FullName == "UI.Page"), "Helper page ABI absent");

var set = Method(detail, "SetItem");
Require(set.Body.ExceptionHandlers.Count == 0 && set.Body.Instructions.Count(i => i.OpCode == OpCodes.Ret) == 1,
    "Unexpected SetItem control flow");
var ending = set.Body.Instructions.Last();
Require(ending.OpCode == OpCodes.Ret, "SetItem return differs");
var finalReturn = Instruction.Create(OpCodes.Ret);
ending.OpCode = OpCodes.Ldarg_1;
ending.Operand = null;
foreach (var instruction in new[] {
    Instruction.Create(OpCodes.Call, isEligible), Instruction.Create(OpCodes.Brfalse, finalReturn),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldc_I4_1),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Ldc_I4_0),
    Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Ldc_I4_1),
    Instruction.Create(OpCodes.Ldstr, "UI_beibao_word_shiyong"), Instruction.Create(OpCodes.Ldc_I4_1),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Call, Method(detail, "UpdateDetailParts")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_0),
    Instruction.Create(OpCodes.Ldfld, Field(detail, "totalHaveNum")), Instruction.Create(OpCodes.Call, limit),
    Instruction.Create(OpCodes.Stfld, Field(detail, "totalHaveNum")), finalReturn
}) set.Body.Instructions.Add(instruction);
set.Body.MaxStack = Math.Max(set.Body.MaxStack, (ushort)9);
set.Body.KeepOldMaxStack = true;

var open = Method(detail, "OnPublicButtonClick");
var opens = open.Body.Instructions.Where((i, n) => Called(i, "UI.PageManager", "OpenPage") && n >= 7 &&
    open.Body.Instructions[n - 7].OpCode == OpCodes.Ldstr && (string)open.Body.Instructions[n - 7].Operand == "BagChoseReward").ToArray();
Require(opens.Length == 1, "Original BagChoseReward open differs");
var callOpen = opens[0];
int openIndex = open.Body.Instructions.IndexOf(callOpen);
Require(!Incoming(open.Body, callOpen) && Called(open.Body.Instructions[openIndex - 1], "System.String", "Concat") &&
    FieldOperand(open.Body.Instructions[openIndex - 2], "Dict.DataModel.TableItem", "itemId"), "Choice page options needle differs");
var item = Type("UI.UISimpleItem");
var tableItem = Type("Dict.DataModel.TableItem");
var additions = new[] {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, Field(detail, "curSelectNum")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, Field(detail, "m_item")),
    Instruction.Create(OpCodes.Ldfld, Field(item, "itemData")), Instruction.Create(OpCodes.Ldfld, Field(tableItem, "itemId")),
    Instruction.Create(OpCodes.Call, appendCount)
};
foreach (var instruction in additions) open.Body.Instructions.Insert(openIndex++, instruction);
open.Body.MaxStack = Math.Max(open.Body.MaxStack, (ushort)6);
open.Body.KeepOldMaxStack = true;

var confirm = Method(choice, "OnConfirmBtn");
var setters = confirm.Body.Instructions.Where(i => Called(i, "ProtoBuf.Message.BaseResource", "set_num")).ToArray();
Require(setters.Length == 1, "Choice count setter differs");
var setter = setters[0];
int n = confirm.Body.Instructions.IndexOf(setter);
var literal = confirm.Body.Instructions[n - 1];
Require(literal.OpCode == OpCodes.Ldc_I4_1 && !Incoming(confirm.Body, literal) &&
    confirm.Body.Instructions[n - 2].OpCode == OpCodes.Ldloc_0, "Original count literal differs");
literal.OpCode = OpCodes.Ldarg_0;
literal.Operand = null;
confirm.Body.Instructions.Insert(n++, Instruction.Create(OpCodes.Ldarg_0));
confirm.Body.Instructions.Insert(n++, Instruction.Create(OpCodes.Ldfld, Field(choice, "itemId")));
confirm.Body.Instructions.Insert(n, Instruction.Create(OpCodes.Call, requestedCount));
confirm.Body.MaxStack = Math.Max(confirm.Body.MaxStack, (ushort)4);
confirm.Body.KeepOldMaxStack = true;

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Patched the original quantity view, page options and count source; callback unchanged");
