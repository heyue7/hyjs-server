using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 5) throw new ArgumentException("input, bridge, output, local origin, enabled required");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
var target = module.GetTypes().Single(t => t.FullName == "Battle.FieldPVEWorldBossBattleOver").Methods.Single(m => m.Name == "End");
var impl = bridge.GetTypes().Single(t => t.Name == "LocalWorldBossSettlement");
if (target.Body == null || target.Body.ExceptionHandlers.Count != 0 || target.Parameters.Count != 1 ||
    target.MethodSig.RetType.ElementType != ElementType.Void || target.Body.Instructions.Count == 0)
    throw new InvalidOperationException("Original world boss End shape changed");
if (target.Body.Instructions.Any(i => i.Operand is IMethod m && m.DeclaringType.FullName == "LocalWorldBossSettlement"))
    throw new InvalidOperationException("World boss settlement bridge was already injected");
var originalEntry = target.Body.Instructions[0];
var prefix = new[] {
    Instruction.Create(OpCodes.Ldstr, args[3]),
    Instruction.Create(args[4] == "true" ? OpCodes.Ldc_I4_1 : OpCodes.Ldc_I4_0),
    Instruction.Create(OpCodes.Call, module.Import(impl.Methods.Single(m => m.Name == "Configure"))),
    Instruction.Create(OpCodes.Ldarg_0),
    Instruction.Create(OpCodes.Call, module.Import(impl.Methods.Single(m => m.Name == "TryFinish"))),
    Instruction.Create(OpCodes.Brfalse, originalEntry),
    Instruction.Create(OpCodes.Ret)
};
for (int i = 0; i < prefix.Length; i++) target.Body.Instructions.Insert(i, prefix[i]);
target.Body.KeepOldMaxStack = false;
var position = module.GetTypes().Single(t => t.FullName == "Room.RoomNetField").Methods.Single(m => m.Name == "SCCurrentPosition");
if (position.Body == null || position.Body.ExceptionHandlers.Count != 0 || position.Parameters.Count != 2 ||
    position.MethodSig.RetType.ElementType != ElementType.Void)
    throw new InvalidOperationException("Original field current-position callback shape changed");
var positionReturns = position.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray();
if (positionReturns.Length != 1 || position.Body.Instructions.Last() != positionReturns[0])
    throw new InvalidOperationException("Original field ready callback return shape changed");
var recoveryHook = module.Import(impl.Methods.Single(m => m.Name == "ObserveFieldReady"));
foreach (var ret in positionReturns)
{
    int at = position.Body.Instructions.IndexOf(ret);
    var observer = new[] {
        Instruction.Create(OpCodes.Ldstr, args[3]),
        Instruction.Create(args[4] == "true" ? OpCodes.Ldc_I4_1 : OpCodes.Ldc_I4_0),
        Instruction.Create(OpCodes.Call, module.Import(impl.Methods.Single(m => m.Name == "Configure"))),
        Instruction.Create(OpCodes.Ldarg_0),
        Instruction.Create(OpCodes.Ldarg_1),
        Instruction.Create(OpCodes.Call, recoveryHook)
    };
    foreach (var branch in position.Body.Instructions) if (branch.Operand == ret) branch.Operand = observer[0];
    for (int i = 0; i < observer.Length; i++) position.Body.Instructions.Insert(at + i, observer[i]);
}
position.Body.KeepOldMaxStack = false;
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Local world boss End gate and field-ready observer injected; enabled=" + args[4]);
