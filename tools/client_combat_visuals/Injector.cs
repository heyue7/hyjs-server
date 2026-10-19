using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length != 2) throw new ArgumentException("input and output DLL required");
using var module = ModuleDefMD.Load(args[0]);
var iterator=module.GetTypes().Single(t=>t.Name=="<CreateModels>c__Iterator14F");
var method=iterator.Methods.Single(m=>m.Name=="MoveNext");
var owner=iterator.Fields.Single(f=>f.Name=="<>f__this");
var page=module.GetTypes().Single(t=>t.FullName=="UI.UIFormationPopupPage");
var tab=page.Fields.Single(f=>f.Name=="currTab");
var refresh=page.Methods.Single(m=>m.Name=="OnTabClick");
var code=method.Body.Instructions;
var set=code.Single(i=>i.OpCode==OpCodes.Callvirt && i.Operand is IMethod m && m.DeclaringType.FullName=="UIToggle" && m.Name=="Set");
int index=code.IndexOf(set)+1;
// A fast initial 82000 callback can place an empty model list. Loading then
// reselects the already active toggle, whose Set(true) returns without events.
// Refresh through the original page callback once all models have loaded.
code.Insert(index++,Instruction.Create(OpCodes.Ldarg_0));
code.Insert(index++,Instruction.Create(OpCodes.Ldfld,owner));
code.Insert(index++,Instruction.Create(OpCodes.Ldarg_0));
code.Insert(index++,Instruction.Create(OpCodes.Ldfld,owner));
code.Insert(index++,Instruction.Create(OpCodes.Ldfld,tab));
code.Insert(index,Instruction.Create(OpCodes.Call,refresh));
method.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("Formation model load completion refreshes original selected tab and authoritative positions");
