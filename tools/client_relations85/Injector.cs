using System;
using System.Collections.Generic;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("input and output managed paths required");
using var module = ModuleDefMD.Load(args[0]);
void Need(bool ok, string message) { if (!ok) throw new InvalidOperationException(message); }
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(TypeDef type, string name) => type.Methods.Single(m => m.Name == name);
FieldDef F(TypeDef type, string name) => type.Fields.Single(f => f.Name == name);
bool IsField(Instruction ins, string owner, string name) => ins.Operand is IField f && f.DeclaringType.FullName == owner && f.Name == name;
bool IsCall(Instruction ins, string owner, string name) => ins.Operand is IMethod m && m.DeclaringType.FullName == owner && m.Name == name;
IMethod Ref(string owner, string name) => module.GetMemberRefs().Single(m => m.DeclaringType.FullName == owner && m.Name == name);
void Insert(IList<Instruction> code, int index, IEnumerable<Instruction> addition) { foreach (var i in addition) code.Insert(index++, i); }

var luck = T("UI.UILuck");
var events = T("UI.UILuckEvent");
var formation = T("UI.UIHeroFormation");
var hero = T("UI.UIHero");
var staticData = T("StaticData");
var player = T("UI.UIPlayerData");
var inst = T("UI.UIInstPlayerData");
var icon = T("UI.HeroIcon");
var closure = module.GetTypes().Single(t => t.DeclaringType == luck && t.Name == "<SetInBattleHeroIcon>c__AnonStorey1A3");
var init = M(luck, "Init");
var draw = M(luck, "SetInBattleHeroIcon");
var update = M(luck, "UpdateCirclePanel");
var select = M(closure, "<>m__20B");
var help = M(events, "OpenHelpPage");
var heroField = F(formation, "hero");
var guid = F(hero, "guid");
var heroId = F(hero, "heroId");
var current = F(luck, "currIndex");
var playerData = F(staticData, "playerData");
var getInst = M(player, "get_InstPlayerData");
var getCamps = M(inst, "get_BattleCamps");
var isEmpty = Ref("System.String", "IsNullOrEmpty");
var setActive = Ref("UnityEngine.GameObject", "SetActive");
var getObject = Ref("UnityEngine.Component", "get_gameObject");
var stop = Ref("UnityEngine.MonoBehaviour", "StopAllCoroutines");
var setText = M(T("UILabel"), "set_text");
var getUIHero = M(player, "GetUIHero");
var center = M(luck, "SetCenterHero");
var uninit = M(luck, "Uninit");
var equal = select.Body.Instructions.Single(i => IsCall(i, "System.String", "op_Equality")).Operand as IMethod;
Need(equal != null, "original GUID equality reference absent");

// Init paints each fixed slot. Capture the owned instance GUID, preserving
// the original icon art lookup below, and skip malformed empty GUIDs.
var ic = init.Body.Instructions;
var drawCall = ic.Single(i => IsCall(i, luck.FullName, draw.Name));
int drawAt = ic.IndexOf(drawCall);
var paintedId = ic.Skip(drawAt - 10).Take(10).Single(i => IsField(i, hero.FullName, "heroId"));
paintedId.Operand = guid;
int paintAt = ic.IndexOf(paintedId);
var paintGuard = ic.Take(paintAt).Last(i => i.OpCode.Code == Code.Brfalse);
Need(paintGuard.Operand is Instruction && init.Body.Variables.Count > 5 && init.Body.Variables[5].Type.FullName == hero.FullName,
    "slot paint guard differs");
int guardAt = ic.IndexOf(paintGuard) + 1;
Insert(ic, guardAt, new[] {
    Instruction.Create(OpCodes.Ldloc, init.Body.Variables[5]), Instruction.Create(OpCodes.Ldfld, guid),
    Instruction.Create(OpCodes.Call, isEmpty), Instruction.Create(OpCodes.Brtrue, (Instruction)paintGuard.Operand)
});

// Before selecting the center, retain a still-valid selected physical slot;
// otherwise select the first occupied slot without moving any formation data.
int selectedRead = ic.Select((x, n) => (x, n)).Single(x => IsField(x.x, luck.FullName, "currIndex") &&
    x.n + 1 < ic.Count && ic[x.n + 1].Operand is IMethod m && m.Name == "get_Item").n;
Need(selectedRead >= 4 && IsField(ic[selectedRead - 4], staticData.FullName, "playerData") &&
    IsCall(ic[selectedRead - 3], player.FullName, "get_InstPlayerData") &&
    IsCall(ic[selectedRead - 2], inst.FullName, "get_BattleCamps"), "center slot read differs");
var originalSelection = ic[selectedRead - 4];
var dictionaryGet = (IMethod)ic[selectedRead + 1].Operand;
var fallback = Instruction.Create(OpCodes.Nop);
var selected = Instruction.Create(OpCodes.Nop);
var resolver = new List<Instruction>();
void Add(params Instruction[] values) => resolver.AddRange(values);
void LoadCamp(Instruction slot) => Add(Instruction.Create(OpCodes.Ldsfld, playerData),
    Instruction.Create(OpCodes.Callvirt, getInst), Instruction.Create(OpCodes.Callvirt, getCamps), slot,
    Instruction.Create(OpCodes.Callvirt, dictionaryGet), Instruction.Create(OpCodes.Ldfld, heroField),
    Instruction.Create(OpCodes.Stloc, init.Body.Variables[5]));
