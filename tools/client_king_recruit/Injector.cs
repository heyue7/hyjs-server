using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length!=2) throw new ArgumentException("input managed, output managed");
using var module=ModuleDefMD.Load(args[0]);
TypeDef T(string name)=>module.GetTypes().Single(t=>t.FullName==name);
MethodDef M(string type,string method)=>T(type).Methods.Single(m=>m.Name==method);
var reference=new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerKingRecruit, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper=new TypeRefUser(module,"HYJS.KingRecruit","KingRecruitWorldUI",reference);
var tab=T("UI.TabKingsRecruit");
MemberRefUser TabCall(string name,TypeSig result)=>new(module,name,MethodSig.CreateStatic(result,tab.ToTypeSig()),helper);
var ensure=TabCall("Ensure",module.CorLibTypes.Void);
var received=TabCall("Received",module.CorLibTypes.Void);
var canAct=TabCall("CanAct",module.CorLibTypes.Boolean);
void Prefix(MethodDef method,Instruction[] prefix) {foreach(var i in prefix.Reverse()) method.Body.Instructions.Insert(0,i);}
void Tail(MethodDef method,Func<Instruction[]> added) {
 foreach(var ret in method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()) {
  int at=method.Body.Instructions.IndexOf(ret);var sequence=added();ret.OpCode=sequence[0].OpCode;ret.Operand=sequence[0].Operand;
  foreach(var i in sequence.Skip(1)) method.Body.Instructions.Insert(++at,i);
  method.Body.Instructions.Insert(++at,Instruction.Create(OpCodes.Ret));
 }
}
var update=M("UI.TabKingsRecruit","UpdateAll");
Prefix(update,new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,ensure)});
var info=M("UI.TabKingsRecruit","OnMsgCallback");
Tail(info,()=>new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,received)});
foreach(string name in new[]{"OnRecruitOnce","OnRecruitTen","OnRewardBtn","OnPreviewBtn","OnBattlePreviewBtn"}) {
 var method=M("UI.TabKingsRecruit",name);var start=method.Body.Instructions[0];
 Prefix(method,new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,canAct),Instruction.Create(OpCodes.Brtrue,start),Instruction.Create(OpCodes.Ret)});
}
// Track the original request lifecycle, including its false/error callback.
var beginDraw=TabCall("BeginDraw",module.CorLibTypes.Void);
var endDraw=TabCall("EndDraw",module.CorLibTypes.Void);
foreach(string name in new[]{"OnRecruitOnce","OnRecruitTen"}) {
 var method=M("UI.TabKingsRecruit",name);
 var send=method.Body.Instructions.Single(i=>i.Operand is IMethod m && m.Name=="SendHttp");
 int at=method.Body.Instructions.IndexOf(send);
 method.Body.Instructions.Insert(at++,Instruction.Create(OpCodes.Ldarg_0));
 method.Body.Instructions.Insert(at,Instruction.Create(OpCodes.Call,beginDraw));
}
var callback=M("UI.TabKingsRecruit","RecruitCallback");
Tail(callback,()=>new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,endDraw)});
var card=M("UI.RecruitResultCardItem","SetHeroBuyId");
var instructions=card.Body.Instructions;
var pathCall=instructions.Single(i=>i.Operand is IMethod m && m.Name=="SetPath");
int atPath=instructions.IndexOf(pathCall);
if(instructions[atPath-2].OpCode!=OpCodes.Ldc_I4_0 || instructions[atPath-1].OpCode!=OpCodes.Ldnull || instructions[atPath-3].Operand is not IMethod concat || concat.Name!="Concat") throw new InvalidOperationException("Original recruit card path sequence differs");
var path=new MemberRefUser(module,"OriginalCardPath",MethodSig.CreateStatic(module.CorLibTypes.String,module.CorLibTypes.String),helper);
instructions.Insert(atPath-2,Instruction.Create(OpCodes.Call,path));
var bind=new MemberRefUser(module,"BindCard",MethodSig.CreateStatic(module.CorLibTypes.Void,card.DeclaringType.ToTypeSig(),module.CorLibTypes.String),helper);
Tail(card,()=>new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldarg_1),Instruction.Create(OpCodes.Call,bind)});
foreach(var method in new[]{update,info,card,callback}.Concat(tab.Methods.Where(m=>new[]{"OnRecruitOnce","OnRecruitTen","OnRewardBtn","OnPreviewBtn","OnBattlePreviewBtn"}.Contains(m.Name.String)))) {
 method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)8);
 method.Body.KeepOldMaxStack=true;method.Body.SimplifyBranches();method.Body.OptimizeBranches();
}
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
