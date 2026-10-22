using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input DLL, helper DLL, output DLL required");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);

var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.ArenaState.ArenaStateSync");
MethodDef HelperMethod(string name)
{
    var method = helperType.Methods.Single(m => m.Name == name);
    if (!method.IsStatic || method.MethodSig == null || method.MethodSig.RetType.FullName != "System.Void")
        throw new Exception("Unexpected helper method contract: " + name);
    return method;
}
var syncGrabCache = HelperMethod("SyncGrabCache");
var clearTargets = HelperMethod("ClearPlunderTargets");
var showEmptyRivals = HelperMethod("ShowEmptyRivals");

var grabItem = module.GetTypes().Single(t => t.FullName == "UI.GrabPVPItem");
var grabPanel = module.GetTypes().Single(t => t.FullName == "UI.GrabPVPPanel");
var plunderPage = module.GetTypes().Single(t => t.FullName == "UI.PlunderSelectPage");
var grabModel = module.GetTypes().Single(t => t.FullName == "ProtoBuf.Message.GrabPositionModel");
var rivalList = module.GetTypes().Single(t => t.FullName == "ProtoBuf.Message.SCTreasureBattleList");

var setItem = grabItem.Methods.Single(m => m.Name == "SetItem" && m.MethodSig != null
    && m.MethodSig.HasThis && m.MethodSig.Params.Count == 1
    && m.MethodSig.Params[0].FullName == "ProtoBuf.Message.GrabPositionModel");
var doOpen = plunderPage.Methods.Single(m => m.Name == "DoOpen" && m.MethodSig != null
    && m.MethodSig.HasThis && m.MethodSig.Params.Count == 0);
var rivalCallback = plunderPage.Methods.Single(m => m.Name == "GetRivalCallback" && m.MethodSig != null
    && m.MethodSig.HasThis && m.MethodSig.Params.Count == 2
    && m.MethodSig.Params[0].FullName == "NetWork.Packets.Packet"
    && m.MethodSig.Params[1].FullName == "System.Boolean");
foreach (var target in new[] { setItem, doOpen, rivalCallback })
{
    if (target.Body == null || target.Body.Instructions.Count == 0)
        throw new Exception("Target body missing: " + target.Name);
    if (target.Body.ExceptionHandlers.Count != 0)
        throw new Exception("Target body gained exception handlers: " + target.Name);
}

// Unsigned helper: empty public key token and no PublicKey flag.
var helperAssembly = new AssemblyRefUser(helper.Assembly.Name, helper.Assembly.Version,
    new PublicKeyToken(), helper.Assembly.Culture);
var helperTypeRef = new TypeRefUser(module, helperType.Namespace, helperType.Name, helperAssembly);
MemberRefUser Bind(MethodDef definition, params TypeDef[] parameters)
{
    var sig = MethodSig.CreateStatic(module.CorLibTypes.Void,
        parameters.Select(parameter => parameter.ToTypeSig()).ToArray());
    if (definition.MethodSig.Params.Count != parameters.Length)
        throw new Exception("Helper parameter count differs: " + definition.Name);
    for (int index = 0; index < parameters.Length; index++)
    {
        if (definition.MethodSig.Params[index].FullName != parameters[index].FullName)
            throw new Exception("Helper parameter type differs: " + definition.Name);
    }
    return new MemberRefUser(module, definition.Name, sig, helperTypeRef);
}
var syncRef = Bind(syncGrabCache, grabPanel, grabModel);
var clearRef = Bind(clearTargets, plunderPage);
var emptyRef = Bind(showEmptyRivals, plunderPage, rivalList);

// Anchor 1: SetItem ends with listener binding and a single ret; render + cache sync.
var setBody = setItem.Body;
var setInstructions = setBody.Instructions;
if (setInstructions.Last().OpCode != OpCodes.Ret)
    throw new Exception("SetItem does not end with ret");
int retIndex = setInstructions.Count - 1;
if (retIndex < 3 || setInstructions[retIndex - 1].OpCode != OpCodes.Stfld
    || !(setInstructions[retIndex - 1].Operand is IField stored) || stored.Name != "onClick"
    || setInstructions[retIndex - 2].OpCode != OpCodes.Newobj
    || setInstructions[retIndex - 3].OpCode != OpCodes.Ldftn)
    throw new Exception("SetItem listener anchor differs");
setInstructions.Insert(retIndex, Instruction.Create(OpCodes.Call, syncRef));
setInstructions.Insert(retIndex, Instruction.Create(OpCodes.Ldarg_1));
setInstructions.Insert(retIndex, Instruction.Create(OpCodes.Ldfld, grabItem.Fields.Single(f => f.Name == "grabPanel")));
setInstructions.Insert(retIndex, Instruction.Create(OpCodes.Ldarg_0));
setBody.MaxStack = (ushort)Math.Max((int)setBody.MaxStack, 2);

// Anchor 2: DoOpen entry; hide prefab slots before the original delay/request.
var openBody = doOpen.Body;
if (openBody.Instructions[0].OpCode != OpCodes.Ldarg_0
    || openBody.Instructions[1].OpCode != OpCodes.Ldfld
    || !(openBody.Instructions[1].Operand is IField options) || options.Name != "options")
    throw new Exception("DoOpen entry anchor differs");
openBody.Instructions.Insert(0, Instruction.Create(OpCodes.Call, clearRef));
openBody.Instructions.Insert(0, Instruction.Create(OpCodes.Ldarg_0));
openBody.MaxStack = (ushort)Math.Max((int)openBody.MaxStack, 1);

// Anchor 3: GetRivalCallback success fall-through only; failure branches to ret.
var callbackBody = rivalCallback.Body;
var callbackInstructions = callbackBody.Instructions;
if (callbackInstructions.Last().OpCode != OpCodes.Ret)
    throw new Exception("GetRivalCallback does not end with ret");
if (callbackBody.Variables == null || callbackBody.Variables.Count < 2
    || callbackBody.Variables[1].Type.FullName != "ProtoBuf.Message.SCTreasureBattleList")
    throw new Exception("GetRivalCallback local 1 is not SCTreasureBattleList");
int callbackRet = callbackInstructions.Count - 1;
if (callbackInstructions[callbackRet - 1].OpCode != OpCodes.Blt)
    throw new Exception("GetRivalCallback success fall-through anchor differs");
callbackInstructions.Insert(callbackRet, Instruction.Create(OpCodes.Call, emptyRef));
callbackInstructions.Insert(callbackRet, Instruction.Create(OpCodes.Ldloc_1));
callbackInstructions.Insert(callbackRet, Instruction.Create(OpCodes.Ldarg_0));
callbackBody.MaxStack = (ushort)Math.Max((int)callbackBody.MaxStack, 2);

var writerOptions = new ModuleWriterOptions(module);
writerOptions.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], writerOptions);
Console.WriteLine("Injected 3 arena-state hooks: GrabPVPItem.SetItem cache sync, PlunderSelectPage.DoOpen prefab hide, GetRivalCallback empty-rival notice; no server or resource change");
