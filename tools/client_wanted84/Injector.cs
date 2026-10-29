using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 4) throw new ArgumentException("input Main, Feedback65 helper, output Main and input SHA required");
byte[] raw = File.ReadAllBytes(args[0]);
string Hash(byte[] bytes) => Convert.ToHexString(SHA256.HashData(bytes)).ToLowerInvariant();
if (args[3].Length != 64 || Hash(raw) != args[3]) throw new Exception("Composed Main SHA drift");
using var main = ModuleDefMD.Load(raw);
using var helper = ModuleDefMD.Load(args[1]);
var hooks = helper.GetTypes().Single(t => t.FullName == "HYJS.Feedback65.Runtime");
var importer = new Importer(main);
MethodDef Target(uint token) => (MethodDef)main.ResolveToken(token);
foreach (var pin in new[] {
    (Token: 0x06002b6bu, Type: "Battle.WantedBattleOver", Name: "AskWantedFightResult", SHA: "3342c34c9bae5cced1305fe32cd96a6ab76810fc970bc4fd800b46a453619e4e"),
    (Token: 0x06005043u, Type: "UI.WantedMission", Name: "Show", SHA: "3ac5e074a911ac2c77da715b699a3f265d4cfd4b1d86ad9dc039ed39ed2eb0a5"),
    (Token: 0x06005044u, Type: "UI.WantedMission", Name: "OnMsgRecv", SHA: "00127b1c2a0bcfa337362bea71458f753f450d5ded38795f835cd14f62cd5c44"),
    (Token: 0x0600504eu, Type: "UI.WantedMission", Name: "SendEnterBattle", SHA: "d40aef1d6316eb46ab4436120ddfdf19ccbd37d7ea0cb7595cfe64c9c8b4fb87")
}) {
    var method = Target(pin.Token);
    int offset = checked((int)(uint)main.Metadata.PEImage.ToFileOffset(method.RVA));
    int flags = BitConverter.ToUInt16(raw, offset);
    int length = (raw[offset] & 3) == 2 ? 1 + (raw[offset] >> 2) : ((flags >> 12) * 4) + checked((int)BitConverter.ToUInt32(raw, offset + 4));
    if ((raw[offset] & 3) != 2 && (flags & 8) != 0) throw new Exception("Unexpected target extra section");
    if (method.DeclaringType.FullName != pin.Type || method.Name != pin.Name || method.Body.ExceptionHandlers.Count != 0 ||
        Hash(raw.Skip(offset).Take(length).ToArray()) != pin.SHA) throw new Exception("Wanted target body drift");
}
IMethod Hook(string name, string ret, params string[] parameters)
{
    var method = hooks.Methods.Single(m => m.Name == name);
    if (!method.IsStatic || method.MethodSig.RetType.FullName != ret ||
        !method.MethodSig.Params.Select(p => p.FullName).SequenceEqual(parameters)) throw new Exception("Wanted hook ABI drift: " + name);
    // The original callback type is defined in Main; it must never be imported
    // through the circular helper reference as a helper ModuleRef.
    TypeSig Native(string fullName) => fullName switch {
        "System.Void" => main.CorLibTypes.Void,
        "System.Object" => main.CorLibTypes.Object,
        "System.Boolean" => main.CorLibTypes.Boolean,
        "System.Int32" => main.CorLibTypes.Int32,
        "NetWork.Layer.HttpHandler" => main.GetTypes().Single(t => t.FullName == fullName).ToTypeSig(),
        _ => throw new Exception("Unreviewed Wanted hook signature: " + fullName)
    };
    return new MemberRefUser(main, method.Name,
        MethodSig.CreateStatic(Native(ret), parameters.Select(Native).ToArray()), importer.Import(hooks));
}
void Insert(MethodDef method, Instruction before, IEnumerable<Instruction> items)
{
    int at = method.Body.Instructions.IndexOf(before);
    foreach (var item in items) method.Body.Instructions.Insert(at++, item);
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)6);
}
var end = Target(0x06002b6b);
var marker = end.Body.Instructions.Single(i => i.OpCode == OpCodes.Ldc_I4 && (int)i.Operand == 55011);
Insert(end, marker, new[] {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldloc, end.Body.Variables[0]),
    Instruction.Create(OpCodes.Call, Hook("WantedCaptureEnd", "System.Void", "System.Object", "System.Object"))
});
var send = end.Body.Instructions.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName == "NetAPI" && m.Name == "SendHttp");
Insert(end, send, new[] { Instruction.Create(OpCodes.Call, Hook("WantedWrapEnd", "NetWork.Layer.HttpHandler", "NetWork.Layer.HttpHandler")) });
end.Body.Instructions.Insert(end.Body.Instructions.IndexOf(send) + 1,
    Instruction.Create(OpCodes.Call, Hook("WantedEndSent", "System.Boolean", "System.Boolean")));
var start = Target(0x0600504e);
var startMarker = start.Body.Instructions.Single(i => i.OpCode == OpCodes.Ldc_I4 && (int)i.Operand == 55003);
Insert(start, startMarker, new[] {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldloc, start.Body.Variables[0]),
    Instruction.Create(OpCodes.Call, Hook("WantedPrepareStart", "System.Boolean", "System.Object", "System.Object")),
    Instruction.Create(OpCodes.Brtrue, startMarker), Instruction.Create(OpCodes.Ret)
});
var startSend = start.Body.Instructions.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName == "NetAPI" && m.Name == "SendHttp");
Insert(start, startSend, new[] { Instruction.Create(OpCodes.Call, Hook("WantedWrapStart", "NetWork.Layer.HttpHandler", "NetWork.Layer.HttpHandler")) });
start.Body.Instructions.Insert(start.Body.Instructions.IndexOf(startSend) + 1,
    Instruction.Create(OpCodes.Call, Hook("WantedStartSent", "System.Boolean", "System.Boolean")));
var show = Target(0x06005043);
var finish = show.Body.Instructions.Single(i => i.OpCode == OpCodes.Ret);
Insert(show, finish, new[] {
    Instruction.Create(OpCodes.Ldarg_0),
    Instruction.Create(OpCodes.Call, Hook("WantedPageShown", "System.Void", "System.Object"))
});
var listResult = Target(0x06005044);
Insert(listResult, listResult.Body.Instructions[0], new[] {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2),
    Instruction.Create(OpCodes.Call, Hook("WantedListReceived", "System.Void", "System.Object", "System.Object", "System.Boolean"))
});
foreach (var method in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody)) method.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
Console.WriteLine("Wanted End capture, exact retry callback and page recovery applied");
