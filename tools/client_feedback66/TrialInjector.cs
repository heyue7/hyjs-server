using System;
using System.Linq;
using System.Collections.Generic;
using System.IO;
using System.Security.Cryptography;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using dnlib.PE;

// Invoke against the exact feedback65-r4 Main first; later independent overlays may consume its output.
if (args.Length != 3) throw new ArgumentException("input Main, Trial helper, output Main required");
byte[] raw = File.ReadAllBytes(args[0]);
string Hash(byte[] bytes) => Convert.ToHexString(SHA256.HashData(bytes)).ToLowerInvariant();
if (Hash(raw) != "5842a437f402d73dd209b85960bde78ce750150bf79719bc766d2b6d6bc6593d")
    throw new Exception("Feedback65-r4 Main SHA drift");
using var main = ModuleDefMD.Load(raw);
using var helper = ModuleDefMD.Load(args[1]);
var hooks = helper.GetTypes().Single(t => t.FullName == "HYJS.Feedback66.TrialRuntime");
var importer = new Importer(main);
MethodDef Target(uint token) => (MethodDef)main.ResolveToken(token);
var pins = new Dictionary<uint, (string Type, string Name, string SHA)> {
    [0x06005e8b] = ("RushPage", "OnFightBtnClick", "dfea5f0cae19d03e8f2d50f5ee54b5aee64e1c0744bf4a97a48179c49c9fcef8"),
    [0x06005e71] = ("RushFightReadyItem", "OnFightBtnClick", "c32b9aaa935f1f30678e4265af6c0512db211ff8c027310e6593ba5d61730e1c"),
    [0x06005e66] = ("RushEliteBossPage", "OnFightBtnClick", "7749b2ded65dc54eeec153a922487f7e5b5f14d35630e21e6c6b6b79b9b16621"),
    [0x06006cfe] = ("<OnFightBtnClick>c__AnonStorey1A7", "<>m__219", "24f8054042f41ef4071c36b6979490442115bcf37078e956de2bf1e8a1f9f630"),
    [0x06005e6c] = ("RushEliteBossPage", "<OnFightBtnClick>m__218", "dbf3c549462666cf8b9530d0829fad32cc4a4b584c4c7e3a93a2a55c12611367"),
    [0x06002b53] = ("RushBattleOver", "AskRushFightResult", "7b0a84ab3eab65b2129ab490cc97f60b85bc665b9a4250acd088126432c26b37"),
    [0x06002b59] = ("RushBossBattleOver", "AskRushFightResult", "d0ea91136f24a3c8d6211c4102926a58090652b1fc2d6654293a064197bfbc2d"),
    [0x06002b55] = ("RushBattleOver", "<AskRushFightResult>m__C", "8bc537694cc9f848540f7a36943e187e7b9bf3c55e8249c185bc32cda9e05dd6"),
    [0x06002b5b] = ("RushBossBattleOver", "<AskRushFightResult>m__D", "53133b118114aa504dedf7dcddd03ea1e8a6124a299be3515679f48e16fd2487"),
    [0x06003ddc] = ("HTTPManager", "HttpError", "d4aaee96c017d5e5cff680c2e61b81893cd5a702b46acfc95cc36691df1e2972")
};
foreach (var pair in pins)
{
    var method = Target(pair.Key);
    int offset = checked((int)(uint)main.Metadata.PEImage.ToFileOffset(method.RVA));
    int flags = BitConverter.ToUInt16(raw, offset);
    int length = (raw[offset] & 3) == 2 ? 1 + (raw[offset] >> 2) : ((flags >> 12) * 4) + checked((int)BitConverter.ToUInt32(raw, offset + 4));
    if ((raw[offset] & 3) != 2 && (flags & 8) != 0) throw new Exception("Unexpected target extra section");
    if (method.DeclaringType.Name != pair.Value.Type || method.Name != pair.Value.Name ||
        method.Body.ExceptionHandlers.Count != 0 || Hash(raw.Skip(offset).Take(length).ToArray()) != pair.Value.SHA)
        throw new Exception("Trial target body drift: " + pair.Key.ToString("x8"));
}
IMethod Hook(string name, string ret, params string[] parameters)
{
    var method = hooks.Methods.Single(m => m.Name == name);
    if (!method.IsStatic || method.MethodSig.RetType.FullName != ret ||
        !method.MethodSig.Params.Select(p => p.FullName).SequenceEqual(parameters))
        throw new Exception("Trial hook ABI drift: " + name);
    // Importer.Import(method) mis-scopes this circular reference: HttpHandler is
    // defined by Main, while the helper references Main. Bind native signature
    // types directly to Main's existing TypeDef instead of a helper ModuleRef.
    TypeSig SignatureType(string fullName) => fullName switch {
        "System.Void" => main.CorLibTypes.Void,
        "System.Object" => main.CorLibTypes.Object,
        "System.Boolean" => main.CorLibTypes.Boolean,
        "System.Int32" => main.CorLibTypes.Int32,
        "NetWork.Layer.HttpHandler" => main.GetTypes().Single(t => t.FullName == fullName).ToTypeSig(),
        _ => throw new Exception("Unreviewed hook signature type: " + fullName)
    };
    var signature = MethodSig.CreateStatic(SignatureType(ret), parameters.Select(SignatureType).ToArray());
    var imported = new MemberRefUser(main, method.Name, signature, importer.Import(hooks));
    foreach (var type in signature.Params.Concat(new[] { signature.RetType })) {
        if (type.FullName != "NetWork.Layer.HttpHandler") continue;
        if (type is not TypeDefOrRefSig native || native.TypeDefOrRef is not TypeDef definition || definition.Module != main)
            throw new Exception("Native callback signature must use Main TypeDef");
    }
    return imported;
}
void Insert(MethodDef method, Instruction before, IEnumerable<Instruction> items)
{
    int at = method.Body.Instructions.IndexOf(before);
    foreach (var item in items) method.Body.Instructions.Insert(at++, item);
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)6);
}
void Guard(uint token, string name, int mode, bool callback)
{
    var method = Target(token);
    var first = method.Body.Instructions[0];
    var prefix = new List<Instruction> { Instruction.Create(OpCodes.Ldarg_0) };
    if (callback) { prefix.Add(Instruction.Create(OpCodes.Ldarg_1)); prefix.Add(Instruction.Create(OpCodes.Ldarg_2)); }
    prefix.Add(Instruction.Create(OpCodes.Ldc_I4, mode));
    prefix.Add(Instruction.Create(OpCodes.Call, callback
        ? Hook(name, "System.Boolean", "System.Object", "System.Object", "System.Boolean", "System.Int32")
        : Hook(name, "System.Boolean", "System.Object", "System.Int32")));
    prefix.Add(Instruction.Create(OpCodes.Brfalse, first));
    prefix.Add(Instruction.Create(OpCodes.Ret));
    Insert(method, first, prefix);
}
void RequestAndWrap(uint token, int mode, int opcode, string capture, string wrap, string sent)
{
    var method = Target(token);
    var marker = method.Body.Instructions.Single(i => i.OpCode == OpCodes.Ldc_I4 && (int)i.Operand == opcode);
    Insert(method, marker, new[] {
        Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldloc, method.Body.Variables[0]),
        Instruction.Create(OpCodes.Ldc_I4, mode), Instruction.Create(OpCodes.Call, Hook(capture, "System.Void", "System.Object", "System.Object", "System.Int32"))
    });
    var send = method.Body.Instructions.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName == "NetAPI" && m.Name == "SendHttp");
    Insert(method, send, new[] { Instruction.Create(OpCodes.Call, Hook(wrap, "NetWork.Layer.HttpHandler", "NetWork.Layer.HttpHandler")) });
    int after = method.Body.Instructions.IndexOf(send) + 1;
    method.Body.Instructions.Insert(after, Instruction.Create(OpCodes.Call, Hook(sent, "System.Boolean", "System.Boolean")));
}
Guard(0x06005e71, "Click", 1, false);
Guard(0x06005e66, "Click", 2, false);
Guard(0x06005e8b, "PageClick", 1, false);
RequestAndWrap(0x06005e71, 1, 54501, "Request", "WrapStart", "StartSent");
RequestAndWrap(0x06005e66, 2, 54501, "Request", "WrapStart", "StartSent");
Guard(0x06006cfe, "Start", 1, true);
Guard(0x06005e6c, "Start", 2, true);
Guard(0x06002b53, "BeginEnd", 1, false);
Guard(0x06002b59, "BeginEnd", 2, false);
RequestAndWrap(0x06002b53, 1, 54503, "CaptureEnd", "WrapEnd", "EndSent");
RequestAndWrap(0x06002b59, 2, 54503, "CaptureEnd", "WrapEnd", "EndSent");
Guard(0x06002b55, "Result", 1, true);
Guard(0x06002b5b, "Result", 2, true);
var transport = Target(0x06003ddc);
Insert(transport, transport.Body.Instructions[0], new[] {
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2), Instruction.Create(OpCodes.Ldarg_3),
    Instruction.Create(OpCodes.Call, Hook("TransportError", "System.Void", "System.Object", "System.Object", "System.Int32"))
});
foreach (var method in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody)) method.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
Console.WriteLine("Trial/Elite recovery: 10 pinned methods; original instructions, branches, locals and engine bodies retained");
