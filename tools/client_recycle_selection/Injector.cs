using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

using var module=ModuleDefMD.Load(args[0]);
var panel=module.GetTypes().Single(t=>t.FullName=="UI.RecycleSelectPanel");
var assembly=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerRecycle, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper=new TypeRefUser(module,"HYJS.Recycle","SelectionControl",assembly);
var select=new MemberRefUser(module,"Select",MethodSig.CreateStatic(module.CorLibTypes.Void,panel.ToTypeSig()),helper);
foreach(var name in new[]{"UI.RecycleBreakupSelectHeroPanel","UI.RecycleBreakupSelectEquipPanel","UI.RecycleBreakupSelectPetPanel"}) {
    var method=module.GetTypes().Single(t=>t.FullName==name).Methods.Single(m=>m.Name=="OnPetBreakupButtionOneKeyClick");
    method.Body=new CilBody();
    method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
    method.Body.Instructions.Add(Instruction.Create(OpCodes.Call,select));
    method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
    method.Body.MaxStack=1;method.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
