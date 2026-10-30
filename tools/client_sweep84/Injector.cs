using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input and output managed paths required");
using var module = ModuleDefMD.Load(args[0]);
void Require(bool value, string message) { if (!value) throw new InvalidOperationException(message); }
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(TypeDef owner, string name) => owner.Methods.Single(m => m.Name == name);
bool Called(Instruction i, Code op, string owner, string name) => i.OpCode.Code == op && i.Operand is IMethod m && m.DeclaringType.FullName == owner && m.Name == name;
bool Field(Instruction i, string owner, string name) => i.OpCode.Code == Code.Ldfld && i.Operand is IField f && f.DeclaringType.FullName == owner && f.Name == name;
bool Float(Instruction i, float f) => i.OpCode.Code == Code.Ldc_R4 && i.Operand is float x && x == f;

var pvp = T("UI.PVPSweepPage");
var pvpIterator = module.GetTypes().Single(t => t.Name == "<SweepNext>c__IteratorF4" && t.DeclaringType == pvp);
var pvpMove = M(pvpIterator, "MoveNext");
var pvpCode = pvpMove.Body.Instructions;
var pvpWait = pvpCode.Single(i => Called(i, Code.Newobj, "UnityEngine.WaitForSeconds", ".ctor"));
int pvpWaitIndex = pvpCode.IndexOf(pvpWait);
Require(pvpWaitIndex >= 1 && Float(pvpCode[pvpWaitIndex - 1], .5f), "arena wait needle differs");
pvpCode[pvpWaitIndex - 1].Operand = .05f;

var copy = T("UI.UICopySweep");
var copyIterator = module.GetTypes().Single(t => t.Name == "<_StartSweep>c__Iterator141" && t.DeclaringType == copy);
var copyMove = M(copyIterator, "MoveNext");
var code = copyMove.Body.Instructions;
var wait = code.Single(i => Called(i, Code.Newobj, "UnityEngine.WaitForSeconds", ".ctor"));
int n = code.IndexOf(wait);
Require(n >= 9 && code[n - 9].OpCode == OpCodes.Ldarg_0 && Field(code[n - 8], copyIterator.FullName, "<>f__this") &&
        Field(code[n - 7], copy.FullName, "titleAlpha") && Field(code[n - 6], "UITweener", "duration") &&
        code[n - 5].OpCode == OpCodes.Ldarg_0 && Field(code[n - 4], copyIterator.FullName, "<>f__this") &&
        Field(code[n - 3], copy.FullName, "bodyScale") && Field(code[n - 2], "UITweener", "duration") &&
        code[n - 1].OpCode == OpCodes.Add, "copy wait expression differs");
// Preserve every original expression instruction. Change only the value
// supplied to the existing WaitForSeconds constructor.
Require(!code.Any(i => i.Operand == wait || (i.Operand is Instruction[] targets && targets.Contains(wait))), "branch to constructor bypasses inserted value");
code.Insert(n, Instruction.Create(OpCodes.Pop));
code.Insert(n + 1, Instruction.Create(OpCodes.Ldc_R4, .05f));
Require(code.Any(i => Called(i, Code.Call, "HYJS.Stamina.StaminaSweep", "CanContinue")), "copy resume guard missing");
copyMove.Body.KeepOldMaxStack = true;

var create = M(pvp, "CreateItem");
var pvpScroll = create.Body.Instructions.Single(i => Called(i, Code.Call, "TweenPosition", "Begin"));
int p = create.Body.Instructions.IndexOf(pvpScroll);
Require(p >= 2 && create.Body.Instructions[p - 1].OpCode == OpCodes.Ldloc_1 && Float(create.Body.Instructions[p - 2], .2f), "arena scroll needle differs");
create.Body.Instructions[p - 2].Operand = 0f;

var refresh = M(copy, "Refresh");
var copyScroll = refresh.Body.Instructions.Single(i => Called(i, Code.Call, copy.FullName, "MoveUpPosition"));
int q = refresh.Body.Instructions.IndexOf(copyScroll);
Require(q >= 2 && refresh.Body.Instructions[q - 2].OpCode == OpCodes.Ldarg_0 && Float(refresh.Body.Instructions[q - 1], .2f), "copy scroll needle differs");
refresh.Body.Instructions[q - 1].Operand = 0f;

var opts = new ModuleWriterOptions(module);
opts.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], opts);
Console.WriteLine("Patched four semantic needles; original wait expression and guards retained");
