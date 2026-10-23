using System;
using System.IO;
using System.Linq;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("inputMain, helper HYJS.DungeonContent.dll, output required");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
if (helper.Assembly.Name != "HYJS.DungeonContent") throw new InvalidOperationException("Unexpected helper assembly identity");
var runtime = helper.GetTypes().Single(t => t.FullName == "HYJS.DungeonContent.DungeonContent");
var pins = new Dictionary<string, string> {
    { "AssetLoad.AssetBundleManager::GetReplaceSceneName", "3ae4978817bd81069da2e391697e1f41512a7f9f0e06036fb7f585749d749163" },
    { "Room.RCAIMonster::Init", "7fcd92e32e3b9b64949121f40fbfc0815a41acca49cde074921c8a7bb4305ba9" },
    { "Room.RCMoveNavigation::CalculatePath", "723ef5a5701aafce02aab5cd467e8bccddfe47d1bdcfd686fee28e719ddcc890" },
    { "Room.RoomBase::LoadAreaDataFromTable", "948f297fa8c3d3fab684321eae011d6467876bb14947ae34216ca18d50bc2eca" },
    { "Room.RoomCopy::StartLoad", "a4ced815d31a224f2b19935d97580782898052875fc5d2a8c655c7288aacb7e1" },
    { "Room.RoomCopy::OnLoadComplete", "6e8e61e2b58ecf09bd9f9c2753b979fd08c6f55bbce9fd583962ed5e2dd6d6df" },
    { "Room.RoomCopy::LoadPlayer", "5f77a8158f4b862c7b81182deec0848fa1ae1197e4698280b459e4ecff8e7659" },
    { "UI.UICopyEvent::_OnChallengeClick", "343788be4176e832e18b61588888a76528db4a973d5c2db41e6c4866bd1b36f4" },
    { "Room.RoomChallenge::StartLoad", "f97185fd15d0b5c485ecfe0d05b9dfeba7e8b5cc58f011db6fc34333c450dbb9" },
    { "Room.RoomChallenge::OnLoadComplete", "e54e485fbbfe61b24ebd33f1402293ba027d98b92848da8f19c7ba55039f21d4" },
    { "Room.RoomChallenge::LoadPlayer", "46a18b438c99b07d5f635c0ead937bea030b434251b9a0cac54b707a88830622" },
    { "UI.FallItem::OnGoSailClick", "e75f5760e4a71f06f9001df4fea5d184da42e3fdca3809071c0509f57a6cade8" },
};
var changed = new List<object>();
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef Target(string type, string name)
{
    var methods = T(type).Methods.Where(m => m.Name == name);
    if (type == "Room.RCAIMonster") methods = methods.Where(m => m.MethodSig.Params.Count == 1 && m.MethodSig.Params[0].FullName == "Dict.DataModel.TableCopyTask");
    var m = methods.Single();
    if (!m.HasBody || m.Body.Instructions.Any(i => i.Operand is IMethod x && x.DeclaringType.FullName == runtime.FullName))
        throw new InvalidOperationException("Missing body or already patched: " + type + "::" + name);
    var reader = module.Metadata.PEImage.CreateReader(m.RVA);
    uint start = reader.Position;
    byte first = reader.ReadByte();
    uint size;
    if ((first & 3) == 2) size = (uint)(first >> 2);
    else if ((first & 3) == 3)
    {
        int header = ((first | (reader.ReadByte() << 8)) >> 12) * 4;
        reader.ReadUInt16();
        size = reader.ReadUInt32();
        reader.Position = start + (uint)header;
    }
    else throw new InvalidOperationException("Unsupported CIL header");
    string hash = Convert.ToHexString(SHA256.HashData(reader.ReadBytes((int)size))).ToLowerInvariant();
    string key = type + "::" + name;
    if (hash != pins[key]) throw new InvalidOperationException("Fixed55 CIL pin differs: " + key + " " + hash);
    m.Body.MaxStack = Math.Max(m.Body.MaxStack, (ushort)8);
    m.Body.KeepOldMaxStack = true;
    changed.Add(new { method = key, token = m.MDToken.ToString(), inputCILSHA256 = hash });
    return m;
}
var importer = new Importer(module, ImporterOptions.TryToUseTypeDefs);
IMethod H(string name) => importer.Import(runtime.Methods.Single(m => m.Name == name));
bool Calls(Instruction i, string type, string name) => i.Operand is IMethod m && m.DeclaringType.FullName == type && m.Name == name;
void Insert(MethodDef method, Instruction anchor, params Instruction[] added)
{
    // Redirect original branches to the whole added block, including the no-server-override branch.
    foreach (var i in method.Body.Instructions)
    {
        if (ReferenceEquals(i.Operand, anchor)) i.Operand = added[0];
        else if (i.Operand is IList<Instruction> list)
            for (int j = 0; j < list.Count; j++) if (ReferenceEquals(list[j], anchor)) list[j] = added[0];
    }
    foreach (var h in method.Body.ExceptionHandlers)
    {
        if (h.TryStart == anchor) h.TryStart = added[0];
        if (h.HandlerStart == anchor) h.HandlerStart = added[0];
        if (h.FilterStart == anchor) h.FilterStart = added[0];
        // End boundaries remain at the original instruction. None of these eight scoped insertion anchors is an EH end.
        if (h.TryEnd == anchor || h.HandlerEnd == anchor) throw new InvalidOperationException("Unexpected EH end at hook");
    }
    int at = method.Body.Instructions.IndexOf(anchor);
    foreach (var i in added) method.Body.Instructions.Insert(at++, i);
}
void Tail(MethodDef method, string helperMethod)
{
    var returns = method.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray();
    if (returns.Length != 1) throw new InvalidOperationException("Expected one original return: " + method.FullName);
    var ret = returns[0];
    // Mutate the original ret object so every existing branch continues through the hook.
    ret.OpCode = OpCodes.Ldarg_0;
    ret.Operand = null;
    method.Body.Instructions.Add(Instruction.Create(OpCodes.Call, H(helperMethod)));
    method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
}

