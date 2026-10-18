using System;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Collections.Generic;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input/output DLL and frozen unavailable catalogue required");
using var module = ModuleDefMD.Load(args[0]);
using var config = JsonDocument.Parse(File.ReadAllText(args[2]));
var settings = config.RootElement;
int maxFloor = settings.GetProperty("availableTrialLastFloor").GetInt32();
var deniedElite = settings.GetProperty("unavailableEliteIDs").EnumerateArray().Select(x => x.GetString()!).ToArray();
var popup = module.GetTypes().Single(t => t.FullName == "BoxManager").Methods.Single(m => m.Name == "CreatePopupTextBox" && m.MethodSig.Params.Count == 2);
var equality = module.GetMemberRefs().Single(m => m.FullName == "System.Boolean System.String::op_Equality(System.String,System.String)");
var changes = new List<string>();

MethodDef Target(string typeName, string methodName) {
    var method = module.GetTypes().Single(t => t.FullName == typeName).Methods.Single(m => m.Name == methodName);
    if (!method.HasBody || method.Body.ExceptionHandlers.Count != 0 || method.MethodSig.RetType.ElementType != ElementType.Void)
        throw new InvalidOperationException("Unexpected guard method ABI/EH: " + method.FullName);
    if (method.Body.Instructions.Any(i => i.OpCode == OpCodes.Ldstr && ((string)i.Operand).StartsWith("暂未开放")))
        throw new InvalidOperationException("Unavailable guard already applied: " + method.FullName);
    return method;
}

List<Instruction> Notice(string text) => new List<Instruction> {
    Instruction.Create(OpCodes.Ldstr,text), Instruction.Create(OpCodes.Ldc_R4,0f),
    Instruction.Create(OpCodes.Call,popup), Instruction.Create(OpCodes.Ret)
};

void Prefix(MethodDef target, List<Instruction> instructions) {
    for (int i=0; i<instructions.Count; i++) target.Body.Instructions.Insert(i,instructions[i]);
    target.Body.MaxStack = Math.Max(target.Body.MaxStack,(ushort)3);
    target.Body.KeepOldMaxStack = true;
    target.Body.UpdateInstructionOffsets();
    changes.Add(target.DeclaringType.FullName+"::"+target.Name);
}

// Reject absent field assets before registering callbacks or mutating willTransferScene.
{
    var target = Target("ManagerController","TrySceneTransfer");
    var original = target.Body.Instructions[0];
    var notice = Notice("暂未开放：该野外地图的原场景资源尚未补齐");
    var prefix = new List<Instruction>();
    foreach (string field in settings.GetProperty("unavailableFieldIDs").EnumerateArray().Select(x=>x.GetString()!)) {
        prefix.Add(Instruction.Create(OpCodes.Ldarg_1));
        prefix.Add(Instruction.Create(OpCodes.Ldstr,field));
        prefix.Add(Instruction.Create(OpCodes.Call,equality));
        prefix.Add(Instruction.Create(OpCodes.Brtrue,notice[0]));
    }
    prefix.Add(Instruction.Create(OpCodes.Br,original)); prefix.AddRange(notice); Prefix(target,prefix);
}

// The current qualified catalogue is frozen from the real enabled rows, so
// existing early content remains available and the next missing floor never loads.
{
    var target = Target("UI.RushPage","OnFightBtnClick");
    var floor = target.DeclaringType.Fields.Single(f=>f.Name=="currentFloor");
    var prefix = new List<Instruction> {Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldfld,floor),
        Instruction.Create(OpCodes.Ldc_I4,maxFloor),Instruction.Create(OpCodes.Ble,target.Body.Instructions[0])};
    prefix.AddRange(Notice("暂未开放：后续试炼楼层配置尚未恢复"));Prefix(target,prefix);
}
{
    var target = Target("UI.RushFightReadyItem","OnFightBtnClick");
    var floor = module.GetTypes().Single(t=>t.FullName=="StaticData").Fields.Single(f=>f.Name=="rushCurFloorNum");
    var prefix = new List<Instruction> {Instruction.Create(OpCodes.Ldsfld,floor),Instruction.Create(OpCodes.Ldc_I4,maxFloor),
        Instruction.Create(OpCodes.Ble,target.Body.Instructions[0])};
    prefix.AddRange(Notice("暂未开放：后续试炼楼层配置尚未恢复"));Prefix(target,prefix);
}
{
    var target = Target("UI.RushEliteBossPage","OnFightBtnClick");
    var selected = target.DeclaringType.Fields.Single(f=>f.Name=="currentSelectBossId");
    var notice = Notice("暂未开放：该精英首领的原战斗配置尚未恢复");
    var prefix = new List<Instruction>();
    foreach (string boss in deniedElite) {
        prefix.Add(Instruction.Create(OpCodes.Ldarg_0));prefix.Add(Instruction.Create(OpCodes.Ldfld,selected));
        prefix.Add(Instruction.Create(OpCodes.Ldstr,boss));prefix.Add(Instruction.Create(OpCodes.Call,equality));
        prefix.Add(Instruction.Create(OpCodes.Brtrue,notice[0]));
    }
    prefix.Add(Instruction.Create(OpCodes.Br,target.Body.Instructions[0]));prefix.AddRange(notice);Prefix(target,prefix);
}
{
    var target = Target("UI.NPCUIBuildingPage","SendCSMsg");
    Prefix(target,Notice("暂未开放：公会战建筑升级配置尚未恢复"));
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine(string.Join("\n",changes));
