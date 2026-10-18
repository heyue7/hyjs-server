using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input managed, bridge, output managed");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
// Importing AssemblyDef's empty PublicKey object can incorrectly set PublicKey=1
// on the reference. The old C# metadata reader rejects that empty full key.
var helperAssembly = new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerHeroArt, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper = new TypeRefUser(module, "HYJS.HeroArt", "HeroArtFallback", helperAssembly);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string owner, string name) => T(owner).Methods.Single(m => m.Name == name);
TypeDef Nested(string owner, string name) => module.GetTypes().Single(t => t.Name == name && t.DeclaringType?.FullName == owner);
MemberRef B(string name, TypeSig result, params TypeSig[] parameters) => new MemberRefUser(module, name, MethodSig.CreateStatic(result, parameters), helper);
var mapBundle = B("MapBundle", module.CorLibTypes.String, module.CorLibTypes.String);
var mapAsset = B("MapAsset", module.CorLibTypes.String, module.CorLibTypes.String, module.CorLibTypes.String);
void Prefix(MethodDef method, params Instruction[] instructions) {
    foreach (var instruction in instructions.Reverse()) method.Body.Instructions.Insert(0, instruction);
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)3);
    method.Body.KeepOldMaxStack = true;
}
foreach (var name in new[]{"LoadAssetBundle", "LoadAssetBundleCoroutine", "UnloadAssetBundle", "IsLoadedAssetBundle", "IsAssetExist"}) {
    var method = M("AssetLoad.AssetBundleManager", name);
    Prefix(method, Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, mapBundle), Instruction.Create(OpCodes.Starg, method.Parameters[1]));
}
var getAsset = M("AssetLoad.AssetBundleManager", "GetAssetFromBundle");
Prefix(getAsset,
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2), Instruction.Create(OpCodes.Call, mapAsset), Instruction.Create(OpCodes.Starg, getAsset.Parameters[2]),
    Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, mapBundle), Instruction.Create(OpCodes.Starg, getAsset.Parameters[1]));

// Restrict the UV head crop to the original portrait setters, never the shared texture asset.
void ReplaceSetter(MethodDef method, string name) {
    var calls = method.Body.Instructions.Where(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m && m.DeclaringType.FullName == "UITexture" && m.Name == "set_mainTexture").ToArray();
    if (calls.Length != 1) throw new InvalidOperationException("Expected one texture setter: " + method.FullName);
    var original = (IMethod)calls[0].Operand;
    var reference = B(name, module.CorLibTypes.Void, T("UITexture").ToTypeSig(), original.MethodSig.Params[0]);
    calls[0].OpCode = OpCodes.Call;
    calls[0].Operand = reference;
}
ReplaceSetter(Nested("UI.TextureKit", "<_LoadTexture>c__IteratorCD").Methods.Single(m => m.Name == "MoveNext"), "AssignHead");
ReplaceSetter(M("Battle.BattleHeroItem", "Init"), "AssignHead");
ReplaceSetter(Nested("AloneTextureKit", "<SetPath>c__AnonStorey186").Methods.Single(m => m.Name == "<>m__D8"), "AssignCard");
var setPath = M("AloneTextureKit", "SetPath");
Prefix(setPath, Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1),
    Instruction.Create(OpCodes.Call, B("PrepareCard", module.CorLibTypes.Void, T("AloneTextureKit").ToTypeSig(), module.CorLibTypes.String)));

var list = M("UI.AllHerosPage", "SetHeroList");
var flags = list.Body.Instructions.Where(i => i.OpCode == OpCodes.Ldfld && i.Operand is IField f && f.DeclaringType.FullName == "Dict.DataModel.TableHero" && f.Name == "IsShowInCard").ToArray();
if (flags.Length != 1) throw new InvalidOperationException("Expected one atlas flag");
flags[0].OpCode = OpCodes.Call;
flags[0].Operand = B("AtlasHidden", module.CorLibTypes.Int32, T("Dict.DataModel.TableHero").ToTypeSig());

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Three original portrait aliases, per-widget UV crops and bounded atlas visibility injected");
