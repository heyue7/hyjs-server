using System;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input main, helper, output main required");
byte[] input = File.ReadAllBytes(args[0]);
if (Convert.ToHexString(SHA256.HashData(input)).ToLowerInvariant() != "0e0fb0e88cc295877b5c4c74e3bfe3280e12bf0b7aa10e6521a69ad998fdbde1")
    throw new InvalidOperationException("Fixed 1.1.7 managed input differs");
using var main = ModuleDefMD.Load(input);
using var helper = ModuleDefMD.Load(args[1]);
void Require(bool condition, string message) { if (!condition) throw new InvalidOperationException(message); }
MethodDef Get(string type, string method) => main.GetTypes().Single(t => t.FullName == type).Methods.Single(m => m.Name == method);
var original = Get("NetAPI", "SendHttp");
Require(original.MDToken.Raw == 0x06003df4 && original.IsStatic && original.MethodSig.RetType.FullName == "System.Boolean" &&
        original.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "System.Int32", "System.Object", "NetWork.Layer.HttpHandler" }), "Original SendHttp ABI differs");
var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.Ranks.Refresh");
void Helper(string method, string ret, params string[] parameters)
{
    var m = helperType.Methods.Single(x => x.Name == method);
    Require(m.IsStatic && m.IsPublic && m.MethodSig.RetType.FullName == ret &&
            m.MethodSig.Params.Select(p => p.FullName).SequenceEqual(parameters), "Helper ABI differs: " + method);
}
Helper("Send", "System.Boolean", "System.Int32", "System.Object", "NetWork.Layer.HttpHandler");
Helper("WorshipCompleted", "System.Void", "UnityEngine.MonoBehaviour", "NetWork.Packets.Packet", "System.Boolean");
Require(helper.Assembly.Name == "HYJS.Ranks" && helper.Assembly.Version == new Version(0, 0, 0, 0), "Helper identity differs");
var callback = main.GetTypes().Single(t => t.FullName == "NetWork.Layer.HttpHandler").ToTypeSig();
var packet = main.GetTypes().Single(t => t.FullName == "NetWork.Packets.Packet").ToTypeSig();
var mono = main.GetTypeRefs().First(t => t.FullName == "UnityEngine.MonoBehaviour").ToTypeSig();
var reference = new AssemblyRefUser(new AssemblyNameInfo("HYJS.Ranks, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var bridge = new TypeRefUser(main, "HYJS.Ranks", "Refresh", reference);
var send = new MemberRefUser(main, "Send", MethodSig.CreateStatic(main.CorLibTypes.Boolean, main.CorLibTypes.Int32, main.CorLibTypes.Object, callback), bridge);
var worship = new MemberRefUser(main, "WorshipCompleted", MethodSig.CreateStatic(main.CorLibTypes.Void, mono, packet, main.CorLibTypes.Boolean), bridge);
var specs = new (string type, string method, int count)[] {
    ("UI.RankPage", "Refresh", 10), ("UI.PowerPage", "getPowerRank", 1),
    ("UI.LevelPage", "getPowerRank", 1), ("UI.UnionRankPage", "RefreshPage", 1),
    ("UI.UnionFightWeekRankPage", "OnTabBtn", 4), ("UI.ServerBattlePromotion", "UpdatePlayerList", 1),
    ("UI.ServerBattleRankHead", "UpDateServerBattleList", 1)
};
int count = 0;
foreach (var spec in specs)
{
    var method = Get(spec.type, spec.method);
    Require(method.HasBody && !method.IsStatic, "Target method differs: " + method.FullName);
    var sites = method.Body.Instructions.Where(i => i.OpCode == OpCodes.Call && i.Operand is IMethod called && called.MDToken.Raw == original.MDToken.Raw).ToArray();
    Require(sites.Length == spec.count, "HTTP call count differs: " + method.FullName);
    foreach (var site in sites) site.Operand = send;
    count += sites.Length;
}
Require(count == 19, "Total HTTP call count differs");
foreach (string type in new[] { "UI.PowerPage", "UI.LevelPage" })
{
    var method = Get(type, "CSGoodCallback");
    Require(method.HasBody && !method.IsStatic && method.MethodSig.RetType.FullName == "System.Void" &&
        method.MethodSig.Params.Select(p => p.FullName).SequenceEqual(new[] { "NetWork.Packets.Packet", "System.Boolean" }) &&
        method.Body.ExceptionHandlers.Count == 1 && method.Body.MaxStack >= 3, "Worship callback shape differs: " + method.FullName);
    method.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
    method.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Ldarg_1));
    method.Body.Instructions.Insert(2, Instruction.Create(OpCodes.Ldarg_2));
    method.Body.Instructions.Insert(3, Instruction.Create(OpCodes.Call, worship));
}
foreach (var method in main.GetTypes().SelectMany(t => t.Methods).Where(m => m.HasBody)) method.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(main);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
main.Write(args[2], options);
Console.WriteLine("Injected 19 read-only call sites and 2 guarded worship entries");
