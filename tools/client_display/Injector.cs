using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 4) throw new ArgumentException("input DLL, UnityEngine DLL, display bridge DLL, output DLL required");
using var module = ModuleDefMD.Load(args[0]);
using var unity = ModuleDefMD.Load(args[1]);
MethodDef Method(string owner, string name) => module.GetTypes().Single(t => t.FullName == owner).Methods.Single(m => m.Name == name);
using var bridge = ModuleDefMD.Load(args[2]);
var settings = bridge.GetTypes().Single(t => t.FullName == "HYJS.Display.DisplaySettings");
var settingsRef = module.Import(settings);
IMethod Display(string name)
{
    // Importing a helper MethodDef can incorrectly turn its Assembly-CSharp
    // parameter TypeRefs into ModuleRefs scoped to the helper DLL itself.
    // These parameters belong to this main module: bind their actual TypeDefs
    // explicitly, while keeping only the declaring helper class external.
    MethodSig signature = name switch
    {
        "get_TargetFPS" => MethodSig.CreateStatic(module.CorLibTypes.Int32),
        "StartFPS" => MethodSig.CreateStatic(module.CorLibTypes.Void,
            new ClassSig(module.GetTypes().Single(t => t.FullName == "FPSFixer"))),
        "Attach" => MethodSig.CreateStatic(module.CorLibTypes.Void,
            new ClassSig(module.GetTypes().Single(t => t.FullName == "UI.QualityBar"))),
        _ => throw new ArgumentException("Unknown display entry: " + name)
    };
    return new MemberRefUser(module, name, signature, settingsRef);
}

// Preserve the complete aspect classification, design dimensions and anchor path.
// Replace the five-byte void call in-place with three balanced pops and two nops.
var awake = Method("UIAutoSize", "Awake");
var resolution = awake.Body.Instructions.Single(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName == "UnityEngine.Screen" && m.Name == "SetResolution");
if (((IMethod)resolution.Operand).MethodSig.Params.Count != 3) throw new Exception("Unexpected resolution API");
int index = awake.Body.Instructions.IndexOf(resolution);
resolution.OpCode = OpCodes.Pop; resolution.Operand = null;
foreach (var opcode in new[] { OpCodes.Pop, OpCodes.Pop, OpCodes.Nop, OpCodes.Nop }) awake.Body.Instructions.Insert(++index, Instruction.Create(opcode));

// Unity owns pacing. The separate old-runtime bridge persists and applies only
// display settings; no simulation clock, server or account state is involved.
var start = Method("FPSFixer", "Start");
start.Body = new CilBody();
start.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
start.Body.Instructions.Add(Instruction.Create(OpCodes.Call, Display("StartFPS")));
start.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
Method("FPSFixer", "Update").Body = new CilBody();
Method("FPSFixer", "Update").Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var maxFPS = Method("RenderManager", "get_MaxFPS");
maxFPS.Body = new CilBody();
maxFPS.Body.Instructions.Add(Instruction.Create(OpCodes.Call, Display("get_TargetFPS")));
maxFPS.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var quality = Method("UI.QualityBar", "Start");
if (quality.Body.Instructions.Count != 1 || quality.Body.Instructions[0].OpCode != OpCodes.Ret)
    throw new Exception("Original quality entry Start changed");
quality.Body = new CilBody();
quality.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
quality.Body.Instructions.Add(Instruction.Create(OpCodes.Call, Display("Attach")));
quality.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));

// PlayerInfoPage.DoOpen actually invokes Refresh on every opening. Keep Start
// as well, but do not depend on the engine's originally-empty Start callback.
var qualityRefresh = Method("UI.QualityBar", "Refresh");
if (qualityRefresh.Body.ExceptionHandlers.Count != 0 || qualityRefresh.Body.Instructions.Count(i => i.OpCode == OpCodes.Ret) != 1)
    throw new Exception("Review original quality Refresh control flow");
var qualityEnd = qualityRefresh.Body.Instructions.Last();
if (qualityEnd.OpCode != OpCodes.Ret) throw new Exception("Expected quality Refresh terminal return");
qualityEnd.OpCode = OpCodes.Ldarg_0;
qualityRefresh.Body.Instructions.Add(Instruction.Create(OpCodes.Call, Display("Attach")));
qualityRefresh.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[3], options);
Console.WriteLine("Six display-only methods patched; native resolution, main-module parameter scopes and persistent 60/120 FPS options");
