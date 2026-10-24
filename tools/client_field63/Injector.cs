using System;
using System.IO;
using System.Linq;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using var main=ModuleDefMD.Load(args[0]);
using var helper=ModuleDefMD.Load(args[1]);
var importer=new Importer(main,ImporterOptions.TryToUseTypeDefs);
foreach(var target in JsonDocument.Parse(File.ReadAllText(args[3])).RootElement.EnumerateArray()) {
 var method=(MethodDef)main.ResolveToken(Convert.ToUInt32(target.GetProperty("token").GetString()![2..],16));
 if(method.DeclaringType.FullName!=target.GetProperty("type").GetString() || method.Name!=target.GetProperty("method").GetString())throw new Exception("Wrong pinned target");
 if(target.TryGetProperty("retained",out var retained) && retained.GetBoolean())continue;
 var helperMethod=helper.GetTypes().Single(t=>t.FullName=="HYJS.Field63.Runtime").Methods.Single(m=>m.Name==target.GetProperty("helper").GetString());
 if(helperMethod.MethodSig.Params.Any(p=>p.ElementType!=ElementType.Object))throw new Exception("Hook boundary must use object parameters");
 var hook=importer.Import(helperMethod);
 if(target.GetProperty("mode").GetString()=="replace") {
  method.Body.Instructions.Clear();method.Body.ExceptionHandlers.Clear();
  method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
  if(target.GetProperty("arguments").GetInt32()==2)method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_1));
  method.Body.Instructions.Add(Instruction.Create(OpCodes.Call,hook));method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
 } else {
  var returns=method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray();
  if(returns.Length!=1 || method.Body.ExceptionHandlers.Count!=0)throw new Exception("Unexpected tail shape");
  int at=method.Body.Instructions.IndexOf(returns[0]);
  method.Body.Instructions.Insert(at++,Instruction.Create(OpCodes.Ldarg_0));
  method.Body.Instructions.Insert(at,Instruction.Create(OpCodes.Call,hook));
 }
 method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)2);method.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(main);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;main.Write(args[2],options);
