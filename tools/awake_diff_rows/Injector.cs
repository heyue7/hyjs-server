using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input/output DLL required");
using var module = ModuleDefMD.Load(args[0]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
FieldDef F(string type, string name) => T(type).Fields.Single(f => f.Name == name);
MethodDef M(string type, string name) => T(type).Methods.Single(m => m.Name == name);
MemberRef R(string type, string name) => module.GetMemberRefs().First(m => m.DeclaringType.FullName == type && m.Name == name);
var diff = M("AwakeDiffItem", "SetDiffItem");
var page = M("ChallengeActivityCopyPage", "SetSecondPage");
if (diff.Body.ExceptionHandlers.Count != 0 || page.Body.ExceptionHandlers.Count != 0 ||
    diff.Body.Variables.Count != 1 || page.Body.Variables.Count != 2 ||
    diff.Parameters.Count != 6 || page.Parameters.Count != 2)
    throw new InvalidOperationException("Unexpected original two-method ABI/locals/EH");
var getObject = R("UnityEngine.Component", "get_gameObject");
var setActive = R("UnityEngine.GameObject", "SetActive");

// Private task/page fields are reset inside their own declaring type.
var active = Instruction.Create(OpCodes.Ldarg_0);
var prefix = new List<Instruction> {
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Brtrue, active),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldnull),
    Instruction.Create(OpCodes.Stfld, F("AwakeDiffItem", "taskInfo")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldnull),
    Instruction.Create(OpCodes.Stfld, F("AwakeDiffItem", "page")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Callvirt, getObject),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Callvirt, setActive),
    Instruction.Create(OpCodes.Ret), active,
    Instruction.Create(OpCodes.Callvirt, getObject), Instruction.Create(OpCodes.Ldc_I4_1),
    Instruction.Create(OpCodes.Callvirt, setActive)
};
for (int i = 0; i < prefix.Count; i++) diff.Body.Instructions.Insert(i, prefix[i]);

// Clear every slot before the unchanged original loop binds this chapter's tasks.
var index = new Local(module.CorLibTypes.Int32);
page.Body.Variables.Add(index);
var loop = Instruction.Create(OpCodes.Ldarg_0);
var check = Instruction.Create(OpCodes.Ldloc, index);
var rows = F("ChallengeActivityCopyPage", "AwakeDiffTab");
var empty = R("System.String", "Empty");
var clear = new List<Instruction> {
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Stloc, index),
    Instruction.Create(OpCodes.Br, check), loop,
    Instruction.Create(OpCodes.Ldfld, rows), Instruction.Create(OpCodes.Ldloc, index),
    Instruction.Create(OpCodes.Ldelem_Ref), Instruction.Create(OpCodes.Ldnull),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Ldsfld, empty),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Ldarg_0),
    Instruction.Create(OpCodes.Callvirt, diff), Instruction.Create(OpCodes.Ldloc, index),
    Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Add),
    Instruction.Create(OpCodes.Stloc, index), check,
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, rows),
    Instruction.Create(OpCodes.Ldlen), Instruction.Create(OpCodes.Conv_I4),
    Instruction.Create(OpCodes.Blt, loop)
};
for (int i = 0; i < clear.Count; i++) page.Body.Instructions.Insert(i, clear[i]);
foreach (var method in new[] { diff, page }) {
    method.Body.MaxStack = 12;
    method.Body.KeepOldMaxStack = true;
    method.Body.UpdateInstructionOffsets();
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Changed only AwakeDiffItem.SetDiffItem and ChallengeActivityCopyPage.SetSecondPage prefixes");
