using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;

// Compose with the existing qualified managed overlay. The original Engine
// methods and all ordinary permanent ArrayPage formation actions stay intact.
internal static class FormationGuard
{
    internal static void Apply(ModuleDefMD module)
    {
        TypeDef view = module.GetTypes().Single(t => t.FullName == "Battle.View.BattleViewCtrl");
        FieldDef round = view.Fields.Single(f => f.Name == "roundNum");
        FieldDef fought = view.Fields.Single(f => f.Name == "hasRoundFight");
        MethodDef can = view.Methods.Single(m => m.Name == "IsCanTransfer");
        MethodDef end = view.Methods.Single(m => m.Name == "AskTransEnd");
        if (can.Body.ExceptionHandlers.Count != 0 || end.Body.ExceptionHandlers.Count != 0)
            throw new InvalidOperationException("Original formation guard EH differs");
        Instruction originalCan = can.Body.Instructions[0];
        var beginPrefix = new[] {
            Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, round),
            Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Ble, originalCan),
            Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Ret)
        };
        for (int i=0; i<beginPrefix.Length; i++) can.Body.Instructions.Insert(i, beginPrefix[i]);
        Instruction originalEnd = end.Body.Instructions[0];
        Instruction reject = Instruction.Create(OpCodes.Ret);
        var endPrefix = new[] {
            Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, round),
            Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Bgt, reject),
            Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, fought),
            Instruction.Create(OpCodes.Brfalse, originalEnd), reject
        };
        for (int i=0; i<endPrefix.Length; i++) end.Body.Instructions.Insert(i, endPrefix[i]);
        can.Body.MaxStack = 2; can.Body.KeepOldMaxStack = true;
        end.Body.MaxStack = Math.Max(end.Body.MaxStack, (ushort)2); end.Body.KeepOldMaxStack = true;
        can.Body.UpdateInstructionOffsets(); end.Body.UpdateInstructionOffsets();
    }
}
