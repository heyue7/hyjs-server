using System;
using System.IO;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

using var main = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
var runtime = helper.GetTypes().Single(t => t.FullName == "HYJS.DungeonContent.DungeonContent");
var importer = new Importer(main, ImporterOptions.TryToUseTypeDefs);
IMethod H(string name) => importer.Import(runtime.Methods.Single(m => m.Name == name));
MethodDef Target(uint token, string owner, string name)
{
    var method = (MethodDef)main.ResolveToken(token);
    if (method.DeclaringType.FullName != owner || method.Name != name || !method.HasBody || method.Body.ExceptionHandlers.Count != 0)
        throw new InvalidOperationException("Dungeon63 target differs");
    method.Body.MaxStack = Math.Max(method.Body.MaxStack, (ushort)4);
    method.Body.KeepOldMaxStack = true;
    return method;
}
foreach (uint token in new uint[] { 0x06004192, 0x06004193 })
{
    var method = Target(token, "Room.RoomCharactorManager", "CreateMonster");
    var type = method.MethodSig.Params.Single().ToTypeDefOrRef().ResolveTypeDef();
    if (type == null || !(type.FullName == "ProtoBuf.Message.CopyJobInfo" || type.FullName == "Dict.DataModel.TableCopyTask"))
        throw new InvalidOperationException("Wrong CreateMonster overload");
    var original = method.Body.Instructions[0];
    var added = new System.Collections.Generic.List<Instruction> { Instruction.Create(OpCodes.Ldarg_1) };
    if (type.FullName == "ProtoBuf.Message.CopyJobInfo")
        added.Add(Instruction.Create(OpCodes.Callvirt, type.Methods.Single(m => m.Name == "get_taskId")));
    else added.Add(Instruction.Create(OpCodes.Ldfld, type.Fields.Single(f => f.Name == "TaskId")));
    added.Add(Instruction.Create(OpCodes.Call, H("AllowTask")));
    added.Add(Instruction.Create(OpCodes.Brtrue, original));
    added.Add(Instruction.Create(OpCodes.Ret));
    for (int i = 0; i < added.Count; i++) method.Body.Instructions.Insert(i, added[i]);
}
var complete = Target(0x06004497, "Room.RoomCopy", "OnLoadComplete");
// Keep every original exit reachable, including already-loaded battle return.
foreach (var ret in complete.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
{
    int at = complete.Body.Instructions.IndexOf(ret);
    ret.OpCode = OpCodes.Ldarg_0; ret.Operand = null;
    complete.Body.Instructions.Insert(at + 1, Instruction.Create(OpCodes.Call, H("AdvanceBoss")));
    complete.Body.Instructions.Insert(at + 2, Instruction.Create(OpCodes.Ret));
}

void Write(ModuleDef module, string path)
{
    var options = new ModuleWriterOptions(module);
    options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
    module.Write(path, options);
}
Write(main, args[2]);
// Imported helper signatures refer to existing Main types; normalize actual serialized metadata.
using (var serialized = ModuleDefMD.Load(args[2]))
{
    foreach (var type in serialized.GetTypeRefs())
        if (type.ResolutionScope is ModuleRef scope && scope.Name == "HYJS.DungeonContent.dll")
        {
            if (!serialized.GetTypes().Any(t => t.FullName == type.FullName)) throw new InvalidOperationException("Unknown Main type scope");
            type.ResolutionScope = serialized;
        }
    Write(serialized, args[2] + ".normalized");
    File.Move(args[2] + ".normalized", args[2], true);
}
using (var final = ModuleDefMD.Load(args[2]))
    if (final.GetTypeRefs().Any(t => t.ResolutionScope is ModuleRef scope && scope.Name == "HYJS.DungeonContent.dll"))
        throw new InvalidOperationException("Residual helper-scoped Main type");
