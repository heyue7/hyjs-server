using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length != 2) throw new ArgumentException("input output");
using var module = ModuleDefMD.Load(args[0]);
var page = module.GetTypes().Single(t => t.FullName == "UI.WorldMapPage");
MethodDef M(string name) => page.Methods.Single(m => m.Name == name);
FieldDef F(string name) => page.Fields.Single(f => f.Name == name);
Instruction A() => Instruction.Create(OpCodes.Ldarg_0);
void Require(bool ok, string why) { if(!ok) throw new InvalidOperationException(why); }
void Changed(MethodDef m) { m.Body.KeepOldMaxStack = true; m.Body.MaxStack = Math.Max(m.Body.MaxStack,(ushort)4); }
foreach(var prefix in new[]{"shamo","senlin","huoshan","waiyu"}) {
 var name = "is" + char.ToUpper(prefix[0]) + prefix.Substring(1) + "Open";
 var method = M(name);
 var old = method.Body.Instructions.ToArray();
 Require(old.Length == 54 && old[8].Operand is IMethod icon && icon.Name == "getTypePic", "Original lock body anchor differs: " + name);
 var getObject = (IMethod)old[23].Operand;
 var getComponent = (IMethod)old[28].Operand;
 var active = (IMethod)old[25].Operand;
 var text = (IMethod)old[42].Operand;
 Require(getObject.Name == "get_gameObject" && getComponent.Name == "get_gameObject" && active.Name == "SetActive" && text.Name == "set_text", "Original NGUI references differ");
 method.Body.Instructions.Clear();
 foreach(var i in old.Take(9)) method.Body.Instructions.Add(i);
 var added = new[]{A(),Instruction.Create(OpCodes.Ldc_I4_0),Instruction.Create(OpCodes.Stfld,F(prefix+"Flag")),
 A(),Instruction.Create(OpCodes.Ldfld,F(prefix+"Text")),Instruction.Create(OpCodes.Ldstr,"暂未开放"),Instruction.Create(OpCodes.Callvirt,text),
 A(),Instruction.Create(OpCodes.Ldfld,F(prefix+"Text")),Instruction.Create(OpCodes.Callvirt,getComponent),Instruction.Create(OpCodes.Ldc_I4_1),Instruction.Create(OpCodes.Callvirt,active),
 A(),Instruction.Create(OpCodes.Ldfld,F(prefix+"Lock")),Instruction.Create(OpCodes.Callvirt,getObject),Instruction.Create(OpCodes.Ldc_I4_1),Instruction.Create(OpCodes.Callvirt,active),Instruction.Create(OpCodes.Ret)};
 foreach(var i in added) method.Body.Instructions.Add(i);
 Changed(method);
}
var click=M("onMapClick");
var clickOld=click.Body.Instructions.ToArray();
Require(clickOld[2].OpCode == OpCodes.Stloc_0 && clickOld[3].OpCode == OpCodes.Ldloc_0,"Map index parse anchor differs");
var resume=clickOld[3];
var guard=new[]{Instruction.Create(OpCodes.Ldloc_0),Instruction.Create(OpCodes.Ldc_I4_2),Instruction.Create(OpCodes.Blt,resume),Instruction.Create(OpCodes.Ldloc_0),Instruction.Create(OpCodes.Ldc_I4_5),Instruction.Create(OpCodes.Bgt,resume),A(),Instruction.Create(OpCodes.Call,M("mapCloseTip")),Instruction.Create(OpCodes.Ret)};
for(int i=0;i<guard.Length;i++) click.Body.Instructions.Insert(3+i,guard[i]);
Changed(click);
var transfer=M("onTransfer");
var first=transfer.Body.Instructions[0];
var equalityCalls=transfer.Body.Instructions.Where(i=>i.OpCode==OpCodes.Call && i.Operand is IMethod m && m.Name=="op_Equality").ToArray();
Require(equalityCalls.Length == 2 && equalityCalls.All(i => i.Operand == equalityCalls[0].Operand),"Original equality references differ");
var equal=(IMethod)equalityCalls[0].Operand;
var transferGuard=new[]{Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Ldstr,"field_001"),Instruction.Create(OpCodes.Call,equal),Instruction.Create(OpCodes.Brtrue,first),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Ldstr,"field_002"),Instruction.Create(OpCodes.Call,equal),Instruction.Create(OpCodes.Brtrue,first),A(),Instruction.Create(OpCodes.Call,M("mapCloseTip")),Instruction.Create(OpCodes.Ret)};
for(int i=0;i<transferGuard.Length;i++) transfer.Body.Instructions.Insert(i,transferGuard[i]);
Changed(transfer);
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("World map 003..006 locked with original NGUI labels and lock layers; 001/002 retained");
