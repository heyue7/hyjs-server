using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

// Static preview only. Server routes, map audit and Android acceptance remain gates.
if (args.Length!=2) throw new ArgumentException("input and output managed assemblies required");
using var module=ModuleDefMD.Load(args[0]);
var reference=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerDesertV4, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var hooks=new TypeRefUser(module,"HYJS.DesertV4","FieldHooks",reference);
var mapViewer=new TypeRefUser(module,"HYJS.DesertV4","MapViewer86",reference);
var teamMap=new TypeRefUser(module,"HYJS.DesertV4","TeamMap86",reference);
MemberRef Hook(string name,TypeSig ret,params TypeSig[] p) => new MemberRefUser(module,name,MethodSig.CreateStatic(ret,p),hooks);
var b=module.CorLibTypes.Boolean; var v=module.CorLibTypes.Void; var i4=module.CorLibTypes.Int32;
var str=module.CorLibTypes.String; var obj=module.CorLibTypes.Object;
var openMap=new MemberRefUser(module,"Open",MethodSig.CreateStatic(b,obj),mapViewer);
var teamObserve=new MemberRefUser(module,"Observe",MethodSig.CreateStatic(v,obj,obj),teamMap);
var teamQueue=new MemberRefUser(module,"Queue",MethodSig.CreateStatic(v,obj),teamMap);
var teamTickSent=new MemberRefUser(module,"TickSent",MethodSig.CreateStatic(v,obj),teamMap);
var teamDispatch=new MemberRefUser(module,"InspectDispatch",MethodSig.CreateStatic(v,obj,obj),teamMap);
var teamPositionObserve=new MemberRefUser(module,"ObservePosition",MethodSig.CreateStatic(v,obj,obj),teamMap);
var teamScopeStarting=new MemberRefUser(module,"ScopeStarting",MethodSig.CreateStatic(v,i4),teamMap);
var changed=Hook("ResourceChanged",b); var notification=Hook("ResourceNotification",b,obj); var check=Hook("ResourceCheck",b);
var gather=Hook("Gather",b,str,str); var monsterChanged=Hook("MonsterChanged",b);
var pvpResponse=Hook("PvpResponse",b,obj);
var npcType=module.GetTypes().Single(t=>t.FullName=="Room.RCUINormalNPC");
var openNpc=Hook("OpenNpc",b,new ClassSig(npcType));
var playerType=module.GetTypes().Single(t=>t.FullName=="Room.RCUIPlayer");
var openPlayer=Hook("OpenPlayer",b,new ClassSig(playerType));
var beforeEncounter=Hook("BeforeEncounter",b,i4,str); var fightStart=Hook("FightStartResponse",v,i4,obj);
var patrol=Hook("MonsterPatrolTick",v,obj); var preserve=Hook("PreserveBattle",b);
var tcpConnecting=Hook("TcpConnecting",v); var tcpLogin=Hook("TcpLoginResponse",v,obj);
var endSend=Hook("FightEndSend",v,i4,obj,obj); var endResponse=Hook("FightEndResponse",b,i4,obj,obj);
var resultObjectType=module.GetTypes().Single(t=>t.FullName=="ProtoBuf.Message.ResultObject");
var monsterRewardDisplay=Hook("MonsterRewardDisplay",new ClassSig(resultObjectType),new ClassSig(resultObjectType));
var legacyObserve=Hook("LegacyBossObserve",v,obj,obj); var legacyCompleted=Hook("LegacyBossCompleted",v,obj,obj);
var transferSend=Hook("TransferSend",b,i4,obj); var transferResponse=Hook("TransferResponse",b,i4,obj,obj);
var trackBase=Hook("TrackPositionBase",v,obj,obj);
var trackField=Hook("TrackPositionField",v,obj); var trackPacket=Hook("TrackPositionPacket",v,obj);
var roomLoaded=Hook("RoomLoaded",v,obj); var fieldPositionReady=Hook("FieldPositionReady",v,obj,obj);
var playerDataReady=Hook("PlayerDataReady",v,obj);
var dropPacket=Hook("DropPositionPacket",b,obj); var dropSocket=Hook("DropOldSocket",b,obj,obj);
var actorType=module.GetTypes().Single(t=>t.FullName=="Room.RoomCharactor");
var blockMove=Hook("BlockMove",b,new ClassSig(actorType));
var monsterMoving=Hook("MonsterMoving",b,new ClassSig(actorType));

