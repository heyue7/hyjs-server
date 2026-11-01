// Compare all inherited helper metadata and semantically decoded IL.
using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;

public static class VerifyRechargeHelper
{
    static void Require(bool value, string message) { if (!value) throw new Exception(message); }
    static int At(IList<Instruction> instructions, Instruction value)
    {
        return value == null ? -1 : instructions.IndexOf(value);
    }
    static string Operand(object value, IList<Instruction> instructions)
    {
        if (value == null) return "";
        Instruction target = value as Instruction;
        if (target != null) return "branch:" + At(instructions, target);
        IList<Instruction> targets = value as IList<Instruction>;
        if (targets != null) return "switch:" + String.Join(",", targets.Select(x => At(instructions, x).ToString()).ToArray());
        Local local = value as Local;
        if (local != null) return "local:" + local.Index;
        Parameter parameter = value as Parameter;
        if (parameter != null) return "parameter:" + parameter.Index;
        IFullName full = value as IFullName;
        if (full != null) return "member:" + full.FullName;
        string text = value as string;
        if (text != null) return "string:" + text;
        IFormattable formattable = value as IFormattable;
        return formattable == null ? value.ToString() : formattable.ToString(null, CultureInfo.InvariantCulture);
    }
    static string Body(MethodDef method)
    {
        if (!method.HasBody) return "<no body>";
        CilBody body = method.Body;
        IList<Instruction> instructions = body.Instructions;
        List<string> lines = new List<string>();
        lines.Add("header|" + body.InitLocals + "|" + body.MaxStack);
        foreach (Local local in body.Variables) lines.Add("local|" + local.Type.FullName);
        foreach (Instruction instruction in instructions)
            lines.Add("il|" + instruction.OpCode.Name + "|" + Operand(instruction.Operand, instructions));
        foreach (ExceptionHandler handler in body.ExceptionHandlers)
            lines.Add("eh|" + handler.HandlerType + "|" + At(instructions, handler.TryStart) + "|" +
                At(instructions, handler.TryEnd) + "|" + At(instructions, handler.HandlerStart) + "|" +
                At(instructions, handler.HandlerEnd) + "|" + At(instructions, handler.FilterStart) + "|" +
                (handler.CatchType == null ? "" : handler.CatchType.FullName));
        return String.Join("\n", lines.ToArray());
    }
    static Dictionary<string, T> ByName<T>(IEnumerable<T> items, Func<T, string> name)
    {
        return items.ToDictionary(name, StringComparer.Ordinal);
    }
    public static void Main(string[] args)
    {
        Require(args.Length == 2, "expected shipped and candidate helpers");
        using (ModuleDefMD before = ModuleDefMD.Load(args[0]))
        using (ModuleDefMD after = ModuleDefMD.Load(args[1]))
        {
            Require(before.Assembly.FullName == after.Assembly.FullName && before.Name == after.Name, "assembly identity changed");
            string[] oldRefs = before.GetAssemblyRefs().Select(x => x.FullName).OrderBy(x => x, StringComparer.Ordinal).ToArray();
            string[] newRefs = after.GetAssemblyRefs().Select(x => x.FullName).OrderBy(x => x, StringComparer.Ordinal).ToArray();
            Require(oldRefs.SequenceEqual(newRefs), "assembly references changed");
            var oldTypes = ByName(before.GetTypes(), t => t.FullName);
            var newTypes = ByName(after.GetTypes(), t => t.FullName);
            int inheritedMethods = 0, inheritedFields = 0, inheritedProperties = 0, inheritedEvents = 0, changed = 0;
            foreach (var pair in oldTypes)
            {
                TypeDef current;
                Require(newTypes.TryGetValue(pair.Key, out current), "missing type " + pair.Key);
                TypeDef old = pair.Value;
                Require(old.Attributes == current.Attributes && (old.BaseType == null ? "" : old.BaseType.FullName) ==
                    (current.BaseType == null ? "" : current.BaseType.FullName), "type ABI changed " + pair.Key);
                var fields = ByName(current.Fields, f => f.FullName);
                foreach (FieldDef field in old.Fields)
                {
                    FieldDef replacement;
                    Require(fields.TryGetValue(field.FullName, out replacement) && field.Attributes == replacement.Attributes,
                        "field ABI changed " + field.FullName);
                    inheritedFields++;
                }
                var methods = ByName(current.Methods, m => m.FullName);
                foreach (MethodDef method in old.Methods)
                {
                    MethodDef replacement;
                    Require(methods.TryGetValue(method.FullName, out replacement) && method.Attributes == replacement.Attributes &&
                        method.ImplAttributes == replacement.ImplAttributes, "method ABI changed " + method.FullName);
                    bool allowed = pair.Key == "HYJS.ActivitySafety.RechargeDisplay" && method.Name == "Apply";
                    if (Body(method) != Body(replacement))
                    {
                        Require(allowed, "unrelated method behavior changed " + method.FullName);
                        changed++;
                    }
                    inheritedMethods++;
                }
                var properties = ByName(current.Properties, p => p.FullName);
                foreach (PropertyDef property in old.Properties)
                {
                    PropertyDef replacement;
                    Require(properties.TryGetValue(property.FullName, out replacement) && property.Attributes == replacement.Attributes,
                        "property ABI changed " + property.FullName);
                    inheritedProperties++;
                }
                var events = ByName(current.Events, e => e.FullName);
                foreach (EventDef evt in old.Events)
                {
                    EventDef replacement;
                    Require(events.TryGetValue(evt.FullName, out replacement) && evt.Attributes == replacement.Attributes,
                        "event ABI changed " + evt.FullName);
                    inheritedEvents++;
                }
            }
            Require(changed == 1, "expected exactly the recharge display bridge to change");
            Console.WriteLine("PASS inherited helper metadata: {0} types, {1} methods, {2} fields, {3} properties, {4} events; 1 intended method changed; {5} references unchanged",
                oldTypes.Count, inheritedMethods, inheritedFields, inheritedProperties, inheritedEvents, oldRefs.Length);
        }
    }
}