Add(Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, current), Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Blt, fallback),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, current), Instruction.Create(OpCodes.Ldc_I4_5), Instruction.Create(OpCodes.Bgt, fallback),
    Instruction.Create(OpCodes.Ldsfld, playerData), Instruction.Create(OpCodes.Callvirt, getInst), Instruction.Create(OpCodes.Callvirt, getCamps),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, current), Instruction.Create(OpCodes.Callvirt, dictionaryGet),
    Instruction.Create(OpCodes.Ldfld, heroField), Instruction.Create(OpCodes.Stloc, init.Body.Variables[5]),
    Instruction.Create(OpCodes.Ldloc, init.Body.Variables[5]), Instruction.Create(OpCodes.Brfalse, fallback),
    Instruction.Create(OpCodes.Ldloc, init.Body.Variables[5]), Instruction.Create(OpCodes.Ldfld, guid),
    Instruction.Create(OpCodes.Call, isEmpty), Instruction.Create(OpCodes.Brfalse, selected), fallback);
for (int slot = 1; slot <= 5; slot++)
{
    var next = Instruction.Create(OpCodes.Nop);
    LoadCamp(Instruction.CreateLdcI4(slot));
    Add(Instruction.Create(OpCodes.Ldloc, init.Body.Variables[5]), Instruction.Create(OpCodes.Brfalse, next),
        Instruction.Create(OpCodes.Ldloc, init.Body.Variables[5]), Instruction.Create(OpCodes.Ldfld, guid),
        Instruction.Create(OpCodes.Call, isEmpty), Instruction.Create(OpCodes.Brtrue, next),
        Instruction.Create(OpCodes.Ldarg_0), Instruction.CreateLdcI4(slot), Instruction.Create(OpCodes.Stfld, current),
        Instruction.Create(OpCodes.Br, selected), next);
}

// With no valid battle hero, remove previous icons and detail, prevent help
// additions, and leave the existing empty battle-slot buttons available.
Add(Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, stop),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, uninit),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Stfld, current),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldnull), Instruction.Create(OpCodes.Stfld, F(luck, "guid")),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(luck, "panel")),
    Instruction.Create(OpCodes.Callvirt, getObject), Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Callvirt, setActive));
foreach (string name in new[] { "luckHeroName", "luckCount", "luckTotalCount" })
    Add(Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(luck, name)),
        Instruction.Create(OpCodes.Ldstr, ""), Instruction.Create(OpCodes.Callvirt, setText));
for (int n = 0; n < 6; n++)
    Add(Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(luck, "helpNamePos")), Instruction.CreateLdcI4(n),
        Instruction.Create(OpCodes.Ldelem_Ref), Instruction.Create(OpCodes.Ldstr, ""), Instruction.Create(OpCodes.Callvirt, setText),
        Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(luck, "sixAdds")), Instruction.CreateLdcI4(n),
        Instruction.Create(OpCodes.Ldelem_Ref), Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Callvirt, setActive));
for (int n = 0; n < 5; n++)
    Add(Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(luck, "mask" + n)),
        Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Callvirt, setActive));
Add(Instruction.Create(OpCodes.Ret), selected,
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(luck, "panel")),
    Instruction.Create(OpCodes.Callvirt, getObject), Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Callvirt, setActive));

// Both original conditional paths to the old selection must run the resolver.
int redirected = 0;
foreach (var i in ic)
    if (ReferenceEquals(i.Operand, originalSelection)) { i.Operand = resolver[0]; redirected++; }
Need(redirected == 2, "selection branch predecessors differ");
Insert(ic, ic.IndexOf(originalSelection), resolver);
init.Body.MaxStack = Math.Max(init.Body.MaxStack, (ushort)8);
init.Body.KeepOldMaxStack = true;

// The capture string now holds a GUID. Recover the same hero ID solely for
// the original SetHeroWithId presentation call.
var dc = draw.Body.Instructions;
var iconCall = dc.Single(i => IsCall(i, icon.FullName, "SetHeroWithId"));
int di = dc.IndexOf(iconCall);
Need(di >= 3 && IsField(dc[di - 1], closure.FullName, "heroId") && dc[di - 2].OpCode.Code == Code.Ldloc_2,
    "icon presentation operand differs");
Insert(dc, di - 2, new[] { Instruction.Create(OpCodes.Ldsfld, playerData) });
di = dc.IndexOf(iconCall);
Insert(dc, di, new[] { Instruction.Create(OpCodes.Callvirt, getUIHero), Instruction.Create(OpCodes.Ldfld, heroId) });
draw.Body.MaxStack = Math.Max(draw.Body.MaxStack, (ushort)4);
draw.Body.KeepOldMaxStack = true;