var scene = Target("AssetLoad.AssetBundleManager", "GetReplaceSceneName");
var equal = module.GetMemberRefs().First(m => m.DeclaringType.FullName == "System.String" && m.Name == "op_Equality");
var originalSceneEntry = scene.Body.Instructions[0];
var keepOriginal = Instruction.Create(OpCodes.Pop);
Insert(scene, originalSceneEntry,
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, H("Scene")),
    Instruction.Create(OpCodes.Dup), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, equal),
    Instruction.Create(OpCodes.Brtrue, keepOriginal), Instruction.Create(OpCodes.Ret), keepOriginal);

var entry = Target("UI.UICopyEvent", "_OnChallengeClick");
var full = entry.Body.Instructions.Where(i => Calls(i, "GameConfig", "get_IsAssetFull")).ToArray();
if (full.Length != 1) throw new InvalidOperationException("Expected single original main full getter");
int gate = entry.Body.Instructions.IndexOf(full[0]);
if (gate < 1 || !Calls(entry.Body.Instructions[gate - 1], "GameConfig", "get_Instance") ||
    (entry.Body.Instructions[gate + 1].OpCode != OpCodes.Brtrue && entry.Body.Instructions[gate + 1].OpCode != OpCodes.Brtrue_S))
    throw new InvalidOperationException("Main full expression/branch shape differs");
entry.Body.Instructions[gate - 1].Operand = H("MainEntryReady");
full[0].OpCode = OpCodes.Nop;
full[0].Operand = null;
for (int i = 0; i < 4; i++) entry.Body.Instructions.Insert(gate + 1, Instruction.Create(OpCodes.Nop));

var start = Target("Room.RoomCopy", "StartLoad");
var showMini = start.Body.Instructions.Single(i => Calls(i, "Room.RoomSmallMap", "ShowMiniMap"));
int showAt = start.Body.Instructions.IndexOf(showMini);
var startAnchor = start.Body.Instructions.Single(i => i.OpCode == OpCodes.Ldstr && (string)i.Operand == "[LOGIN] server give me scene:");
if (start.Body.Instructions.IndexOf(startAnchor) >= showAt ||
    !start.Body.Instructions.Take(start.Body.Instructions.IndexOf(startAnchor)).Any(i => i.OpCode == OpCodes.Stfld && i.Operand is IField f && f.DeclaringType.FullName == "Dict.DataModel.TableCopyMission" && f.Name == "SceneName"))
    throw new InvalidOperationException("Expected server scene override before mini-map");
Insert(start, startAnchor, Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, H("AdaptMission")));

var complete = Target("Room.RoomCopy", "OnLoadComplete");
var loadPlayer = complete.Body.Instructions.Single(i => Calls(i, "Room.RoomCopy", "LoadPlayer"));
int loadAt = complete.Body.Instructions.IndexOf(loadPlayer);
if (complete.Body.Instructions[loadAt - 1].OpCode != OpCodes.Ldarg_0 ||
    !complete.Body.Instructions.Take(loadAt).Any(i => i.OpCode == OpCodes.Ldfld && i.Operand is IField f && f.Name == "missionLoaded"))
    throw new InvalidOperationException("Expected native first-load guard");
Insert(complete, complete.Body.Instructions[loadAt - 1], Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, H("Prepare")));
Tail(Target("Room.RoomCopy", "LoadPlayer"), "AlignPlayer");
Tail(Target("Room.RCAIMonster", "Init"), "AlignMonster");

var navigation = Target("Room.RCMoveNavigation", "CalculatePath");
Insert(navigation, navigation.Body.Instructions[0], Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, H("ProjectTarget")),
    Instruction.Create(OpCodes.Starg, navigation.Parameters[1]));
var area = Target("Room.RoomBase", "LoadAreaDataFromTable");
var originalAreaEntry = area.Body.Instructions[0];
Insert(area, originalAreaEntry, Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, H("SkipArea")),
    Instruction.Create(OpCodes.Brfalse, originalAreaEntry), Instruction.Create(OpCodes.Ret));
