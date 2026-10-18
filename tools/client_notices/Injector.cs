using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length != 3) throw new ArgumentException("input managed, notice bridge, output managed");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
var reference = new AssemblyRefUser(new AssemblyNameInfo("SoulFlowerNotices, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
var helper = new TypeRefUser(module, "HYJS.Notices", "OriginalNotices", reference);
TypeDef T(string name) => module.GetTypes().Single(t=>t.FullName == name);
MethodDef M(string owner,string name) => T(owner).Methods.Single(m=>m.Name == name);
IMethod B(string name,params TypeSig[] parameters) {
    var declared = bridge.GetTypes().Single(t=>t.FullName=="HYJS.Notices.OriginalNotices").Methods.Single(m=>m.Name==name);
    if(declared.MethodSig.RetType.ElementType!=ElementType.Void || !declared.MethodSig.Params.Select(p=>p.FullName).SequenceEqual(parameters.Select(p=>p.FullName)))
        throw new InvalidOperationException("Helper signature differs: " + name);
    return new MemberRefUser(module,name,MethodSig.CreateStatic(module.CorLibTypes.Void,parameters),helper);
}
var callback = module.GetTypes().Single(t=>t.Name == "<DoOpen>c__AnonStorey189" && t.DeclaringType?.FullName == "UI.AnnouncePage").Methods.Single(m=>m.Name == "<>m__E8");
void ReplaceWebview(MethodDef target, string name, bool owner) {
    var call = target.Body.Instructions.Single(i => i.Operand is IMethod method && method.DeclaringType.FullName == "supersdkUnity.YunchangPlugin" && method.Name == "OpenWebview");
    var original = (IMethod)call.Operand;
    if(original.MethodSig.Params.Count != 6 || original.MethodSig.RetType.ElementType != ElementType.Void || original.MethodSig.Params[0].ElementType != ElementType.String || original.MethodSig.Params.Skip(1).Any(p=>p.ElementType != ElementType.I4)) throw new InvalidOperationException("Original notice webview ABI differs");
    var parameters = original.MethodSig.Params.ToList();
    if(owner) {
        parameters.Add(T("SystemAnnouncePage").ToTypeSig());
        target.Body.Instructions.Insert(target.Body.Instructions.IndexOf(call),Instruction.Create(OpCodes.Ldarg_0));
    }
    call.OpCode = OpCodes.Call;
    call.Operand = B(name,parameters.ToArray());
    target.Body.MaxStack = Math.Max(target.Body.MaxStack,(ushort)8);
    target.Body.KeepOldMaxStack = true;
}
ReplaceWebview(callback,"ShowAnnouncement",false);
ReplaceWebview(M("SystemAnnouncePage","Show"),"ShowSystem",true);
foreach(var item in new[]{("UI.AnnouncePage","DoClose","HideAnnouncement"),("SystemAnnouncePage","Close","HideSystem")}) {
    var method = M(item.Item1,item.Item2);
    method.Body.Instructions.Insert(0,Instruction.Create(OpCodes.Ldarg_0));
    method.Body.Instructions.Insert(1,Instruction.Create(OpCodes.Call,B(item.Item3,T(item.Item1).ToTypeSig())));
    method.Body.MaxStack = Math.Max(method.Body.MaxStack,(ushort)1);
    method.Body.KeepOldMaxStack = true;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2],options);
Console.WriteLine("Original announcement webview calls replaced by game Words-font content inside original frames");
