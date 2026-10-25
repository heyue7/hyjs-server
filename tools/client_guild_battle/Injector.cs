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
  "Speed" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCMoveNavigation").ToTypeSig(),module.CorLibTypes.Single),
  "Sample" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,vector,new ByRefSig(vector)),
  "Siege" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCLODBase").ToTypeSig(),module.CorLibTypes.String),
  "TryEnemyGuardClick" => MethodSig.CreateStatic(module.CorLibTypes.Boolean,T("Room.RCUIUnionGuard").ToTypeSig()),
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
foreach(var n in new[]{"MoveTo","StopTo","LerpPosition"}) TryVoid(M("Room.RCMoveNavigation",n,1),"MoveTo",2);
TryVoid(M("Room.RCMoveNavigation","SetSpeed",1),"Speed",2);
TryVoid(M("Room.RCMoveNavigation","SamplePosition",1),"Reposition",2);
TryVoid(M("Room.RCMoveNavigation","ResetPosition",0),"Reset",1);
var sample=M("Room.RCMoveNavigation","SamplePosition",2);
Prefix(sample,new List<Instruction>{Instruction.Create(OpCodes.Call,H("get_Active")),Instruction.Create(OpCodes.Brfalse,sample.Body.Instructions[0]),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,H("Sample")),Instruction.Create(OpCodes.Ret)});
TryVoid(M("Room.RCLODBase","LoadModel",1),"Siege",2);
foreach(var mini in T("UI.MiniMapComponent").Methods.Where(m=>m.Name=="SetData").ToArray()) TryVoid(mini,"MiniMap",1);
TryVoid(M("Room.RCUIUnionGuard","OnClick",0),"TryEnemyGuardClick",1);
foreach(var pair in new[]{("SCUFPlayerFightBegin","ObservePlayerBegin"),("SCUFPlayerFightEnd","ObservePlayerEnd")}) {
 var method=M("Room.RoomNetUnionFight",pair.Item1,1);
 Prefix(method,new List<Instruction>{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,H(pair.Item2))});
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
System.IO.File.WriteAllText(args[3],JsonSerializer.Serialize(records,new JsonSerializerOptions{WriteIndented=true}));
Console.WriteLine("Scoped guild scenery hooks: " + records.Count);
