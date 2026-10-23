using System;
using System.IO;
using System.Linq;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using var main=ModuleDefMD.Load(args[0]);
using var helper=ModuleDefMD.Load(args[1]);
var importer=new Importer(main);
var targets=JsonDocument.Parse(File.ReadAllText(args[3])).RootElement.EnumerateArray().ToArray();
foreach(var target in targets) {
 uint token=Convert.ToUInt32(target.GetProperty("token").GetString()![2..],16);
 var m=(MethodDef)main.ResolveToken(token);
 if(m.DeclaringType.FullName!=target.GetProperty("type").GetString() || m.Name!=target.GetProperty("method").GetString())throw new Exception("Wrong target");
 if(m.Body.ExceptionHandlers.Count!=0)throw new Exception("Unexpected EH");
 string type=target.GetProperty("helperType").GetString()!;
 var call=importer.Import(helper.GetTypes().Single(t=>t.FullName==type).Methods.Single(x=>x.Name==target.GetProperty("helper").GetString()));
 if(target.GetProperty("mode").GetString()=="replace") {
  m.Body.Instructions.Clear();
  m.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
  m.Body.Instructions.Add(Instruction.Create(OpCodes.Call,call));
  m.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
 } else {
  var exits=m.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray();
  if(exits.Length!=1)throw new Exception("Expected one request return");
  int at=m.Body.Instructions.IndexOf(exits[0]);
  if(target.TryGetProperty("local",out var local))m.Body.Instructions.Insert(at++,Instruction.Create(OpCodes.Ldloc_0));
  m.Body.Instructions.Insert(at,Instruction.Create(OpCodes.Call,call));
 }
 m.Body.MaxStack=Math.Max(m.Body.MaxStack,(ushort)2);m.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(main);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
main.Write(args[2],options);
