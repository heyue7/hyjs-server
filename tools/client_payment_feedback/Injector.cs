using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length!=3)throw new ArgumentException("input helper output");
using var module=ModuleDefMD.Load(args[0]);using var helper=ModuleDefMD.Load(args[1]);
var target=module.GetTypes().Single(t=>t.FullName=="UI.ChargeData").Methods.Single(m=>m.Name=="ChargeBegin");
if(target.IsStatic || target.MethodSig.Params.Count!=3 || target.MethodSig.Params.Any(p=>p.ElementType!=ElementType.String) || target.MethodSig.RetType.ElementType!=ElementType.Void)throw new Exception("ChargeBegin contract changed");
var entry=helper.GetTypes().Single(t=>t.FullName=="HYJS.PaymentFeedback.PaymentFeedback").Methods.Single(m=>m.Name=="Begin");
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
var call=new Importer(module, ImporterOptions.TryToUseTypeDefs).Import(entry);
NormalizeCall(call);
target.Body=new CilBody();target.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));target.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_1));target.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_2));target.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_3));target.Body.Instructions.Add(Instruction.Create(OpCodes.Call,call));target.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));target.Body.MaxStack=4;
foreach (var reference in module.GetTypeRefs()) {
 if (reference.ResolutionScope is ModuleRef scope && scope.Name == helper.Name) {
  if (!module.GetTypes().Any(t => t.FullName == reference.FullName)) throw new Exception("Unexpected helper-module type: " + reference.FullName);
  reference.ResolutionScope = module;
 }
}
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[2],options);
