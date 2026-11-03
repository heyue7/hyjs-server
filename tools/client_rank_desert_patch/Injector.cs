using System;
using System.Collections.Generic;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

internal static class Injector
{
    const uint TargetToken = 0x06005b6e;
    const uint MonsterGetterToken = 0x06001452;
    const uint BossGetterToken = 0x060014fa;
    const uint ResultToken = 0x06005b6f;
    const uint HookToken = 0x0a0021c7;

    static void Need(bool condition, string message)
    {
        if (!condition) throw new InvalidOperationException(message);
    }

    static bool Call(Instruction instruction, Code opcode, uint token)
    {
        return instruction.OpCode.Code == opcode && instruction.Operand is IMethod method && method.MDToken.Raw == token;
    }

    static bool Targets(Instruction instruction, Instruction target)
    {
        if (ReferenceEquals(instruction.Operand, target)) return true;
        return instruction.Operand is IList<Instruction> branches && branches.Any(i => ReferenceEquals(i, target));
    }

    static SortedSet<string> PublicApi(ModuleDefMD module)
    {
        var api = new SortedSet<string>(StringComparer.Ordinal);
        foreach (var type in module.GetTypes().Where(t => t.IsPublic || t.IsNestedPublic))
        {
            api.Add("type " + type.FullName + " " + (int)type.Attributes + " " + (type.BaseType == null ? "" : type.BaseType.FullName));
            foreach (var method in type.Methods.Where(m => m.IsPublic))
                api.Add("method " + method.FullName + " " + (int)method.Attributes + " " + (int)method.ImplAttributes);
            foreach (var field in type.Fields.Where(f => f.IsPublic))
                api.Add("field " + field.FullName + " " + (int)field.Attributes);
        }
        return api;
    }

    static void VerifyHelper(string originalPath, string candidatePath)
    {
        using var original = ModuleDefMD.Load(originalPath);
        using var candidate = ModuleDefMD.Load(candidatePath);
        Need(original.Assembly != null && candidate.Assembly != null && original.Assembly.FullName == candidate.Assembly.FullName,
            "Helper assembly identity changed");
        var before = PublicApi(original);
        var after = PublicApi(candidate);
        Need(before.SetEquals(after), "Existing public helper API changed: removed=" +
            string.Join(", ", before.Except(after)) + "; added=" + string.Join(", ", after.Except(before)));
        var boss = candidate.GetTypes().Single(t => t.FullName == "HYJS.DesertV4.BossRewardView");
        Need(!boss.IsPublic && !boss.IsNestedPublic, "Boss reward view must remain internal");
        var hook = candidate.GetTypes().Single(t => t.FullName == "HYJS.DesertV4.FieldHooks").Methods
            .Single(m => m.Name == "MonsterRewardDisplay");
        Need(hook.IsPublic && hook.IsStatic && hook.MethodSig.Params.Count == 1 &&
             hook.MethodSig.RetType.FullName == "ProtoBuf.Message.ResultObject" &&
             hook.MethodSig.Params[0].FullName == "ProtoBuf.Message.ResultObject", "Helper display hook ABI changed");
        var oldResources = original.Resources.OfType<EmbeddedResource>().ToDictionary(r => r.Name.ToString());
        var newResources = candidate.Resources.OfType<EmbeddedResource>().ToDictionary(r => r.Name.ToString());
        Need(oldResources.Count == original.Resources.Count && newResources.Count == candidate.Resources.Count &&
             oldResources.Keys.OrderBy(x => x).SequenceEqual(newResources.Keys.OrderBy(x => x)), "Helper resources changed");
        foreach (var entry in oldResources)
            Need(entry.Value.Attributes == newResources[entry.Key].Attributes &&
                 entry.Value.CreateReader().ToArray().SequenceEqual(newResources[entry.Key].CreateReader().ToArray()),
                "Helper resource changed: " + entry.Key);
        Console.WriteLine("PASS helper identity, existing public API and embedded resources");
    }

    static void Patch(string input, string output)
    {
        using var module = ModuleDefMD.Load(input);
        var target = module.GetTypes().Single(t => t.FullName == "Battle.CopyBalance").Methods
            .Single(m => m.Name == "Refresh" && m.MDToken.Raw == TargetToken);
        Need(target.HasBody, "Original CopyBalance.Refresh has no CIL body");
        var code = target.Body.Instructions;
        var originalHookSites = code.Where(i => Call(i, Code.Call, HookToken)).ToArray();
        Need(originalHookSites.Length == 1 && originalHookSites[0].Operand is MemberRef,
            "Expected the existing unique monster display MemberRef");
        var hook = (MemberRef)originalHookSites[0].Operand;
        var type = hook.DeclaringType as TypeRef;
        var scope = type == null ? null : type.ResolutionScope as AssemblyRef;
        Need(hook.Name == "MonsterRewardDisplay" && type != null && type.FullName == "HYJS.DesertV4.FieldHooks" &&
             scope != null && scope.Name == "SoulFlowerDesertV4" && !hook.MethodSig.HasThis &&
             hook.MethodSig.Params.Count == 1 && hook.MethodSig.RetType.FullName == "ProtoBuf.Message.ResultObject" &&
             hook.MethodSig.Params[0].FullName == "ProtoBuf.Message.ResultObject", "Existing display MemberRef ABI changed");
        int oldHook = code.IndexOf(originalHookSites[0]);
        Need(oldHook > 0 && oldHook + 1 < code.Count && Call(code[oldHook - 1], Code.Callvirt, MonsterGetterToken) &&
             Call(code[oldHook + 1], Code.Call, ResultToken), "Original monster display path changed");
        var bossSites = code.Select((instruction, index) => new { instruction, index })
            .Where(x => x.index + 1 < code.Count && Call(x.instruction, Code.Callvirt, BossGetterToken) &&
                        Call(code[x.index + 1], Code.Call, ResultToken)).ToArray();
        Need(bossSites.Length == 1, "Expected one original boss display path");
        int position = bossSites[0].index + 1;
        var resultCall = code[position];
        Need(!code.Any(i => Targets(i, resultCall)), "Boss display call is a branch target");
        Need(!target.Body.ExceptionHandlers.Any(e => ReferenceEquals(e.TryStart, resultCall) || ReferenceEquals(e.TryEnd, resultCall) ||
             ReferenceEquals(e.HandlerStart, resultCall) || ReferenceEquals(e.HandlerEnd, resultCall) || ReferenceEquals(e.FilterStart, resultCall)),
            "Boss display call is an exception boundary");
        code.Insert(position, Instruction.Create(OpCodes.Call, hook));
        target.Body.KeepOldMaxStack = true;
        var options = new ModuleWriterOptions(module);
        options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
        module.Write(output, options);
        Console.WriteLine("PASS inserted existing monster display MemberRef on boss result path");
    }

    public static void Main(string[] args)
    {
        if (args.Length == 3 && args[0] == "--verify-helper") { VerifyHelper(args[1], args[2]); return; }
        if (args.Length != 2) throw new ArgumentException("input and output managed paths required");
        Patch(args[0], args[1]);
    }
}
