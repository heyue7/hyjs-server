using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 2) throw new ArgumentException("current managed, output managed");
using var module = ModuleDefMD.Load(args[0]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string owner, string name) => T(owner).Methods.Single(m => m.Name == name);
var helperAssembly = module.GetAssemblyRefs().Single(a => a.Name == "SoulFlowerHeroArt");
var helper = new TypeRefUser(module, "HYJS.HeroArt", "HeroArtFallback", helperAssembly);
var ufHelper = new TypeRefUser(module, "HYJS.Shop", "UFQuotedCost", helperAssembly);
var shopHelper = new TypeRefUser(module, "HYJS.Shop", "QuotedShopCost", helperAssembly);

// Preserve the unaliased request before the inherited bundle/asset prefix.
// Every original return still executes; only the six original missing UI
// requests can receive a derived texture with the normal resource dimensions.
var getter = M("AssetLoad.AssetBundleManager", "GetAssetFromBundle");
var body = getter.Body;
var requested = new Local(module.CorLibTypes.String);
var returned = new Local(getter.MethodSig.RetType);
body.Variables.Add(requested);
body.Variables.Add(returned);
body.InitLocals = true;
// Several inherited loaders share the original local-signature token. Allocate
// a fresh signature for this method so their original body headers stay exact.
body.LocalVarSigTok = 0;
body.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_1));
body.Instructions.Insert(1, Instruction.Create(OpCodes.Stloc, requested));
var crop = new MemberRefUser(module, "MapRequestedObject", MethodSig.CreateStatic(getter.MethodSig.RetType, module.CorLibTypes.String, getter.MethodSig.RetType), helper);
foreach (var exit in body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
{
    int index = body.Instructions.IndexOf(exit);
    exit.OpCode = OpCodes.Stloc;
    exit.Operand = returned;
    body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ldloc, requested));
    body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ldloc, returned));
    body.Instructions.Insert(++index, Instruction.Create(OpCodes.Call, crop));
    body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ret));
}
body.MaxStack = Math.Max(body.MaxStack, (ushort)3);

// The original table reader handles every existing key unchanged. Only exact
// missing character/skill keys get an explicit source-status label.
var text = M("TableTextBlo", "GetText");
var missing = new MemberRefUser(module, "MissingOriginalText", MethodSig.CreateStatic(module.CorLibTypes.String, module.CorLibTypes.String), helper);
var empty = Instruction.Create(OpCodes.Pop);
Instruction[] prefix = {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, missing),
    Instruction.Create(OpCodes.Dup), Instruction.Create(OpCodes.Brfalse, empty),
    Instruction.Create(OpCodes.Ret), empty
};
foreach (var instruction in prefix.Reverse()) text.Body.Instructions.Insert(0, instruction);
text.Body.MaxStack = Math.Max(text.Body.MaxStack, (ushort)3);

// Clear recycled luck widgets before checking whether the next hero has a
// visible/nonempty luck module; retain all original creation and calculations.
var luck = M("UI.UIHeroLogic", "FillLucksData");
var instructions = luck.Body.Instructions;
if (instructions.Count < 9 || instructions[1].Operand is not IField first || first.Name != "module3" || instructions[7].OpCode != OpCodes.Ble)
    throw new InvalidOperationException("Original luck guard differs");
var guards = instructions.Take(8).ToArray();
foreach (var guard in guards) instructions.Remove(guard);
var resetHeight = instructions.Single(i => i.OpCode == OpCodes.Stfld && i.Operand is IField f && f.Name == "module3Height" && instructions.IndexOf(i) < 50);
int afterReset = instructions.IndexOf(resetHeight) + 1;
foreach (var guard in guards) instructions.Insert(afterReset++, guard);

// Both tab entry points now use the original named-coroutine channel. Passing
// a newly-created enumerator to StopCoroutine cannot stop the running list.
var equipment = M("UI.EquipTab", "UpdateTab");
var tabClick = M("UI.EquipTab", "OnTabClick");
foreach (string name in new[] { "StopCoroutine", "StartCoroutine" })
{
    var originalNamedCall = tabClick.Body.Instructions.Single(i => i.Operand is IMethod m && m.Name == name);
    var oldCall = equipment.Body.Instructions.Single(i => i.Operand is IMethod m && m.Name == name);
    int index = equipment.Body.Instructions.IndexOf(oldCall);
    if (equipment.Body.Instructions[index - 1].Operand is not IMethod list || list.Name != "UpdateList" || equipment.Body.Instructions[index - 2].OpCode != OpCodes.Ldarg_0)
        throw new InvalidOperationException("Original equipment coroutine channel differs");
    equipment.Body.Instructions[index - 2].OpCode = OpCodes.Ldstr;
    equipment.Body.Instructions[index - 2].Operand = "UpdateList";
    equipment.Body.Instructions[index - 1].OpCode = OpCodes.Nop;
    equipment.Body.Instructions[index - 1].Operand = null;
    oldCall.Operand = originalNamedCall.Operand;
}