MethodDef Get(string type,string name) => module.GetTypes().Single(t=>t.FullName==type).Methods.Single(m=>m.Name==name);
void Head(MethodDef m,params Instruction[] add)
{
 if(!m.HasBody) throw new InvalidOperationException("Missing body: "+m.FullName);
 for(int x=add.Length-1;x>=0;x--) m.Body.Instructions.Insert(0,add[x]);
 m.Body.MaxStack=(ushort)Math.Max(8,(int)m.Body.MaxStack);
}
void GuardNoArg(MethodDef m,IMethod hook)
{ var first=m.Body.Instructions[0]; Head(m,Instruction.Create(OpCodes.Call,hook),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret)); }
bool Match(Instruction x,string type,string name)
{ return x.Operand is IMethod m && m.DeclaringType.FullName==type && m.Name==name; }
bool Targets(Instruction x,Instruction target)
{ return ReferenceEquals(x.Operand,target) || (x.Operand is System.Collections.Generic.IList<Instruction> targets && targets.Contains(target)); }

{
 var m=Get("StaticData","InitPlayerData");
 var expected=new[]{"ProtoBuf.Message.SCPlayerLogin","ProtoBuf.Message.SCEquipmentTreasureList","ProtoBuf.Message.SCWarriorList",
  "ProtoBuf.Message.SCItemList","ProtoBuf.Message.SCFragmentList","ProtoBuf.Message.SCFormationList",
  "ProtoBuf.Message.SCChangeNameTimes","ProtoBuf.Message.SCGemAndCookingList","ProtoBuf.Message.SCPetList","ProtoBuf.Message.SCTitleList"};
 if(!m.HasBody || !m.IsStatic || !m.IsPublic || m.MethodSig.HasThis || m.MethodSig.RetType.ElementType!=ElementType.Void ||
    m.MethodSig.Params.Count!=expected.Length || !m.MethodSig.Params.Select(x=>x.FullName).SequenceEqual(expected))
  throw new InvalidOperationException("Original login initialization signature changed");
 var ret=m.Body.Instructions.Last();
 if(ret.OpCode!=OpCodes.Ret || m.Body.Instructions.Count(x=>x.OpCode==OpCodes.Ret)!=1 ||
    m.Body.Instructions.Any(x=>Targets(x,ret)) || m.Body.ExceptionHandlers.Count!=0)
  throw new InvalidOperationException("Original login initialization tail changed");
 int pos=m.Body.Instructions.Count-1;
 m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Ldarg_0));
 m.Body.Instructions.Insert(pos,Instruction.Create(OpCodes.Call,playerDataReady));
}

