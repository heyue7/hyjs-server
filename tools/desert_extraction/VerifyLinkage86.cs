// Read-only linkage audit for the eight 86 output assemblies. It does not run
// game code and it does not modify either the inherited or replacement DLLs.
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using System.Text.Json;
using dnlib.DotNet;

internal static class VerifyLinkage86
{
    private static readonly string[] Roots = {
        "Assembly-CSharp", "SoulFlowerDesert53", "SoulFlowerDesertV4",
        "SoulFlowerGGOShop", "HYJS.Feedback65", "HYJS.FragmentBoxes85",
        "HYJS.DungeonContent", "HYJS.WantedSkip87"
    };
    private static readonly string[] InheritedRoots = {
        "Assembly-CSharp", "SoulFlowerGGOShop", "HYJS.Feedback65", "HYJS.DungeonContent"
    };
    private static readonly HashSet<string> RootNames = new HashSet<string>(Roots, StringComparer.OrdinalIgnoreCase);
    private static readonly SigComparerOptions SignatureOptions =
        SigComparerOptions.CompareAssemblyVersion | SigComparerOptions.CompareAssemblyLocale |
        SigComparerOptions.CompareAssemblyPublicKeyToken | SigComparerOptions.CompareSentinelParams |
        SigComparerOptions.MscorlibIsNotSpecial;

    // These five rows are the actual v80 APK's inherited metadata edges.  They
    // are not treated as proof of a Mono redirect or a resolved runtime edge.
    private static readonly FrameworkEdge[] OriginalFrameworkEdges = {
        new FrameworkEdge("UnityEngine", "0.0.0.0", "8d4bd6321a453263f8f97047be50dd04a6f2b8f247456ad23fddfbc75f2c2fa4", "mscorlib", "2.0.0.0", "7f16171cf5f87ca1e4c229727526d2fbc786079fc0c960f36197013ea0d986e1"),
        new FrameworkEdge("UnityEngine", "0.0.0.0", "8d4bd6321a453263f8f97047be50dd04a6f2b8f247456ad23fddfbc75f2c2fa4", "System.Core", "3.5.0.0", "104278a0bf67b4de8fd1e910e3292a3ef51808ebee7a5f2a229125f18463c98c"),
        new FrameworkEdge("UnityEngine", "0.0.0.0", "8d4bd6321a453263f8f97047be50dd04a6f2b8f247456ad23fddfbc75f2c2fa4", "System", "2.0.0.0", "7e72b92ddcd6b7b791ab24dd5756544f2da81f65e7ba7d1ebe8223123720d0ac"),
        new FrameworkEdge("LitJson", "1.0.0.0", "151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230", "mscorlib", "2.0.0.0", "7f16171cf5f87ca1e4c229727526d2fbc786079fc0c960f36197013ea0d986e1"),
        new FrameworkEdge("LitJson", "1.0.0.0", "151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230", "System", "2.0.0.0", "7e72b92ddcd6b7b791ab24dd5756544f2da81f65e7ba7d1ebe8223123720d0ac")
    };

    private sealed class FrameworkEdge
    {
        public readonly string Source, SourceIdentity, SourceSha, Target, Requested, TargetIdentity, TargetSha;
        public FrameworkEdge(string source, string sourceVersion, string sourceSha, string target, string requestedVersion, string targetSha)
        {
            Source = source;
            SourceIdentity = source + ", Version=" + sourceVersion + ", Culture=neutral, PublicKeyToken=null";
            SourceSha = sourceSha;
            Target = target;
            Requested = target + ", Version=" + requestedVersion + ", Culture=neutral, PublicKeyToken=b77a5c561934e089";
            TargetIdentity = target + ", Version=2.0.5.0, Culture=neutral, PublicKeyToken=7cec85d7bea7798e";
            TargetSha = targetSha;
        }
    }

    private sealed class Problem
    {
        public string kind { get; set; } = "";
        public string source { get; set; } = "";
        public string requested { get; set; } = "";
        public string detail { get; set; } = "";
        public bool presentInOriginalClosure { get; set; }
        public string Key() { return kind + "|" + source + "|" + requested + "|" + detail; }
    }

