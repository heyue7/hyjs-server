// Preserve the original metadata identities and every unmodified instruction.
// Python verifies all original engine bodies after writing before packaging.
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3 && args.Length != 4) throw new ArgumentException("input assembly, diagnostic runner, output assembly required");
bool localPVE = args.Length == 4 && args[3] == "--local-pve-round-compat";
if (args.Length == 4 && !localPVE) throw new ArgumentException("unknown original UI hook option");
using var original = ModuleDefMD.Load(args[0]);
using var runner = ModuleDefMD.Load(args[1]);
var target = original.GetTypes().Single(t => t.Name == "PlaybackKit").Methods.Single(m => m.Name == "OnFightButtonClick");
var hook = runner.GetTypes().Single(t => t.Name == "NativeBatchRunner").Methods.Single(m => m.Name == "TryStart");
if (target.Body.Instructions.Count == 0 || target.Body.ExceptionHandlers.Count != 0)
    throw new InvalidOperationException("PlaybackKit caller shape changed");
var reference = original.Import(hook);
var entry = target.Body.Instructions[0];
var instructions = new[] {
    Instruction.Create(OpCodes.Ldarg_0),
    Instruction.Create(OpCodes.Call, reference),
    Instruction.Create(OpCodes.Brfalse, entry),
    Instruction.Create(OpCodes.Ret)
};
for (int i = 0; i < instructions.Length; i++) target.Body.Instructions.Insert(i, instructions[i]);
var positionHook = original.Import(runner.GetTypes().Single(t => t.Name == "NativeBatchRunner").Methods.Single(m => m.Name == "LogRoomPosition"));
foreach (var pair in new[] { ("RoomNetBase", 0), ("RoomNetUnion", 2) }) {
    var callback = original.GetTypes().Single(t => t.Name == pair.Item1).Methods.Single(m => m.Name == "SCCurrentPosition");
    foreach (var ret in callback.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray()) {
        int at = callback.Body.Instructions.IndexOf(ret);
        var capture = new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldc_I4, pair.Item2 + 1), Instruction.Create(OpCodes.Call, positionHook) };
        foreach (var branch in callback.Body.Instructions) if (branch.Operand == ret) branch.Operand = capture[0];
        for (int i = 0; i < capture.Length; i++) callback.Body.Instructions.Insert(at + i, capture[i]);
    }
    var captureEntry = new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldc_I4, pair.Item2), Instruction.Create(OpCodes.Call, positionHook) };
    for (int i = 0; i < captureEntry.Length; i++) callback.Body.Instructions.Insert(i, captureEntry[i]);
    callback.Body.KeepOldMaxStack = false;
}
var options = new ModuleWriterOptions(original);
// Named local objectives and the aggregated login directory read their server
// captions after the original callbacks. Preserve all original engine bodies.
var activityHook = original.Import(runner.GetTypes().Single(t => t.Name == "NativeBatchRunner").Methods.Single(m => m.Name == "ApplyActivityInformation"));
foreach (var name in new[] { "UI.UIActivityRechargeSurprise", "UI.UIActivityLoginAward", "UI.AcLoginAwardItem" }) {
    bool itemCaption = name == "UI.AcLoginAwardItem";
    var activity = original.GetTypes().Single(t => t.FullName == name).Methods.Single(m => m.Name == (itemCaption ? "SetActivityItem" : "MsgCallback"));
    if ((!itemCaption && activity.Body.ExceptionHandlers.Count != 0) || activity.Body.ExceptionHandlers.Any(h => h.HandlerType != ExceptionHandlerType.Finally)) throw new InvalidOperationException("activity caption callback shape changed");
    foreach (var ret in activity.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray()) {
        int at = activity.Body.Instructions.IndexOf(ret);
        var inserted = new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(itemCaption ? OpCodes.Ldc_I4_1 : OpCodes.Ldarg_2), Instruction.Create(OpCodes.Call, activityHook) };
        foreach (var branch in activity.Body.Instructions) if (branch.Operand == ret) branch.Operand = inserted[0];
        for (int i = 0; i < inserted.Length; i++) activity.Body.Instructions.Insert(at + i, inserted[i]);
    }
    activity.Body.KeepOldMaxStack = false;
}
var previewMethod = original.GetTypes().Single(t => t.FullName == "UI.SelectPlayerPanel").Methods.Single(m => m.Name == "showGift");
if (previewMethod.Body.ExceptionHandlers.Any(h => h.HandlerType != ExceptionHandlerType.Finally)) throw new InvalidOperationException("expedition preview callback shape changed");
var previewHook = original.Import(runner.GetTypes().Single(t => t.Name == "NativeBatchRunner").Methods.Single(m => m.Name == "ApplyExpeditionPreview"));
foreach (var ret in previewMethod.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray()) {
    int at = previewMethod.Body.Instructions.IndexOf(ret);
    var inserted = new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, previewHook) };
    foreach (var branch in previewMethod.Body.Instructions) if (branch.Operand == ret) branch.Operand = inserted[0];
    for (int i = 0; i < inserted.Length; i++) previewMethod.Body.Instructions.Insert(at + i, inserted[i]);
}
previewMethod.Body.KeepOldMaxStack = false;
if (localPVE) {
    // Only non-calculation callbacks receive observers. Preserve the complete
    // original Engine bytecode and issue recovery through its original UI route.
    var bridgeType = runner.GetTypes().Single(t => t.Name == "NativeBatchRunner");
    void BeforeReturns(MethodDef method, Instruction[] loads, string callback) {
        if (method.Body.ExceptionHandlers.Count != 0) throw new InvalidOperationException("original UI observer method shape changed");
        var hookMethod = original.Import(bridgeType.Methods.Single(m => m.Name == callback));
        foreach (var ret in method.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray()) {
            int at = method.Body.Instructions.IndexOf(ret);
            var inserted = loads.Select(i => i.Clone()).Append(Instruction.Create(OpCodes.Call, hookMethod)).ToArray();
            foreach (var branch in method.Body.Instructions) if (branch.Operand == ret) branch.Operand = inserted[0];
            for (int i = 0; i < inserted.Length; i++) method.Body.Instructions.Insert(at + i, inserted[i]);
        }
        method.Body.KeepOldMaxStack = false;
    }
    var callbackType = original.GetTypes().Single(t => t.FullName == "Battle.Communication.EngineMsgCallback");
    BeforeReturns(callbackType.Methods.Single(m => m.Name == "SkillPress"),
        new[] { Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Conv_I4), Instruction.Create(OpCodes.Ldarg_2), Instruction.Create(OpCodes.Ldarg_3) }, "ObservePVEUIPress");
    // Capture SkillBegin at entry so even an original view callback exception
    // cannot retain a stale no-target observation after a real skill begins.
    var begin = callbackType.Methods.Single(m => m.Name == "SkillBegin");
    begin.Body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_1));
    begin.Body.Instructions.Insert(1, Instruction.Create(OpCodes.Call,
        original.Import(bridgeType.Methods.Single(m => m.Name == "ObservePVEUIBegin"))));
    begin.Body.KeepOldMaxStack = false;
    var manager = original.GetTypes().Single(t => t.FullName == "Battle.BattleManager");
    BeforeReturns(manager.Methods.Single(m => m.Name == "Update"),
        new[] { Instruction.Create(OpCodes.Ldarg_0) }, "AfterPVEUIUpdate");
}
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
original.Write(args[2], options);
Console.WriteLine("Appended diagnostic PlaybackKit hook with original metadata identities preserved");
