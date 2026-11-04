using System;
using System.IO;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3)
    throw new ArgumentException("input Assembly-CSharp, compiled GameplayFixes helper, output Assembly-CSharp required");
if (Path.GetFullPath(args[0]) == Path.GetFullPath(args[2]))
    throw new ArgumentException("input and output must be different files");

using var main = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
var targetType = main.GetTypes().Single(t => t.FullName == "UI.UnionWarehouseTabBox");
var method = targetType.Methods.Single(m => m.Name == "OpenBoxCallHandle");
var packetType = main.GetTypes().Single(t => t.FullName == "NetWork.Packets.Packet");
if (method.IsStatic || !method.HasBody || method.Body.ExceptionHandlers.Count != 0 ||
    method.MethodSig.RetType.FullName != "System.Void" ||
    !method.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "NetWork.Packets.Packet", "System.Boolean" }))
    throw new InvalidOperationException("OpenBoxCallHandle signature or body drift");

var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.GameplayFixes.GuildRewardFeedback");
var hook = helperType.Methods.Single(m => m.Name == "ShowGuildReward");
if (!hook.IsPublic || !hook.IsStatic || hook.MethodSig.RetType.FullName != "System.Void" ||
    !hook.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "NetWork.Packets.Packet", "System.Boolean" }))
    throw new InvalidOperationException("ShowGuildReward signature drift");

var instructions = method.Body.Instructions;
if (instructions.Any(i => i.Operand is IMethod m &&
    m.DeclaringType.FullName == helperType.FullName && m.Name == hook.Name))
    throw new InvalidOperationException("Guild reward feedback is already injected");
var matches = instructions.Where(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m &&
    m.DeclaringType.FullName == "UI.UIPlayerData" && m.Name == "AddResultObj").ToArray();
if (matches.Length != 1)
    throw new InvalidOperationException("Expected one original AddResultObj call");
var apply = (IMethod)matches[0].Operand;
if (apply.MethodSig == null || apply.MethodSig.RetType.FullName != "System.Void" ||
    !apply.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "ProtoBuf.Message.ResultObject" }))
    throw new InvalidOperationException("AddResultObj signature drift");

// Build the circular-reference signature with the Packet TypeDef from Main.
var importer = new Importer(main);
var signature = MethodSig.CreateStatic(main.CorLibTypes.Void, packetType.ToTypeSig(), main.CorLibTypes.Boolean);
var imported = new MemberRefUser(main, hook.Name, signature, importer.Import(helperType));
int at = instructions.IndexOf(matches[0]) + 1;
instructions.Insert(at++, Instruction.Create(OpCodes.Ldarg_1));
instructions.Insert(at++, Instruction.Create(OpCodes.Ldarg_2));
instructions.Insert(at, Instruction.Create(OpCodes.Call, imported));
method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)2);

// Admission reads the actual WebClient state for normal sends and timeout resends.
var busyType = helper.GetTypes().Single(t => t.FullName == "HYJS.GameplayFixes.TransportBusy");
var busyHook = busyType.Methods.Single(m => m.Name == "IsSending");
if (!busyHook.IsPublic || !busyHook.IsStatic || busyHook.MethodSig.RetType.FullName != "System.Boolean" ||
    !busyHook.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "System.Object" }))
    throw new InvalidOperationException("TransportBusy signature drift");
var importedBusy = new MemberRefUser(main, busyHook.Name,
    MethodSig.CreateStatic(main.CorLibTypes.Boolean, main.CorLibTypes.Object), importer.Import(busyType));

var http = main.GetTypes().Single(t => t.FullName == "NetWork.Layer.HTTPManager");
var send = http.Methods.Single(m => m.Name == "Send" && m.MethodSig.Params.Count == 2 &&
    m.MethodSig.Params[0].FullName == "NetWork.Packets.Packet" &&
    m.MethodSig.Params[1].FullName == "NetWork.Layer.HttpHandler");
