using System;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length!=2) throw new ArgumentException("Input and output managed assemblies required");
byte[] raw=File.ReadAllBytes(args[0]);
string hash=Convert.ToHexString(SHA256.HashData(raw)).ToLowerInvariant();
if (hash!="d7e48841b355d19332a986355dc88f3a5dce93c722e610a073fe589980429e87") throw new Exception("Unexpected shipped Main assembly");
using var main=ModuleDefMD.Load(raw);
var method=(MethodDef)main.ResolveToken(0x06005009u);
if (method.DeclaringType.FullName!="UI.SelectPlayerPanel" || method.Name!="getHeroList" ||
    !method.HasBody || method.Body.ExceptionHandlers.Count!=0 || method.MethodSig.Params.Count!=0 ||
    method.MethodSig.RetType.FullName!="System.Void") throw new Exception("Unexpected selection method");
int offset=checked((int)(uint)main.Metadata.PEImage.ToFileOffset(method.RVA));
int flags=BitConverter.ToUInt16(raw,offset);
int header=(raw[offset]&3)==2 ? 1 : (flags>>12)*4;
int size=(raw[offset]&3)==2 ? raw[offset]>>2 : checked((int)BitConverter.ToUInt32(raw,offset+4));
if (size!=43 || Convert.ToHexString(SHA256.HashData(raw.Skip(offset+header).Take(size).ToArray())).ToLowerInvariant()!=
    "f0ddef9044037d64fda75ba5e6a90de7eab07b9bda5f6766fba81a6785b8df64") throw new Exception("Original selection IL changed");
var call=method.Body.Instructions.Single(i=>i.OpCode==OpCodes.Call && i.Operand is IMethod m && m.DeclaringType.FullName=="NetAPI" && m.Name=="SendHttp");
var original=(IMethod)call.Operand;
if (original.MethodSig.RetType.FullName!="System.Boolean" || !original.MethodSig.Params.Select(p=>p.FullName).SequenceEqual(
    new[]{"System.Int32","System.Object","NetWork.Layer.HttpHandler"})) throw new Exception("Original HTTP signature changed");
var reference=main.GetAssemblyRefs().Single(r=>r.Name=="SoulFlowerDesertV4");
var targetType=new TypeRefUser(main,"HYJS.DesertV4","ExpeditionHeroList",reference);
var callbackType=main.GetTypes().Single(t=>t.FullName=="NetWork.Layer.HttpHandler").ToTypeSig();
call.Operand=new MemberRefUser(main,"Send",MethodSig.CreateStatic(main.CorLibTypes.Boolean,main.CorLibTypes.Int32,main.CorLibTypes.Object,callbackType),targetType);
foreach (var m in main.GetTypes().SelectMany(t=>t.Methods).Where(m=>m.HasBody)) m.Body.KeepOldMaxStack=true;
var options=new ModuleWriterOptions(main);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
main.Write(args[1],options);
Console.WriteLine("Patched the single expedition selection HTTP call");
