using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using System.Linq;
using var module=ModuleDefMD.Load(args[0]);
var row=module.GetTypes().Single(t=>t.FullName=="UI.VipBenefitDailyReward");
var item=module.GetTypes().Single(t=>t.FullName=="ProtoBuf.Message.ActivityDailyBoonItem");
var helper=new TypeRefUser(module,"HYJS.Membership","MembershipFeedback",new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerMembership, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null")));
var bridge=new MemberRefUser(module,"Show",MethodSig.CreateStatic(module.CorLibTypes.Void,row.ToTypeSig(),item.ToTypeSig(),module.CorLibTypes.Boolean),helper);
var method=row.Methods.Single(m=>m.Name=="SetRewardInfo");
foreach(var ret in method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToList()){
 int index=method.Body.Instructions.IndexOf(ret);ret.OpCode=OpCodes.Ldarg_0;ret.Operand=null;
 var tail=new[]{Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Ldarg_2),Instruction.Create(OpCodes.Call,bridge),Instruction.Create(OpCodes.Ret)};
 foreach(var instruction in tail)method.Body.Instructions.Insert(++index,instruction);
}
method.Body.MaxStack=8;method.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[1],options);
