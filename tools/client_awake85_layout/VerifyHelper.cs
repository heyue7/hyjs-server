using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;

if (args.Length != 3) throw new ArgumentException("old helper, new helper, report required");
using var oldModule = ModuleDefMD.Load(args[0]);
using var newModule = ModuleDefMD.Load(args[1]);
if (oldModule.Assembly.FullName != newModule.Assembly.FullName)
    throw new InvalidOperationException("Helper assembly identity changed");

var changed = new HashSet<string>(StringComparer.Ordinal) {
    "HYJS.DungeonContent.DungeonContent::AwakeAdapt",
    "HYJS.DungeonContent.DungeonContent::AwakePrepare",
    "HYJS.DungeonContent.DungeonContent::AwakeAlignPlayer"
};

static string Name(MethodDef m) => m.DeclaringType.FullName + "::" + m.Name + "|" + m.MethodSig;
static string TypeScope(ITypeDefOrRef? t)
{
    if (t == null) return "";
    var scope = t.DefinitionAssembly;
    return t.FullName + "@" + (scope?.FullName ?? "");
}
static string Operand(object? value, Dictionary<Instruction, int> offsets)
{
    if (value == null) return "";
    if (value is Instruction branch) return "branch:" + offsets[branch];
    if (value is IList<Instruction> branches) return "switch:" + string.Join(",", branches.Select(x => offsets[x]));
    if (value is IMethod method) return "method:" + method.FullName + "@" + TypeScope(method.DeclaringType);
    if (value is IField field) return "field:" + field.FullName + "@" + TypeScope(field.DeclaringType);
    if (value is ITypeDefOrRef type) return "type:" + TypeScope(type);
    if (value is Local local) return "local:" + local.Index;
    if (value is Parameter parameter) return "arg:" + parameter.Index;
    if (value is string text) return "string:" + text;
    if (value is IFormattable formattable) return "value:" + formattable.ToString(null, CultureInfo.InvariantCulture);
    return "other:" + value;
}
static string Canonical(MethodDef m)
{
    if (!m.HasBody) return "no-body";
    var b = m.Body;
    var offsets = b.Instructions.Select((ins, i) => (ins, i)).ToDictionary(x => x.ins, x => x.i);
    string Index(Instruction? ins) => ins == null ? "end" : offsets[ins].ToString(CultureInfo.InvariantCulture);
    var record = new {
        initLocals = b.InitLocals,
        maxStack = b.MaxStack,
        locals = b.Variables.Select(x => x.Type.FullName).ToArray(),
        code = b.Instructions.Select(i => new { op = i.OpCode.Name, operand = Operand(i.Operand, offsets) }).ToArray(),
        exceptions = b.ExceptionHandlers.Select(h => new {
            kind = h.HandlerType.ToString(), catchType = TypeScope(h.CatchType),
            start = Index(h.TryStart), end = Index(h.TryEnd),
            handlerStart = Index(h.HandlerStart), handlerEnd = Index(h.HandlerEnd), filter = Index(h.FilterStart)
        }).ToArray()
    };
    return JsonSerializer.Serialize(record);
}

var oldTypes = oldModule.GetTypes().ToDictionary(t => t.FullName, StringComparer.Ordinal);
var newTypes = newModule.GetTypes().ToDictionary(t => t.FullName, StringComparer.Ordinal);
var oldMethods = oldModule.GetTypes().SelectMany(t => t.Methods).ToDictionary(Name, StringComparer.Ordinal);
var newMethods = newModule.GetTypes().SelectMany(t => t.Methods).ToDictionary(Name, StringComparer.Ordinal);
var missing = new List<string>();
var abiDifferences = new List<string>();
var semanticDifferences = new List<string>();
var verified = new List<string>();

