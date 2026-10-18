using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length!=5)throw new ArgumentException("input, bridge, output, public origin, enabled required");
using var module=ModuleDefMD.Load(args[0]);using var bridge=ModuleDefMD.Load(args[1]);
var voice=module.GetTypes().Single(t=>t.FullName=="VoicePlugin");var impl=bridge.GetTypes().Single(t=>t.Name=="LocalVoiceBridge");
var entries=new Dictionary<string,string>{{"InitGVoice","Init"},{"StartRecord","StartRecord"},{"StopRecord","StopRecord"},{"ClearRecordDelage","ClearRecord"},{"PlayRecord","PlayRecord"},{"DownLoadFile","Download"},{"Play","Play"},{"StopPlay","StopPlay"},{"DeleFile","Evict"},{"Tick","Tick"}};
foreach(var pair in entries){
 var method=voice.Methods.Single(m=>m.Name==pair.Key);var target=impl.Methods.Single(m=>m.Name==pair.Value);
 if(method.Body==null||method.Body.ExceptionHandlers.Count!=0||method.MethodSig.RetType.ElementType!=ElementType.Void||method.Parameters.Count!=target.MethodSig.Params.Count)throw new InvalidOperationException("Original voice entry signature drift: "+pair.Key);
 var body=new CilBody{InitLocals=false,KeepOldMaxStack=false};
 if(pair.Key=="InitGVoice") {body.Instructions.Add(Instruction.Create(OpCodes.Ldstr,args[3]));body.Instructions.Add(Instruction.Create(args[4]=="true"?OpCodes.Ldc_I4_1:OpCodes.Ldc_I4_0));body.Instructions.Add(Instruction.Create(OpCodes.Call,module.Import(impl.Methods.Single(m=>m.Name=="Configure"))));}
 for(int i=0;i<method.Parameters.Count;i++){
  body.Instructions.Add(Instruction.Create(OpCodes.Ldarg,method.Parameters[i]));
  if(pair.Key=="DownLoadFile"&&i==2)body.Instructions.Add(Instruction.Create(OpCodes.Conv_I4));
 }
 body.Instructions.Add(Instruction.Create(OpCodes.Call,module.Import(target)));body.Instructions.Add(Instruction.Create(OpCodes.Ret));method.Body=body;
}
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[2],options);
Console.WriteLine("Local voice entries: "+entries.Count+"; original SDK credentials unused; adapter enabled="+args[4]);
