using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("current managed, output managed");
using var module = ModuleDefMD.Load(args[0]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string owner, string name) => T(owner).Methods.Single(m => m.Name == name);
void Require(bool value, string message) { if (!value) throw new InvalidOperationException(message); }

var buy = T("UI.BuyItemPage");
var sweep = T("UI.UICopySweep");
var sweepEvent = T("UI.UICopySweepEvent");
var iterator = module.GetTypes().Single(t => t.Name == "<_StartSweep>c__Iterator141" && t.DeclaringType == sweep);
var anon = module.GetTypes().Single(t => t.Name == "<OnSweepCopyAuto>c__AnonStorey19B" && t.DeclaringType == sweep);
var iteratorThis = iterator.Fields.Single(f => f.Name == "<>f__this");
var anonThis = anon.Fields.Single(f => f.Name == "<>f__this");

var helperAssembly = module.GetAssemblyRefs().FirstOrDefault(a => a.Name == "SoulFlowerStaminaSweep");
if (helperAssembly == null)
{
    helperAssembly = new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerStaminaSweep, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
}
var shop = new TypeRefUser(module, "HYJS.Stamina", "StaminaShop", helperAssembly);
var automaton = new TypeRefUser(module, "HYJS.Stamina", "StaminaSweep", helperAssembly);
MethodSig Void(TypeSig one) => MethodSig.CreateStatic(module.CorLibTypes.Void, one);
var isStamina = new MemberRefUser(module, "IsStamina", MethodSig.CreateStatic(module.CorLibTypes.Boolean, buy.ToTypeSig()), shop);
var render = new MemberRefUser(module, "Render", Void(buy.ToTypeSig()), shop);
var opened = new MemberRefUser(module, "Opened", Void(sweep.ToTypeSig()), automaton);
var closed = new MemberRefUser(module, "Closed", Void(sweep.ToTypeSig()), automaton);
var stop = new MemberRefUser(module, "Stop", Void(sweep.ToTypeSig()), automaton);
var canContinue = new MemberRefUser(module, "CanContinue", MethodSig.CreateStatic(module.CorLibTypes.Boolean, sweep.ToTypeSig()), automaton);
var beginSend = new MemberRefUser(module, "BeginSend", MethodSig.CreateStatic(module.CorLibTypes.Boolean, sweep.ToTypeSig()), automaton);
var capture = new MemberRefUser(module, "Capture", MethodSig.CreateStatic(module.CorLibTypes.Void, module.CorLibTypes.Object, sweep.ToTypeSig()), automaton);
var packet = T("NetWork.Packets.Packet");
var accept = new MemberRefUser(module, "Accept", MethodSig.CreateStatic(module.CorLibTypes.Boolean, module.CorLibTypes.Object, sweep.ToTypeSig(), packet.ToTypeSig(), module.CorLibTypes.Boolean), automaton);
var canStartNext = new MemberRefUser(module, "CanStartNext", MethodSig.CreateStatic(module.CorLibTypes.Boolean, sweep.ToTypeSig()), automaton);

Instruction Ldarg0() => Instruction.Create(OpCodes.Ldarg_0);
void InsertBefore(MethodDef method, Instruction anchor, params Instruction[] added)
{
    var list = method.Body.Instructions;
    int index = list.IndexOf(anchor);
    Require(index >= 0, "Insert anchor missing in " + method.Name);
    foreach (var instruction in added) list.Insert(index++, instruction);
}
void InsertAfter(MethodDef method, Instruction anchor, params Instruction[] added)
{
    var list = method.Body.Instructions;
    int index = list.IndexOf(anchor);
    Require(index >= 0, "Insert anchor missing in " + method.Name);
    foreach (var instruction in added) list.Insert(++index, instruction);
}
void Changed(MethodDef method)
{
    method.Body.KeepOldMaxStack = true;
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)4);
}

// Stamina purchase page: keep every original label/price/limit calculation,
// skip only the quoted-cost hold suffix for the potion, then apply the
// original-font layout. Non-stamina items still run QuotedShopCost unchanged.
var setItem = M("UI.BuyItemPage", "SetItem");
var quoted = setItem.Body.Instructions.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.Name == "BuyPage" && m.DeclaringType != null && m.DeclaringType.Name == "QuotedShopCost");
int quotedIndex = setItem.Body.Instructions.IndexOf(quoted);
var quotedArg = setItem.Body.Instructions[quotedIndex - 1];
var quotedAfter = setItem.Body.Instructions[quotedIndex + 1];
Require(quotedArg.OpCode == OpCodes.Ldarg_0, "Quoted page argument differs");
InsertBefore(setItem, quotedArg,
    Ldarg0(), Instruction.Create(OpCodes.Call, isStamina), Instruction.Create(OpCodes.Brfalse, quotedArg),
    Ldarg0(), Instruction.Create(OpCodes.Call, render), Instruction.Create(OpCodes.Br, quotedAfter));
