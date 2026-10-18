using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

// A local numeric player ID is unique only together with its original realm.
if (args.Length != 2) throw new ArgumentException("input and output required");
using var module = ModuleDefMD.Load(args[0]);
var method = module.GetTypes().Single(t => t.FullName == "UI.ServerBattlePromotion").Methods.Single(m => m.Name == "UpdateList");
var code = method.Body.Instructions;
var pid = code.Single(i => i.Operand is IMethod m && m.DeclaringType.FullName == "ProtoBuf.Message.RankRowInfo" && m.Name == "get_pid");
int at = code.IndexOf(pid);
if (at < 1 || code[at - 1].OpCode != OpCodes.Ldloc_0 || code[at + 1].OpCode != OpCodes.Ldsfld ||
    code[at + 3].OpCode != OpCodes.Bne_Un || code[at + 3].Operand is not Instruction next)
    throw new InvalidOperationException("Original promotion self comparison shape changed");
var player = (IField)code[at + 1].Operand;
if (player.Name != "playerData") throw new InvalidOperationException("Original player identity field changed");
IMethod Read(string name) => code.Select(i => i.Operand).OfType<IMethod>().First(m => m.Name == name);
var values = Read("get_values");
var count = Read("get_Count");
var item = Read("get_Item");
var zone = module.GetTypes().Single(t => t.FullName == "UI.UIPlayerData").Methods.Single(m => m.Name == "get_ZoneId");
var start = code[at - 1];
var prefix = new[] {
    Instruction.Create(OpCodes.Ldloc_0), Instruction.Create(OpCodes.Callvirt, values),
    Instruction.Create(OpCodes.Callvirt, count), Instruction.Create(OpCodes.Ldc_I4_3),
    Instruction.Create(OpCodes.Blt, next),
    Instruction.Create(OpCodes.Ldloc_0), Instruction.Create(OpCodes.Callvirt, values),
    Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Callvirt, item),
    Instruction.Create(OpCodes.Ldsfld, player), Instruction.Create(OpCodes.Callvirt, zone),
    Instruction.Create(OpCodes.Bne_Un, next)
};
// Existing branches and exception regions retain their original instructions.
foreach (var instruction in code) if (instruction.Operand == start) instruction.Operand = prefix[0];
for (int i = 0; i < prefix.Length; i++) code.Insert(at - 1 + i, prefix[i]);
method.Body.KeepOldMaxStack = false;
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll;
module.Write(args[1], options);
Console.WriteLine("Promotion self labels now require original player ID and original realm ID");
