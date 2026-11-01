using System;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input Main, helper, output Main required");
byte[] bytes = File.ReadAllBytes(args[0]);
string Hash(byte[] value) => Convert.ToHexString(SHA256.HashData(value)).ToLowerInvariant();
if (Hash(bytes) != "3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a")
    throw new InvalidOperationException("Fixed 1.1.2 managed input differs");
using var main = ModuleDefMD.Load(bytes);
using var helper = ModuleDefMD.Load(args[1]);
void Require(bool value, string reason) { if (!value) throw new InvalidOperationException(reason); }
MethodDef Target(uint token, string owner, string name, string expected)
{
    var method = (MethodDef)main.ResolveToken(token);
    Require(method.DeclaringType.FullName == owner && method.Name == name && method.HasBody &&
            method.Body.ExceptionHandlers.Count == 0, "target identity differs: " + owner + "::" + name);
    int offset = checked((int)(uint)main.Metadata.PEImage.ToFileOffset(method.RVA));
    int flags = BitConverter.ToUInt16(bytes, offset);
    int length = (bytes[offset] & 3) == 2 ? 1 + (bytes[offset] >> 2) :
        ((flags >> 12) * 4) + checked((int)BitConverter.ToUInt32(bytes, offset + 4));
    Require((bytes[offset] & 3) == 2 || (flags & 8) == 0, "unexpected target exception section");
    Require(Hash(bytes.Skip(offset).Take(length).ToArray()) == expected, "target body differs: " + name);
    return method;
}
var open = Target(0x06004571, "Room.RCUIBox", "CopyOpenBox", "3baf463efd62b4538f2763abcd51177dfef061b8f5d1d73936bbdbce85aa2239");
var leave = Target(0x060044a3, "Room.RoomCopy", "Exit", "ee390fb8cae828e16683e463a9dc5670686ef8d0ce911c9ac0e6e03118f87ff4");
var send = Target(0x06003df4, "NetAPI", "SendHttp", "936753cd38339a2cc8f59855df331c2d5e963303f86113144921060f2d8b6c08");
Require(send.IsStatic && send.MethodSig.RetType.FullName == "System.Boolean" &&
    send.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "System.Int32", "System.Object", "NetWork.Layer.HttpHandler" }), "SendHttp ABI differs");
var callback = main.GetTypes().Single(t => t.FullName == "NetWork.Layer.HttpHandler").ToTypeSig();
var reference = new AssemblyRefUser(new AssemblyNameInfo("HYJS.CopyBoxSync, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var bridge = new TypeRefUser(main, "HYJS.CopyBoxSync", "Runtime", reference);
var wrapOpen = new MemberRefUser(main, "WrapOpen", MethodSig.CreateStatic(callback, callback), bridge);
var wrapLeave = new MemberRefUser(main, "WrapLeave", MethodSig.CreateStatic(callback, callback, main.CorLibTypes.Object), bridge);
var observe = new MemberRefUser(main, "ObserveSend", MethodSig.CreateStatic(main.CorLibTypes.Void, main.CorLibTypes.Int32, main.CorLibTypes.Object), bridge);
var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.CopyBoxSync.Runtime");
foreach (var expected in new[] { wrapOpen, wrapLeave, observe })
{
    var actual = helperType.Methods.Single(m => m.Name == expected.Name);
    Require(actual.IsPublic && actual.IsStatic && actual.MethodSig.RetType.FullName == expected.MethodSig.RetType.FullName &&
        actual.MethodSig.Params.Select(p => p.FullName).SequenceEqual(expected.MethodSig.Params.Select(p => p.FullName)), "helper ABI differs: " + expected.Name);
}
Instruction SendCall(MethodDef method) => method.Body.Instructions.Single(i => i.OpCode == OpCodes.Call &&
    i.Operand is IMethod target && target.MDToken.Raw == 0x06003df4);
var openCall = SendCall(open);
open.Body.Instructions.Insert(open.Body.Instructions.IndexOf(openCall), Instruction.Create(OpCodes.Call, wrapOpen));
var leaveCall = SendCall(leave);
int at = leave.Body.Instructions.IndexOf(leaveCall);
leave.Body.Instructions.Insert(at++, Instruction.Create(OpCodes.Ldarg_0));
leave.Body.Instructions.Insert(at, Instruction.Create(OpCodes.Call, wrapLeave));
send.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
send.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Ldarg_1));
send.Body.Instructions.Insert(2, Instruction.Create(OpCodes.Call, observe));
foreach (var method in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody)) method.Body.KeepOldMaxStack = true;
open.Body.MaxStack = Math.Max(open.Body.MaxStack, (ushort)4);
leave.Body.MaxStack = Math.Max(leave.Body.MaxStack, (ushort)4);
send.Body.MaxStack = Math.Max(send.Body.MaxStack, (ushort)3);
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
Console.WriteLine("Installed scoped chest callback, exit callback and HTTP observation hooks");
