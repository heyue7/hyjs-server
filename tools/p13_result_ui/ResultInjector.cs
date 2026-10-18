using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

// Hide prefab placeholders before the original result binding activates real rows.
if (args.Length != 2) throw new ArgumentException("input and output required");
using var module = ModuleDefMD.Load(args[0]);
var getObject = module.GetMemberRefs().Single(m => m.DeclaringType.FullName == "UnityEngine.Component" && m.Name == "get_gameObject");
var setActive = module.GetMemberRefs().Single(m => m.DeclaringType.FullName == "UnityEngine.GameObject" && m.Name == "SetActive");
var result = module.GetTypes().Single(t => t.FullName == "UI.UIUnionFightBattleResult");

void Prepend(MethodDef method, List<Instruction> prefix) {
    if (method.Body == null)
        throw new InvalidOperationException("Unexpected original UI body: " + method.FullName);
    // Existing exception boundaries keep their original Instruction references;
    // the visibility prefix executes before the original protected region.
    for (int i = prefix.Count - 1; i >= 0; i--) method.Body.Instructions.Insert(0, prefix[i]);
    method.Body.InitLocals = true;
    method.Body.KeepOldMaxStack = false;
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)4);
}

foreach (var pair in new[] { ("SetPerson", "bestPerson"), ("SetAward", "awards") }) {
    var method = result.Methods.Single(m => m.Name == pair.Item1);
    var field = result.Fields.Single(f => f.Name == pair.Item2);
    if (!(field.FieldSig.Type is SZArraySig)) throw new InvalidOperationException("Expected original result array");
    var index = new Local(module.CorLibTypes.Int32);
    method.Body.Variables.Add(index);
    var check = Instruction.Create(OpCodes.Ldloc, index);
    var loop = Instruction.Create(OpCodes.Ldarg_0);
    var next = Instruction.Create(OpCodes.Ldloc, index);
    var empty = Instruction.Create(OpCodes.Pop);
    var p = new List<Instruction> {
        Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Stloc, index),
        Instruction.Create(OpCodes.Br, check), loop,
        Instruction.Create(OpCodes.Ldfld, field), Instruction.Create(OpCodes.Ldloc, index),
        Instruction.Create(OpCodes.Ldelem_Ref), Instruction.Create(OpCodes.Dup),
        Instruction.Create(OpCodes.Brfalse, empty),
        Instruction.Create(OpCodes.Callvirt, getObject), Instruction.Create(OpCodes.Ldc_I4_0),
        Instruction.Create(OpCodes.Callvirt, setActive), Instruction.Create(OpCodes.Br, next), empty, next,
        Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Add), Instruction.Create(OpCodes.Stloc, index),
        check, Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, field),
        Instruction.Create(OpCodes.Ldlen), Instruction.Create(OpCodes.Conv_I4), Instruction.Create(OpCodes.Blt, loop)
    };
    Prepend(method, p);
}
foreach (var pair in new[] { ("UI.UIUnionFightPlayerItem", "SetPlayerItem"), ("UI.UIUnionFightAwardItem", "SetItem") }) {
    var method = module.GetTypes().Single(t => t.FullName == pair.Item1).Methods.Single(m => m.Name == pair.Item2);
    Prepend(method, new List<Instruction> { Instruction.Create(OpCodes.Ldarg_0),
        Instruction.Create(OpCodes.Callvirt, getObject), Instruction.Create(OpCodes.Ldc_I4_1),
        Instruction.Create(OpCodes.Callvirt, setActive) });
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll;
module.Write(args[1], options);
Console.WriteLine("Patched four original guild result UI entries; protocol data binding preserved");