var field=module.GetTypes().Single(t=>t.FullName=="Room.RoomNetField");
foreach(string name in new[]{"SCFieldResourceInfo","SCSideResourceCheck","SCFieldResourceStartGather","SCFieldGetResource","SendResourceCheck","SendGetFieldRes"})
{
 var m=field.Methods.Single(x=>x.Name==name);
 if(!m.HasBody || m.MethodSig.RetType.ElementType!=ElementType.Void) throw new InvalidOperationException("Unknown resource signature "+name);
 if(name=="SendGetFieldRes")
 {
  if(m.MethodSig.Params.Count!=2 || m.MethodSig.Params.Any(t=>t.ElementType!=ElementType.String)) throw new InvalidOperationException("Gather signature changed");
  var first=m.Body.Instructions[0];
  Head(m,Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Ldarg_2),Instruction.Create(OpCodes.Call,gather),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
 }
 else if(name=="SCFieldResourceInfo")
 {
  var first=m.Body.Instructions[0];
  Head(m,Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,notification),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
 }
 else GuardNoArg(m,check);
}
var onLoaded=Get("Room.RoomField","OnMainCharactorLoaded");
Head(onLoaded,Instruction.Create(OpCodes.Call,changed),Instruction.Create(OpCodes.Pop));
{
 var m=Get("Room.RoomField","OnLoadComplete");
 if(!m.HasBody || !m.MethodSig.HasThis || m.MethodSig.Params.Count!=0 || m.MethodSig.RetType.ElementType!=ElementType.Void)
  throw new InvalidOperationException("Room load completion signature changed");
 var ret=m.Body.Instructions.Last();
 if(ret.OpCode!=OpCodes.Ret || m.Body.Instructions.Count(x=>x.OpCode==OpCodes.Ret)!=1 ||
    m.Body.Instructions.Any(x=>Targets(x,ret)) || m.Body.ExceptionHandlers.Count!=0)
  throw new InvalidOperationException("Unexpected room load completion tail");
 int pos=m.Body.Instructions.Count-1;
 m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Ldarg_0));
 m.Body.Instructions.Insert(pos,Instruction.Create(OpCodes.Call,roomLoaded));
 m.Body.MaxStack=(ushort)Math.Max(8,(int)m.Body.MaxStack);
}
foreach(string name in new[]{"SCSideMonster","SCSideBoss","SCSideWorldBoss","SCSideBossCountdown"})
 GuardNoArg(Get("Room.RoomNetField",name),monsterChanged);
{
 var m=Get("Room.RoomNetField","SCSidePvpFightResult"); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,pvpResponse),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
{
 var m=Get("Room.RCUIPlayer","OnClick"); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,openPlayer),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
{
 var m=Get("Room.RCUINormalNPC","OpenPage"); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,openNpc),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
{
 var m=Get("UI.MiniMapComponent","OnClick"); var first=m.Body.Instructions[0];
 if(m.MethodSig.Params.Count!=0 || m.MethodSig.RetType.ElementType!=ElementType.Void) throw new InvalidOperationException("Mini map click signature changed");
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,openMap),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
foreach(string name in new[]{"SCQueryTeam","SCQueryTeamTick","SCJoinTeam","SCExitTeam","SCTeamKick","SCDisbandTeam","SCTeamCreateAndInvite"})
{
 var m=Get("Room.RoomNetField",name);
 if(!m.MethodSig.HasThis || m.MethodSig.Params.Count!=1 || m.MethodSig.Params[0].ElementType!=ElementType.Object || m.MethodSig.RetType.ElementType!=ElementType.Void)
  throw new InvalidOperationException("Team response signature changed: "+name);
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,teamObserve));
}
foreach(string name in new[]{"SCPlayerAppear","SCMovePush","SCPlayerDisappear"})
{
 var m=Get("Room.RoomNetBase",name);
 if(!m.MethodSig.HasThis || m.MethodSig.Params.Count!=1 || m.MethodSig.Params[0].ElementType!=ElementType.Object || m.MethodSig.RetType.ElementType!=ElementType.Void)
  throw new InvalidOperationException("Position response signature changed: "+name);
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,teamPositionObserve));
}
{
 var m=Get("Room.RoomNetField","SendCSQueryTeamTick");
 if(!m.MethodSig.HasThis || m.MethodSig.Params.Count!=0 || m.MethodSig.RetType.ElementType!=ElementType.Void)
  throw new InvalidOperationException("Team tick send signature changed");
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,teamTickSent));
}
foreach(var item in new[]{(Type:6,Send:"SendPve",Reply:"SCSidePveFightStart"),(Type:12,Send:"SendPveBoss",Reply:"SCSidePveBossFightStart")})
{
 var m=Get("Room.RoomNetField",item.Send); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldc_I4,item.Type),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,beforeEncounter),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
 var reply=Get("Room.RoomNetField",item.Reply);
 Head(reply,Instruction.Create(OpCodes.Ldc_I4,item.Type),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,fightStart));
}
{
 var m=Get("Room.RCAIServerMonster","Tick"); var matches=m.Body.Instructions.Where(x=>Match(x,"HYJS.Field63.Runtime","PatrolTick")).ToArray();
 if(matches.Length!=1) throw new InvalidOperationException("Original patrol call changed");
 matches[0].Operand=patrol;
 var moving=m.Body.Instructions.Where(x=>Match(x,"Room.RoomCharactor","IsMoving")).ToArray();
 if(moving.Length!=1) throw new InvalidOperationException("Original monster animation call changed");
 moving[0].OpCode=OpCodes.Call; moving[0].Operand=monsterMoving;
}
{
 var m=Get("BoxManager","CreateSocketReconnect");
 var normal=m.Body.Instructions.Single(x=>x.OpCode==OpCodes.Ldstr && (string)x.Operand=="UI/MessageBox/NetError");
 var clear=m.Body.Instructions.Single(x=>Match(x,"BoxManager","ClearSocketBox"));
 int pos=m.Body.Instructions.IndexOf(clear)+1;
 m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Call,preserve));
 m.Body.Instructions.Insert(pos,Instruction.Create(OpCodes.Brtrue,normal));
 m.Body.MaxStack=(ushort)Math.Max(8,(int)m.Body.MaxStack);
}
foreach(string name in new[]{"Reconnect","SendSocketLogin","Disconnect"})
 Head(Get("NetAPI",name),Instruction.Create(OpCodes.Call,tcpConnecting));
