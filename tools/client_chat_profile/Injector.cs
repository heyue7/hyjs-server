using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input DLL, helper DLL, output DLL required");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
var factory = helper.GetTypes().Single(t => t.FullName == "HYJS.ChatProfile.ProfilePet");
var create = factory.Methods.Single(m => m.Name == "Create");
var formation = module.GetTypes().Single(t => t.FullName == "UI.UIHeroFormation");
var overloads = formation.Methods.Where(m => m.Name == "InitData" && m.MethodSig != null
    && m.MethodSig.Params.Count == 1
    && m.MethodSig.Params[0].FullName == "ProtoBuf.Message.FormationItemSimpleInfo").ToList();
if (overloads.Count != 1) throw new Exception("Unexpected InitData(FormationItemSimpleInfo) overloads");
var method = overloads[0];
if (method.Body == null) throw new Exception("Target body missing");
var body = method.Body;
int site = -1;
for (int index = 0; index < body.Instructions.Count; index++)
{
    var instruction = body.Instructions[index];
    if (instruction.OpCode != OpCodes.Newobj) continue;
    if (!(instruction.Operand is IMethod constructor)) continue;
    if (constructor.Name != ".ctor" || constructor.DeclaringType.FullName != "UI.UIPet") continue;
    if (site >= 0) throw new Exception("Multiple UIPet construction sites");
    site = index;
}
if (site < 3 || site + 1 >= body.Instructions.Count) throw new Exception("Pinned pet construction site missing");
string Name(Instruction instruction)
{
    if (instruction.Operand is IMethod called) return called.Name;
    if (instruction.Operand is IField field) return field.Name;
    return string.Empty;
}
// The guarded site must be exactly: ldarg.0; ldarg.1; callvirt get_petInfo; newobj UIPet; stfld pet.
if (body.Instructions[site - 3].OpCode != OpCodes.Ldarg_0
    || body.Instructions[site - 2].OpCode != OpCodes.Ldarg_1
    || body.Instructions[site - 1].OpCode != OpCodes.Callvirt
    || Name(body.Instructions[site - 1]) != "get_petInfo"
    || body.Instructions[site + 1].OpCode != OpCodes.Stfld
    || Name(body.Instructions[site + 1]) != "pet")
    throw new Exception("Review changed pet binding sequence");
var changed = body.Instructions[site];
if (create.MethodSig == null || create.MethodSig.RetType.FullName != "UI.UIPet"
    || create.MethodSig.Params.Count != 1
    || create.MethodSig.Params[0].FullName != "ProtoBuf.Message.PetSimpleInfo")
    throw new Exception("Helper Create signature differs");
var petType = module.GetTypes().Single(t => t.FullName == "UI.UIPet");
var infoType = module.GetTypes().Single(t => t.FullName == "ProtoBuf.Message.PetSimpleInfo");
// Unsigned helper: empty public key token and no PublicKey flag.
var helperAssembly = new AssemblyRefUser(helper.Assembly.Name, helper.Assembly.Version,
    new PublicKeyToken(), helper.Assembly.Culture);
var helperType = new TypeRefUser(module, factory.Namespace, factory.Name, helperAssembly);
var helperCall = new MemberRefUser(module, create.Name, MethodSig.CreateStatic(petType.ToTypeSig(), infoType.ToTypeSig()), helperType);
changed.OpCode = OpCodes.Call;
changed.Operand = helperCall;
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Patched only UIHeroFormation.InitData pet construction at IL_" + changed.Offset.ToString("x4")
    + "; no server or collection mutation");
