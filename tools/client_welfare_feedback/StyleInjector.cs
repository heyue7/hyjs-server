using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using var module=ModuleDefMD.Load(args[0]);
var item=module.GetTypes().Single(t=>t.FullName=="UI.AcRechargeSingleItem");
var method=item.Methods.Single(m=>m.Name=="SetActivityItem");
var il=method.Body.Instructions;
if(il[27].OpCode!=OpCodes.Ldfld||((IField)il[27].Operand).Name!="labTitle"||
   il[33].OpCode!=OpCodes.Callvirt||((IMethod)il[33].Operand).FullName!="System.Void UILabel::set_text(System.String)")
 throw new InvalidOperationException("Native title assignment differs");
var reference=module.GetAssemblyRefs().Single(r=>r.Name=="SoulFlowerWelfareFeedback");
var helper=new TypeRefUser(module,"HYJS.WelfareFeedback","FeedbackBridge",reference);
var call=new MemberRefUser(module,"GuideTitle",MethodSig.CreateStatic(module.CorLibTypes.Void,item.ToTypeSig()),helper);
il.Insert(34,Instruction.Create(OpCodes.Ldarg_0));
il.Insert(35,Instruction.Create(OpCodes.Call,call));
method.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