// Set the actual badge to the original eligibility result in both directions;
// do not let prefab or previously-rendered state supply the false result.
var iterator = module.GetTypes().Single(t => t.Name == "<UpdateList>c__IteratorB3" && t.DeclaringType == T("UI.EquipTab"));
var listMove = iterator.Methods.Single(m => m.Name == "MoveNext");
var eligible = new Local(module.CorLibTypes.Boolean);
listMove.Body.Variables.Add(eligible);
listMove.Body.LocalVarSigTok = 0;
listMove.Body.InitLocals = true;
var checks = listMove.Body.Instructions.Where(i => i.Operand is IMethod m && m.Name == "CheckLuck").ToArray();
if (checks.Length != 2) throw new InvalidOperationException("Original equipment/treasure luck branches differ");
foreach (var check in checks)
{
    int index = listMove.Body.Instructions.IndexOf(check);
    var branch = listMove.Body.Instructions[index + 1];
    if (branch.OpCode != OpCodes.Brfalse && branch.OpCode != OpCodes.Brfalse_S)
        throw new InvalidOperationException("Original equipment luck guard differs");
    branch.OpCode = OpCodes.Stloc;
    branch.Operand = eligible;
    var active = listMove.Body.Instructions.Skip(index + 2).First(i => i.Operand is IMethod m && m.Name == "SetActive");
    var value = listMove.Body.Instructions[listMove.Body.Instructions.IndexOf(active) - 1];
    if (value.OpCode != OpCodes.Ldc_I4_1) throw new InvalidOperationException("Original badge state differs");
    value.OpCode = OpCodes.Ldloc;
    value.Operand = eligible;
}

// Half-month items retain the original visibility/claim state; only remove
// the client-only selected-day timeout gate, which conflicts with the new
// whole-period rule. The seven-day branch and real expiry stay untouched.
var award = M("UI.ActivityAwardItem", "InitActivityAward");
var dayStart = award.Body.Instructions.Single(i => i.Offset == 0x250);
var dayNormal = award.Body.Instructions.Single(i => i.Offset == 0x293);
if (dayStart.OpCode != OpCodes.Ldarg_0 || dayNormal.OpCode != OpCodes.Ldarg_0)
    throw new InvalidOperationException("Original half-month branch differs");
// Keep the skipped original block internally well-formed for old Mono's IL
// verifier; replacing its first ldarg would leave an orphaned ldfld block.
award.Body.Instructions.Insert(award.Body.Instructions.IndexOf(dayStart), Instruction.Create(OpCodes.Br, dayNormal));

// Type28 state0 otherwise displays a clickable "receive" label but its
// OnGoToClick switch returns without action. Reuse the original unavailable
// presentation used by login tasks; state1 still opens the real choice page.
var stateGetter = (IMethod)award.Body.Instructions.First(i => i.Operand is IMethod m && m.Name == "get_state").Operand;
var taskType = T("Dict.DataModel.TableActivityNewOpen7Day").Fields.Single(f => f.Name == "taskType");
var gotoButton = T("UI.ActivityAwardItem").Fields.Single(f => f.Name == "gotoBtn");
var unavailable = T("UI.ActivityAwardItem").Fields.Single(f => f.Name == "cantReceive");
var setActive = (IMethod)award.Body.Instructions.First(i => i.Operand is IMethod m && m.Name == "SetActive").Operand;
foreach (var exit in award.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
{
    int index = award.Body.Instructions.IndexOf(exit);
    var finish = Instruction.Create(OpCodes.Ret);
    exit.OpCode = OpCodes.Ldarg_1;
    Instruction[] unavailableState = {
        Instruction.Create(OpCodes.Callvirt, stateGetter), Instruction.Create(OpCodes.Brtrue, finish),
        Instruction.Create(OpCodes.Ldarg_2), Instruction.Create(OpCodes.Ldfld, taskType),
        Instruction.Create(OpCodes.Ldc_I4, 28), Instruction.Create(OpCodes.Bne_Un, finish),
        Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, gotoButton),
        Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Callvirt, setActive),
        Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, unavailable),
        Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Callvirt, setActive), finish
    };
    foreach (var instruction in unavailableState) award.Body.Instructions.Insert(++index, instruction);
}

