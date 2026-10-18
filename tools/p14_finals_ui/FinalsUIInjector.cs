using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

// Proposal only. Root owns controlled input SHA, verification, APK build/install.
if (args.Length != 2) throw new ArgumentException("input and output required");
using var module = ModuleDefMD.Load(args[0]);
var finals = module.GetTypes().Single(t => t.FullName == "UI.ServerBattleFinals");
var init = finals.Methods.Single(m => m.Name == "InitData");
var itemType = module.GetTypes().Single(t => t.FullName == "UI.ServerBattlePlayerUIItem");
var owner = itemType.Methods.Single(m => m.Name == "isOwner");
if (init.Body.ExceptionHandlers.Count != 0 || owner.Body.ExceptionHandlers.Count != 0)
    throw new InvalidOperationException("Unexpected target EH; stop without rewriting");

// Preserve every original target instruction. Bounds-check only the first-list
// read; original InitPlayerInfo(null,...) already draws its genuine empty slot.
var code = init.Body.Instructions;
var first = code.Single(i => i.Operand is IField f && f.Name == "firstList");
int at = code.IndexOf(first);
if (first.OpCode != OpCodes.Ldfld || code[at - 1].OpCode != OpCodes.Ldarg_0 ||
    code[at + 1].OpCode != OpCodes.Ldloc_0 || code[at + 2].OpCode != OpCodes.Callvirt ||
    code[at + 2].Operand is not IMethod getItem || getItem.Name != "get_Item")
    throw new InvalidOperationException("Original Finals firstList index shape changed");
var count = new MemberRefUser(module, "get_Count", MethodSig.CreateInstance(module.CorLibTypes.Int32), getItem.DeclaringType);
var missing = Instruction.Create(OpCodes.Pop);
var after = code[at + 3];
var prefix = new[] {
    Instruction.Create(OpCodes.Dup), Instruction.Create(OpCodes.Brfalse, missing),
    Instruction.Create(OpCodes.Dup), Instruction.Create(OpCodes.Callvirt, count),
    Instruction.Create(OpCodes.Ldloc_0), Instruction.Create(OpCodes.Ble, missing)
};
for (int i = 0; i < prefix.Length; i++) code.Insert(at + 1 + i, prefix[i]);
int end = code.IndexOf(after);
code.Insert(end++, Instruction.Create(OpCodes.Br, after));
code.Insert(end++, missing);
code.Insert(end, Instruction.Create(OpCodes.Ldnull));

// randItem is assigned in original InitPlayerInfo before isOwner is called.
// Preserve the original pid comparison, and reject another realm first.
var owns = owner.Body.Instructions;
var falseResult = owns.Single(i => i.OpCode == OpCodes.Ldc_I4_0);
if (owns[owns.IndexOf(falseResult) + 1].OpCode != OpCodes.Ret)
    throw new InvalidOperationException("Original owner false return shape changed");
var originalStart = owns[0];
var player = owns.Select(i => i.Operand).OfType<IField>().Single(f => f.Name == "playerData");
var rand = itemType.Fields.Single(f => f.Name == "randItem");
var info = itemType.Methods.Single(m => m.Name == "InitPlayerInfo");
var getServer = info.Body.Instructions.Select(i => i.Operand).OfType<IMethod>().First(m => m.Name == "get_serverId");
var zone = module.GetTypes().Single(t => t.FullName == "UI.UIPlayerData").Methods.Single(m => m.Name == "get_ZoneId");
var identity = new[] {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, rand),
    Instruction.Create(OpCodes.Brfalse, falseResult),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, rand),
    Instruction.Create(OpCodes.Callvirt, getServer),
    Instruction.Create(OpCodes.Ldsfld, player), Instruction.Create(OpCodes.Callvirt, zone),
    Instruction.Create(OpCodes.Bne_Un, falseResult)
};
foreach (var instruction in owns) if (instruction.Operand == originalStart) instruction.Operand = identity[0];
for (int i = 0; i < identity.Length; i++) owns.Insert(i, identity[i]);
init.Body.KeepOldMaxStack = false;
owner.Body.KeepOldMaxStack = false;
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll;
module.Write(args[1], options);
Console.WriteLine("Only Finals.InitData safe missing-slot and PlayerUIItem.isOwner joint identity changed");