Head(Get("NetAPI","SCLoginTcpCallback"),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,tcpLogin));
{
 var m=Get("NetAPI","SendSocket");
 var resume=Instruction.Create(OpCodes.Ldarg_0);
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,transferSend),Instruction.Create(OpCodes.Brfalse,resume),Instruction.Create(OpCodes.Ret),
      resume,Instruction.Create(OpCodes.Call,teamScopeStarting));
}
foreach(var item in new[]{(Class:"ManagerController",Name:"SceneTransferCallback",Opcode:30033),(Class:"Room.RoomNetField",Name:"SCTransferLine",Opcode:30062)})
{
 var m=Get(item.Class,item.Name); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldc_I4,item.Opcode),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),
      Instruction.Create(OpCodes.Call,transferResponse),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
Head(Get("NetWork.Sockets.Event.EventManger","AddPacket"),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,trackPacket));
Head(Get("NetWork.Sockets.Event.EventManger","AddPacket"),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,teamQueue));
{
 var m=Get("NetWork.Layer.SocketTool","ReceiveHandler"); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,dropSocket),
      Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
{
 var m=Get("NetWork.Sockets.Event.EventManger","Tick"); var ins=m.Body.Instructions;
 var first=ins[0]; if(first.OpCode.FlowControl!=FlowControl.Branch || !(first.Operand is Instruction condition)) throw new InvalidOperationException("Unexpected event loop");
 var q=ins.Where(x=>Match(x,"System.Collections.Generic.Queue`1<NetWork.Packets.Packet>","Dequeue")).ToArray();
 if(q.Length!=1) q=ins.Where(x=>x.Operand is IMethod cm && cm.Name=="Dequeue" && x.OpCode==OpCodes.Callvirt).ToArray();
 if(q.Length!=1) throw new InvalidOperationException("Unexpected event dequeue");
 int pos=ins.IndexOf(q[0])+1;
 if(ins[pos].OpCode!=OpCodes.Stloc_0) throw new InvalidOperationException("Unexpected event local");
 pos++;
 ins.Insert(pos++,Instruction.Create(OpCodes.Ldloc_0));
 ins.Insert(pos++,Instruction.Create(OpCodes.Call,dropPacket));
 ins.Insert(pos++,Instruction.Create(OpCodes.Brtrue,condition));
 ins.Insert(pos++,Instruction.Create(OpCodes.Ldarg_0));
 ins.Insert(pos++,Instruction.Create(OpCodes.Ldloc_0));
 ins.Insert(pos,Instruction.Create(OpCodes.Call,teamDispatch));
 m.Body.MaxStack=(ushort)Math.Max(8,(int)m.Body.MaxStack);
}
foreach(var item in new[]{(Class:"Room.RoomNetBase",Hook:trackBase,WithThis:true),(Class:"Room.RoomNetField",Hook:trackField,WithThis:false)})
{
 var m=Get(item.Class,"SCCurrentPosition"); var ret=m.Body.Instructions.Last();
 if(ret.OpCode!=OpCodes.Ret || m.Body.Instructions.Count(x=>x.OpCode==OpCodes.Ret)!=1 || m.Body.Instructions.Any(x=>ReferenceEquals(x.Operand,ret))) throw new InvalidOperationException("Unexpected position handler tail");
 if(!item.WithThis && (!m.MethodSig.HasThis || m.MethodSig.Params.Count!=1 || m.MethodSig.Params[0].ElementType!=ElementType.Object ||
    m.MethodSig.RetType.ElementType!=ElementType.Void || m.Body.ExceptionHandlers.Count!=0 || m.Body.Instructions.Any(x=>Targets(x,ret))))
  throw new InvalidOperationException("Field position completion signature or tail changed");
 int pos=m.Body.Instructions.Count-1;
 if(item.WithThis) m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Ldarg_0));
 m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Ldarg_1));
 m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Call,item.Hook));
 if(!item.WithThis)
 {
  m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Ldarg_0));
  m.Body.Instructions.Insert(pos++,Instruction.Create(OpCodes.Ldarg_1));
  m.Body.Instructions.Insert(pos,Instruction.Create(OpCodes.Call,fieldPositionReady));
 }
 m.Body.MaxStack=(ushort)Math.Max(8,(int)m.Body.MaxStack);
}
{
 var m=Get("Room.RoomCharactor","MoveTo"); var first=m.Body.Instructions[0];
 Head(m,Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,blockMove),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
}
foreach(var item in new[]{(Type:6,Class:"Battle.FieldPVEBattleOver",Reply:"FieldPVEFightResultHandler"),(Type:12,Class:"Battle.FieldPVEBossBattleOver",Reply:"FieldPVEBossFightResultHandler")})
{
 var send=Get(item.Class,"AskFieldPVEFightResult");
 var calls=send.Body.Instructions.Where(x=>Match(x,"NetAPI","SendSocket")).ToArray();
 if(calls.Length!=1) throw new InvalidOperationException("Original end send changed: "+item.Class);
 int index=send.Body.Instructions.IndexOf(calls[0]);
 send.Body.Instructions.Insert(index,Instruction.Create(OpCodes.Ldarg_0)); calls[0].Operand=endSend;
 send.Body.MaxStack=(ushort)Math.Max(8,(int)send.Body.MaxStack);
 var reply=Get(item.Class,item.Reply); var first=reply.Body.Instructions[0];
 Head(reply,Instruction.Create(OpCodes.Ldc_I4,item.Type),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),
      Instruction.Create(OpCodes.Call,endResponse),Instruction.Create(OpCodes.Brfalse,first),Instruction.Create(OpCodes.Ret));
 if(item.Type==12)
 {
  var observe=send.Body.Instructions.Where(x=>Match(x,"LocalFieldSettlement","Observe")).ToArray();
  var complete=reply.Body.Instructions.Where(x=>Match(x,"LocalFieldSettlement","Completed")).ToArray();
  if(observe.Length!=1 || complete.Length!=1) throw new InvalidOperationException("Original boss settlement bridge changed");
  observe[0].Operand=legacyObserve; complete[0].Operand=legacyCompleted;
 }
}
{
 var m=Get("Battle.CopyBalance","Refresh");
 var instructions=m.Body.Instructions;
 int installed=0;
 foreach(var item in new[]{(Type:"ProtoBuf.Message.SCSidePveFightEnd",Getter:"get_getn"),
                           (Type:"ProtoBuf.Message.SCSidePveBossFightEnd",Getter:"get_get")})
 {
  var matches=instructions.Where((x,index)=>Match(x,"Battle.CopyBalance","GetResultItem") && index>0 &&
      Match(instructions[index-1],item.Type,item.Getter)).ToArray();
  if(matches.Length!=1 || matches[0].OpCode!=OpCodes.Call ||
     instructions[instructions.IndexOf(matches[0])-1].OpCode!=OpCodes.Callvirt ||
     instructions.Any(x=>Targets(x,matches[0])) ||
     instructions.Count(x=>Match(x,"HYJS.DesertV4.FieldHooks","MonsterRewardDisplay"))!=installed)
   throw new InvalidOperationException("Original reward display changed: "+item.Type);
  instructions.Insert(instructions.IndexOf(matches[0]),Instruction.Create(OpCodes.Call,monsterRewardDisplay));
  installed++;
 }
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("Field V4 static hooks written; no original combat engine method is patched");