    private sealed class ModuleInventory : IDisposable
    {
        public readonly AssemblyResolver Resolver;
        public readonly ModuleContext Context;
        public readonly Dictionary<string, ModuleDefMD> Modules =
            new Dictionary<string, ModuleDefMD>(StringComparer.OrdinalIgnoreCase);
        public readonly Dictionary<string, string> Hashes = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        public readonly List<Problem> InventoryProblems = new List<Problem>();
        public readonly List<object> Inventory = new List<object>();

        public ModuleInventory(string directory)
        {
            Resolver = new AssemblyResolver {
                FindExactMatch = true, EnableFrameworkRedirect = false, UseGAC = false
            };
            Context = new ModuleContext(Resolver);
            Resolver.DefaultModuleContext = Context;
            Resolver.PreSearchPaths.Add(directory);
            foreach (string path in Directory.EnumerateFiles(directory, "*.dll").OrderBy(x => x, StringComparer.Ordinal))
            {
                ModuleDefMD module = null;
                try
                {
                    module = ModuleDefMD.Load(path, Context);
                    if (module.Assembly == null)
                        throw new InvalidDataException("DLL has no assembly definition");
                    string name = module.Assembly.Name.ToString();
                    if (!String.Equals(Path.GetFileNameWithoutExtension(path), name, StringComparison.OrdinalIgnoreCase))
                        InventoryProblems.Add(new Problem { kind = "filename_identity_mismatch", source = Path.GetFileName(path), requested = name });
                    if (Modules.ContainsKey(name))
                    {
                        InventoryProblems.Add(new Problem { kind = "duplicate_assembly_name", source = Path.GetFileName(path), requested = name });
                        module.Dispose();
                        continue;
                    }
                    Modules.Add(name, module);
                    if (!Resolver.AddToCache(module))
                        InventoryProblems.Add(new Problem { kind = "resolver_cache_rejected", source = Path.GetFileName(path), requested = module.Assembly.FullNameToken });
                    using var stream = File.OpenRead(path);
                    string sha = Convert.ToHexString(SHA256.HashData(stream)).ToLowerInvariant();
                    Hashes.Add(name, sha);
                    Inventory.Add(new { file = Path.GetFileName(path), identity = module.Assembly.FullNameToken, sha256 = sha });
                }
                catch (Exception ex)
                {
                    if (module != null && !Modules.Values.Contains(module)) module.Dispose();
                    InventoryProblems.Add(new Problem { kind = "assembly_load_failed", source = Path.GetFileName(path), detail = ex.GetType().Name + ": " + ex.Message });
                }
            }
        }

