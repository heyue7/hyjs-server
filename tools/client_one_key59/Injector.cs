using System;
using System.IO;
using System.Linq;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length<3)throw new Exception("inputMain helper outputMain [--proto-only]");
using var module=ModuleDefMD.Load(args[0]);
var importer=new Importer(module,ImporterOptions.TryToUseTypeDefs);
var repair = new List<string>();
foreach (var reference in module.GetTypeRefs()) {
 if (reference.ResolutionScope is ModuleRef scope && scope.Name == "InstanceProtection.dll") {
  if (!new[]{"UI.UIHero","UI.UIEquipment","UI.UITreasure"}.Contains(reference.FullName) || !module.GetTypes().Any(t=>t.FullName==reference.FullName)) throw new Exception("Unexpected protection reference "+reference.FullName);
  repair.Add(reference.FullName);reference.ResolutionScope=module;
 }
}
if (repair.Count != 3) throw new Exception("Expected exactly three inherited protection references");
var metadataOptions=new ModuleWriterOptions(module);metadataOptions.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!,"metadata-only-Assembly-CSharp.dll"),metadataOptions);
File.WriteAllText(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!,"metadata-repairs.json"),JsonSerializer.Serialize(repair));
{
using var helper=ModuleDefMD.Load(args[1]);
if(helper.Assembly.Name!="HYJS.OneKey59")throw new Exception("Unexpected helper identity");
var runtime=helper.GetTypes().Single(t=>t.FullName=="HYJS.OneKey59.Selection");
var targets=JsonDocument.Parse(File.ReadAllText(args.Length>3?args[3]:Path.Combine(Path.GetDirectoryName(args[0])!,"targets.json"))).RootElement.GetProperty("targets");
var changed=new List<object>();
void Normalize(TypeSig signature) {
 if(signature==null)return;
 if(signature is TypeDefOrRefSig named && named.TypeDefOrRef is TypeRef reference && reference.ResolutionScope is ModuleRef scope && scope.Name=="HYJS.OneKey59.dll") {
  if(!module.GetTypes().Any(t=>t.FullName==reference.FullName))throw new Exception("Unexpected helper-scoped original type "+reference.FullName);
  reference.ResolutionScope=module;
 }
 if(signature is GenericInstSig generic) { Normalize(generic.GenericType);foreach(var argument in generic.GenericArguments)Normalize(argument); }
 if(signature.Next!=null)Normalize(signature.Next);
}
foreach(var plan in targets.EnumerateArray()) {
 string type=plan.GetProperty("type").GetString()!,name=plan.GetProperty("method").GetString()!;
 var m=module.GetTypes().Single(t=>t.FullName==type).Methods.Single(m=>m.Name==name);
 if(!m.HasBody || m.Body.Instructions.Any(i=>i.Operand is IMethod x&&x.DeclaringType.FullName==runtime.FullName))throw new Exception("Missing/already patched body "+m.FullName);
 var reader=module.Metadata.PEImage.CreateReader(m.RVA);uint start=reader.Position;byte first=reader.ReadByte();uint length;
 if((first&3)==2)length=(uint)(first>>2);else{int header=((first|(reader.ReadByte()<<8))>>12)*4;reader.ReadUInt16();length=reader.ReadUInt32();reader.Position=start+(uint)header;}
 string hash=Convert.ToHexString(SHA256.HashData(reader.ReadBytes((int)length))).ToLowerInvariant();
 if(hash!=plan.GetProperty("inputCILSHA256").GetString())throw new Exception("Fixed55 target CIL differs: "+m.FullName+" "+hash);
 var call=importer.Import(runtime.Methods.Single(h=>h.Name==plan.GetProperty("helper").GetString()));
 Normalize(call.MethodSig.RetType);foreach(var parameterType in call.MethodSig.Params)Normalize(parameterType);
 var added=new List<Instruction>();
 foreach(var parameter in plan.GetProperty("args").EnumerateArray())added.Add(Instruction.Create(OpCodes.Ldarg,m.Parameters[parameter.GetInt32()]));
 added.Add(Instruction.Create(OpCodes.Call,call));
 string mode=plan.GetProperty("mode").GetString()!;
 if(mode=="replace") {
  m.Body.ExceptionHandlers.Clear(); // Reviewed complete replacement of selection enumeration or typed page opening.
  m.Body.Instructions.Clear();foreach(var i in added)m.Body.Instructions.Add(i);m.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
 } else if(mode=="prefix") {
  var original=m.Body.Instructions[0];added.Add(Instruction.Create(OpCodes.Brfalse,original));added.Add(Instruction.Create(OpCodes.Ret));
  for(int i=0;i<added.Count;i++)m.Body.Instructions.Insert(i,added[i]);
 } else if (mode=="success-prefix") {
  var original=m.Body.Instructions;
  if(original.Count<3 || original[0].OpCode!=OpCodes.Ldarg_2 || original[1].OpCode!=OpCodes.Brfalse || original[2].OpCode!=OpCodes.Ldnull)
    throw new Exception("Original success branch anchor differs");
  int at=2;foreach(var i in added)original.Insert(at++,i);
 } else if (mode=="iterator-tail") {
  var owner = m.DeclaringType.Fields.Single(f=>f.Name=="<>f__this");
  foreach (var exit in m.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()) {
   int at=m.Body.Instructions.IndexOf(exit);exit.OpCode=OpCodes.Ldarg_0;exit.Operand=null;
   m.Body.Instructions.Insert(++at,Instruction.Create(OpCodes.Ldfld,owner));
   m.Body.Instructions.Insert(++at,Instruction.Create(OpCodes.Call,call));
   m.Body.Instructions.Insert(++at,Instruction.Create(OpCodes.Ret));
  }
 } else {
  var exits=m.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray();
  foreach(var exit in exits) {
   int at=m.Body.Instructions.IndexOf(exit);var sequence=new List<Instruction>();
   foreach(var parameter in plan.GetProperty("args").EnumerateArray())sequence.Add(Instruction.Create(OpCodes.Ldarg,m.Parameters[parameter.GetInt32()]));
   sequence.Add(Instruction.Create(OpCodes.Call,call));sequence.Add(Instruction.Create(OpCodes.Ret));
   exit.OpCode=sequence[0].OpCode;exit.Operand=sequence[0].Operand;
   foreach(var i in sequence.Skip(1))m.Body.Instructions.Insert(++at,i);
  }
 }
 m.Body.MaxStack=Math.Max(m.Body.MaxStack,(ushort)8);m.Body.KeepOldMaxStack=true;
 changed.Add(new{type,name,token=m.MDToken.ToString(),inputCILSHA256=hash,mode,helper=call.FullName});
}
File.WriteAllText(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!,"changed-methods.json"),JsonSerializer.Serialize(changed,new JsonSerializerOptions{WriteIndented=true}));
}


int rebound=0;
foreach(var reference in module.GetTypeRefs()) {
 if(reference.ResolutionScope is ModuleRef scope && scope.Name=="HYJS.OneKey59.dll") {
  if(!module.GetTypes().Any(t=>t.FullName==reference.FullName))throw new Exception("Unknown main type imported through helper ModuleRef: "+reference.FullName);
  reference.ResolutionScope=module;rebound++;
 }
}
if(module.GetTypeRefs().Any(r=>r.ResolutionScope is ModuleRef scope&&scope.Name=="HYJS.OneKey59.dll"))throw new Exception("Unnormalized helper ModuleRef");
Console.WriteLine("Rebound original Main types imported through helper ModuleRef: "+rebound);
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[2],options);
