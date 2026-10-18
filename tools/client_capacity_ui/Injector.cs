using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input DLL, output DLL required");
using var module = ModuleDefMD.Load(args[0]);
var method = module.GetTypes().Single(t => t.FullName == "UI.BagPage").Methods.Single(m => m.Name == "OnCapacityClick");
var body = method.Body;
var differences = body.Instructions.Where((instruction, index) => instruction.OpCode == OpCodes.Sub && index > 0 && body.Instructions[index - 1].Operand is IField field && field.DeclaringType.FullName == "Dict.DataModel.TableVipPerks" && (field.Name == "numPerk3" || field.Name == "numPerk4")).ToArray();
if (differences.Length != 4 || differences.GroupBy(i => ((IField)body.Instructions[body.Instructions.IndexOf(i) - 1].Operand).Name.String).Any(g => g.Count() != 2))
    throw new Exception("Review original character/equipment bag preview branches");
foreach (var difference in differences)
{
    int at = body.Instructions.IndexOf(difference);
    body.Instructions.Insert(at + 1, Instruction.Create(OpCodes.Ldc_I4_S, (sbyte)10));
    body.Instructions.Insert(at + 2, Instruction.Create(OpCodes.Mul));
}
var description = module.GetTypes().Single(t => t.FullName == "VIPData").Methods.Single(m => m.Name == "InitVipDes");
var textValues = description.Body.Instructions.Where((instruction, index) => instruction.OpCode == OpCodes.Ldfld && instruction.Operand is IField field && field.DeclaringType.FullName == "Dict.DataModel.TableVipPerks" && (field.Name == "numPerk3" || field.Name == "numPerk4") && description.Body.Instructions[index + 1].OpCode == OpCodes.Box).ToArray();
if (textValues.Length != 2 || textValues.Select(i => ((IField)i.Operand).Name.String).Distinct().Count() != 2)
    throw new Exception("Review original character/equipment VIP benefit text values");
foreach (var value in textValues)
{
    int at = description.Body.Instructions.IndexOf(value);
    description.Body.Instructions.Insert(at + 1, Instruction.Create(OpCodes.Ldc_I4_S, (sbyte)10));
    description.Body.Instructions.Insert(at + 2, Instruction.Create(OpCodes.Mul));
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Scaled four character/equipment bag preview differences and two VIP benefit text values by ten; prices and levels retained");