// After an accepted purchase, ask the original current top tab for fresh red
// indicators. Original reward, state and stock UI updates execute first.
var halfPrice = M("HalfPrice", "OnReceiveCallBack");
var refresh = new MemberRefUser(module, "RefreshOpeningIndicators", MethodSig.CreateStatic(module.CorLibTypes.Void), helper);
foreach (var exit in halfPrice.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
{
    int index = halfPrice.Body.Instructions.IndexOf(exit);
    var finish = Instruction.Create(OpCodes.Ret);
    exit.OpCode = OpCodes.Ldarg_2;
    halfPrice.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Brfalse, finish));
    halfPrice.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ldloc_0));
    halfPrice.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Brfalse, finish));
    halfPrice.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Call, refresh));
    halfPrice.Body.Instructions.Insert(++index, finish);
}

// Original UF protobuf fields stay exact. Only quoted non-token SKUs get a
// cost-aware UI and confirmation; old SKU/token behavior returns to original IL.
var ufSet = M("UI.NPCUIShopItem", "SetItem");
var ufRender = new MemberRefUser(module, "Render", MethodSig.CreateStatic(module.CorLibTypes.Void, ufSet.DeclaringType.ToTypeSig(), ufSet.MethodSig.Params[0]), ufHelper);
foreach (var exit in ufSet.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
{
    int index = ufSet.Body.Instructions.IndexOf(exit);
    exit.OpCode = OpCodes.Ldarg_0;
    ufSet.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ldarg_1));
    ufSet.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Call, ufRender));
    ufSet.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ret));
}
var ufBuy = M("UI.NPCUIShopItem", "OnBuyClick");
var ufFields = ufBuy.DeclaringType.Fields;
var ufItem = ufFields.Single(f => f.Name == "item");
var ufPage = ufFields.Single(f => f.Name == "page");
var ufIdentity = ufFields.Single(f => f.Name == "idendityLimit");
var ufLeft = ufFields.Single(f => f.Name == "restTimes");
var ufTryBuy = new MemberRefUser(module, "TryBuy", MethodSig.CreateStatic(module.CorLibTypes.Boolean, ufBuy.DeclaringType.ToTypeSig(), ufItem.FieldSig.Type, ufPage.FieldSig.Type, module.CorLibTypes.Int32, module.CorLibTypes.Int32), ufHelper);
var ufOriginal = ufBuy.Body.Instructions[0];
Instruction[] ufPrefix = {
    Instruction.Create(OpCodes.Ldarg_0),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, ufItem),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, ufPage),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, ufIdentity),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, ufLeft),
    Instruction.Create(OpCodes.Call, ufTryBuy), Instruction.Create(OpCodes.Brfalse, ufOriginal),
    Instruction.Create(OpCodes.Ret)
};
foreach (var instruction in ufPrefix.Reverse()) ufBuy.Body.Instructions.Insert(0, instruction);
ufBuy.Body.MaxStack = Math.Max(ufBuy.Body.MaxStack, (ushort)6);

var isQuoted = new MemberRefUser(module, "IsQuoted", MethodSig.CreateStatic(module.CorLibTypes.Boolean, module.CorLibTypes.String), shopHelper);
var priceReader = M("Dict.Blo.TableItemShopPriceBlo", "GetShopPrice");
var quotedPrice = new MemberRefUser(module, "Price", MethodSig.CreateStatic(priceReader.MethodSig.RetType, module.CorLibTypes.String, module.CorLibTypes.Int32), shopHelper);
var priceOriginal = priceReader.Body.Instructions[0];
Instruction[] pricePrefix = {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, isQuoted), Instruction.Create(OpCodes.Brfalse, priceOriginal),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, quotedPrice), Instruction.Create(OpCodes.Ret)
};
foreach (var instruction in pricePrefix.Reverse()) priceReader.Body.Instructions.Insert(0, instruction);

var countReader = M("CommonMethod", "GetObjectHaveNum");
var supplement = new MemberRefUser(module, "SupplementBalance", MethodSig.CreateStatic(module.CorLibTypes.Int64, module.CorLibTypes.String), shopHelper);
var countPop = Instruction.Create(OpCodes.Pop);
Instruction[] countPrefix = {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, supplement), Instruction.Create(OpCodes.Dup),
    Instruction.Create(OpCodes.Ldc_I4_0), Instruction.Create(OpCodes.Conv_I8), Instruction.Create(OpCodes.Blt, countPop),
    Instruction.Create(OpCodes.Ret), countPop
};
foreach (var instruction in countPrefix.Reverse()) countReader.Body.Instructions.Insert(0, instruction);
countReader.Body.MaxStack = Math.Max(countReader.Body.MaxStack, (ushort)3);