Changed(setItem);

// Sweep: start a fresh generation on open and invalidate it on close.
var sweepOpen = M("UI.UICopySweep", "DoOpen");
var openStop = sweepOpen.Body.Instructions.Single(i => i.OpCode == OpCodes.Stfld && i.Operand is IField f && f.Name == "needStop");
InsertAfter(sweepOpen, openStop, Ldarg0(), Instruction.Create(OpCodes.Call, opened));
Changed(sweepOpen);
var sweepClose = M("UI.UICopySweep", "DoClose");
InsertBefore(sweepClose, sweepClose.Body.Instructions[0], Ldarg0(), Instruction.Create(OpCodes.Call, closed));
Changed(sweepClose);

// Check immediately before every auto request; a stopped page cannot send.
var auto = M("UI.UICopySweep", "OnSweepCopyAuto");
var autoFirst = auto.Body.Instructions[0];
InsertBefore(auto, autoFirst, Ldarg0(), Instruction.Create(OpCodes.Call, beginSend),
    Instruction.Create(OpCodes.Brtrue, autoFirst), Instruction.Create(OpCodes.Ret));
var captureStore = auto.Body.Instructions.Single(i => i.OpCode == OpCodes.Stfld && i.Operand == anonThis);
InsertAfter(auto, captureStore, Instruction.Create(OpCodes.Ldloc_1), Ldarg0(), Instruction.Create(OpCodes.Call, capture));
Changed(auto);

// Stop sets needStop in the original instruction, then stops the waiting
// iterator and restores the original submit/stop button pair immediately.
var stopClick = M("UI.UICopySweepEvent", "OnStopClick");
var stopStore = stopClick.Body.Instructions.Single(i => i.OpCode == OpCodes.Stfld && i.Operand is IField f && f.Name == "needStop");
var pageGetterCall = stopClick.Body.Instructions.Single(i => i.OpCode == OpCodes.Call);
var pageGetter = (IMethod)pageGetterCall.Operand;
InsertAfter(stopClick, stopStore, Ldarg0(), Instruction.Create(OpCodes.Call, pageGetter), Instruction.Create(OpCodes.Call, stop));
Changed(stopClick);

// A stop during the animation wait must not reach OnSweepCopyAuto.
var move = iterator.Methods.Single(m => m.Name == "MoveNext");
var switchInstruction = move.Body.Instructions.Single(i => i.OpCode == OpCodes.Switch);
int switchIndex = move.Body.Instructions.IndexOf(switchInstruction);
var waitDone = ((Instruction[])switchInstruction.Operand)[1];
var moveEnd = (Instruction)move.Body.Instructions[switchIndex + 1].Operand;
var moveGuard = Ldarg0();
((Instruction[])switchInstruction.Operand)[1] = moveGuard;
InsertBefore(move, waitDone, moveGuard, Instruction.Create(OpCodes.Ldfld, iteratorThis),
    Instruction.Create(OpCodes.Call, canContinue), Instruction.Create(OpCodes.Brfalse, moveEnd));
Changed(move);

// An in-flight response is accepted only for the page generation that sent
// it, so a closed/reopened page cannot consume an old response. A stopped
// page still applies this response's rewards but never starts another round.
var callback = anon.Methods.Single(m => m.Name == "<>m__1E7");
var startSweep = callback.Body.Instructions.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.Name == "_StartSweep");
int startSweepIndex = callback.Body.Instructions.IndexOf(startSweep);
var nextRound = callback.Body.Instructions[startSweepIndex - 8];
var callbackJoin = (Instruction)callback.Body.Instructions[startSweepIndex + 3].Operand;
Require(nextRound.OpCode == OpCodes.Ldarg_0, "Next-round branch start differs");
Require(callback.Body.Instructions[startSweepIndex + 3].OpCode == OpCodes.Br, "Next-round join differs");
var callbackFirst = callback.Body.Instructions[0];
var nextGuard = Ldarg0();
foreach (var branch in callback.Body.Instructions.Where(i => i.Operand == nextRound &&
             (i.OpCode.FlowControl == FlowControl.Cond_Branch || i.OpCode.FlowControl == FlowControl.Branch)).ToArray())
{
    branch.Operand = nextGuard;
}
InsertBefore(callback, nextRound, nextGuard, Instruction.Create(OpCodes.Ldfld, anonThis),
    Instruction.Create(OpCodes.Call, canStartNext), Instruction.Create(OpCodes.Brfalse, callbackJoin));
InsertBefore(callback, callbackFirst, Ldarg0(), Ldarg0(), Instruction.Create(OpCodes.Ldfld, anonThis),
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2), Instruction.Create(OpCodes.Call, accept), Instruction.Create(OpCodes.Brtrue, callbackFirst), Instruction.Create(OpCodes.Ret));
Changed(callback);

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Bounded stamina purchase layout and sweep stop/ownership guards updated");
