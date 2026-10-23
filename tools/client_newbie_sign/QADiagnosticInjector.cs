using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length!=3)throw new ArgumentException("input helper output");
using var module=ModuleDefMD.Load(args[0]);using var helper=ModuleDefMD.Load(args[1]);
void Normalize(TypeSig signature) {
 if(signature == null) return;
 if(signature is TypeDefOrRefSig named && named.TypeDefOrRef is TypeRef reference && reference.ResolutionScope is ModuleRef scope && scope.Name == helper.Name) {
  if(!module.GetTypes().Any(t => t.FullName == reference.FullName)) throw new Exception("Unexpected helper signature: " + reference.FullName);
  reference.ResolutionScope = module;
 }
 if(signature is GenericInstSig generic) { Normalize(generic.GenericType); foreach(var arg in generic.GenericArguments) Normalize(arg); }
 Normalize(signature.Next);
}
void NormalizeCall(IMethod call) { Normalize(call.MethodSig.RetType); foreach(var arg in call.MethodSig.Params) Normalize(arg); }
var type=helper.GetTypes().Single(t=>t.FullName=="HYJS.NewbieSign.NewbieSignGuard");var importer=new Importer(module, ImporterOptions.TryToUseTypeDefs);
var render=module.GetTypes().Single(t=>t.FullName=="UI.TenDaySignPage").Methods.Single(m=>m.Name=="UpdateLoginReward");
if(render.Body.Instructions.Count(i=>i.OpCode==OpCodes.Call&&i.Operand is IMethod m&&m.Name=="CanRender")!=1)throw new Exception("Existing single render guard required");
var exit=importer.Import(type.Methods.Single(m=>m.Name=="RenderExit"));NormalizeCall(exit);
foreach(var ins in render.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()){
 var index=render.Body.Instructions.IndexOf(ins);ins.OpCode=OpCodes.Ldarg_0;ins.Operand=null;
 render.Body.Instructions.Insert(index+1,Instruction.Create(OpCodes.Call,exit));render.Body.Instructions.Insert(index+2,Instruction.Create(OpCodes.Ret));
}
render.Body.MaxStack=(ushort)Math.Max(2,(int)render.Body.MaxStack);render.Body.KeepOldMaxStack=true;
var auto=module.GetTypes().Single(t=>t.FullName=="Room.RoomTown").Methods.Single(m=>m.Name=="<ShowTenDaySign>m__8D");
var call=importer.Import(type.Methods.Single(m=>m.Name=="DiagnoseAuto"));NormalizeCall(call);
var prefix=new[]{Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Ldarg_2),Instruction.Create(OpCodes.Call,call)};
foreach(var ins in prefix.Reverse())auto.Body.Instructions.Insert(0,ins);auto.Body.MaxStack=(ushort)Math.Max(2,(int)auto.Body.MaxStack);auto.Body.KeepOldMaxStack=true;
var opts=new ModuleWriterOptions(module);opts.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[2],opts);
Console.WriteLine("QA-only anonymous auto entry and all render return exits; existing guards preserved");
