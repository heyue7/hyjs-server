using System;
using System.Linq;
using System.Collections.Generic;
using dnlib.DotNet;

using var before = ModuleDefMD.Load(args[0]);
using var after = ModuleDefMD.Load(args[1]);
bool Exported(TypeDef type) => type.IsPublic || type.IsNestedPublic && type.DeclaringType != null && Exported(type.DeclaringType);
string[] Api(ModuleDefMD module)
{
    var rows = new List<string>();
    foreach (var type in module.GetTypes().Where(Exported))
    {
        rows.Add("T|" + type.FullName + "|" + type.Attributes + "|" + type.BaseType?.FullName);
        foreach (var field in type.Fields.Where(x => x.IsPublic || x.IsFamily || x.IsFamilyOrAssembly))
            rows.Add("F|" + type.FullName + "|" + field.Name + "|" + field.Attributes + "|" + field.FieldType.FullName);
        foreach (var method in type.Methods.Where(x => x.IsPublic || x.IsFamily || x.IsFamilyOrAssembly))
            rows.Add("M|" + type.FullName + "|" + method.Name + "|" + method.Attributes + "|" + method.MethodSig);
    }
    return rows.OrderBy(x => x, StringComparer.Ordinal).ToArray();
}
if (before.Assembly.FullName != after.Assembly.FullName) throw new Exception("assembly identity changed");
foreach (var row in Api(before).Except(Api(after))) throw new Exception("inherited API missing: " + row);
Console.WriteLine("PASS inherited public/protected API " + Api(before).Length);