void Tail(MethodDef method, Func<Instruction[]> make)
{
    foreach (var exit in method.Body.Instructions.Where(i => i.OpCode == OpCodes.Ret).ToArray())
    {
        int index = method.Body.Instructions.IndexOf(exit);
        var added = make();
        exit.OpCode = added[0].OpCode;
        exit.Operand = added[0].Operand;
        foreach (var instruction in added.Skip(1)) method.Body.Instructions.Insert(++index, instruction);
        method.Body.Instructions.Insert(++index, Instruction.Create(OpCodes.Ret));
    }
}
var shopList = M("UI.ShopProductItem", "SetShopItem");
var listRender = new MemberRefUser(module, "List", MethodSig.CreateStatic(module.CorLibTypes.Void, shopList.DeclaringType.ToTypeSig(), shopList.MethodSig.Params[0]), shopHelper);
Tail(shopList, () => new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, listRender) });
var activityShopList = M("UI.ShopProductItem", "SetActivityItem");
var activityRender = new MemberRefUser(module, "Activity", MethodSig.CreateStatic(module.CorLibTypes.Void, activityShopList.DeclaringType.ToTypeSig(), activityShopList.MethodSig.Params[0]), shopHelper);
Tail(activityShopList, () => new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, activityRender) });
var batch = M("UI.ShopBatchBuy", "UpdateAll");
var batchProduct = batch.DeclaringType.Fields.Single(f => f.Name == "productInfo");
var batchRender = new MemberRefUser(module, "Batch", MethodSig.CreateStatic(module.CorLibTypes.Void, batch.DeclaringType.ToTypeSig(), batchProduct.FieldSig.Type), shopHelper);
Tail(batch, () => new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, batchProduct), Instruction.Create(OpCodes.Call, batchRender) });
var priceIcon = M("UI.BuyShopItem", "UpdatePriceIcon");
var iconRender = new MemberRefUser(module, "CostIcon", MethodSig.CreateStatic(module.CorLibTypes.Void, priceIcon.MethodSig.Params[0], module.CorLibTypes.String, module.CorLibTypes.Boolean), shopHelper);
Tail(priceIcon, () => new[] { Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Ldarg_2), Instruction.Create(OpCodes.Ldc_I4_1), Instruction.Create(OpCodes.Call, iconRender) });
priceIcon.Body.MaxStack = Math.Max(priceIcon.Body.MaxStack, (ushort)3);
var buyPage = M("UI.BuyItemPage", "SetItem");
var buyRender = new MemberRefUser(module, "BuyPage", MethodSig.CreateStatic(module.CorLibTypes.Void, buyPage.DeclaringType.ToTypeSig()), shopHelper);
Tail(buyPage, () => new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Call, buyRender) });
var afford = M("UI.BuyItemPage", "IsHaveEnoughBuyCost");
var currentProduct = afford.DeclaringType.Fields.Single(f => f.Name == "curProduct");
var productID = T("UI.UIShopProduct").Fields.Single(f => f.Name == "productId");
var affordCall = new MemberRefUser(module, "CanAfford", MethodSig.CreateStatic(module.CorLibTypes.Boolean, currentProduct.FieldSig.Type), shopHelper);
var affordOriginal = afford.Body.Instructions[0];
Instruction[] affordPrefix = {
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, currentProduct), Instruction.Create(OpCodes.Ldfld, productID),
    Instruction.Create(OpCodes.Call, isQuoted), Instruction.Create(OpCodes.Brfalse, affordOriginal),
    Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, currentProduct), Instruction.Create(OpCodes.Call, affordCall), Instruction.Create(OpCodes.Ret)
};
foreach (var instruction in affordPrefix.Reverse()) afford.Body.Instructions.Insert(0, instruction);

// A quoted SKU changes when its price changes. The native merge keeps the old
// full-ID dictionary entry; remove only superseded versions of newly-listed
// stable products after that whole merge, before any list/quick-buy consumer.
var mergeShop = M("UI.UIPlayerData", "UpdateItemShopProducts");
var shopCache = mergeShop.DeclaringType.Fields.Single(f => f.Name == "itemShopProductList");
var pruneQuotes = new MemberRefUser(module, "RemoveSuperseded", MethodSig.CreateStatic(module.CorLibTypes.Void, shopCache.FieldSig.Type, mergeShop.MethodSig.Params[0]), shopHelper);
Tail(mergeShop, () => new[] { Instruction.Create(OpCodes.Ldarg_0), Instruction.Create(OpCodes.Ldfld, shopCache), Instruction.Create(OpCodes.Ldarg_1), Instruction.Create(OpCodes.Call, pruneQuotes) });

foreach (var method in new[] { getter, text, luck, equipment, listMove, award, halfPrice, ufSet, ufBuy, priceReader, countReader, shopList, activityShopList, batch, priceIcon, buyPage, afford, mergeShop })
{
    method.Body.KeepOldMaxStack = true;
    method.Body.SimplifyBranches();
    method.Body.OptimizeBranches();
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Bounded portrait, source text, equipment state and opening activity UI targets updated");
