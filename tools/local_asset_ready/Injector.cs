using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input, bridge, output required");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
var adapter = bridge.GetTypes().Single(t => t.FullName == "HYJS.LocalAssets.LocalAssetReady");
foreach (var entry in new[] {
    ("Room.RCUINormalNPC", "OpenPage", "CatalogueReady"),
    ("AwakeDiffItem", "OnChallengeClick", "EntryReady") })
{
    var target = module.GetTypes().Single(t => t.FullName == entry.Item1)
        .Methods.Single(m => m.Name == entry.Item2);
    if (!target.HasBody || target.IsStatic) throw new InvalidOperationException("Original gate shape changed");
    var instructions = target.Body.Instructions;
    if (instructions.Any(i => i.Operand is IMethod m && m.DeclaringType.FullName == adapter.FullName))
        throw new InvalidOperationException("Resource readiness gate already installed");
    var gates = instructions.Where(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m &&
        m.DeclaringType.FullName == "GameConfig" && m.Name == "get_IsAssetFull" &&
        m.MethodSig.Params.Count == 0 && m.MethodSig.RetType.ElementType == ElementType.Boolean).ToArray();
    if (gates.Length != 1) throw new InvalidOperationException("Expected exactly one original full gate");
    int at = instructions.IndexOf(gates[0]) + 1;
    if (instructions[at].OpCode != OpCodes.Brtrue && instructions[at].OpCode != OpCodes.Brtrue_S)
        throw new InvalidOperationException("Original gate branch changed");
    var original = instructions.ToArray();
    var load = Instruction.Create(OpCodes.Ldarg_0);
    var call = Instruction.Create(OpCodes.Call, module.Import(adapter.Methods.Single(m => m.Name == entry.Item3)));
    instructions.Insert(at, load); instructions.Insert(at + 1, call);
    if (!instructions.Where(i => i != load && i != call).SequenceEqual(original))
        throw new InvalidOperationException("Original instructions changed");
    target.Body.KeepOldMaxStack = false;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Installed only two scoped Awake501 readiness call sites; global full state unchanged");
