using System;
using System.Linq;
using System.Collections.Generic;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
var type = helper.GetTypes().Single(t => t.FullName == "HYJS.InstanceProtection.Protection");
var importer = new Importer(module);
IMethod H(string name) => importer.Import(type.Methods.Single(m => m.Name == name));
MethodDef M(string type, string name) => module.GetTypes().Single(t => t.FullName == type).Methods.Single(m => m.Name == name);
var changed = new HashSet<MethodDef>();
void Prefix(MethodDef m, string call) {
    m.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
    m.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Call, H(call)));
    changed.Add(m);
}
var login = M("Login.LoginPlayerLogin", "KVInfoCallBack");
login.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_1));
login.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Ldarg_2));
login.Body.Instructions.Insert(2, Instruction.Create(OpCodes.Call, H("ObserveKV")));
changed.Add(login);
foreach (var target in new[]{("UI.BagMemberDetail","SetHero"),("UI.BagMemberDetail","SetEquipment"),("UI.ArrayPage","DoOpen"),("UI.EquipMentUIPanelPage","DoOpen")}) Prefix(M(target.Item1,target.Item2),"Attach");
foreach (var t in module.GetTypes().Where(t => t.FullName is "UI.HeroIcon" or "UI.EquipIcon")) {
    foreach (var m in t.Methods.Where(m => m.Name == "SetHero" || m.Name == "SetEquip" || m.Name == "SetTreasure")) Prefix(m,"Icon");
    foreach (var m in t.Methods.Where(m => m.Name.String.StartsWith("Set") && (m.Name.String.Contains("Frag") || m.Name.String.EndsWith("WithId")))) Prefix(m,"ClearIcon");
}
foreach (var target in new[]{"GetHeroUpgradeAvailableListInOrder","HeroBreakupAutoSelect","EquipBreakupAutoSelect","GetHeroListForSellInOrder","GetEquipListForSell","GetTreasureListForSell","GetAdvenceAvailableHeroList","GetTreasureRefineAvailableList"}) {
    var method = M("UI.UIInstPlayerData",target);
    var filter = type.Methods.Single(m => m.Name.String.StartsWith("Filter") && m.MethodSig.Params.Single().FullName == method.MethodSig.RetType.FullName);
    var call = importer.Import(filter);
    foreach (var ret in method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()) {
        ret.OpCode = OpCodes.Call; ret.Operand = call;
        method.Body.Instructions.Insert(method.Body.Instructions.IndexOf(ret)+1,Instruction.Create(OpCodes.Ret));
    }
    changed.Add(method);
}
foreach (var method in new[]{M("UI.UIInstPlayerData","GetHeroUpgradeAvailableListInOrder"),M("UI.HeroSelectPage","OnOneKeySelect")}) {
    int anchors=0;
    var ins=method.Body.Instructions;
    for(int i=0;i+2<ins.Count;i++) if(ins[i].Operand is IField f && f.FullName=="System.Int32 Dict.DataModel.TableHero::quality" && ins[i+1].IsLdcI4() && ins[i+1].GetLdcI4Value()==13 && ins[i+2].OpCode==OpCodes.Blt) {ins[i+2].OpCode=OpCodes.Ble;anchors++;}
    if(anchors!=1)throw new InvalidOperationException("Blue selection anchor differs: "+method.FullName);
    changed.Add(method);
}
foreach (var method in new[]{M("UI.LevelUpPage","OnAutoSelect"),M("UI.HeroSelectPage","OnOneKeySelect")}) {
    var ins=method.Body.Instructions;
    for(int i=0;i+1<ins.Count;i++) if(ins[i].Operand is string key && key=="no_card_under_blue" && ins[i+1].Operand is IMethod m && m.Name=="GetText") {ins[i+1].Operand=H("SelectionText");changed.Add(method);}
}
// Limit treasure filtering to material selectors; locked equipment remains visible in bag/team.
foreach(var t in module.GetTypes().Where(t=>t.FullName=="UI.TreasureStrengthTab" || t.FullName.StartsWith("UI.TreasureSelectPage/"))) foreach(var m in t.Methods.Where(m=>m.HasBody)) {
    var ins=m.Body.Instructions;
    foreach(var call in ins.Where(i=>i.Operand is IMethod x && x.DeclaringType.FullName=="UI.UIInstPlayerData" && x.Name=="GetUITreasureByType").ToArray()) {
        ins.Insert(ins.IndexOf(call)+1,Instruction.Create(OpCodes.Call,H("FilterTreasures")));changed.Add(m);
    }
}
foreach(var m in changed)m.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
System.IO.File.WriteAllText(args[3],JsonSerializer.Serialize(changed.Select(m=>new{token=m.MDToken.Raw,method=m.FullName}).OrderBy(x=>x.token)));
Console.WriteLine("Locked instance controls, material/sale filters and inclusive blue auto selection composed");
