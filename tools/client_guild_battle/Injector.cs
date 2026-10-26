using System;
using System.Linq;
using System.Collections.Generic;
using System.Text.Json;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 4) throw new ArgumentException("input, helper, output, hook manifest");
using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
TypeDef T(string name) => module.GetTypes().Single(t => t.FullName == name);
MethodDef M(string type, string name, int count) => T(type).Methods.Single(m => m.Name == name && m.MethodSig.Params.Count == count);
var importer = new Importer(module, ImporterOptions.TryToUseDefs | ImporterOptions.TryToUseExistingAssemblyRefs);
var helperType = helper.GetTypes().Single(t => t.FullName == "HYJS.GuildBattle.GuildBattleScene");
var owner = importer.Import(helperType);
var vector = M("Room.RCMoveNavigation", "MoveTo", 1).MethodSig.Params[0];
var coroutine = M("Room.RoomUnionFight", "_WaitForLoadComplete", 0).MethodSig.RetType;
IMethod H(string name) {
 MethodSig sig = name switch {
  "CanLoad" or "get_Active" => MethodSig.CreateStatic(module.CorLibTypes.Boolean),
  "LoadRoom" => MethodSig.CreateStatic(coroutine, T("Room.RoomUnionFight").ToTypeSig()),
  "RegisterParticle" => MethodSig.CreateStatic(coroutine, T("Room.RoomUnionFight").ToTypeSig(), module.CorLibTypes.String),
  "TickMove" or "Reset" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCMoveNavigation").ToTypeSig()),
  "MoveTo" or "Reposition" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCMoveNavigation").ToTypeSig(),vector),
  "ApproachDistance" => MethodSig.CreateStatic(module.CorLibTypes.Single,module.CorLibTypes.Single),
  "Speed" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCMoveNavigation").ToTypeSig(),module.CorLibTypes.Single),
  "Sample" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,vector,new ByRefSig(vector)),
  "Siege" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCLODBase").ToTypeSig(),module.CorLibTypes.String),
  "TryEnemyGuardClick" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCUIUnionGuard").ToTypeSig()),
  "TryPrepareEntry" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("UI.UnionFightSignUpItem").ToTypeSig()),
  "ObserveTown" => MethodSig.CreateStatic(module.CorLibTypes.Void,T("UI.TownPage").ToTypeSig()),
  "BlockTownEntry" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("UI.TownPage").ToTypeSig()),
  "SendScannedPlayerAsk" => MethodSig.CreateStatic(module.CorLibTypes.Void,T("Room.RoomNetUnionFight").ToTypeSig(),module.CorLibTypes.Int64,T("Room.RoomUnionFightCheckPvp").ToTypeSig()),
  "ObservePlayerClick" => MethodSig.CreateStatic(module.CorLibTypes.Void,T("Room.RCUIUnionActor").ToTypeSig()),
  "ObservePlayerBegin" or "ObservePlayerEnd" => MethodSig.CreateStatic(module.CorLibTypes.Void,T("Room.RoomNetUnionFight").ToTypeSig(),module.CorLibTypes.Object),
  "MiniMap" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("UI.MiniMapComponent").ToTypeSig()),
  _ => throw new InvalidOperationException(name)
 };
 if (!helperType.Methods.Any(m=>m.Name==name)) throw new InvalidOperationException("Missing helper API " + name);
 return new MemberRefUser(module,name,sig,owner);
}
var records = new List<object>();
void Prefix(MethodDef method, List<Instruction> prefix) {
 if (method.Body == null || method.Body.Instructions.Any(i=> i.Operand is IMethod m && m.DeclaringType.FullName==helperType.FullName)) throw new InvalidOperationException("Invalid/repeated hook " + method.FullName);
 for (int i=0;i<prefix.Count;i++) method.Body.Instructions.Insert(i,prefix[i]);
 method.Body.MaxStack = Math.Max(method.Body.MaxStack,(ushort)8); method.Body.KeepOldMaxStack=true;
 records.Add(new { method = method.DeclaringType.FullName + "::" + method.Name, rid=method.Rid, prefixCount=prefix.Count });
}
void ReturnCoroutine(MethodDef method, string gate, string api, bool text) {
 var p = new List<Instruction>{Instruction.Create(OpCodes.Call,H(gate)),Instruction.Create(OpCodes.Brfalse,method.Body.Instructions[0]),Instruction.Create(OpCodes.Ldarg_0)};
 if(text)p.Add(Instruction.Create(OpCodes.Ldarg_1));
 p.Add(Instruction.Create(OpCodes.Call,H(api)));p.Add(Instruction.Create(OpCodes.Ret));Prefix(method,p);
}
void TryVoid(MethodDef method,string api,int arguments) {
 var p=new List<Instruction>();
 for(int i=0;i<arguments;i++) p.Add(i==0?Instruction.Create(OpCodes.Ldarg_0):Instruction.Create(OpCodes.Ldarg_1));
 p.Add(Instruction.Create(OpCodes.Call,H(api)));p.Add(Instruction.Create(OpCodes.Brfalse,method.Body.Instructions[0]));p.Add(Instruction.Create(OpCodes.Ret));Prefix(method,p);
}
ReturnCoroutine(M("Room.RoomUnionFight","_WaitForLoadComplete",0),"CanLoad","LoadRoom",false);
ReturnCoroutine(M("Room.RoomUnionFight","RegistAssets",1),"get_Active","RegisterParticle",true);
TryVoid(M("Room.RCMoveNavigation","Tick",0),"TickMove",1);
var approach=M("Room.RCAIUnionSelf","AutoGoto",3);
if (approach.MDToken.Raw != 0x06004092 || approach.MethodSig.Params[2].ElementType != ElementType.R4) throw new InvalidOperationException("Guild approach ABI changed");
Prefix(approach,new List<Instruction>{Instruction.Create(OpCodes.Ldarg_3),Instruction.Create(OpCodes.Call,H("ApproachDistance")),Instruction.Create(OpCodes.Starg,approach.Parameters[3])});

