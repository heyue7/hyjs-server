using System;
using System.Linq;
using System.Collections.Generic;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=3)throw new ArgumentException("current DLL, original APK DLL and output DLL required");
using var module=ModuleDefMD.Load(args[0]);
using var original=ModuleDefMD.Load(args[1]);
var importer=new Importer(module,ImporterOptions.TryToUseTypeDefs|ImporterOptions.TryToUseMethodDefs|ImporterOptions.TryToUseFieldDefs);
MethodDef Method(ModuleDef owner,string type,string name)=>owner.GetTypes().Single(t=>t.FullName==type).Methods.Single(m=>m.Name==name);

// Restore only the three original town event handlers disabled by the old optional demo flag.
foreach(string name in new[]{"OnLogin","OnLevelUp","OnQuestComplete"}){
    var target=Method(module,"GuideManager",name);var source=Method(original,"GuideManager",name);
    if(!target.Body.Instructions.All(i=>i.OpCode==OpCodes.Ret||i.OpCode==OpCodes.Nop))throw new InvalidOperationException("Guide handler is no longer disabled: "+name);
    var body=source.Body;
    foreach(var instruction in body.Instructions){
        if(instruction.Operand is IMDTokenProvider token)instruction.Operand=module.ResolveToken(token.MDToken.Raw);
        else if(instruction.Operand is Parameter parameter)instruction.Operand=target.Parameters[parameter.Index];
    }
    // Preserve the original retained StandAloneSig row instead of reimporting
    // its local types as new self-assembly references.
    if(body.Variables.Count>0){
        var locals=(module.ResolveToken(body.LocalVarSigTok) as StandAloneSig)?.LocalSig?.Locals;
        if(locals==null||locals.Count!=body.Variables.Count)throw new InvalidOperationException("Original retained guide local signature is unavailable");
        for(int i=0;i<locals.Count;i++)body.Variables[i].Type=locals[i];
    }
    foreach(var handler in body.ExceptionHandlers)if(handler.CatchType!=null)handler.CatchType=module.ResolveToken(handler.CatchType.MDToken.Raw) as ITypeDefOrRef;
    body.KeepOldMaxStack=true;target.Body=body;
}

