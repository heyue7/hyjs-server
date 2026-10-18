using System;
using System.Linq;
using System.Collections.Generic;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=2)throw new ArgumentException("input/output DLL required");
using var module=ModuleDefMD.Load(args[0]);
MethodDef Method(string type,string name)=>module.GetTypes().Single(t=>t.FullName==type).Methods.Single(m=>m.Name==name);
var unity=module.GetAssemblyRefs().Single(a=>a.Name=="UnityEngine");
var javaClass=new TypeRefUser(module,"UnityEngine","AndroidJavaClass",unity);
var javaObject=new TypeRefUser(module,"UnityEngine","AndroidJavaObject",unity);
var ctor=new MemberRefUser(module,".ctor",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String),javaClass);
var voidCall=new MemberRefUser(module,"CallStatic",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String,new SZArraySig(module.CorLibTypes.Object)),javaObject);
var valueCall=new MemberRefUser(module,"CallStatic",MethodSig.CreateInstanceGeneric(1,new GenericMVar(0),module.CorLibTypes.String,new SZArraySig(module.CorLibTypes.Object)),javaObject);
var dispose=new MemberRefUser(module,"Dispose",MethodSig.CreateInstance(module.CorLibTypes.Void),javaObject);
var component=new TypeRefUser(module,"UnityEngine","Component",unity);var go=new TypeRefUser(module,"UnityEngine","GameObject",unity);var obj=new TypeRefUser(module,"UnityEngine","Object",unity);
var getGo=new MemberRefUser(module,"get_gameObject",MethodSig.CreateInstance(new ClassSig(go)),component);
var getName=new MemberRefUser(module,"get_name",MethodSig.CreateInstance(module.CorLibTypes.String),obj);
List<Instruction> Owner()=>new(){Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,getGo),Instruction.Create(OpCodes.Callvirt,getName)};
List<Instruction> Call(string name,List<Instruction>[] arguments,TypeSig? result=null){
    var list=new List<Instruction>{Instruction.Create(OpCodes.Ldstr,"com.hyjs.wallet.LoginCache"),Instruction.Create(OpCodes.Newobj,ctor)};
    if(result==null)list.Add(Instruction.Create(OpCodes.Dup));
    list.Add(Instruction.Create(OpCodes.Ldstr,name));list.Add(Instruction.Create(OpCodes.Ldc_I4,arguments.Length));list.Add(Instruction.Create(OpCodes.Newarr,module.CorLibTypes.Object.TypeDefOrRef));
    for(int i=0;i<arguments.Length;i++){list.Add(Instruction.Create(OpCodes.Dup));list.Add(Instruction.Create(OpCodes.Ldc_I4,i));list.AddRange(arguments[i]);list.Add(Instruction.Create(OpCodes.Stelem_Ref));}
    list.Add(Instruction.Create(OpCodes.Callvirt,result==null?voidCall:new MethodSpecUser(valueCall,new GenericInstMethodSig(result))));
    if(result==null)list.Add(Instruction.Create(OpCodes.Callvirt,dispose));
    return list;
}
void Insert(MethodDef method,int at,List<Instruction> list){for(int i=0;i<list.Count;i++)method.Body.Instructions.Insert(at+i,list[i]);method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)12);method.Body.KeepOldMaxStack=true;}
void Prefix(string type,string name,List<Instruction> list)=>Insert(Method(type,name),0,list);
void AtReturns(MethodDef method,Func<List<Instruction>> factory){foreach(var ret in method.Body.Instructions.Where(i=>i.OpCode==OpCodes.Ret).ToArray()){
 var list=factory();foreach(var i in method.Body.Instructions)if(ReferenceEquals(i.Operand,ret))i.Operand=list[0];
 foreach(var h in method.Body.ExceptionHandlers){if(h.TryEnd==ret)h.TryEnd=list[0];if(h.HandlerEnd==ret)h.HandlerEnd=list[0];}
 Insert(method,method.Body.Instructions.IndexOf(ret),list);
}}
var data=module.GetTypes().Single(t=>t.FullName=="StaticData");
foreach(var property in new[]{"UserName","PassWord"}){
 var get=Method("StaticData","get_"+property);get.Body=new CilBody();foreach(var i in Call("transient"+property,Array.Empty<List<Instruction>>(),module.CorLibTypes.String))get.Body.Instructions.Add(i);get.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
 var set=Method("StaticData","set_"+property);set.Body=new CilBody();foreach(var i in Call("setTransient"+property,new[]{new List<Instruction>{Instruction.Create(OpCodes.Ldarg_0)}}))set.Body.Instructions.Add(i);set.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
}
// Delete only the two original sensitive PlayerPrefs keys; never import them.
var prefs=new TypeRefUser(module,"UnityEngine","PlayerPrefs",unity);var delete=new MemberRefUser(module,"DeleteKey",MethodSig.CreateStatic(module.CorLibTypes.Void,module.CorLibTypes.String),prefs);
var savePrefs=new MemberRefUser(module,"Save",MethodSig.CreateStatic(module.CorLibTypes.Void),prefs);
var clean=new List<Instruction>();foreach(var key in new[]{"userName","passWord"}){clean.Add(Instruction.Create(OpCodes.Ldstr,key));clean.Add(Instruction.Create(OpCodes.Call,delete));}clean.Add(Instruction.Create(OpCodes.Call,savePrefs));
Prefix("LoginUI","Init",clean);
var user=module.GetTypes().Single(t=>t.FullName=="UserInput");
var loginButton=Method("UserInput","OnLoginButton");
var getInput=(IMethod)loginButton.Body.Instructions.First(i=>i.Operand is MethodSpec s&&s.Name=="GetComponent").Operand;
var valueGetter=(IMethod)loginButton.Body.Instructions.First(i=>i.Operand is IMethod m&&m.Name=="get_value").Operand;
var valueSetter=new MemberRefUser(module,"set_value",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String),valueGetter.DeclaringType);
AtReturns(Method("UserInput","Show"),()=>{
 var list=new List<Instruction>();
 foreach(var pair in new[]{("nameInput","username"),("passWordInput","password")}){
  list.Add(Instruction.Create(OpCodes.Ldarg_0));list.Add(Instruction.Create(OpCodes.Ldfld,user.Fields.Single(f=>f.Name==pair.Item1)));list.Add(Instruction.Create(OpCodes.Callvirt,getInput));list.AddRange(Call(pair.Item2,Array.Empty<List<Instruction>>(),module.CorLibTypes.String));list.Add(Instruction.Create(OpCodes.Callvirt,valueSetter));
 }
 list.AddRange(Call("show",new[]{Owner()}));return list;
});
foreach(string name in new[]{"Hide","OnRegistButtonClick"})Prefix("UserInput",name,Call("hide",Array.Empty<List<Instruction>>()));
Prefix("UserInput","OnCloseButtonClick",Call("pause",Array.Empty<List<Instruction>>()));
// Successful input validation owns the hide; do not hide on empty input.
var setPassword=loginButton.Body.Instructions.First(i=>i.Operand is IMethod m&&m.Name=="set_PassWord");Insert(loginButton,loginButton.Body.Instructions.IndexOf(setPassword)+1,Call("hide",Array.Empty<List<Instruction>>()));
AtReturns(Method("LoginUI","ShowNormalUI"),()=>Call("boot",new[]{Owner()}));
AtReturns(Method("Login.LoginManager","InitServerInfo"),()=>{
 var list=new List<Instruction>();var player=data.Fields.Single(f=>f.Name=="playerData");
 var playerType=((ClassSig)player.FieldSig.Type).TypeDefOrRef.ResolveTypeDef();var zone=playerType.Fields.Single(f=>f.Name=="currentZone");var end=Instruction.Create(OpCodes.Nop);
 list.Add(Instruction.Create(OpCodes.Ldsfld,player));list.Add(Instruction.Create(OpCodes.Ldfld,zone));list.Add(Instruction.Create(OpCodes.Brfalse,end));list.AddRange(Call("ready",Array.Empty<List<Instruction>>()));list.Add(end);return list;
});
foreach(var pair in new[]{("LoginUI","OnSwitchAccountButtonClick"),("ManagerController","BackToLogin")})Prefix(pair.Item1,pair.Item2,Call("pause",Array.Empty<List<Instruction>>()));
var isLogining=(IField)Method("LoginUI","OnPressLogin").Body.Instructions.First(i=>i.OpCode==OpCodes.Ldsfld&&i.Operand is IField f&&f.Name=="isLogining").Operand;
Prefix("LoginUI","OnSwitchAccountButtonClick",new List<Instruction>{Instruction.Create(OpCodes.Ldc_I4_0),Instruction.Create(OpCodes.Stsfld,isLogining)});
var login=module.GetTypes().Single(t=>t.FullName=="Login.LoginUserLogin");
var start=Method("Login.LoginUserLogin","UserLogin");
// No added managed fields: the original old Unity loader does not accept FieldPtr.
// Bind the original network delegate to a separate shallow instance snapshot.
var snapshot=new Local(new ClassSig(login));start.Body.Variables.Add(snapshot);start.Body.InitLocals=true;
var clone=new MemberRefUser(module,"MemberwiseClone",MethodSig.CreateInstance(module.CorLibTypes.Object),module.CorLibTypes.Object.TypeDefOrRef);
var helpers=new TypeRefUser(module,"System.Runtime.CompilerServices","RuntimeHelpers",module.CorLibTypes.AssemblyRef);
var identity=new MemberRefUser(module,"GetHashCode",MethodSig.CreateStatic(module.CorLibTypes.Int32,module.CorLibTypes.Object),helpers);
// Ldarg_0 has no operand; construct explicitly for dnlib's instruction contract.
List<Instruction> SnapshotIdentity()=>new(){Instruction.Create(OpCodes.Ldloc,snapshot),Instruction.Create(OpCodes.Call,identity),Instruction.Create(OpCodes.Box,module.CorLibTypes.Int32.TypeDefOrRef)};
List<Instruction> Attempt()=>new(){Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,identity),Instruction.Create(OpCodes.Box,module.CorLibTypes.Int32.TypeDefOrRef)};
var cloneCode=new List<Instruction>{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,clone),Instruction.Create(OpCodes.Castclass,login),Instruction.Create(OpCodes.Stloc,snapshot)};
cloneCode.AddRange(Call("begin",new[]{SnapshotIdentity(),new List<Instruction>{Instruction.Create(OpCodes.Call,Method("StaticData","get_UserName"))},new List<Instruction>{Instruction.Create(OpCodes.Call,Method("StaticData","get_PassWord"))}},module.CorLibTypes.Boolean));
var accepted=Instruction.Create(OpCodes.Nop);cloneCode.Add(Instruction.Create(OpCodes.Brtrue,accepted));cloneCode.Add(Instruction.Create(OpCodes.Ret));cloneCode.Add(accepted);
var delegateMethod=start.Body.Instructions.Single(i=>i.OpCode==OpCodes.Ldftn&&i.Operand is IMethod m&&m.Name=="UserLoginCallBack");
var delegateOwner=start.Body.Instructions[start.Body.Instructions.IndexOf(delegateMethod)-1];
if(delegateOwner.OpCode!=OpCodes.Ldarg_0)throw new InvalidOperationException("Unexpected original login delegate owner");
Insert(start,0,cloneCode);delegateOwner.OpCode=OpCodes.Ldloc;delegateOwner.Operand=snapshot;
var callback=Method("Login.LoginUserLogin","UserLoginCallBack");var allow=Instruction.Create(OpCodes.Nop);var guard=Call("accepts",new[]{Attempt()},module.CorLibTypes.Boolean);guard.Add(Instruction.Create(OpCodes.Brtrue,allow));guard.Add(Instruction.Create(OpCodes.Ret));guard.Add(allow);Insert(callback,0,guard);
var sessionStore=callback.Body.Instructions.Single(i=>i.OpCode==OpCodes.Stsfld&&i.Operand is IField f&&f.Name=="sessionKey");
List<Instruction> Complete(bool success)=>Call("complete",new[]{Attempt(),new List<Instruction>{Instruction.Create(success?OpCodes.Ldc_I4_1:OpCodes.Ldc_I4_0),Instruction.Create(OpCodes.Box,module.CorLibTypes.Boolean.TypeDefOrRef)}});
Insert(callback,callback.Body.Instructions.IndexOf(sessionStore)+1,Complete(true));AtReturns(callback,()=>Complete(false));
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;module.Write(args[1],options);
Console.WriteLine("Encrypted successful-login cache hooks; original login/role creation transport retained");
