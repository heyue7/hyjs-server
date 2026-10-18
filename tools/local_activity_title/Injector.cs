using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input managed DLL and output managed DLL required");
using var module = ModuleDefMD.Load(args[0]);
var method = module.GetTypes().Single(t => t.FullName == "UI.UIActivity")
    .Methods.Single(m => m.Name == "InitTabs");
if (!method.HasBody || method.IsStatic) throw new InvalidOperationException("Original activity method shape differs");
var code = method.Body.Instructions;
var matches = new List<int>();
for (var i = 1; i + 8 < code.Count; i++)
{
    if (code[i].OpCode != OpCodes.Ldsfld || code[i].Operand is not IField empty ||
        empty.DeclaringType.FullName != "System.String" || empty.Name != "Empty") continue;
    if (code[i-1].OpCode != OpCodes.Ldfld || code[i-1].Operand is not IField label ||
        label.DeclaringType.FullName != "UI.UIActivityTabs" || label.Name != "name_label") continue;
    if (code[i+1].OpCode != OpCodes.Call || code[i+1].Operand is not IMethod getText ||
        getText.DeclaringType.FullName != "TableTextBlo" || getText.Name != "GetText") continue;
    if (code[i+2].OpCode != OpCodes.Callvirt || code[i+2].Operand is not IMethod setText ||
        setText.DeclaringType.FullName != "UILabel" || setText.Name != "set_text") continue;
    if (code.Skip(i+3).Take(8).Count(x => x.OpCode == OpCodes.Ldftn && x.Operand is IMethod callback &&
        callback.DeclaringType.FullName == "UI.UIActivityEvent" && callback.Name == "OnClickRechangeDouble") != 1) continue;
    matches.Add(i);
}
if (matches.Count != 1) throw new InvalidOperationException("Expected one untouched original Type33 title; already patched or unknown input");
var at = matches[0];
// Both the old field load and new ldstr occupy five bytes. Replace the old
// GetText call by five nops so branch targets and all EH offsets remain fixed.
code[at].OpCode = OpCodes.Ldstr;
code[at].Operand = "首充双倍";
code[at+1].OpCode = OpCodes.Nop;
code[at+1].Operand = null;
for (var n = 0; n < 4; n++) code.Insert(at+2+n, Instruction.Create(OpCodes.Nop));
method.Body.KeepOldMaxStack = true;
var page = module.GetTypes().Single(t => t.FullName == "UI.CopyBenefit")
    .Methods.Single(m => m.Name == "PrepareData");
if (!page.HasBody || page.IsStatic) throw new InvalidOperationException("Original CopyBenefit shape differs");
var pageCode = page.Body.Instructions;
var pageLoad = pageCode.Single(i => i.Offset == 0x55 && i.OpCode == OpCodes.Ldstr &&
    i.Operand is string key && key == "activity_warrior_weight_improve_title");
var pageAt = pageCode.IndexOf(pageLoad);
if (pageCode[pageAt+1].OpCode != OpCodes.Call || pageCode[pageAt+1].Operand is not IMethod pageGet ||
    pageGet.DeclaringType.FullName != "TableTextBlo" || pageGet.Name != "GetText" ||
    !pageCode.Take(pageAt).TakeLast(6).Any(i => i.OpCode == OpCodes.Ldc_I4_S && Convert.ToInt32(i.Operand) == 33))
    throw new InvalidOperationException("Expected original type33-only page title");
pageLoad.Operand = "首充双倍";
pageCode[pageAt+1].OpCode = OpCodes.Nop;
pageCode[pageAt+1].Operand = null;
for (var n = 0; n < 4; n++) pageCode.Insert(pageAt+2+n, Instruction.Create(OpCodes.Nop));
page.Body.KeepOldMaxStack = true;
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Patched only original Type33 tab and page titles to 首充双倍; original type13/31 and gameplay gates retained");
