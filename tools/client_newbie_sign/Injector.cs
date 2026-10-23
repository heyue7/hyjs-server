using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length!=3)throw new ArgumentException("input, helper, output DLL required");
using var module=ModuleDefMD.Load(args[0]);
using var helper=ModuleDefMD.Load(args[1]);
var type=helper.GetTypes().Single(t=>t.FullName=="HYJS.NewbieSign.NewbieSignGuard");
var importer=new Importer(module);
void Prefix(string owner,string name,string guard,bool instance){
 var method=module.GetTypes().Single(t=>t.FullName==owner).Methods.Single(m=>m.Name==name);
 if(method.Body==null||method.Body.ExceptionHandlers.Count!=(instance?2:0))throw new Exception("Original target body/exception regions differ");
 var call=importer.Import(type.Methods.Single(m=>m.Name==guard));
 var body=method.Body;var first=body.Instructions[0];
 var prefix=new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,call),Instruction.Create(OpCodes.Brtrue,first),Instruction.Create(OpCodes.Ret)};
 foreach(var ins in prefix.Reverse())body.Instructions.Insert(0,ins);
 body.MaxStack=(ushort)Math.Max(2,(int)body.MaxStack);body.KeepOldMaxStack=true;
}
Prefix("UI.ShowRight","<OnTenDaySignClick>m__154","CanOpen",false);
Prefix("UI.TenDaySignPage","UpdateLoginReward","CanRender",true);
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
Console.WriteLine("Injected two conditional newbie-sign guards; all original valid-list instructions retained");
