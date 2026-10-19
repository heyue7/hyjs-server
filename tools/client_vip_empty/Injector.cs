using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=2) throw new ArgumentException("input managed, output managed");
using var module=ModuleDefMD.Load(args[0]);
var type=module.GetTypes().Single(t=>t.FullName=="UI.TabVipShop");
var method=type.Methods.Single(m=>m.Name=="OnMsgCallback");
var reference=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerVipEmpty, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper=new TypeRefUser(module,"HYJS.VipShop","VipShopEmpty",reference);
var call=new MemberRefUser(module,"Received",MethodSig.CreateStatic(module.CorLibTypes.Void,type.ToTypeSig(),method.MethodSig.Params[0],method.MethodSig.Params[1]),helper);
foreach(var exit in method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()) {
 int index=method.Body.Instructions.IndexOf(exit);exit.OpCode=OpCodes.Ldarg_0;exit.Operand=null;
 method.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Ldarg_1));
 method.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Ldarg_2));
 method.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Call,call));
 method.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Ret));
}
method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)8);method.Body.KeepOldMaxStack=true;method.Body.SimplifyBranches();method.Body.OptimizeBranches();
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[1],options);