foreach(var n in new[]{"MoveTo","StopTo","LerpPosition"}) TryVoid(M("Room.RCMoveNavigation",n,1),"MoveTo",2);
TryVoid(M("Room.RCMoveNavigation","SetSpeed",1),"Speed",2);
TryVoid(M("Room.RCMoveNavigation","SamplePosition",1),"Reposition",2);
TryVoid(M("Room.RCMoveNavigation","ResetPosition",0),"Reset",1);
var sample=M("Room.RCMoveNavigation","SamplePosition",2);
Prefix(sample,new List<Instruction>{Instruction.Create(OpCodes.Call,H("get_Active")),Instruction.Create(OpCodes.Brfalse,sample.Body.Instructions[0]),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,H("Sample")),Instruction.Create(OpCodes.Ret)});
TryVoid(M("Room.RCLODBase","LoadModel",1),"Siege",2);
foreach(var mini in T("UI.MiniMapComponent").Methods.Where(m=>m.Name=="SetData").ToArray()) TryVoid(mini,"MiniMap",1);
TryVoid(M("Room.RCUIUnionGuard","OnClick",0),"TryEnemyGuardClick",1);
var preparationEntry=M("UI.UnionFightSignUpItem","GotoFightScene",0);
if (preparationEntry.MDToken.Raw != 0x06005597) throw new InvalidOperationException("Guild preparation entry ABI changed");
TryVoid(preparationEntry,"TryPrepareEntry",1);
var townUpdate=M("UI.TownPage","Update",0);
if(townUpdate.MDToken.Raw != 0x0600542a) throw new InvalidOperationException("Town update ABI changed");
Prefix(townUpdate,new List<Instruction>{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,H("ObserveTown"))});
foreach(var name in new[]{"OnClickUnionFightBtn","OnOpenUnionFight"}) TryVoid(M("UI.TownPage",name,0),"BlockTownEntry",1);
var playerClick=M("Room.RCUIUnionActor","OnClick",0);
Prefix(playerClick,new List<Instruction>{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,H("ObservePlayerClick"))});
var scannedSend=M("Room.RoomUnionFightCheckPvp","SendWillFightCharactors",0);
var sendCalls=scannedSend.Body.Instructions.Where(i=>i.OpCode==OpCodes.Callvirt && i.Operand is IMethod method && method.DeclaringType.FullName=="Room.RoomNetUnionFight" && method.Name=="SendCSUFPlayerAskFight").ToArray();
if (scannedSend.MDToken.Raw != 0x06006018 || sendCalls.Length != 1) throw new InvalidOperationException("Original player scan sender changed");
int sendAt=scannedSend.Body.Instructions.IndexOf(sendCalls[0]);
scannedSend.Body.Instructions.Insert(sendAt,Instruction.Create(OpCodes.Ldarg_0));
sendCalls[0].OpCode=OpCodes.Call; sendCalls[0].Operand=H("SendScannedPlayerAsk");
scannedSend.Body.MaxStack=Math.Max(scannedSend.Body.MaxStack,(ushort)8);scannedSend.Body.KeepOldMaxStack=true;
records.Add(new { method="Room.RoomUnionFightCheckPvp::SendWillFightCharactors",rid=scannedSend.Rid,prefixCount=0,scannedSenderAdapter=true });
foreach(var pair in new[]{("SCUFPlayerFightBegin","ObservePlayerBegin"),("SCUFPlayerFightEnd","ObservePlayerEnd")}) {
 var method=M("Room.RoomNetUnionFight",pair.Item1,1);
 if (method.Body.ExceptionHandlers.Count != 0 || method.Body.Instructions.Count(i=>i.OpCode==OpCodes.Ret) != 1 || method.Body.Instructions.Last().OpCode != OpCodes.Ret || method.Body.Instructions.Any(i=>i.OpCode.FlowControl==FlowControl.Branch || i.OpCode.FlowControl==FlowControl.Cond_Branch)) throw new InvalidOperationException("Original player reply consumer shape changed");
 int tailAt=method.Body.Instructions.Count-1;
 foreach(var instruction in new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,H(pair.Item2))}) method.Body.Instructions.Insert(tailAt++,instruction);
 method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)8);method.Body.KeepOldMaxStack=true;
 records.Add(new { method=method.DeclaringType.FullName+"::"+method.Name,rid=method.Rid,prefixCount=0,afterOriginalReplyConsumer=true });
}
// Sample local time once: the original second UtcNow sample can round a minute down.
var clock = T("ServerTimerTool").Methods.Single(m => m.Name == "CorrectTime" && m.MDToken.Raw == 0x06003303);
if (clock.Body == null) throw new InvalidOperationException("CorrectTime body missing");
var clockCode = clock.Body.Instructions;
bool DateCall(Instruction i, string name) => i.OpCode == OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName == "System.DateTime" && m.Name == name;
var nowCalls = clockCode.Where(i => DateCall(i,"get_Now")).ToArray();
var utcCalls = clockCode.Where(i => DateCall(i,"get_UtcNow")).ToArray();
if (nowCalls.Length != 1 || utcCalls.Length != 2 || clockCode.IndexOf(utcCalls[0]) >= clockCode.IndexOf(nowCalls[0]) || clockCode.IndexOf(nowCalls[0]) >= clockCode.IndexOf(utcCalls[1])) throw new InvalidOperationException("CorrectTime clock sample shape changed");
if (clock.Body.ExceptionHandlers.Count != 0 || clock.Body.Variables.Count != 4 || clockCode.Count != 22 || clockCode.IndexOf(nowCalls[0]) != 11 || clockCode.IndexOf(utcCalls[0]) != 5 || clockCode.IndexOf(utcCalls[1]) != 15 || clockCode[12].OpCode != OpCodes.Stloc_2 || clockCode[13].OpCode != OpCodes.Ldloca_S || !DateCall(clockCode[14],"get_Ticks") || clockCode[16].OpCode != OpCodes.Stloc_3 || clockCode[17].OpCode != OpCodes.Ldloca_S || !DateCall(clockCode[18],"get_Ticks") || clockCode[19].OpCode != OpCodes.Sub) throw new InvalidOperationException("CorrectTime exact original offset expression changed");
var nowMethod = (IMethod)nowCalls[0].Operand;
var utcMethod = (IMethod)utcCalls[1].Operand;
if (nowMethod.MethodSig.HasThis || utcMethod.MethodSig.HasThis || nowMethod.MethodSig.Params.Count != 0 || utcMethod.MethodSig.Params.Count != 0 || nowMethod.MethodSig.RetType.FullName != "System.DateTime" || utcMethod.MethodSig.RetType.FullName != "System.DateTime") throw new InvalidOperationException("CorrectTime clock signatures changed");
// Preserve every original instruction object, branch target and EH boundary.
// The inserted pair is stack-neutral; only the UTC producer changes to an
// instance conversion of the exact local sample (no second wall-clock read).
var clockSample = new Local(nowMethod.MethodSig.RetType);
clock.Body.Variables.Add(clockSample);
int nowAt = clockCode.IndexOf(nowCalls[0]);
clockCode.Insert(nowAt+1,Instruction.Create(OpCodes.Stloc,clockSample));
clockCode.Insert(nowAt+2,Instruction.Create(OpCodes.Ldloc,clockSample));
var universal = new MemberRefUser(module,"ToUniversalTime",MethodSig.CreateInstance(nowMethod.MethodSig.RetType),nowMethod.DeclaringType);
int utcAt = clockCode.IndexOf(utcCalls[1]);
utcCalls[1].OpCode = OpCodes.Ldloca;
utcCalls[1].Operand = clockSample;
clockCode.Insert(utcAt+1,Instruction.Create(OpCodes.Call,universal));
clock.Body.KeepOldMaxStack = true;
records.Add(new { method="ServerTimerTool::CorrectTime", rid=clock.Rid, prefixCount=0, singleLocalClockSample=true });
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
System.IO.File.WriteAllText(args[3],JsonSerializer.Serialize(records,new JsonSerializerOptions{WriteIndented=true}));
Console.WriteLine("Scoped guild scenery hooks: " + records.Count);
