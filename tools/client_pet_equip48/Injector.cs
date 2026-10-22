using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
if(args.Length!=2) throw new ArgumentException("input managed, output managed");
using var module=ModuleDefMD.Load(args[0]);
var reference=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerPetEquip48, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper=new TypeRefUser(module,"HYJS.PetEquip48","PetEquipFeedback",reference);
foreach(var target in new[]{("UI.PetEquipAwakeTab","SetCost","AwakeCosts"),("UI.PetEquipStrengthTab","isCanStrength","CanStrength")}) {
 var type=module.GetTypes().Single(t=>t.FullName==target.Item1);
 var method=type.Methods.Single(m=>m.Name==target.Item2);
 var parameters=new[]{type.ToTypeSig()}.Concat(method.MethodSig.Params).ToArray();
 var call=new MemberRefUser(module,target.Item3,MethodSig.CreateStatic(method.MethodSig.RetType,parameters),helper);
 method.Body=new CilBody(); method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
 if(method.MethodSig.Params.Count==1) method.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_1));
 method.Body.Instructions.Add(Instruction.Create(OpCodes.Call,call));method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
 method.Body.MaxStack=8;method.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[1],options);