var sending = http.Fields.Single(f => f.Name == "sending" && f.FieldType.FullName == "System.Boolean");
var net = main.GetTypes().Single(t => t.FullName == "NetAPI");
var clean = net.Methods.Single(m => m.Name == "CleanNetBox");
var busy = net.Methods.Single(m => m.Name == "IsSendingHttp");
if (send.IsStatic || !send.HasBody || send.Body.ExceptionHandlers.Count != 0 ||
    send.MethodSig.RetType.FullName != "System.Boolean" || !clean.IsStatic ||
    clean.MethodSig.Params.Count != 0 || clean.MethodSig.RetType.FullName != "System.Void" ||
    !busy.IsStatic || !busy.HasBody || busy.MethodSig.Params.Count != 0 ||
    busy.MethodSig.RetType.FullName != "System.Boolean")
    throw new InvalidOperationException("HTTP send contract drift");

var sendIL = send.Body.Instructions;
if (sendIL.Count < 3 || sendIL[0].OpCode != OpCodes.Ldarg_0 || sendIL[1].OpCode != OpCodes.Ldfld ||
    sendIL[1].Operand is not IField gate || gate.FullName != sending.FullName || sendIL[2].OpCode != OpCodes.Brfalse)
    throw new InvalidOperationException("HTTP send admission drift");
var show = sendIL.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m &&
    m.DeclaringType.FullName == "NetAPI" && m.Name == "ShowNetWaitBox");
var encode = sendIL.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m &&
    m.DeclaringType.FullName == "NetWork.Packets.Factory.PacketFactory" && m.Name == "WebPB2Bytes");
var transport = sendIL.Single(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m &&
    m.DeclaringType.FullName == "NetWork.Layer.HttpUtil" && m.Name == "Send");
var failure = sendIL.Where((i, n) => n > sendIL.IndexOf(show) && i.OpCode == OpCodes.Ldc_I4_0 &&
    n + 1 < sendIL.Count && sendIL[n + 1].OpCode == OpCodes.Ret).Single();
foreach (var call in new[] { encode, transport })
{
    int branch = sendIL.IndexOf(call) + 1;
    if (branch >= sendIL.Count || sendIL[branch].OpCode != OpCodes.Brfalse ||
        !ReferenceEquals(sendIL[branch].Operand, failure))
        throw new InvalidOperationException("HTTP post-mask failure branch drift");
}
if (sendIL.Count(i => ReferenceEquals(i.Operand, failure)) != 2)
    throw new InvalidOperationException("Unexpected HTTP failure branch");
sendIL[1].OpCode = OpCodes.Call;
sendIL[1].Operand = importedBusy;
var cleanup = Instruction.Create(OpCodes.Call, clean);
foreach (var instruction in sendIL)
    if (ReferenceEquals(instruction.Operand, failure)) instruction.Operand = cleanup;
sendIL.Insert(sendIL.IndexOf(failure), cleanup);

// NetAPI owns its own http field; the helper receives that exact instance.
var busyIL = busy.Body.Instructions;
if (busyIL.Count != 3 || busyIL[0].OpCode != OpCodes.Ldsfld ||
    busyIL[0].Operand is not IField currentHttp || currentHttp.DeclaringType.FullName != "NetAPI" ||
    currentHttp.Name != "http" || busyIL[1].OpCode != OpCodes.Callvirt ||
    busyIL[1].Operand is not IMethod oldBusy || oldBusy.DeclaringType.FullName != http.FullName ||
    oldBusy.Name != "isSending" || busyIL[2].OpCode != OpCodes.Ret)
    throw new InvalidOperationException("NetAPI.IsSendingHttp body drift");
busyIL[1].OpCode = OpCodes.Call;
busyIL[1].Operand = importedBusy;

foreach (var body in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody))
    body.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
Console.WriteLine("Guild reward feedback and HTTP send admission/failure cleanup inserted");
