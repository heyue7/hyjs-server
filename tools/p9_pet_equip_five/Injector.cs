using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input/output DLL required");
using var module = ModuleDefMD.Load(args[0]);
TypeDef T(string n) => module.GetTypes().Single(t => t.FullName == n);
FieldDef F(string t, string n) => T(t).Fields.Single(f => f.Name == n);
MethodDef M(string t, string n) => T(t).Methods.Single(m => m.Name == n);
var method = M("UI.PetEquipStrengthTab", "GetFiveCost");
var body = method.Body;
if (body.Variables.Count != 4 || body.ExceptionHandlers.Count != 0 || method.IsStatic)
    throw new InvalidOperationException("Unexpected original GetFiveCost locals/EH/ABI");
var code = body.Instructions;
var bound = code.Single(i => i.Offset == 0x66 && i.OpCode == OpCodes.Ldc_I4_5);
if (code[code.IndexOf(bound)+1].OpCode != OpCodes.Ble)
    throw new InvalidOperationException("Expected original five-step loop condition");
var cap = new Local(module.CorLibTypes.Int32);
var advance = new Local(T("Dict.DataModel.TablePetEquipAdvance").ToTypeSig());
body.Variables.Add(cap); body.Variables.Add(advance);
var pre = new List<Instruction>();
void I(OpCode op) => pre.Add(Instruction.Create(op));
void Field(string t,string n) => pre.Add(Instruction.Create(OpCodes.Ldfld,F(t,n)));
void Equip(string n) { I(OpCodes.Ldarg_0); Field("UI.PetEquipStrengthTab","petEquip"); Field("UI.UIPetEquip",n); }
void L(Local l) => pre.Add(Instruction.Create(OpCodes.Ldloc,l));
void S(Local l) => pre.Add(Instruction.Create(OpCodes.Stloc,l));
void B(OpCode op,Instruction target) => pre.Add(Instruction.Create(op,target));
var rawCapDone=Instruction.Create(OpCodes.Nop);
var levelNonzero=Instruction.Create(OpCodes.Nop);
var levelReady=Instruction.Create(OpCodes.Nop);
var advancePresent=Instruction.Create(OpCodes.Nop);
var useCurrentMax=Instruction.Create(OpCodes.Nop);
var limitReady=Instruction.Create(OpCodes.Nop);
var advanceCapDone=Instruction.Create(OpCodes.Nop);
var nonnegative=Instruction.Create(OpCodes.Nop);
var capped=Instruction.Create(OpCodes.Nop);
// cap=min(pet.level, raw max). Leave original isCanStrength gates untouched.
I(OpCodes.Ldarg_0); Field("UI.PetEquipStrengthTab","page");
pre.Add(Instruction.Create(OpCodes.Callvirt,M("UI.PetEquipImprove","GetCurrPet")));
Field("UI.UIPet","level"); S(cap);
L(cap); Equip("maxStrLevel"); B(OpCodes.Ble,rawCapDone);
Equip("maxStrLevel"); S(cap); pre.Add(rawCapDone);
// The service uses first.needStrengthLevel before advancement, current.max thereafter.
Equip("equipId"); Equip("advanLevel"); I(OpCodes.Dup); B(OpCodes.Brtrue,levelNonzero);
I(OpCodes.Pop); I(OpCodes.Ldc_I4_1); B(OpCodes.Br,levelReady);
pre.Add(levelNonzero); pre.Add(levelReady);
pre.Add(Instruction.Create(OpCodes.Call,M("Dict.Blo.DictPetEquipBlo","GetPetEquipAdvance")));
S(advance); L(advance); B(OpCodes.Brtrue,advancePresent);
pre.Add(Instruction.Create(OpCodes.Ldc_I4,int.MaxValue)); I(OpCodes.Ret);
pre.Add(advancePresent);
Equip("advanLevel"); B(OpCodes.Brtrue,useCurrentMax);
L(advance); Field("Dict.DataModel.TablePetEquipAdvance","needStrengthLevel"); B(OpCodes.Br,limitReady);
pre.Add(useCurrentMax); L(advance); Field("Dict.DataModel.TablePetEquipAdvance","maxStrengthLevel");
pre.Add(limitReady);
// Use the evaluation stack to keep limit across the comparison.
I(OpCodes.Dup); L(cap); B(OpCodes.Bge,advanceCapDone);
S(cap); B(OpCodes.Br,nonnegative);
pre.Add(advanceCapDone); I(OpCodes.Pop);
pre.Add(nonnegative);
L(cap); Equip("strLevel"); I(OpCodes.Sub); S(cap);
var positive=Instruction.Create(OpCodes.Nop);
L(cap); I(OpCodes.Ldc_I4_0); B(OpCodes.Bge,positive); I(OpCodes.Ldc_I4_0); S(cap);
pre.Add(positive); L(cap); I(OpCodes.Ldc_I4_5); B(OpCodes.Ble,capped);
I(OpCodes.Ldc_I4_5); S(cap); pre.Add(capped);
for (int i=0;i<pre.Count;i++) code.Insert(i,pre[i]);
bound.OpCode=OpCodes.Ldloc; bound.Operand=cap;
body.MaxStack=20; body.KeepOldMaxStack=true;
body.UpdateInstructionOffsets();
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("Changed only UI.PetEquipStrengthTab.GetFiveCost actual-step loop bound");
