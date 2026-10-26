using dnlib.DotNet;
using dnlib.DotNet.Emit;
using System.Text.Json;

static class RecoveryProof {
 public static string[] Verify(string before,string after) {
  using var a=ModuleDefMD.Load(before);using var b=ModuleDefMD.Load(after);
  var left=a.GetTypes().ToArray();var right=b.GetTypes().ToArray();
  if(left.Length!=right.Length)throw new InvalidOperationException("Recovery types changed");
  var changed=new List<string>();
  for(int n=0;n<left.Length;n++) {
   var x=left[n];var y=right[n];
   if(x.FullName!=y.FullName||x.Fields.Count!=y.Fields.Count||x.Methods.Count!=y.Methods.Count)throw new InvalidOperationException("Recovery members changed");
   for(int f=0;f<x.Fields.Count;f++)if(x.Fields[f].FullName!=y.Fields[f].FullName||x.Fields[f].Attributes!=y.Fields[f].Attributes)throw new InvalidOperationException("Recovery field changed");
   for(int m=0;m<x.Methods.Count;m++) {
    var u=x.Methods[m];var v=y.Methods[m];
    if(u.FullName!=v.FullName||u.Attributes!=v.Attributes||u.ImplAttributes!=v.ImplAttributes)throw new InvalidOperationException("Recovery method ABI changed");
    if(Body(u)!=Body(v))changed.Add(x.FullName+"::"+u.Name);
   }
  }
  return changed.ToArray();
 }
 static string Body(MethodDef method) {
  if(!method.HasBody)return "";var body=method.Body;var index=body.Instructions.Select((v,n)=>(v,n)).ToDictionary(p=>p.v,p=>p.n);
  string Point(Instruction? value)=>value==null?"end":index[value].ToString();
  string Value(object? value)=>value switch {
   null=>"null",Instruction i=>Point(i),IList<Instruction> items=>String.Join(",",items.Select(Point)),
   IMethod m=>m.FullName,IField f=>f.FullName,ITypeDefOrRef t=>t.FullName,
   MethodSig s=>s.ToString(),Local l=>"local:"+l.Index,Parameter p=>"arg:"+p.Index,
   _=>JsonSerializer.Serialize(value)
  };
  return JsonSerializer.Serialize(new {
   body.InitLocals,locals=body.Variables.Select(v=>v.Type.FullName),
   instructions=body.Instructions.Select(i=>new {op=i.OpCode.Name,arg=Value(i.Operand)}),
   handlers=body.ExceptionHandlers.Select(h=>new {h.HandlerType,tryStart=Point(h.TryStart),tryEnd=Point(h.TryEnd),handlerStart=Point(h.HandlerStart),handlerEnd=Point(h.HandlerEnd),filter=Point(h.FilterStart),type=h.CatchType?.FullName})
  });
 }
}