var awakeStart = Target("Room.RoomChallenge", "StartLoad");
Insert(awakeStart, awakeStart.Body.Instructions[0], Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, H("AwakeAdapt")));
var awakeComplete = Target("Room.RoomChallenge", "OnLoadComplete");
var awakePlayer = awakeComplete.Body.Instructions.Single(i => Calls(i, "Room.RoomChallenge", "LoadPlayer"));
int awakeAt = awakeComplete.Body.Instructions.IndexOf(awakePlayer);
if (awakeComplete.Body.Instructions[awakeAt - 1].OpCode != OpCodes.Ldarg_0 ||
    !awakeComplete.Body.Instructions.Take(awakeAt).Any(i => i.OpCode == OpCodes.Ldfld && i.Operand is IField f && f.Name == "sceneLoaded"))
    throw new InvalidOperationException("Expected awake first-load guard");
Insert(awakeComplete, awakeComplete.Body.Instructions[awakeAt - 1], Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, H("AwakePrepare")));
Tail(Target("Room.RoomChallenge", "LoadPlayer"), "AwakeAlignPlayer");
var drop = Target("UI.FallItem", "OnGoSailClick");
var dropFull = drop.Body.Instructions.Where(i => Calls(i, "GameConfig", "get_IsAssetFull")).ToArray();
if (dropFull.Length != 1) throw new InvalidOperationException("Expected one drop-source awake full expression");
int dropAt = drop.Body.Instructions.IndexOf(dropFull[0]);
if (!Calls(drop.Body.Instructions[dropAt - 1], "GameConfig", "get_Instance") ||
    !drop.Body.Instructions.Skip(dropAt + 1).Take(3).Any(i => i.OpCode == OpCodes.Ldstr && (string)i.Operand == "AwakeCopyResourceUpdate"))
    throw new InvalidOperationException("Drop-source gate is not the original awake branch");
drop.Body.Instructions[dropAt - 1].Operand = H("AwakeEntryReady");
dropFull[0].OpCode = OpCodes.Nop; dropFull[0].Operand = null;
for (int i = 0; i < 4; i++) drop.Body.Instructions.Insert(dropAt + 1, Instruction.Create(OpCodes.Nop));
if (changed.Count != 12) throw new InvalidOperationException("Expected twelve scoped target methods");
// Helper signatures may refer back to main types through a self ModuleRef.
// Bind those names to the actual main module before writing, not the helper DLL.
var rebound = new List<string>();
foreach (var type in module.GetTypeRefs())
{
    if (type.ResolutionScope is ModuleRef scope && scope.Name == "HYJS.DungeonContent.dll")
    {
        if (!module.GetTypes().Any(t => t.FullName == type.FullName))
            throw new InvalidOperationException("Unexpected helper-scoped type: " + type.FullName);
        type.ResolutionScope = module;
        rebound.Add(type.FullName);
    }
}
if (module.GetTypeRefs().Any(t => t.ResolutionScope is ModuleRef scope && scope.Name == "HYJS.DungeonContent.dll"))
    throw new InvalidOperationException("Residual helper-scoped main types");
File.WriteAllText(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!, "helper-scope-normalization.json"), JsonSerializer.Serialize(new { reboundMainTypes = rebound, remainingHelperModuleRefs = 0 }, new JsonSerializerOptions { WriteIndented = true }) + "\n");
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
// Imported TypeRefs may be outside GetTypeRefs() until serialized. Re-open and
// normalize the actual output metadata, then verify that final bytes are clean.
using (var serialized = ModuleDefMD.Load(args[2]))
{
    var serializedRebound = new List<string>();
    foreach (var type in serialized.GetTypeRefs())
    {
        if (type.ResolutionScope is ModuleRef scope && scope.Name == "HYJS.DungeonContent.dll")
        {
            if (!serialized.GetTypes().Any(t => t.FullName == type.FullName)) throw new InvalidOperationException("Unknown serialized helper-scoped type: " + type.FullName);
            type.ResolutionScope = serialized;
            serializedRebound.Add(type.FullName);
        }
    }
    var finalOptions = new ModuleWriterOptions(serialized);
    finalOptions.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
    string normalizedPath = args[2] + ".normalized";
    serialized.Write(normalizedPath, finalOptions);
    File.Move(normalizedPath, args[2], true);
    File.WriteAllText(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!, "helper-scope-normalization.json"), JsonSerializer.Serialize(new { prewriteReboundMainTypes = rebound, serializedReboundMainTypes = serializedRebound, remainingHelperModuleRefs = 0 }, new JsonSerializerOptions { WriteIndented = true }) + "\n");
}
using (var verified = ModuleDefMD.Load(args[2]))
{
    if (verified.GetTypeRefs().Any(t => t.ResolutionScope is ModuleRef scope && scope.Name == "HYJS.DungeonContent.dll")) throw new InvalidOperationException("Final serialized helper scope remains invalid");
}
File.WriteAllText(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!, "changed-methods.json"), JsonSerializer.Serialize(changed, new JsonSerializerOptions { WriteIndented = true }) + "\n");
Console.WriteLine("Installed twelve pinned dungeon visual/entry hooks; network, global download flags and battle engine preserved");
