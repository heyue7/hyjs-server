using dnlib.DotNet;
using dnlib.DotNet.Emit;
using System.Text.Json;
using dnlib.DotNet.Writer;

var main=ModuleDefMD.Load(args[0]);
if(args.Length==6)File.WriteAllText(args[5],JsonSerializer.Serialize(RecoveryProof.Verify(args[3],args[4])));
var helper=ModuleDefMD.Load(args[1]);
var bridge=helper.GetTypes().Single(t=>t.FullName=="HYJS.TrialDemand.Bridge");
IMethod Hook(string name)=>main.Import(bridge.Methods.Single(m=>m.Name==name));
var page=main.GetTypes().Single(t=>t.FullName=="UI.RushEliteBossPage");
var changes=new List<string>();
void Mark(MethodDef method){changes.Add(method.DeclaringType.FullName+"::"+method.Name);}
void Replace(MethodDef method,string name,bool icon=false){
 if(method.Body.ExceptionHandlers.Any(h=>h.HandlerType!=ExceptionHandlerType.Finally))throw new InvalidOperationException("Unexpected handler: "+method.FullName);
 method.Body=new CilBody();method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
 if(icon)method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_1));
 method.Body.Instructions.Add(Instruction.Create(OpCodes.Call,Hook(name)));method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));Mark(method);
}
var open=page.Methods.Single(m=>m.Name=="DoOpen");
var cut=open.Body.Instructions.Single(i=>i.OpCode==OpCodes.Newobj&&i.Operand is IMethod m&&m.DeclaringType.FullName=="ProtoBuf.Message.CSGetRushEliteBossInfo");
if(open.Body.ExceptionHandlers.Count!=0)throw new InvalidOperationException("Unexpected open handler");
while(open.Body.Instructions.Count>open.Body.Instructions.IndexOf(cut)+1)open.Body.Instructions.RemoveAt(open.Body.Instructions.Count-1);
open.Body.Instructions.Remove(cut);open.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));open.Body.Instructions.Add(Instruction.Create(OpCodes.Call,Hook("Open")));open.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));Mark(open);
Replace(page.Methods.Single(m=>m.Name=="DoReopen"),"Refresh");
Replace(page.Methods.Single(m=>m.Name=="OnCoverPageRemove"),"Refresh");
Replace(page.Methods.Single(m=>m.Name=="OnBossIconClick"),"Select",true);
var close=page.Methods.Single(m=>m.Name=="DoClose");close.Body.Instructions.Insert(0,Instruction.Create(OpCodes.Call,Hook("Close")));close.Body.Instructions.Insert(0,Instruction.Create(OpCodes.Ldarg_0));Mark(close);
var normal=main.GetTypes().Single(t=>t.FullName=="UI.RushPage");
var callback=normal.Methods.Single(m=>m.Name.StartsWith("<AskRushInfo>")&&m.HasBody&&m.Body.Instructions.Any(i=>i.Operand is IMethod target&&target.Name=="IsRushBossInfoGot"));
if(callback.Body.ExceptionHandlers.Count!=0)throw new InvalidOperationException("Unexpected entry handler");
var call=callback.Body.Instructions.Single(i=>i.Operand is IMethod target&&target.Name=="IsRushBossInfoGot");
int index=callback.Body.Instructions.IndexOf(call);var end=callback.Body.Instructions.Last();if(end.OpCode!=OpCodes.Ret)throw new InvalidOperationException("Unexpected entry tail");
while(callback.Body.Instructions.Count>index)callback.Body.Instructions.RemoveAt(callback.Body.Instructions.Count-1);
callback.Body.Instructions.Add(end);Mark(callback);
var options=new ModuleWriterOptions(main);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;main.Write(args[2],options);Console.WriteLine(JsonSerializer.Serialize(changes));