        public List<Problem> CheckClosure(IEnumerable<string> roots, out List<string> visited, out List<object> preserved,
            HashSet<string> baselineKeys, Dictionary<string, string> originalIdentities, Dictionary<string, string> originalHashes)
        {
            var problems = new List<Problem>();
            preserved = new List<object>();
            var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            var queue = new Queue<string>();
            foreach (string name in roots)
            {
                if (Modules.ContainsKey(name)) queue.Enqueue(name);
                else problems.Add(new Problem { kind = "root_missing", source = name, requested = name });
            }
            while (queue.Count > 0)
            {
                string sourceName = queue.Dequeue();
                if (!seen.Add(sourceName)) continue;
                ModuleDefMD source = Modules[sourceName];
                var references = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
                foreach (AssemblyRef reference in source.GetAssemblyRefs())
                {
                    string name = reference.Name.ToString();
                    string requested = reference.FullNameToken;
                    if (!references.Add(name))
                        problems.Add(new Problem { kind = "duplicate_assembly_reference", source = sourceName, requested = requested });
                    if (!Modules.TryGetValue(name, out ModuleDefMD target))
                    {
                        problems.Add(new Problem { kind = "assembly_reference_missing", source = sourceName, requested = requested });
                        continue;
                    }
                    if (!AssemblyNameComparer.CompareAll.Equals(reference, target.Assembly))
                    {
                        var mismatch = new Problem { kind = "assembly_identity_mismatch", source = sourceName,
                            requested = requested, detail = "available: " + target.Assembly.FullNameToken };
                        string sourceIdentity = source.Assembly.FullNameToken;
                        string targetIdentity = target.Assembly.FullNameToken;
                        string sourceHash = Hashes.TryGetValue(sourceName, out string sh) ? sh : "";
                        string targetHash = Hashes.TryGetValue(name, out string th) ? th : "";
                        if (IsPreservedFrameworkEdge(mismatch, sourceIdentity, sourceHash, targetIdentity, targetHash,
                                                     baselineKeys, originalIdentities, originalHashes))
                        {
                            preserved.Add(new { source = sourceName, sourceIdentity = sourceIdentity, sourceSHA256 = sourceHash,
                                requestedIdentity = requested, target = name, targetIdentity = targetIdentity,
                                targetSHA256 = targetHash, presentInOriginalClosure = true,
                                exactIdentityResolved = false, runtimeResolutionProven = false });
                            // Scan the pinned target's own references as well; this is not a claim
                            // that the Mono runtime resolved this mismatched edge in the verifier.
                            queue.Enqueue(name);
                            continue;
                        }
                        problems.Add(mismatch);
                        continue;
                    }
                    try
                    {
                        AssemblyDef resolved = Resolver.Resolve(reference, source);
                        if (resolved == null || !ReferenceEquals(resolved, target.Assembly))
                        {
                            problems.Add(new Problem { kind = "assembly_resolver_mismatch", source = sourceName, requested = requested,
                                detail = resolved == null ? "unresolved" : "resolved outside the pinned inventory: " + resolved.FullNameToken });
                            continue;
                        }
                    }
                    catch (Exception ex)
                    {
                        problems.Add(new Problem { kind = "assembly_resolver_exception", source = sourceName, requested = requested,
                            detail = ex.GetType().Name + ": " + ex.Message });
                        continue;
                    }
                    queue.Enqueue(name);
                }
            }
            visited = seen.OrderBy(x => x, StringComparer.Ordinal).ToList();
            return problems;
        }

        public void Dispose()
        {
            // Every retained module is in the resolver cache and is disposed here.
            Resolver.Clear();
            foreach (ModuleDefMD module in Modules.Values) module.Dispose();
        }
    }

    private static bool IsPreservedFrameworkEdge(Problem mismatch, string sourceIdentity, string sourceHash,
        string targetIdentity, string targetHash, HashSet<string> baselineKeys,
        Dictionary<string, string> originalIdentities, Dictionary<string, string> originalHashes)
    {
        if (mismatch.kind != "assembly_identity_mismatch" || baselineKeys == null ||
            originalIdentities == null || originalHashes == null || !baselineKeys.Contains(mismatch.Key())) return false;
        foreach (FrameworkEdge edge in OriginalFrameworkEdges)
        {
            if (!String.Equals(mismatch.source, edge.Source, StringComparison.Ordinal) ||
                !String.Equals(mismatch.requested, edge.Requested, StringComparison.Ordinal) ||
                !String.Equals(sourceIdentity, edge.SourceIdentity, StringComparison.Ordinal) ||
                !String.Equals(targetIdentity, edge.TargetIdentity, StringComparison.Ordinal) ||
                !String.Equals(mismatch.detail, "available: " + edge.TargetIdentity, StringComparison.Ordinal) ||
                !String.Equals(sourceHash, edge.SourceSha, StringComparison.Ordinal) ||
                !String.Equals(targetHash, edge.TargetSha, StringComparison.Ordinal)) continue;
            return originalIdentities.TryGetValue(edge.Source, out string originalSourceIdentity) &&
                   originalIdentities.TryGetValue(edge.Target, out string originalTargetIdentity) &&
                   originalHashes.TryGetValue(edge.Source, out string originalSourceHash) &&
                   originalHashes.TryGetValue(edge.Target, out string originalTargetHash) &&
                   originalSourceIdentity == edge.SourceIdentity && originalTargetIdentity == edge.TargetIdentity &&
                   originalSourceHash == edge.SourceSha && originalTargetHash == edge.TargetSha;
        }
        return false;
    }

