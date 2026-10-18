using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input/output DLL required");
using var module = ModuleDefMD.Load(args[0]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string type, string name) => T(type).Methods.Single(m => m.Name == name);
FieldDef F(string name) => T("StaticData").Fields.Single(f => f.Name == name);
var callback=M("Battle.AwakeCopyBattleOver","OnAskAwakeCopyFightEndCallback");
if(callback.Parameters.Count!=3 || callback.Body.ExceptionHandlers.Count!=0)
    throw new InvalidOperationException("Original Awake callback ABI/EH differs");
var original=callback.Body.Instructions[0];
// A rejected/failed HTTP End carries no accepted battle result. Return through
// the original exploration room path, without calling onResult or inventing End.
var prefix=new List<Instruction> {
    Instruction.Create(OpCodes.Ldarg_2),Instruction.Create(OpCodes.Brtrue,original),
    Instruction.Create(OpCodes.Ldnull),Instruction.Create(OpCodes.Stsfld,F("lastBattleEndMsg")),
    Instruction.Create(OpCodes.Ldc_I4_0),Instruction.Create(OpCodes.Stsfld,F("isLastBattleWin")),
    Instruction.Create(OpCodes.Call,M("ManagerController","get_Instance")),
    Instruction.Create(OpCodes.Callvirt,M("ManagerController","BattleToAwakeCopy")),
    Instruction.Create(OpCodes.Ret)
};
for(int i=0;i<prefix.Count;i++)callback.Body.Instructions.Insert(i,prefix[i]);
callback.Body.MaxStack=4;callback.Body.KeepOldMaxStack=true;callback.Body.UpdateInstructionOffsets();
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("Added failure-only Awake End room recovery; accepted callback suffix preserved");
