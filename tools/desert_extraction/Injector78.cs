using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2 && args.Length != 4 && args.Length != 5 && args.Length != 6 && args.Length != 7) throw new ArgumentException("input managed, output managed [, bundle size, loader [, walk-only [, entry [, resume]]]]");
using var module = ModuleDefMD.Load(args[0]);
var method = module.GetTypes().Single(t => t.FullName == "CameraTool").Methods.Single(m => m.Name == "SetTransRender");
var assembly = new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerDesert53, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper = new TypeRefUser(module, "HYJS.Desert53", "DesertOcclusion", assembly);
var cameraUpdate = module.GetTypes().Single(t => t.FullName == "CameraTool").Methods.Single(m => m.Name == "Update");
var rays = cameraUpdate.Body.Instructions.Where(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName == "UnityEngine.Physics" && m.Name == "RaycastAll").ToArray();
if (rays.Length != 1) throw new InvalidOperationException("Expected one original camera ray query");
var originalRay = (IMethod)rays[0].Operand;
if (originalRay.MethodSig.Params.Count != 3 || originalRay.MethodSig.Params[0].FullName != "UnityEngine.Ray" || originalRay.MethodSig.Params[1].ElementType != ElementType.R4 || originalRay.MethodSig.Params[2].ElementType != ElementType.I4) throw new InvalidOperationException("Unknown camera ray signature");
rays[0].Operand = new MemberRefUser(module, "RaycastAll", originalRay.MethodSig, helper);
foreach (var pair in new[] { new[] { "set_shader", "SetShader" }, new[] { "set_color", "SetColor" } })
{
    var calls = method.Body.Instructions.Where(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m && m.DeclaringType.FullName == "UnityEngine.Material" && m.Name == pair[0]).ToArray();
    if (calls.Length != 1) throw new InvalidOperationException("Expected one original material setter: " + pair[0]);
    var original = (IMethod)calls[0].Operand;
    calls[0].OpCode = OpCodes.Call;
    calls[0].Operand = new MemberRefUser(module, pair[1], MethodSig.CreateStatic(module.CorLibTypes.Void, original.DeclaringType.ToTypeSig(), original.MethodSig.Params[0]), helper);
}
if (args.Length >= 4)
{
    if (args[3] != "loader") throw new ArgumentException("Expected explicit loader mode");
    var loader = module.GetTypes().Single(t => t.FullName == "AssetLoad.AssetBundleManager");
    var bundle = new TypeRefUser(module, "HYJS.Desert53", "DesertBundle", assembly);
    var isTarget = new MemberRefUser(module, "IsTarget", MethodSig.CreateStatic(module.CorLibTypes.Boolean, module.CorLibTypes.String), bundle);
    void Prefix(string name, params Instruction[] instructions)
    {
        var target = loader.Methods.Single(m => m.Name == name);
        for (int i = instructions.Length - 1; i >= 0; --i) target.Body.Instructions.Insert(0, instructions[i]);
    }
    foreach (string name in new[] { "IsAssetExist", "GetSize" })
    {
        var first = loader.Methods.Single(m => m.Name == name).Body.Instructions[0];
        Prefix(name, Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, isTarget),
            Instruction.Create(OpCodes.Brfalse, first),
            name == "IsAssetExist" ? Instruction.Create(OpCodes.Ldc_I4_1) : Instruction.Create(OpCodes.Ldc_I8, long.Parse(args[2])),
            Instruction.Create(OpCodes.Ret));
    }
    var resolver = new MemberRefUser(module, "ResolvePath", MethodSig.CreateStatic(module.CorLibTypes.String,
        module.CorLibTypes.String, new ByRefSig(module.CorLibTypes.Boolean), module.CorLibTypes.Boolean), bundle);
    var discard = Instruction.Create(OpCodes.Pop);
    Prefix("GetFullPath", Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2),
        Instruction.Create(OpCodes.Ldarg_3), Instruction.Create(OpCodes.Call, resolver), Instruction.Create(OpCodes.Dup),
        Instruction.Create(OpCodes.Brfalse, discard), Instruction.Create(OpCodes.Ret), discard);
    foreach (string name in new[] { "LoadAssetBundle", "IsLoadedAssetBundle", "UnloadAssetBundle", "UnloadBundleOnly" })
    {
        var target = loader.Methods.Single(m => m.Name == name);
        if (name == "LoadAssetBundle")
        {
            // v78 already routes this site through PetPortraitAssets75. Preserve
            // that exact call for every non-desert bundle; only the new scene is
            // self-contained and must bypass the old manifest lookup.
            var calls = target.Body.Instructions.Where(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m &&
                m.DeclaringType.FullName == "HYJS.GGO.PetPortraitAssets75" && m.Name == "Dependencies").ToArray();
            if (calls.Length != 1) throw new InvalidOperationException("Expected one inherited portrait dependency wrapper in " + name);
            var call = calls[0]; var original = (IMethod)call.Operand;
            if (original.MethodSig.Params.Count != 2 || original.MethodSig.Params[0].FullName != "UnityEngine.AssetBundleManifest" || original.MethodSig.Params[1].FullName != "System.String") throw new InvalidOperationException("Unexpected portrait dependency signature");
            var next = target.Body.Instructions[target.Body.Instructions.IndexOf(call)+1];
            var get = new MemberRefUser(module, "GetDependencies", original.MethodSig, bundle);
            int at = target.Body.Instructions.IndexOf(call);
            foreach (Instruction added in new[] { Instruction.Create(OpCodes.Dup), Instruction.Create(OpCodes.Call,isTarget), Instruction.Create(OpCodes.Brfalse,call), Instruction.Create(OpCodes.Call,get), Instruction.Create(OpCodes.Br,next) }) target.Body.Instructions.Insert(at++,added);
        }
        else
        {
            var calls = target.Body.Instructions.Where(i => i.OpCode == OpCodes.Callvirt && i.Operand is IMethod m &&
                m.DeclaringType.FullName == "UnityEngine.AssetBundleManifest" && m.Name == "GetAllDependencies").ToArray();
            if (calls.Length != 1) throw new InvalidOperationException("Expected one dependency query in " + name);
            var original = (IMethod)calls[0].Operand;
            calls[0].OpCode = OpCodes.Call;
            calls[0].Operand = new MemberRefUser(module, "GetDependencies", MethodSig.CreateStatic(original.MethodSig.RetType,
                original.DeclaringType.ToTypeSig(), module.CorLibTypes.String), bundle);
        }
    }
}
if (args.Length >= 5 && args[4] == "walk-only")
{
    var walk = new TypeRefUser(module, "HYJS.Desert53", "DesertWalkOnly", assembly);
    var active = new MemberRefUser(module, "IsActive", MethodSig.CreateStatic(module.CorLibTypes.Boolean), walk);
    foreach (var pair in new[] {
        new[] {"Room.RoomNetField", "SCFieldResourceInfo"},
        new[] {"Room.RoomNetField", "SCSideMonster"},
        new[] {"Room.RoomNetField", "SCSideBoss"},
        new[] {"Room.RoomNetField", "SCSideWorldBoss"},
        new[] {"Room.RoomBase", "LoadNPCDataFromTable"},
        new[] {"Room.RoomField", "LoadSafeArea"}})
    {
        var target = module.GetTypes().Single(t => t.FullName == pair[0]).Methods.Single(m => m.Name == pair[1]);
        if (target.MethodSig.RetType.ElementType != ElementType.Void) throw new InvalidOperationException("Expected void content handler");
        var first = target.Body.Instructions[0];
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ret));
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Brfalse, first));
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Call, active));
    }
}
if (args.Length >= 5 && args[4] != "walk-only" && args[4] != "live") throw new ArgumentException("Expected walk-only or live mode");
if (args.Length >= 6)
{
    if (args[5] != "entry") throw new ArgumentException("Expected explicit desert entry mode");
    var page = module.GetTypes().Single(t => t.FullName == "UI.WorldMapPage");
    var snow = page.Methods.Single(m => m.Name == "isXueshanOpen");
    var desert = page.Methods.Single(m => m.Name == "isShamoOpen");
    if (snow.Body.Instructions.Count != 54 || snow.Body.ExceptionHandlers.Count != 0) throw new InvalidOperationException("Unknown native eligibility body");
    var body = new CilBody { InitLocals = snow.Body.InitLocals, MaxStack = snow.Body.MaxStack, KeepOldMaxStack = true };
    var locals = snow.Body.Variables.ToDictionary(v => v, v => new Local(v.Type));
    foreach (var v in snow.Body.Variables) body.Variables.Add(locals[v]);
    var clones = snow.Body.Instructions.ToDictionary(i => i, i => Instruction.Create(OpCodes.Nop));
    var fields = new[] { "Type", "Level", "Flag", "Lock", "Text" }.ToDictionary(s => "xueshan" + s, s => page.Fields.Single(f => f.Name == "shamo" + s));
    foreach (var old in snow.Body.Instructions)
    {
        var revised = clones[old]; revised.OpCode = old.OpCode;
        revised.Operand = old.Operand switch {
            Instruction i => clones[i],
            Instruction[] items => items.Select(i => clones[i]).ToArray(),
            Local local => locals[local],
            Parameter p => desert.Parameters[p.Index],
            IField f when fields.ContainsKey(f.Name) => fields[f.Name],
            string s when s == "field_002" => "field_003",
            _ => old.Operand
        };
        body.Instructions.Add(revised);
    }
    desert.Body = body;
    var transfer = page.Methods.Single(m => m.Name == "onTransfer");
    var equality = (IMethod)transfer.Body.Instructions.First(i => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.Name == "op_Equality").Operand;
    void OpenTarget(MethodDef target, int resume, string match)
    {
        var original = target.Body.Instructions[resume];
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Brtrue, original));
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Call, equality));
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldstr, match));
        target.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_1));
    }
    OpenTarget(page.Methods.Single(m => m.Name == "onMapClick"), 63, "2");
    OpenTarget(transfer, 11, "field_003");
    OpenTarget(module.GetTypes().Single(t => t.FullName == "ManagerController").Methods.Single(m => m.Name == "TrySceneTransfer"), 21, "field_003");
}
if (args.Length == 7)
{
    if (args[6] != "resume") throw new ArgumentException("Expected explicit desert login resume mode");
    var login = module.GetTypes().Single(t => t.FullName == "Login.LoginManager").Methods.Single(m => m.Name == "LoginScene");
    var first = login.Body.Instructions[0];
    var walk = new TypeRefUser(module, "HYJS.Desert53", "DesertWalkOnly", assembly);
    var resume = new MemberRefUser(module, "ResumeLogin", MethodSig.CreateStatic(module.CorLibTypes.Boolean), walk);
    login.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ret));
    login.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Brfalse, first));
    login.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Call, resume));
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine(args.Length == 2 ? "Only two CameraTool material setter instructions adapted" : "Camera setters and exact desert bundle loader boundaries adapted");
