using System;
using System.Linq;
using System.Collections.Generic;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=2)throw new ArgumentException("input/output DLL required");
using var module=ModuleDefMD.Load(args[0]);
var user=module.GetTypes().Single(t=>t.FullName=="UserInput");
var method=user.Methods.Single(m=>m.Name=="OnLoginButton");
if(method.Body.Instructions.Any(i=>i.OpCode==OpCodes.Ldstr&&i.Operand is string s&&s=="hasForm"))throw new InvalidOperationException("Native login form prefix already exists");
var getInput=(IMethod)method.Body.Instructions.First(i=>i.Operand is MethodSpec s&&s.Name=="GetComponent").Operand;
var getter=(IMethod)method.Body.Instructions.First(i=>i.Operand is IMethod m&&m.Name=="get_value").Operand;
var setter=new MemberRefUser(module,"set_value",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String),getter.DeclaringType);
var unity=module.GetAssemblyRefs().Single(a=>a.Name=="UnityEngine");
var javaClass=new TypeRefUser(module,"UnityEngine","AndroidJavaClass",unity);
var javaObject=new TypeRefUser(module,"UnityEngine","AndroidJavaObject",unity);
var ctor=new MemberRefUser(module,".ctor",MethodSig.CreateInstance(module.CorLibTypes.Void,module.CorLibTypes.String),javaClass);
var valueCall=new MemberRefUser(module,"CallStatic",MethodSig.CreateInstanceGeneric(1,new GenericMVar(0),module.CorLibTypes.String,new SZArraySig(module.CorLibTypes.Object)),javaObject);
var dispose=new MemberRefUser(module,"Dispose",MethodSig.CreateInstance(module.CorLibTypes.Void),javaObject);
var java=new Local(new ClassSig(javaClass));method.Body.Variables.Add(java);method.Body.InitLocals=true;
var done=Instruction.Create(OpCodes.Ldloc,java);
var prefix=new List<Instruction>{Instruction.Create(OpCodes.Ldstr,"com.hyjs.wallet.LoginCache"),Instruction.Create(OpCodes.Newobj,ctor),Instruction.Create(OpCodes.Stloc,java)};
void Read(string name,TypeSig result){prefix.Add(Instruction.Create(OpCodes.Ldloc,java));prefix.Add(Instruction.Create(OpCodes.Ldstr,name));prefix.Add(Instruction.Create(OpCodes.Ldc_I4_0));prefix.Add(Instruction.Create(OpCodes.Newarr,module.CorLibTypes.Object.TypeDefOrRef));prefix.Add(Instruction.Create(OpCodes.Callvirt,new MethodSpecUser(valueCall,new GenericInstMethodSig(result))));}
Read("hasForm",module.CorLibTypes.Boolean);prefix.Add(Instruction.Create(OpCodes.Brfalse,done));
foreach(var pair in new[]{("nameInput","formUserName"),("passWordInput","formPassWord")}){
    prefix.Add(Instruction.Create(OpCodes.Ldarg_0));prefix.Add(Instruction.Create(OpCodes.Ldfld,user.Fields.Single(f=>f.Name==pair.Item1)));prefix.Add(Instruction.Create(OpCodes.Callvirt,getInput));
    Read(pair.Item2,module.CorLibTypes.String);prefix.Add(Instruction.Create(OpCodes.Callvirt,setter));
}
prefix.Add(done);prefix.Add(Instruction.Create(OpCodes.Callvirt,dispose));
for(int i=0;i<prefix.Count;i++)method.Body.Instructions.Insert(i,prefix[i]);
method.Body.MaxStack=Math.Max(method.Body.MaxStack,(ushort)8);method.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("OnLoginButton reads only synchronized native-form strings then retains original validation and network flow; prefix instructions="+prefix.Count);
