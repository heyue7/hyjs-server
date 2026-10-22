using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using System;
using System.Linq;

using var module = ModuleDefMD.Load(args[0]);
var reference = new AssemblyRefUser(new AssemblyNameInfo("HYJS.Gameplay, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var api = new TypeRefUser(module, "HYJS.Gameplay", "Preferences", reference);
var battleData = module.GetTypes().Single(t => t.FullName == "BattleData");
var restore = new MemberRefUser(module, "Restore", MethodSig.CreateStatic(module.CorLibTypes.Void), api);
var remember = new MemberRefUser(module, "RememberClick", MethodSig.CreateStatic(module.CorLibTypes.Void), api);
var map = new MemberRefUser(module, "ApplySnowBattleMap", MethodSig.CreateStatic(module.CorLibTypes.Void, battleData.ToTypeSig()), api);

void AppendAtReturns(MethodDef method, IMethod call, bool instance)
{
    if (!method.HasBody || method.Body.Instructions.Any(i => i.Operand is IMethod m && m.DeclaringType.FullName == "HYJS.Gameplay.Preferences"))
        throw new InvalidOperationException("Unexpected existing hook: " + method.FullName);
    foreach (var ret in method.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
    {
        int index = method.Body.Instructions.IndexOf(ret);
        // Keep the original ret object as the hook entry, so original branches
        // to a shared return also execute the hook without changing targets.
        ret.OpCode = instance ? OpCodes.Ldarg_0 : OpCodes.Call;
        ret.Operand = instance ? null : call;
        if (instance) method.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Call, call));
        method.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ret));
    }
    method.Body.KeepOldMaxStack = true;
}

AppendAtReturns(module.GetTypes().Single(t => t.FullName == "Battle.SpeedBtn").Methods.Single(m => m.Name == "OnClick"), remember, false);
AppendAtReturns(module.GetTypes().Single(t => t.FullName == "Battle.View.EntranceState").Methods.Single(m => m.Name == "Leave"), restore, false);
foreach (string parameter in new[] { "ProtoBuf.Message.SCSidePveFightStart", "ProtoBuf.Message.SCSidePveBossFightStart" })
{
    var method = battleData.Methods.Single(m => m.Name == "SetData" && m.MethodSig.Params.Count == 1 && m.MethodSig.Params[0].FullName == parameter);
    AppendAtReturns(method, map, true);
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
