using System;
using System.Globalization;
using System.IO;
using System.Reflection.Emit;

// Runs the two actual opcode bytes under the CLR in a small, isolated compare
// method. It does not load or instantiate the Unity game assembly.
internal static class LabelBranchProbe
{
    private static Func<double, double, double, int> Build(byte first, byte second)
    {
        var method = new DynamicMethod("BreachLabels", typeof(int),
            new[] { typeof(double), typeof(double), typeof(double) });
        ILGenerator il = method.GetILGenerator();
        Label secondTier = il.DefineLabel();
        Label thirdTier = il.DefineLabel();
        il.Emit(OpCodes.Ldarg_0);
        il.Emit(OpCodes.Ldarg_1);
        il.Emit(Branch(first), secondTier);
        il.Emit(OpCodes.Ldc_I4_1);
        il.Emit(OpCodes.Ret);
        il.MarkLabel(secondTier);
        il.Emit(OpCodes.Ldarg_0);
        il.Emit(OpCodes.Ldarg_2);
        il.Emit(Branch(second), thirdTier);
        il.Emit(OpCodes.Ldc_I4_2);
        il.Emit(OpCodes.Ret);
        il.MarkLabel(thirdTier);
        il.Emit(OpCodes.Ldc_I4_3);
        il.Emit(OpCodes.Ret);
        return (Func<double, double, double, int>)method.CreateDelegate(typeof(Func<double, double, double, int>));
    }

    private static OpCode Branch(byte opcode) => opcode switch
    {
        0x42 => OpCodes.Bgt_Un,
        0x41 => OpCodes.Bge_Un,
        _ => throw new InvalidOperationException("unexpected branch opcode"),
    };

    private static int Main(string[] args)
    {
        if (args.Length != 5) throw new ArgumentException("before.dll after.dll offset1 offset2 HeroBreached.tsv");
        byte[] before = File.ReadAllBytes(args[0]), after = File.ReadAllBytes(args[1]);
        int first = int.Parse(args[2], CultureInfo.InvariantCulture);
        int second = int.Parse(args[3], CultureInfo.InvariantCulture);
        if (before[first] != 0x42 || before[second] != 0x42 || after[first] != 0x41 || after[second] != 0x41)
            throw new InvalidOperationException("input/output branch identity differs");
        var original = Build(before[first], before[second]);
        var patched = Build(after[first], after[second]);
        string[] lines = File.ReadAllLines(args[4]);
        int stages = 0, comparisons = 0, changed = 0;
        for (int index = 5; index < lines.Length; index++)
        {
            string[] row = lines[index].Split('\t');
            if (row.Length < 11 || string.IsNullOrEmpty(row[0])) continue;
            int max = checked((int)double.Parse(row[1], CultureInfo.InvariantCulture));
            if (max < 0) break;
            double t1 = double.Parse(row[4], CultureInfo.InvariantCulture);
            double t2 = double.Parse(row[5], CultureInfo.InvariantCulture);
            for (int exp = 0; exp < max; exp++)
            {
                int actual = patched(exp, t1, t2);
                int expected = exp >= t2 ? 3 : exp >= t1 ? 2 : 1;
                if (actual != expected) throw new InvalidOperationException($"stage {row[0]} exp {exp} expected {expected}, got {actual}");
                int old = original(exp, t1, t2);
                if (old != actual)
                {
                    if (exp != t1 && exp != t2) throw new InvalidOperationException("non-boundary label changed");
                    changed++;
                }
                comparisons++;
            }
            stages++;
        }
        if (stages != 24 || changed != 48) throw new InvalidOperationException($"unexpected scope: {stages} stages, {changed} boundaries");
        Console.WriteLine($"CLR_OK stages={stages} expStates={comparisons} changedBoundaryCases={changed}");
        return 0;
    }
}
