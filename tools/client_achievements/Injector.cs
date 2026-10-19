using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length != 3) throw new ArgumentException("current managed, achievement helper, output managed");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string owner, string name) => T(owner).Methods.Single(m => m.Name == name);
var reference = new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerAchievements, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper = new TypeRefUser(module, "HYJS.Achievements", "AchievementsBridge", reference);
IMethod B(string name, TypeSig result, params TypeSig[] parameters) {
    var method = bridge.GetTypes().Single(t=>t.FullName=="HYJS.Achievements.AchievementsBridge").Methods.Single(m=>m.Name==name);
    if(method.MethodSig.RetType.FullName!=result.FullName || !method.MethodSig.Params.Select(p=>p.FullName).SequenceEqual(parameters.Select(p=>p.FullName))) throw new InvalidOperationException("Helper ABI mismatch: "+name);
    return new MemberRefUser(module,name,MethodSig.CreateStatic(result,parameters),helper);
}

// New types use the original page fields, preserving all prison/close/open checks
// inside GoBtnClick. The temporary known title key is replaced before Init returns.
var config = M("AchievementItem", "GetItemConfig");
var original = config.Body.Instructions[0];
var item = T("AchievementItem");
Instruction[] prefix = {
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldc_I4,37), Instruction.Create(OpCodes.Blt,original),
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldc_I4,41), Instruction.Create(OpCodes.Bgt,original),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldstr,"achievement_vip"), Instruction.Create(OpCodes.Stfld,item.Fields.Single(f=>f.Name=="titleDesc")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call,B("Page",module.CorLibTypes.String,module.CorLibTypes.Int32)), Instruction.Create(OpCodes.Stfld,item.Fields.Single(f=>f.Name=="pageName")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldstr,""), Instruction.Create(OpCodes.Stfld,item.Fields.Single(f=>f.Name=="option")), Instruction.Create(OpCodes.Ret)
};
foreach(var instruction in prefix.Reverse()) config.Body.Instructions.Insert(0,instruction);
config.Body.MaxStack=Math.Max(config.Body.MaxStack,(ushort)2);
config.Body.KeepOldMaxStack=true;
var init=M("AchievementItem","InitItemWithInfo");
var bind=B("Bind",module.CorLibTypes.Void,item.ToTypeSig(),T("ProtoBuf.Message.AchievementInfo").ToTypeSig());
foreach(var exit in init.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()) {
    int index=init.Body.Instructions.IndexOf(exit);
    exit.OpCode=OpCodes.Ldarg_0; exit.Operand=null;
    init.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Ldarg_1));
    init.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Call,bind));
    init.Body.Instructions.Insert(++index,Instruction.Create(OpCodes.Ret));
}
init.Body.MaxStack=Math.Max(init.Body.MaxStack,(ushort)2);
init.Body.KeepOldMaxStack=true;

// The original cached pet-fragment branch mistakenly asks for HeroIcon; keep
// its existing pool/quantity/red-point behavior and use the matching PetIcon API.
var pet=M("UI.BagIconManager","GetPetFragIcon");
var petGetter=pet.Body.Instructions.Select(i=>i.Operand).OfType<MethodSpec>().Single(m=>m.Name=="GetComponent" && m.GenericInstMethodSig.GenericArguments.Single().FullName=="UI.PetIcon");
int replaced=0;
foreach(var instruction in pet.Body.Instructions) {
    if(instruction.Operand is MethodSpec method && method.Name=="GetComponent" && method.GenericInstMethodSig.GenericArguments.Single().FullName=="UI.HeroIcon") { instruction.Operand=petGetter; replaced++; }
    else if(instruction.Operand is IMethod call && call.DeclaringType.FullName=="UI.HeroIcon" && (call.Name=="UpdateFragNum" || call.Name=="UpdateRedPointForFrag")) { instruction.Operand=M("UI.PetIcon",call.Name); replaced++; }
}
if(replaced!=4) throw new InvalidOperationException("Cached pet-fragment call anchors differ: "+replaced);
pet.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
Console.WriteLine("New achievement display/navigation and original cached pet-fragment API corrected");