// Resolve a captured GUID against the five actual slots at click time.
// The same slot supplies currIndex, hero GUID and center details.
var body = new CilBody { InitLocals = true, MaxStack = 4, KeepOldMaxStack = true };
var camp = new Local(formation.ToTypeSig());
body.Variables.Add(camp);
var code = body.Instructions;
for (int slot = 1; slot <= 5; slot++)
{
    var next = Instruction.Create(OpCodes.Nop);
    foreach (var i in new[] {
        Instruction.Create(OpCodes.Ldsfld, playerData), Instruction.Create(OpCodes.Callvirt, getInst), Instruction.Create(OpCodes.Callvirt, getCamps),
        Instruction.CreateLdcI4(slot), Instruction.Create(OpCodes.Callvirt, dictionaryGet), Instruction.Create(OpCodes.Stloc, camp),
        Instruction.Create(OpCodes.Ldloc, camp), Instruction.Create(OpCodes.Brfalse, next),
        Instruction.Create(OpCodes.Ldloc, camp), Instruction.Create(OpCodes.Ldfld, heroField), Instruction.Create(OpCodes.Brfalse, next),
        Instruction.Create(OpCodes.Ldloc, camp), Instruction.Create(OpCodes.Ldfld, heroField), Instruction.Create(OpCodes.Ldfld, guid),
        Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(closure, "heroId")), Instruction.Create(OpCodes.Call, equal),
        Instruction.Create(OpCodes.Brfalse, next), Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(closure, "<>f__this")),
        Instruction.CreateLdcI4(slot), Instruction.Create(OpCodes.Stfld, current),
        Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(closure, "<>f__this")),
        Instruction.Create(OpCodes.Ldloc, camp), Instruction.Create(OpCodes.Call, center), Instruction.Create(OpCodes.Ret), next
    }) code.Add(i);
}
// A stale icon can outlive a replaced/removed hero; refresh the page against
// the current camps instead of keeping the departed target.
foreach (var i in new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, F(closure, "<>f__this")),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Call, init), Instruction.Create(OpCodes.Ret) }) code.Add(i);
select.Body = body;

// Skip empty formation values/hero entries before the original GUID read.
var uc = update.Body.Instructions;
var loop = (Instruction)uc.First(i => i.OpCode.Code == Code.Br).Operand;
var oldHeroRead = uc.Select((x, n) => (x, n)).First(x => IsField(x.x, formation.FullName, "hero")).n;
Need(oldHeroRead >= 2 && uc[oldHeroRead - 2].OpCode.Code == Code.Ldloca_S && uc[oldHeroRead - 1].Operand is IMethod,
    "circle enumeration differs");
var getValue = (IMethod)uc[oldHeroRead - 1].Operand;
var pair = (Local)uc[oldHeroRead - 2].Operand;
Insert(uc, oldHeroRead - 2, new[] {
    Instruction.Create(OpCodes.Ldloca, pair), Instruction.Create(OpCodes.Call, getValue), Instruction.Create(OpCodes.Brfalse, loop),
    Instruction.Create(OpCodes.Ldloca, pair), Instruction.Create(OpCodes.Call, getValue), Instruction.Create(OpCodes.Ldfld, heroField),
    Instruction.Create(OpCodes.Brfalse, loop)
});
update.Body.MaxStack = Math.Max(update.Body.MaxStack, (ushort)3);
update.Body.KeepOldMaxStack = true;

// The original help page is still responsible for position unlocks and the
// original CSHelperOn flow. Only a completely empty formation is gated.
var hc = help.Body.Instructions;
var helpEntry = hc[0];
var hcamp = new Local(formation.ToTypeSig());
help.Body.Variables.Add(hcamp);
help.Body.InitLocals = true;
var guard = new List<Instruction>();
for (int slot = 1; slot <= 5; slot++)
{
    var next = Instruction.Create(OpCodes.Nop);
    guard.AddRange(new[] { Instruction.Create(OpCodes.Ldsfld, playerData), Instruction.Create(OpCodes.Callvirt, getInst),
        Instruction.Create(OpCodes.Callvirt, getCamps), Instruction.CreateLdcI4(slot), Instruction.Create(OpCodes.Callvirt, dictionaryGet),
        Instruction.Create(OpCodes.Stloc, hcamp), Instruction.Create(OpCodes.Ldloc, hcamp), Instruction.Create(OpCodes.Brfalse, next),
        Instruction.Create(OpCodes.Ldloc, hcamp), Instruction.Create(OpCodes.Ldfld, heroField), Instruction.Create(OpCodes.Brfalse, next),
        Instruction.Create(OpCodes.Ldloc, hcamp), Instruction.Create(OpCodes.Ldfld, heroField), Instruction.Create(OpCodes.Ldfld, guid),
        Instruction.Create(OpCodes.Call, isEmpty), Instruction.Create(OpCodes.Brfalse, helpEntry), next });
}
guard.Add(Instruction.Create(OpCodes.Ret));
Insert(hc, 0, guard);
help.Body.MaxStack = Math.Max(help.Body.MaxStack, (ushort)4);
help.Body.KeepOldMaxStack = true;

var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Patched five relation-page methods; original help eligibility and network request retained");
