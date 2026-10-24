using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using var main=ModuleDefMD.Load(args[0]);
using var helper=ModuleDefMD.Load(args[1]);
var hook=helper.GetTypes().Single(t=>t.FullName=="HYJS.Feedback66.CommerceRuntime").Methods.Single(m=>m.Name=="Quota");
if(hook.MethodSig.Params.Count!=2 || hook.MethodSig.RetType.ElementType!=ElementType.Void || hook.MethodSig.Params.Any(p=>p.ElementType!=ElementType.Object)) throw new Exception("Quota ABI drift");
foreach(uint token in new uint[]{0x0600533a,0x06005341}) {
 var method=(MethodDef)main.ResolveToken(token);
 if(method.Body.ExceptionHandlers.Count!=0 || method.Name!=(token==0x0600533a ? "SetPopularityItem" : "SetScoreItem")) throw new Exception("Quota target drift");
 var exits=method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray();
 if(exits.Length!=1) throw new Exception("Quota exit drift");
 // Branches targeting the original ret must also execute the display update.
 var exit=exits[0]; int at=method.Body.Instructions.IndexOf(exit);
 exit.OpCode=OpCodes.Ldarg_0; exit.Operand=null;
 method.Body.Instructions.Insert(at+1,Instruction.Create(OpCodes.Ldarg_1));
 method.Body.Instructions.Insert(at+2,Instruction.Create(OpCodes.Call,new Importer(main).Import(hook)));
 method.Body.Instructions.Insert(at+3,Instruction.Create(OpCodes.Ret));
 method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)2);
}
foreach(var method in main.GetTypes().SelectMany(t=>t.Methods).Where(m=>m.HasBody)) method.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(main);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
main.Write(args[2],options);
