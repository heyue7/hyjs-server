using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input, bridge and output required");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
var implementation = bridge.GetTypes().Single(t => t.Name == "LocalWalletSessionBridge");
foreach (var entry in new[] { ("GameConfig", "Awake", "Install"),
                             ("LoginUserLogin", "UserLogin", "Pause"),
                             ("LoginUI", "OnSwitchAccountButtonClick", "Pause") })
{
    var target = module.GetTypes().Single(t => t.Name == entry.Item1).Methods.Single(m => m.Name == entry.Item2);
    if (target.Body == null || target.Body.ExceptionHandlers.Count != 0 || target.Body.Instructions.Count == 0)
        throw new InvalidOperationException("Wallet session entry shape changed: " + entry.Item1 + "::" + entry.Item2);
    if (target.Body.Instructions.Any(i => i.Operand is IMethod method && method.DeclaringType.Name == "LocalWalletSessionBridge"))
        throw new InvalidOperationException("Wallet session bridge already injected");
    target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Call,
        module.Import(implementation.Methods.Single(m => m.Name == entry.Item3))));
    target.Body.KeepOldMaxStack = false;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Wallet session bridge installed; original engine unchanged");
