using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using var module=ModuleDefMD.Load(args[0]);
MethodDef M(string owner,string name)=>module.GetTypes().Single(t=>t.FullName==owner).Methods.Single(m=>m.Name==name);
var title=M("UI.AcRechargeSingleItem","GetTextString");
var first=title.Body.Instructions[0];
var field=module.GetTypes().Single(t=>t.FullName=="UI.AcRechargeSingleItem").Fields.Single(f=>f.Name=="type");
var prefix=new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldfld,field),Instruction.Create(OpCodes.Ldc_I4,32),Instruction.Create(OpCodes.Bne_Un,first),Instruction.Create(OpCodes.Ldstr,"主线通关{0}次"),Instruction.Create(OpCodes.Ret)};
foreach(var ins in prefix.Reverse())title.Body.Instructions.Insert(0,ins);
var reference=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerWelfareFeedback, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper=new TypeRefUser(module,"HYJS.WelfareFeedback","FeedbackBridge",reference);
var item=module.GetTypes().Single(t=>t.FullName=="AchievementItem");
var call=new MemberRefUser(module,"AchievementClaimed",MethodSig.CreateStatic(module.CorLibTypes.Void,item.ToTypeSig()),helper);
var method=M("AchievementItem","SetReciveState");
foreach(var exit in method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()){
 int at=method.Body.Instructions.IndexOf(exit);exit.OpCode=OpCodes.Ldarg_0;exit.Operand=null;
 method.Body.Instructions.Insert(at+1,Instruction.Create(OpCodes.Call,call));method.Body.Instructions.Insert(at+2,Instruction.Create(OpCodes.Ret));
}
foreach(var m in new[]{title,method}){m.Body.MaxStack=Math.Max(m.Body.MaxStack,(ushort)2);m.Body.KeepOldMaxStack=true;}
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
