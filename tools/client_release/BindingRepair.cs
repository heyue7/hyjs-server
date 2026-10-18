using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Writer;
if(args.Length!=2)throw new ArgumentException("input output");
using var module=ModuleDefMD.Load(args[0]);
int count=0;
foreach(var type in module.GetTypeRefs()){
    if(type.ResolutionScope is not ModuleRef scope)continue;
    if(scope.Name!="SoulFlowerDisplay.dll" && scope.Name!="SoulFlowerActivity.dll")continue;
    if(module.Find(type.FullName,false)==null)throw new InvalidOperationException("Unknown host type: "+type.FullName);
    Console.WriteLine(type.FullName+": "+scope.Name+" -> current module");
    type.ResolutionScope=module;count++;
}
if(count!=6)throw new InvalidOperationException("Expected exactly 6 inherited host type bindings, got "+count);
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[1],options);
