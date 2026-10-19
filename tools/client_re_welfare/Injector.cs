using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=3) throw new ArgumentException("input main, Re welfare helper, output main");
using var module=ModuleDefMD.Load(args[0]);
using var bridge=ModuleDefMD.Load(args[1]);
TypeDef T(string name)=>module.GetTypes().Single(t=>t.FullName==name);
MethodDef M(string owner,string name)=>T(owner).Methods.Single(m=>m.Name==name);
var reference=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerReWelfare, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper=new TypeRefUser(module,"HYJS.ReWelfare","ReWelfareBridge",reference);
IMethod B(string name){
 var method=bridge.GetTypes().Single(t=>t.FullName=="HYJS.ReWelfare.ReWelfareBridge").Methods.Single(m=>m.Name==name);
 TypeSig S(TypeSig value)=>value.FullName=="System.Boolean"?module.CorLibTypes.Boolean:value.FullName=="System.Void"?module.CorLibTypes.Void:T(value.FullName).ToTypeSig();
 return new MemberRefUser(module,name,MethodSig.CreateStatic(S(method.MethodSig.RetType),method.MethodSig.Params.Select(S).ToArray()),helper);
}
void Prefix(MethodDef method,Instruction[] prefix){foreach(var ins in prefix.Reverse())method.Body.Instructions.Insert(0,ins);method.Body.KeepOldMaxStack=true;method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)4);}
void Suffix(MethodDef method,string helperName,bool packet){
 var exits=method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray();
 if(exits.Length<1)throw new InvalidOperationException("Unexpected target exits "+method.FullName);
 foreach(var exit in exits){int pos=method.Body.Instructions.IndexOf(exit);
 exit.OpCode=OpCodes.Ldarg_0;exit.Operand=null;
 method.Body.Instructions.Insert(++pos,Instruction.Create(OpCodes.Ldarg_1));
 if(packet)method.Body.Instructions.Insert(++pos,Instruction.Create(OpCodes.Ldarg_2));
 method.Body.Instructions.Insert(++pos,Instruction.Create(OpCodes.Call,B(helperName)));
 method.Body.Instructions.Insert(++pos,Instruction.Create(OpCodes.Ret));
 }
 method.Body.KeepOldMaxStack=true;method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)4);
}
var callback=M("UI.UIActivityTreasureCast","ButtonClickCallBack");
var original=callback.Body.Instructions[0];
Prefix(callback,new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldfld,T("UI.UIActivityTreasureCast").Fields.Single(f=>f.Name=="data")),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Ldarg_2),Instruction.Create(OpCodes.Call,B("BeforeCatReply")),Instruction.Create(OpCodes.Brtrue,original),Instruction.Create(OpCodes.Ret)});
Suffix(M("UI.UIActivityTreasureCast","UpdateState"),"BindCat",false);
Suffix(M("UIActivityContinueChargeGift","MsgCallback"),"BindContinued",true);
var click=M("UIActivityContinueChargeGift","OnRechargeBtn");original=click.Body.Instructions[0];
Prefix(click,new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,B("NavigateContinued")),Instruction.Create(OpCodes.Brfalse,original),Instruction.Create(OpCodes.Ret)});
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
Console.WriteLine("Pinned Re cat cache/debit/quote and continued-spend wording/navigation patched");