    private static int TestFrameworkPolicy()
    {
        int checks = 0;
        foreach (FrameworkEdge edge in OriginalFrameworkEdges)
        {
            var mismatch = new Problem { kind = "assembly_identity_mismatch", source = edge.Source,
                requested = edge.Requested, detail = "available: " + edge.TargetIdentity };
            var keys = new HashSet<string>(new[] { mismatch.Key() }, StringComparer.OrdinalIgnoreCase);
            var identities = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase) {
                [edge.Source] = edge.SourceIdentity, [edge.Target] = edge.TargetIdentity
            };
            var hashes = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase) {
                [edge.Source] = edge.SourceSha, [edge.Target] = edge.TargetSha
            };
            bool Check(string source, string sourceHash, string target, string targetHash,
                       HashSet<string> originalKeys, Dictionary<string, string> originalIdentities,
                       Dictionary<string, string> originalHashes) =>
                IsPreservedFrameworkEdge(mismatch, source, sourceHash, target, targetHash,
                                         originalKeys, originalIdentities, originalHashes);
            if (!Check(edge.SourceIdentity, edge.SourceSha, edge.TargetIdentity, edge.TargetSha, keys, identities, hashes))
                throw new InvalidOperationException("Exact original edge rejected");
            checks++;
            foreach (bool accepted in new[] {
                Check(edge.SourceIdentity, new string('0', 64), edge.TargetIdentity, edge.TargetSha, keys, identities, hashes),
                Check(edge.SourceIdentity, edge.SourceSha, edge.TargetIdentity, new string('0', 64), keys, identities, hashes),
                Check(edge.SourceIdentity, edge.SourceSha, edge.TargetIdentity + "-changed", edge.TargetSha, keys, identities, hashes),
                Check(edge.SourceIdentity, edge.SourceSha, edge.TargetIdentity, edge.TargetSha, new HashSet<string>(), identities, hashes)
            })
            {
                if (accepted) throw new InvalidOperationException("Changed or unproven edge was admitted");
                checks++;
            }
            mismatch.kind = "member_reference_exception";
            if (Check(edge.SourceIdentity, edge.SourceSha, edge.TargetIdentity, edge.TargetSha, keys, identities, hashes))
                throw new InvalidOperationException("Non-assembly issue was admitted");
            checks++;
            mismatch.kind = "assembly_identity_mismatch";
            mismatch.source = "HYJS.Feedback65";
            if (Check(edge.SourceIdentity, edge.SourceSha, edge.TargetIdentity, edge.TargetSha, keys, identities, hashes))
                throw new InvalidOperationException("Helper edge was admitted");
            checks++;
        }
        Console.WriteLine(JsonSerializer.Serialize(new { schema = "hyjs-linkage86-framework-policy-fixture-v1", checks = checks }));
        return 0;
    }

    private sealed class BaselineReferences
    {
        public readonly HashSet<string> Types = new HashSet<string>(StringComparer.Ordinal);
        public readonly HashSet<string> Members = new HashSet<string>(StringComparer.Ordinal);
    }

    private static string AssemblyIdentity(IAssembly assembly)
    {
        return assembly == null ? "<no-assembly>" : assembly.FullNameToken;
    }

    private static string TypeKey(TypeRef type)
    {
        string scope = type.ResolutionScope == null ? "<no-scope>" : type.ResolutionScope.ToString();
        return type.FullName + "|" + AssemblyIdentity(type.DefinitionAssembly) + "|" + scope;
    }

    private static string MemberKey(MemberRef member)
    {
        ITypeDefOrRef owner = member.DeclaringType;
        return member.FullName + "|" + (owner == null ? "<no-owner>" : owner.AssemblyQualifiedName) +
               "|" + (member.Signature == null ? "<no-signature>" : member.Signature.ToString());
    }

    private static BaselineReferences CaptureReferences(ModuleDefMD module)
    {
        var refs = new BaselineReferences();
        foreach (TypeRef type in module.GetTypeRefs()) refs.Types.Add(TypeKey(type));
        foreach (MemberRef member in module.GetMemberRefs()) refs.Members.Add(MemberKey(member));
        return refs;
    }

    private static string RootAssemblyName(ITypeDefOrRef type)
    {
        if (type == null) return null;
        IAssembly assembly = type.DefinitionAssembly;
        if (assembly != null)
        {
            string name = assembly.Name.ToString();
            if (RootNames.Contains(name)) return name;
        }
        // Invalid module-scoped references must still be inspected if their
        // scope names one of the outputs; resolution will then fail closed.
        if (type.Scope is ModuleRef moduleRef)
        {
            string name = Path.GetFileNameWithoutExtension(moduleRef.Name.ToString());
            if (RootNames.Contains(name)) return name;
        }
        return null;
    }

    private static bool Exported(TypeDef type)
    {
        if (type.DeclaringType == null) return type.IsPublic;
        return Exported(type.DeclaringType) && (type.IsNestedPublic || type.IsNestedFamily ||
               type.IsNestedFamilyOrAssembly || type.IsNestedFamilyAndAssembly);
    }

    private static Dictionary<string, int> Api(ModuleDefMD module)
    {
        var rows = new Dictionary<string, int>(StringComparer.Ordinal);
        void Add(string value) { rows[value] = rows.TryGetValue(value, out int n) ? n + 1 : 1; }
        foreach (TypeDef type in module.GetTypes())
        {
            if (Exported(type))
                Add("T|" + type.FullName + "|" + type.Attributes + "|" + (type.BaseType == null ? "" : type.BaseType.AssemblyQualifiedName));
            foreach (FieldDef field in type.Fields.Where(x => x.IsPublic || x.IsFamily || x.IsFamilyOrAssembly || x.IsFamilyAndAssembly))
                Add("F|" + field.FullName + "|" + field.Attributes);
            foreach (MethodDef method in type.Methods.Where(x => x.IsPublic || x.IsFamily || x.IsFamilyOrAssembly || x.IsFamilyAndAssembly))
                Add("M|" + method.FullName + "|" + method.Attributes + "|" + method.ImplAttributes);
        }
        return rows;
    }

    private static bool IsPinnedAssembly(ModuleInventory inventory, ModuleDef module)
    {
        if (module == null || module.Assembly == null) return false;
        string name = module.Assembly.Name.ToString();
        return inventory.Modules.TryGetValue(name, out ModuleDefMD pinned) &&
               ReferenceEquals(module.Assembly, pinned.Assembly);
    }

    private static List<Problem> CheckDirectBindings(ModuleInventory inventory,
        Dictionary<string, BaselineReferences> baseline, out List<object> counts)
    {
        var problems = new List<Problem>();
        counts = new List<object>();
        foreach (string name in Roots)
        {
            if (!inventory.Modules.TryGetValue(name, out ModuleDefMD module)) continue;
            baseline.TryGetValue(name, out BaselineReferences old);
            int typeCount = 0, methodCount = 0, fieldCount = 0;
            int newTypeCount = 0, newMethodCount = 0, newFieldCount = 0;
            foreach (TypeRef type in module.GetTypeRefs())
            {
                bool added = old == null || !old.Types.Contains(TypeKey(type));
                string targetName = RootAssemblyName(type);
                if (targetName == null) continue;
                typeCount++;
                if (added) newTypeCount++;
                try
                {
                    TypeDef resolved = type.ResolveTypeDef();
                    if (resolved == null || resolved.FullName != type.FullName || resolved.Module.Assembly == null ||
                        !inventory.Modules.TryGetValue(targetName, out ModuleDefMD expected) ||
                        !ReferenceEquals(resolved.Module.Assembly, expected.Assembly))
                        problems.Add(new Problem { kind = "type_reference_unresolved", source = name,
                            requested = type.AssemblyQualifiedName, detail = resolved == null ? "unresolved" : resolved.AssemblyQualifiedName });
                }
                catch (Exception ex)
                {
                    problems.Add(new Problem { kind = "type_reference_exception", source = name, requested = type.AssemblyQualifiedName,
                        detail = ex.GetType().Name + ": " + ex.Message });
                }
            }
            foreach (MemberRef member in module.GetMemberRefs())
            {
                bool added = old == null || !old.Members.Contains(MemberKey(member));
                if (RootAssemblyName(member.DeclaringType) == null) continue;
                try
                {
                    var comparer = new SigComparer(SignatureOptions);
                    if (member.IsMethodRef)
                    {
                        methodCount++;
                        if (added) newMethodCount++;
                        MethodDef actual = member.ResolveMethod();
                        if (actual == null || !IsPinnedAssembly(inventory, actual.Module) ||
                            !String.Equals(actual.Name.ToString(), member.Name.ToString(), StringComparison.Ordinal) ||
                            !comparer.Equals(member.MethodSig, actual.MethodSig))
                            problems.Add(new Problem { kind = "method_reference_unresolved_or_signature_mismatch", source = name,
                                requested = member.FullName, detail = actual == null ? "unresolved" : actual.FullName });
                    }
                    else if (member.IsFieldRef)
                    {
                        fieldCount++;
                        if (added) newFieldCount++;
                        FieldDef actual = member.ResolveField();
                        if (actual == null || !IsPinnedAssembly(inventory, actual.Module) ||
                            !String.Equals(actual.Name.ToString(), member.Name.ToString(), StringComparison.Ordinal) ||
                            !comparer.Equals(member.FieldSig, actual.FieldSig))
                            problems.Add(new Problem { kind = "field_reference_unresolved_or_signature_mismatch", source = name,
                                requested = member.FullName, detail = actual == null ? "unresolved" : actual.FullName });
                    }
                    else problems.Add(new Problem { kind = "member_signature_invalid", source = name, requested = member.FullName });
                }
                catch (Exception ex)
                {
                    problems.Add(new Problem { kind = "member_reference_exception", source = name, requested = member.FullName,
                        detail = ex.GetType().Name + ": " + ex.Message });
                }
            }
            counts.Add(new { source = name, checkedTypeReferences = typeCount, checkedMethodReferences = methodCount,
                checkedFieldReferences = fieldCount, newTypeReferences = newTypeCount,
                newMethodReferences = newMethodCount, newFieldReferences = newFieldCount });
        }
        return problems;
    }

    public static int Main(string[] args)
    {
        if (args.Length == 1 && args[0] == "--self-test")
        {
            try { return TestFrameworkPolicy(); }
            catch (Exception ex) { Console.Error.WriteLine(ex.ToString()); return 1; }
        }
        if (args.Length != 2)
        {
            Console.Error.WriteLine("Expected original-managed-directory and final-managed-directory");
            return 2;
        }
        try
        {
            var baselineRefs = new Dictionary<string, BaselineReferences>(StringComparer.OrdinalIgnoreCase);
            var baselineProblems = new List<Problem>();
            var baselineIdentities = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
            var baselineHashes = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
            Dictionary<string, int> originalFeedbackApi;
            string originalFeedbackIdentity;
            List<string> baselineVisited;
            using (var original = new ModuleInventory(args[0]))
            {
                baselineProblems.AddRange(original.InventoryProblems);
                baselineProblems.AddRange(original.CheckClosure(InheritedRoots, out baselineVisited, out _, null, null, null));
                foreach (var module in original.Modules)
                    baselineIdentities.Add(module.Key, module.Value.Assembly.FullNameToken);
                foreach (var hash in original.Hashes)
                    baselineHashes.Add(hash.Key, hash.Value);
                foreach (string root in InheritedRoots)
                    if (original.Modules.TryGetValue(root, out ModuleDefMD module))
                    {
                        baselineRefs.Add(root, CaptureReferences(module));
                    }
                if (!original.Modules.TryGetValue("HYJS.Feedback65", out ModuleDefMD oldFeedback))
                    throw new InvalidDataException("Original Feedback65 is missing");
                originalFeedbackApi = Api(oldFeedback);
                originalFeedbackIdentity = oldFeedback.Assembly.FullNameToken;
            }
            var baselineKeys = new HashSet<string>(baselineProblems.Select(x => x.Key()), StringComparer.OrdinalIgnoreCase);
            using var final = new ModuleInventory(args[1]);
            var problems = new List<Problem>(final.InventoryProblems);
            problems.AddRange(final.CheckClosure(Roots, out List<string> visited, out List<object> preserved,
                                                 baselineKeys, baselineIdentities, baselineHashes));
            foreach (Problem problem in problems) problem.presentInOriginalClosure = baselineKeys.Contains(problem.Key());
            foreach (string root in InheritedRoots)
                if (baselineIdentities.TryGetValue(root, out string originalIdentity) &&
                    final.Modules.TryGetValue(root, out ModuleDefMD module) &&
                    !String.Equals(originalIdentity, module.Assembly.FullNameToken, StringComparison.OrdinalIgnoreCase))
                    problems.Add(new Problem { kind = "inherited_assembly_identity_changed", source = root,
                        requested = originalIdentity, detail = module.Assembly.FullNameToken });
            problems.AddRange(CheckDirectBindings(final, baselineRefs, out List<object> bindingCounts));
            int feedbackOriginalRows = originalFeedbackApi.Values.Sum();
            int feedbackFinalRows = 0;
            if (!final.Modules.TryGetValue("HYJS.Feedback65", out ModuleDefMD newFeedback))
                problems.Add(new Problem { kind = "feedback_helper_missing", source = "HYJS.Feedback65" });
            else
            {
                if (!String.Equals(originalFeedbackIdentity, newFeedback.Assembly.FullNameToken, StringComparison.OrdinalIgnoreCase))
                    problems.Add(new Problem { kind = "feedback_assembly_identity_changed", source = "HYJS.Feedback65",
                        requested = originalFeedbackIdentity, detail = newFeedback.Assembly.FullNameToken });
                Dictionary<string, int> actual = Api(newFeedback);
                feedbackFinalRows = actual.Values.Sum();
                foreach (var row in originalFeedbackApi)
                    if (!actual.TryGetValue(row.Key, out int n) || n < row.Value)
                        problems.Add(new Problem { kind = "feedback_existing_api_changed", source = "HYJS.Feedback65", requested = row.Key,
                            detail = "original count=" + row.Value + ", final count=" + n });
            }
            var result = new {
                schema = "hyjs-combined86-linkage-result-v1",
                status = problems.Count == 0 && preserved.Count == OriginalFrameworkEdges.Length ?
                    "LINKAGE_STATIC_NO_NEW_ERRORS_WITH_BASELINE_MISMATCHES" : "LINKAGE_BLOCKED",
                roots = Roots, inventory = final.Inventory, closureVisited = visited,
                directBindingCounts = bindingCounts,
                feedbackOriginalApiRows = feedbackOriginalRows, feedbackFinalApiRows = feedbackFinalRows,
                originalClosureRoots = InheritedRoots, originalClosureVisited = baselineVisited,
                originalClosureProblems = baselineProblems, preservedOriginalFrameworkIdentityMismatches = preserved,
                problems = problems, expectedPreservedFrameworkEdges = OriginalFrameworkEdges.Length,
                strictAssemblyIdentityClosure = false, runtimeResolutionProven = false,
                exactIdentityRequiredForOtherEdges = true, allOutputDirectBindingsMustResolve = true,
                unityExecuted = false, deviceExecuted = false, serverExecuted = false
            };
            Console.WriteLine(JsonSerializer.Serialize(result));
            return problems.Count == 0 && preserved.Count == OriginalFrameworkEdges.Length ? 0 : 1;
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine(ex.ToString());
            return 2;
        }
    }
}
