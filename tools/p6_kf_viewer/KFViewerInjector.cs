using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

// Local non-engine wrapper compatibility. Root owns packaging and phone work.
if (args.Length != 2) throw new ArgumentException("input and output required");
using var module = ModuleDefMD.Load(args[0]);
var owner = module.GetTypes().Single(t => t.Name == "BattleData");
var target = owner.Methods.Single(m => m.Name == "SetData" && m.MethodSig.Params.Count == 1 && m.MethodSig.Params[0].FullName == "ProtoBuf.Message.KFReplayData");
if (!target.HasBody || target.Body.ExceptionHandlers.Count != 0)
    throw new InvalidOperationException("Expected the original KFReplayData wrapper without EH");
var code = target.Body.Instructions;
var pid = code.Single(i => i.Operand is IMethod m && m.Name == "get_playerId");
int at = code.IndexOf(pid);
if (at < 4 || code[at - 4].OpCode != OpCodes.Ldarg_1 || code[at - 3].Operand is not IMethod getList || getList.Name != "get_playerList" ||
    code[at - 2].OpCode != OpCodes.Ldloc_3 || code[at - 1].Operand is not IMethod getItem || getItem.Name != "get_Item" ||
    code[at + 1].OpCode != OpCodes.Ldsfld || code[at + 1].Operand is not IField staticPlayer ||
    code[at + 2].OpCode != OpCodes.Ldfld || code[at + 2].Operand is not IField localPID || localPID.Name != "playerId" ||
    code[at + 3].Operand is not Instruction next || next.OpCode != OpCodes.Ldloc_3)
    throw new InvalidOperationException("Original PID comparison/counter shape changed");
var originalStart = code[at - 4];
var getPID = (IMethod)pid.Operand;
var count = code.Select(i => i.Operand).OfType<IMethod>().Single(m => m.Name == "get_Count");
var getServer = module.GetTypes().Where(t => t.HasMethods).SelectMany(t => t.Methods).Where(m => m.HasBody)
    .SelectMany(m => m.Body.Instructions).Select(i => i.Operand).OfType<IMethod>()
    .First(m => m.Name == "get_serverId" && m.DeclaringType.FullName == "ProtoBuf.Message.BattlePlayer");
var zone = module.GetTypes().Single(t => t.FullName == "UI.UIPlayerData").Methods.Single(m => m.Name == "get_ZoneId");
var guard = new List<Instruction>();
void EmitList() { guard.Add(Instruction.Create(OpCodes.Ldarg_1)); guard.Add(Instruction.Create(OpCodes.Callvirt, getList)); }
void Player(int index) { EmitList(); guard.Add(Instruction.CreateLdcI4(index)); guard.Add(Instruction.Create(OpCodes.Callvirt, getItem)); }
void Server(int index) { Player(index); guard.Add(Instruction.Create(OpCodes.Callvirt, getServer)); }
void OriginalIf(OpCode op) { guard.Add(Instruction.Create(op, originalStart)); }
// Only a two-person duplicate PID across two distinct positive realms enters
// compatibility. Every other original input retains the exact original loop.
EmitList(); guard.Add(Instruction.Create(OpCodes.Callvirt, count)); guard.Add(Instruction.CreateLdcI4(2)); OriginalIf(OpCodes.Bne_Un);
Player(0); guard.Add(Instruction.Create(OpCodes.Callvirt, getPID)); Player(1); guard.Add(Instruction.Create(OpCodes.Callvirt, getPID)); OriginalIf(OpCodes.Bne_Un);
Server(0); guard.Add(Instruction.CreateLdcI4(0)); OriginalIf(OpCodes.Ble);
Server(1); guard.Add(Instruction.CreateLdcI4(0)); OriginalIf(OpCodes.Ble);
Server(0); Server(1); OriginalIf(OpCodes.Beq);
guard.Add(Instruction.Create(OpCodes.Ldsfld, staticPlayer)); guard.Add(Instruction.Create(OpCodes.Callvirt, zone)); guard.Add(Instruction.CreateLdcI4(0)); OriginalIf(OpCodes.Ble);
// The existing PID branch still runs for the candidate from the current realm.
EmitList(); guard.Add(Instruction.Create(OpCodes.Ldloc_3)); guard.Add(Instruction.Create(OpCodes.Callvirt, getItem)); guard.Add(Instruction.Create(OpCodes.Callvirt, getServer));
guard.Add(Instruction.Create(OpCodes.Ldsfld, staticPlayer)); guard.Add(Instruction.Create(OpCodes.Callvirt, zone)); guard.Add(Instruction.Create(OpCodes.Bne_Un, next));
// Repeat the guard on the original loop back-edge. Newly inserted bypass
// branches intentionally enter the preserved original PID comparison directly.
foreach (var instruction in code) if (instruction.Operand == originalStart) instruction.Operand = guard[0];
int insert = code.IndexOf(originalStart);
foreach (var instruction in guard) code.Insert(insert++, instruction);
target.Body.KeepOldMaxStack = false;
var options = new ModuleWriterOptions(module); options.MetadataOptions.Flags |= MetadataFlags.PreserveAll;
module.Write(args[1], options);
Console.WriteLine("Only local KFReplayData duplicate-PID realm selection changed; token=" + target.MDToken.Raw.ToString("x8"));
