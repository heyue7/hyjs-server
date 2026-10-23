using System;
using System.IO;
using System.Linq;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 4) throw new Exception("inputMain helper outputMain targetsJSON");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
if (helper.Assembly.Name != "SoulFlowerRecycle") throw new Exception("Unexpected helper identity");
var flow = helper.GetTypes().Single(t => t.FullName == "HYJS.Recycle61.Flow");
var importer = new Importer(module, ImporterOptions.TryToUseTypeDefs);
var plans = JsonDocument.Parse(File.ReadAllText(args[3])).RootElement.GetProperty("targets");
var changed = new List<object>();
var max = module.GetTypes().Single(t => t.FullName == "StaticData").Fields.Single(f => f.Name == "recycleMax");
var selected = module.GetTypes().Single(t => t.FullName == "StaticData").Fields.Single(f => f.Name == "recycleBreakupSelect");
void Normalize(TypeSig signature)
{
    if (signature == null) return;
    if (signature is TypeDefOrRefSig named && named.TypeDefOrRef is TypeRef reference && reference.ResolutionScope is ModuleRef scope && scope.Name == "SoulFlowerRecycle.dll")
    {
        if (!module.GetTypes().Any(t => t.FullName == reference.FullName)) throw new Exception("Unknown helper parameter Main type: " + reference.FullName);
        reference.ResolutionScope = module;
    }
    if (signature is GenericInstSig generic) { Normalize(generic.GenericType); foreach (var argument in generic.GenericArguments) Normalize(argument); }
    if (signature.Next != null) Normalize(signature.Next);
}
foreach (var plan in plans.EnumerateArray())
{
    var token = Convert.ToUInt32(plan.GetProperty("token").GetString()![2..], 16);
    var method = module.ResolveToken(token) as MethodDef ?? throw new Exception("Invalid target token");
    if (method.DeclaringType.FullName != plan.GetProperty("type").GetString() || method.Name != plan.GetProperty("method").GetString()) throw new Exception("Target identity differs");
    var reader = module.Metadata.PEImage.CreateReader(method.RVA);
    uint at = reader.Position; byte first = reader.ReadByte(); uint length;
    if ((first & 3) == 2) length = (uint)(first >> 2);
    else { int header = ((first | (reader.ReadByte() << 8)) >> 12) * 4; reader.ReadUInt16(); length = reader.ReadUInt32(); reader.Position = at + (uint)header; }
    string hash = Convert.ToHexString(SHA256.HashData(reader.ReadBytes((int)length))).ToLowerInvariant();
    if (hash != plan.GetProperty("inputCILSHA256").GetString()) throw new Exception("Fixed59 target CIL differs: " + method.FullName);
    // All target methods currently have no handlers; retain their original locals and signature tokens.
    if (method.Body.ExceptionHandlers.Count != 0) throw new Exception("Unexpected target exception handlers: " + method.FullName);
    string mode = plan.GetProperty("mode").GetString()!;
    int edits = 0;
    if (mode == "capacity")
    {
        var ins = method.Body.Instructions;
        for (int index = 2; index < ins.Count; index++)
            if (ins[index].OpCode == OpCodes.Stsfld && ins[index].Operand is IField field && field.FullName == selected.FullName)
            {
                if (ins[index - 1].OpCode != OpCodes.Newarr || !ins[index - 2].IsLdcI4() || ins[index - 2].GetLdcI4Value() != 40) throw new Exception("Array allocation anchor differs");
                ins[index - 2].OpCode = OpCodes.Ldc_I4; ins[index - 2].Operand = 100; edits++;
            }
        if (edits != 1) throw new Exception("Expected one dedicated array allocation");
    }
    else if (mode == "loop100" || mode == "quality100")
    {
        foreach (var ins in method.Body.Instructions)
        {
            bool limit = mode == "loop100" && (ins.OpCode == OpCodes.Ldsfld && ins.Operand is IField field && field.FullName == max.FullName || ins.IsLdcI4() && ins.GetLdcI4Value() == 40);
            bool qualityLoop = mode == "quality100" && ins.IsLdcI4() && ins.GetLdcI4Value() == 4;
            if (limit || qualityLoop) { ins.OpCode = OpCodes.Ldc_I4; ins.Operand = 100; edits++; }
        }
        if (edits < 1 || mode == "quality100" && edits != 1) throw new Exception("Limit anchor differs: " + method.FullName);
    }
    List<Instruction> Call(string name)
    {
        var call = importer.Import(flow.Methods.Single(m => m.Name == name));
        Normalize(call.MethodSig.RetType); foreach (var parameter in call.MethodSig.Params) Normalize(parameter);
        var ins = new List<Instruction>();
        if (plan.TryGetProperty("args", out var parameters)) foreach (var parameter in parameters.EnumerateArray()) ins.Add(Instruction.Create(OpCodes.Ldarg, method.Parameters[parameter.GetInt32()]));
        if (plan.TryGetProperty("kind", out var kind)) ins.Add(Instruction.Create(OpCodes.Ldc_I4, kind.GetInt32()));
        ins.Add(Instruction.Create(OpCodes.Call, call));
        return ins;
    }
    if (mode == "replace")
    {
        var added = Call(plan.GetProperty("helper").GetString()!);
        method.Body.Instructions.Clear(); foreach (var ins in added) method.Body.Instructions.Add(ins);
        method.Body.Instructions.Add(Instruction.Create(OpCodes.Ret)); edits++;
    }
    else if (mode == "postfix")
    {
        foreach (var exit in method.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
        {
            var added = Call(plan.GetProperty("helper").GetString()!);
            int index = method.Body.Instructions.IndexOf(exit);
            exit.OpCode = added[0].OpCode; exit.Operand = added[0].Operand;
            foreach (var ins in added.Skip(1)) method.Body.Instructions.Insert(++index, ins);
            method.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ret)); edits++;
        }
    }
    string? gate = mode == "gate" || mode == "quality100" ? plan.GetProperty("helper").GetString() : plan.TryGetProperty("gate", out var gateName) ? gateName.GetString() : null;
    if (gate != null)
    {
        var added = Call(gate); var original = method.Body.Instructions[0];
        added.Add(Instruction.Create(OpCodes.Brtrue, original)); added.Add(Instruction.Create(OpCodes.Ret));
        for (int index = 0; index < added.Count; index++) method.Body.Instructions.Insert(index, added[index]); edits++;
    }
    if (edits < 1) throw new Exception("No mutation planned");
    method.Body.SimplifyBranches();
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)8); method.Body.KeepOldMaxStack = true;
    changed.Add(new { token = method.MDToken.ToString(), type = method.DeclaringType.FullName, method = method.Name.String, inputCILSHA256 = hash, mode, edits, localCount = method.Body.Variables.Count, exceptionHandlerCount = method.Body.ExceptionHandlers.Count });
}
// Roslyn can scope original Main parameter types through the helper module.
// Bind those types to their existing Main definitions without adding definitions or pointer tables.
int rebound = 0;
foreach (var reference in module.GetTypeRefs())
    if (reference.ResolutionScope is ModuleRef scope && scope.Name == "SoulFlowerRecycle.dll")
    {
        if (!module.GetTypes().Any(t => t.FullName == reference.FullName)) throw new Exception("Unknown helper-scoped Main reference: " + reference.FullName);
        reference.ResolutionScope = module; rebound++;
    }
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
File.WriteAllText(Path.Combine(Path.GetDirectoryName(Path.GetFullPath(args[2]))!, "changed-methods.json"), JsonSerializer.Serialize(new { changed, helperScopedMainTypesRebound = rebound }, new JsonSerializerOptions { WriteIndented = true }));
