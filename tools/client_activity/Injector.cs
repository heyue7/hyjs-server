using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=3)throw new ArgumentException("input managed, activity bridge, output managed");
using var module=ModuleDefMD.Load(args[0]);using var bridge=ModuleDefMD.Load(args[1]);
var type=bridge.GetTypes().Single(t=>t.FullName=="HYJS.ActivitySafety.ActivityGuard");
MethodDef M(string owner,string name)=>module.GetTypes().Single(t=>t.FullName==owner).Methods.Single(m=>m.Name==name);
var guardType=new TypeRefUser(module,"HYJS.ActivitySafety","ActivityGuard",new AssemblyRefUser(bridge.Assembly));
var pageType=module.GetTypes().Single(t=>t.FullName=="UI.UIActivity").ToTypeSig();
var panelType=module.GetTypes().Single(t=>t.FullName=="UI.UIActivityPanel").ToTypeSig();
// Keep all host-owned parameter types bound to this module. Importing an entire
// helper method can turn its back-reference into a foreign netmodule scope.
IMethod B(string name){
    MethodSig signature=name switch {
        "PrepareResponse"=>MethodSig.CreateStatic(module.CorLibTypes.Boolean,pageType,M("UI.UIActivity","MsgCallback").MethodSig.Params[0],module.CorLibTypes.Boolean),
        "PrepareTabs"=>MethodSig.CreateStatic(module.CorLibTypes.Boolean,pageType,M("UI.UIActivity","Init").MethodSig.Params[0]),
        "HasCurrent"=>MethodSig.CreateStatic(module.CorLibTypes.Boolean,panelType),
        "Show"=>MethodSig.CreateStatic(module.CorLibTypes.Void,panelType,M("UI.UIActivityPanel","Show").MethodSig.Params[0]),
        _=>throw new ArgumentException(name)
    };
    return new MemberRefUser(module,name,signature,guardType);
}
void Guard(MethodDef method,string name,int args){
    var original=method.Body.Instructions[0];var prefix=new System.Collections.Generic.List<Instruction>();
    prefix.Add(Instruction.Create(OpCodes.Ldarg_0));if(args>1)prefix.Add(Instruction.Create(OpCodes.Ldarg_1));if(args>2)prefix.Add(Instruction.Create(OpCodes.Ldarg_2));
    prefix.Add(Instruction.Create(OpCodes.Call,B(name)));prefix.Add(Instruction.Create(OpCodes.Brtrue,original));prefix.Add(Instruction.Create(OpCodes.Ret));
    for(int n=0;n<prefix.Count;n++)method.Body.Instructions.Insert(n,prefix[n]);
    method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)3);method.Body.KeepOldMaxStack=true;
}
Guard(M("UI.UIActivity","MsgCallback"),"PrepareResponse",3);
Guard(M("UI.UIActivity","Init"),"PrepareTabs",2);
foreach(var name in new[]{"Refersh","OnPageClosed","OnPageCoverPageRemoved","OnPageCoverPageOpen"})Guard(M("UI.UIActivityPanel",name),"HasCurrent",1);
var show=M("UI.UIActivityPanel","Show");show.Body=new CilBody();show.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));show.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_1));show.Body.Instructions.Add(Instruction.Create(OpCodes.Call,B("Show")));show.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[2],options);
Console.WriteLine("Activity directory, original template and null lifecycle boundaries protected");
