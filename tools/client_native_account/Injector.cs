using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;
using System;
using System.Linq;

using var module = ModuleDefMD.Load(args[0]);
using var helper = ModuleDefMD.Load(args[1]);
MethodDef Method(string type, string name) => module.GetTypes().Single(t=>t.FullName==type).Methods.Single(m=>m.Name==name);
var reference = new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerNativeAccount, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var api = new TypeRefUser(module,"HYJS.NativeAccount","AccountUI",reference);
var user = module.GetTypes().Single(t=>t.FullName=="UserInput");
// Use the local TypeDef in signatures. Importing it from the helper can create an unsupported self ModuleRef.
IMethod Bridge(string name) => new MemberRefUser(module,name,MethodSig.CreateStatic(name=="HandleLogin"?module.CorLibTypes.Boolean:module.CorLibTypes.Void,user.ToTypeSig()),api);
var show = Method("UserInput", "Show");
var ret = show.Body.Instructions.Last();
show.Body.Instructions.Insert(show.Body.Instructions.Count-1, Instruction.Create(OpCodes.Ldarg_0));
show.Body.Instructions.Insert(show.Body.Instructions.Count-1, Instruction.Create(OpCodes.Call, Bridge("Show")));
show.Body.MaxStack = 12; show.Body.KeepOldMaxStack = true;
var register = Method("UserInput", "OnRegistButtonClick");
register.Body = new CilBody{MaxStack=2,KeepOldMaxStack=true};
register.Body.Instructions.Add(Instruction.Create(OpCodes.Ldarg_0));
register.Body.Instructions.Add(Instruction.Create(OpCodes.Call, Bridge("Register")));
register.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var login = Method("UserInput", "OnLoginButton");
var original = login.Body.Instructions[0];
var prefix = new[]{Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Call,Bridge("HandleLogin")),Instruction.Create(OpCodes.Brfalse,original),Instruction.Create(OpCodes.Ret)};
for(int i=0;i<prefix.Length;i++)login.Body.Instructions.Insert(i,prefix[i]);
login.Body.MaxStack=12;login.Body.KeepOldMaxStack=true;
// The shipped feedback hook accidentally serialized maxStack=0. Its seven-slot Java call needs a valid header.
var feedback=Method("UI.KeFuPage","DoOpen");feedback.Body.MaxStack=8;feedback.Body.KeepOldMaxStack=true;
var transfer=Method("UI.WorldMapPage","transfer");
int count=0;
foreach(var instruction in transfer.Body.Instructions)if(instruction.OpCode==OpCodes.Ldstr && (string)instruction.Operand=="town_side_"){instruction.Operand="";count++;}
if(count!=2)throw new InvalidOperationException("Expected two original world-map transfer prefixes");
var options=new ModuleWriterOptions(module);options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