var unity=module.GetAssemblyRefs().Single(a=>a.Name=="UnityEngine");
var javaClass=new TypeRefUser(module,"UnityEngine","AndroidJavaClass",unity);
var javaObject=new TypeRefUser(module,"UnityEngine","AndroidJavaObject",unity);
var constructor=new MemberRefUser(module,".ctor",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String),javaClass);
var call=new MemberRefUser(module,"CallStatic",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String,new SZArraySig(module.CorLibTypes.Object)),javaObject);
var dispose=new MemberRefUser(module,"Dispose",MethodSig.CreateInstance(module.CorLibTypes.Void),javaObject);
var getUrl=Method(module,"ServerConfig","GetServerUrl");
var component=new TypeRefUser(module,"UnityEngine","Component",unity);
var gameObject=new TypeRefUser(module,"UnityEngine","GameObject",unity);
var unityObject=new TypeRefUser(module,"UnityEngine","Object",unity);
var getGameObject=new MemberRefUser(module,"get_gameObject",MethodSig.CreateInstance(new ClassSig(gameObject)),component);
var getName=new MemberRefUser(module,"get_name",MethodSig.CreateInstance(module.CorLibTypes.String),unityObject);
List<Instruction> Java(string method,bool feedback){
    var list=new List<Instruction>{Instruction.Create(OpCodes.Ldstr,"com.hyjs.wallet.ClientFeedback"),Instruction.Create(OpCodes.Newobj,constructor),Instruction.Create(OpCodes.Dup),Instruction.Create(OpCodes.Ldstr,method),Instruction.Create(OpCodes.Ldc_I4,feedback?2:0),Instruction.Create(OpCodes.Newarr,module.CorLibTypes.Object.TypeDefOrRef)};
    if(feedback){
        list.AddRange(new[]{Instruction.Create(OpCodes.Dup),Instruction.Create(OpCodes.Ldc_I4_0),Instruction.Create(OpCodes.Call,getUrl),Instruction.Create(OpCodes.Stelem_Ref),Instruction.Create(OpCodes.Dup),Instruction.Create(OpCodes.Ldc_I4_1),Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,getGameObject),Instruction.Create(OpCodes.Callvirt,getName),Instruction.Create(OpCodes.Stelem_Ref)});
    }
    list.Add(Instruction.Create(OpCodes.Callvirt,call));list.Add(Instruction.Create(OpCodes.Callvirt,dispose));return list;
}
foreach(var pair in new[]{("UI.AnnouncePage","DoOpen"),("SystemAnnouncePage","Show")}){
    var target=Method(module,pair.Item1,pair.Item2);var prefix=Java("prepareNotices",false);
    for(int i=0;i<prefix.Count;i++)target.Body.Instructions.Insert(i,prefix[i]);
    target.Body.MaxStack=Math.Max(target.Body.MaxStack,(ushort)7);target.Body.KeepOldMaxStack=true;
}
{
    var target=Method(module,"UI.KeFuPage","DoOpen");
    var baseOpen=(IMethod)target.Body.Instructions.First(i=>i.OpCode==OpCodes.Call&&i.Operand is IMethod method&&method.Name=="DoOpen").Operand;
    target.Body=new CilBody{MaxStack=8,KeepOldMaxStack=true};target.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));target.Body.Instructions.Add(Instruction.Create(OpCodes.Call,baseOpen));
    foreach(var instruction in Java("show",true))target.Body.Instructions.Add(instruction);
    target.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
}
{
    // The successful original callback already owns the list and resets its scroll.
    // Add an original popup only when no operational achievements are configured.
    var target=Method(module,"UI.AchievementPage","<CreateAchievementList>m__BE");
    var response=target.Body.Variables.Single(v=>v.Type.FullName=="ProtoBuf.Message.SCAchievementList");
    var getList=Method(module,"ProtoBuf.Message.SCAchievementList","get_list");
    var count=new MemberRefUser(module,"get_Count",MethodSig.CreateInstance(module.CorLibTypes.Int32),new TypeSpecUser(getList.MethodSig.RetType));
    var popup=Method(module,"BoxManager","CreatePopupTextBox");
    var reset=target.Body.Instructions.First(i=>i.OpCode==OpCodes.Ldfld&&i.Operand is IField f&&f.Name=="scrollView");
    int index=target.Body.Instructions.IndexOf(reset)-1;var continuation=target.Body.Instructions[index];
    var inserted=new[]{Instruction.Create(OpCodes.Ldloc,response),Instruction.Create(OpCodes.Callvirt,getList),Instruction.Create(OpCodes.Callvirt,count),Instruction.Create(OpCodes.Brtrue,continuation),Instruction.Create(OpCodes.Ldstr,"成就奖励待配置"),Instruction.Create(OpCodes.Ldc_R4,0f),Instruction.Create(OpCodes.Call,popup)};
    foreach(var instruction in target.Body.Instructions)if(ReferenceEquals(instruction.Operand,continuation))instruction.Operand=inserted[0];
    foreach(var handler in target.Body.ExceptionHandlers){if(ReferenceEquals(handler.HandlerEnd,continuation))handler.HandlerEnd=inserted[0];if(ReferenceEquals(handler.TryEnd,continuation))handler.TryEnd=inserted[0];}
    for(int i=0;i<inserted.Length;i++)target.Body.Instructions.Insert(index+i,inserted[i]);
    target.Body.MaxStack=Math.Max(target.Body.MaxStack,(ushort)3);target.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);Console.WriteLine("Restored original guides, original announcement transparency, feedback entry and empty-achievement notice");
