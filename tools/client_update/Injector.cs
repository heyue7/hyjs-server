using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using System;
using System.Linq;

using var module = ModuleDefMD.Load(args[0]);
var reference = new AssemblyRefUser(new AssemblyNameInfo("HYJSClientUpdate, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var api = new TypeRefUser(module,"HYJS.ClientUpdate","Bridge",reference);
var bridge = new MemberRefUser(module,"Version",MethodSig.CreateStatic(module.CorLibTypes.String,module.GetTypes().Single(t=>t.FullName=="GameConfig").ToTypeSig()),api);
var targets = new[]{("Login.LoginCheckVersion","CheckVersion"),("Login.LoginUserLogin","UserLogin")};
foreach(var target in targets)
{
    var method=module.GetTypes().Single(t=>t.FullName==target.Item1).Methods.Single(m=>m.Name==target.Item2);
    int changed=0, existing=0;
    foreach(var instruction in method.Body.Instructions)
        if((instruction.OpCode==OpCodes.Call || instruction.OpCode==OpCodes.Callvirt) && instruction.Operand is IMethod call && call.Name=="get_AppVersion" && call.DeclaringType.FullName=="GameConfig")
        {instruction.OpCode=OpCodes.Call;instruction.Operand=bridge;changed++;}
        else if(instruction.OpCode==OpCodes.Call && instruction.Operand is IMethod hooked && hooked.Name=="Version" && hooked.DeclaringType.FullName=="HYJS.ClientUpdate.Bridge" && hooked.DeclaringType.DefinitionAssembly.Name=="HYJSClientUpdate" && hooked.MethodSig.Params.Count==1 && hooked.MethodSig.Params[0].FullName=="GameConfig" && hooked.MethodSig.RetType.ElementType==ElementType.String)existing++;
    if(changed+existing!=1)throw new InvalidOperationException("Expected exactly one original or installed app version call in "+target);
    method.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