foreach (var pair in oldTypes)
{
    if (!newTypes.TryGetValue(pair.Key, out var target)) { missing.Add("type " + pair.Key); continue; }
    var source = pair.Value;
    if (source.Attributes != target.Attributes || TypeScope(source.BaseType) != TypeScope(target.BaseType) ||
        !source.Interfaces.Select(x => TypeScope(x.Interface)).SequenceEqual(target.Interfaces.Select(x => TypeScope(x.Interface))))
        abiDifferences.Add("type " + pair.Key);
    var fields = target.Fields.ToDictionary(f => f.Name.String + "|" + f.FieldSig, StringComparer.Ordinal);
    foreach (var field in source.Fields)
    {
        var key = field.Name.String + "|" + field.FieldSig;
        if (!fields.TryGetValue(key, out var replacement) || field.Attributes != replacement.Attributes ||
            !Equals(field.Constant?.Value, replacement.Constant?.Value)) abiDifferences.Add("field " + pair.Key + "::" + key);
    }
}
foreach (var pair in oldMethods)
{
    if (!newMethods.TryGetValue(pair.Key, out var target)) { missing.Add("method " + pair.Key); continue; }
    var source = pair.Value;
    if (source.Attributes != target.Attributes || source.ImplAttributes != target.ImplAttributes)
        abiDifferences.Add("method " + pair.Key);
    var simple = source.DeclaringType.FullName + "::" + source.Name;
    if (changed.Contains(simple)) continue;
    if (Canonical(source) != Canonical(target)) semanticDifferences.Add(pair.Key);
    else verified.Add(pair.Key);
}
var allowed = oldMethods.Values.Where(m => changed.Contains(m.DeclaringType.FullName + "::" + m.Name)).Select(Name).OrderBy(x => x).ToArray();
var requiredCalls = new Dictionary<string, string[]> {
    { "AwakeAdapt", new[] { "Awake85Transition", "Applies", "Awake85EnsureNightForest", "Scene" } },
    { "Awake85ExpectedNightCell", new[] { "Matches" } },
    { "Awake85EnsureNightForest", new[] { "GetField", "LoadFromString", "Register" } },
    { "AwakePrepare", new[] { "Awake85Current", "Awake85Prepare", "AwakeCurrent" } },
    { "AwakeAlignPlayer", new[] { "Awake85Current", "AwakeCurrent" } },
    { "Awake85Transition", new[] { "Same", "Leave", "Identify", "Bind" } },
    { "Awake85Current", new[] { "Active", "AwakeField" } },
    { "Awake85Prepare", new[] { "Identify", "BoxSlot", "Layout", "CalculatePath", "Select", "CaptureBorn" } }
};
var requiredCallErrors = new List<string>();
var scopeType = newTypes["HYJS.DungeonContent.DungeonContent"];
foreach (var item in requiredCalls)
{
    var candidates = scopeType.Methods.Where(m => m.Name == item.Key && m.HasBody).ToArray();
    if (candidates.Length != 1) { requiredCallErrors.Add(item.Key + ": missing method"); continue; }
    var calls = candidates[0].Body.Instructions.Where(i => i.Operand is IMethod)
        .Select(i => ((IMethod)i.Operand).Name.String).ToArray();
    foreach (var name in item.Value)
        if (!calls.Contains(name)) requiredCallErrors.Add(item.Key + ": missing " + name);
}
var adaptCalls = scopeType.Methods.Single(m => m.Name == "AwakeAdapt").Body.Instructions
    .Where(i => i.Operand is IMethod).Select(i => ((IMethod)i.Operand).Name.String).ToArray();
int transitionIndex = Array.IndexOf(adaptCalls, "Awake85Transition");
int appliesIndex = Array.IndexOf(adaptCalls, "Applies");
int ensureIndex = Array.IndexOf(adaptCalls, "Awake85EnsureNightForest");
int sceneIndex = Array.IndexOf(adaptCalls, "Scene");
if (!(transitionIndex >= 0 && transitionIndex < appliesIndex && appliesIndex < ensureIndex && ensureIndex < sceneIndex))
    requiredCallErrors.Add("AwakeAdapt: night manifest must follow transition and precede scene resolution");
var report = new {
    assembly = oldModule.Assembly.FullName,
    oldTypes = oldTypes.Count, newTypes = newTypes.Count,
    oldMethods = oldMethods.Count, newMethods = newMethods.Count,
    allowedAwakeMethods = allowed, preservedMethodSemantics = verified,
    missing, abiDifferences, semanticDifferences, requiredCallErrors,
    comparison = "resolved instructions, branch targets, locals, exception regions, max stack, method/type/field ABI; static, no execution"
};
System.IO.File.WriteAllText(args[2], JsonSerializer.Serialize(report, new JsonSerializerOptions { WriteIndented = true }) + "\n");
if (missing.Count != 0 || abiDifferences.Count != 0 || semanticDifferences.Count != 0 ||
    requiredCallErrors.Count != 0 || allowed.Length != 3)
    throw new InvalidOperationException("Helper compatibility check failed; inspect report");
Console.WriteLine("Preserved " + verified.Count + " existing helper method semantics; changed awake methods " + allowed.Length);
