using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length!=3) throw new Exception("input helper output required");
using var module=ModuleDefMD.Load(args[0]);
using var helper=ModuleDefMD.Load(args[1]);
var method=module.GetTypes().Single(t=>t.FullName=="AssetLoad.AssetBundleManager").Methods.Single(m=>m.Name=="GetFullPath");
if(method.MethodSig.Params.Count!=3 || method.MethodSig.RetType.ElementType!=ElementType.String || method.Body.ExceptionHandlers.Count!=0) throw new Exception("GetFullPath signature/EH mismatch");
if(method.Body.Instructions.Any(i=>i.OpCode==OpCodes.Call && i.Operand is IMethod c && c.DeclaringType.FullName=="HYJS.DungeonManifest.ManifestPath")) throw new Exception("Already hooked");
var call=new Importer(module,ImporterOptions.TryToUseTypeDefs).Import(helper.GetTypes().Single(t=>t.FullName=="HYJS.DungeonManifest.ManifestPath").Methods.Single(m=>m.Name=="Resolve"));
var returns=method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToList();
if(returns.Count!=3) throw new Exception("Expected three original returns");
foreach(var ret in returns) {
 int index=method.Body.Instructions.IndexOf(ret);
 ret.OpCode=OpCodes.Ldarg_1;ret.Operand=null;
 method.Body.Instructions.Insert(index+1,Instruction.Create(OpCodes.Call,call));
 method.Body.Instructions.Insert(index+2,Instruction.Create(OpCodes.Ret));
}
method.Body.MaxStack=(ushort)Math.Max(2,(int)method.Body.MaxStack);method.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
using(var final=ModuleDefMD.Load(args[2])) {
 var bad=final.GetTypeRefs().Where(t=>t.ResolutionScope is ModuleRef s && s.Name==helper.Name).ToList();
 if(bad.Count!=0) throw new Exception("Final output contains helper-scoped TypeRefs: "+string.Join(",",bad.Select(t=>t.FullName)));
}
Console.WriteLine("three scoped return hooks; final helper ModuleRef type signatures=0");
