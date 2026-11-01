using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Text;
using System.Text.RegularExpressions;
using dnlib.DotNet;
using dnlib.DotNet.Emit;

// Compare decoded IL by resolved member names and branch targets; metadata row
// numbers may move when the narrow helper gains private implementation types.
static class BridgeScopeChecks
{
    static bool candidate;
    static string Normalize(string value)
    {
        // Roslyn numbers generated closures using member ordinals. The exact
        // mappings below are bound to the pinned shipped helper and candidate.
        string[] before={"73_0","76_0","84_0","86_0","87_0"};
        string[] after={"76_0","79_0","87_0","89_0","90_0"};
        string[] names={"Snapshot","OpenPlayer","Protection","ReleaseProtection","RetryDelivery"};
        string[] ids=candidate ? after : before;
        for (int i=0;i<ids.Length;i++) value=value.Replace("FieldRuntime/<>c__DisplayClass"+ids[i],"FieldRuntime/<Closure"+names[i]+">");
        value=value.Replace("TerminalController/<>c__DisplayClass"+(candidate ? "23_0" : "21_0"),"TerminalController/<ClosureSend>");
        value=Regex.Replace(value,@"(<Open>b__)\d+_(\d+)","$1ordinal_$2");
        return value;
    }
    static IEnumerable<TypeDef> Types(ModuleDefMD module) { return module.GetTypes().Where(t=>t.FullName!="<Module>"); }
    static string Operand(object value,Dictionary<Instruction,int> offsets)
    {
        if (value==null) return "";
        var instruction=value as Instruction;
        if (instruction!=null) return "IL#"+offsets[instruction];
        var targets=value as IList<Instruction>;
        if (targets!=null) return "["+String.Join(",",targets.Select(t=>offsets[t].ToString()).ToArray())+"]";
        var local=value as Local; if (local!=null) return "local#"+local.Index;
        var parameter=value as Parameter; if (parameter!=null) return "arg#"+parameter.Index;
        var member=value as IFullName; if (member!=null) return Normalize(member.FullName);
        var number=value as IFormattable; if (number!=null) return number.ToString(null,CultureInfo.InvariantCulture);
        return value.ToString();
    }
    static string Body(MethodDef method)
    {
        if (!method.HasBody) return "<no body>";
        var body=method.Body;
        var positions=body.Instructions.Select((v,i)=>new {v,i}).ToDictionary(p=>p.v,p=>p.i);
        Func<Instruction,string> index=i=>i==null ? "end" : positions[i].ToString();
        var text=new StringBuilder();
        text.Append("init=").Append(body.InitLocals).Append(";stack=").Append(body.MaxStack).Append(";locals=");
        foreach (var local in body.Variables) text.Append(Normalize(local.Type.FullName)).Append(';');
        foreach (var item in body.Instructions) text.Append(item.OpCode.Name).Append(':').Append(Operand(item.Operand,positions)).Append('|');
        foreach (var e in body.ExceptionHandlers)
            text.Append("eh:").Append(e.HandlerType).Append(':').Append(index(e.TryStart)).Append(':').Append(index(e.TryEnd))
                .Append(':').Append(index(e.HandlerStart)).Append(':').Append(index(e.HandlerEnd)).Append(':').Append(index(e.FilterStart))
                .Append(':').Append(e.CatchType==null ? "" : Normalize(e.CatchType.FullName)).Append('|');
        return text.ToString();
    }
    static SortedSet<string> API(ModuleDefMD module)
    {
        var result=new SortedSet<string>();
        foreach (var t in Types(module).Where(t=>t.IsPublic))
        {
            result.Add("type "+t.FullName+" "+(int)t.Attributes+" "+(t.BaseType==null ? "" : t.BaseType.FullName));
            foreach (var m in t.Methods.Where(m=>m.IsPublic)) result.Add("method "+m.FullName+" "+(int)m.Attributes);
            foreach (var f in t.Fields.Where(f=>f.IsPublic)) result.Add("field "+f.FullName+" "+(int)f.Attributes);
        }
        return result;
    }
    public static void Main(string[] args)
    {
        var before=ModuleDefMD.Load(args[0]); var after=ModuleDefMD.Load(args[1]);
        if (before.Assembly.FullName!=after.Assembly.FullName) throw new Exception("Helper assembly identity changed");
        var oldApi=API(before); var newApi=API(after);
        var removedApi=oldApi.Except(newApi).ToArray();
        var extraApi=newApi.Except(oldApi).ToArray();
        if (removedApi.Length!=0 || extraApi.Any(a=>!a.StartsWith("type HYJS.DesertV4.ExpeditionHeroList ") &&
            !a.StartsWith("method System.Boolean HYJS.DesertV4.ExpeditionHeroList::Send(")) || extraApi.Length!=2)
        {
            foreach (string a in oldApi.Except(newApi)) Console.WriteLine("removed API "+a);
            foreach (string a in newApi.Except(oldApi)) Console.WriteLine("added API "+a);
            throw new Exception("Public helper API differs");
        }
        candidate=false;
        var oldMethods=Types(before).SelectMany(t=>t.Methods).ToDictionary(m=>Normalize(m.FullName));
        candidate=true;
        var newMethods=Types(after).SelectMany(t=>t.Methods).ToDictionary(m=>Normalize(m.FullName));
        Func<string,bool> scoped=key=>key.Contains("HYJS.DesertV4.FieldRuntime::Update(") ||
            key.Contains("HYJS.DesertV4.FieldRuntime::ReleaseScene(System.Boolean)") || key.Contains("HYJS.DesertV4.FieldRuntime::StartEncounter(") ||
            key.Contains("HYJS.DesertV4.FieldRuntime/<>c__DisplayClass80_") || key.Contains("HYJS.DesertV4.FieldRuntime/<>c__DisplayClass83_") ||
            key.Contains("HYJS.DesertV4.TerminalController::.ctor(") || key.Contains("HYJS.DesertV4.TerminalController::ApplySnapshot(") ||
            key.Contains("HYJS.DesertV4.TerminalController::Open(") || key.Contains("HYJS.DesertV4.FieldMonsters::HasAvailable(") ||
            key.Contains("HYJS.DesertV4.TerminalCreditQuote") || key.Contains("HYJS.DesertV4.FieldMonsterPosition") ||
            key.Contains("HYJS.DesertV4.ExpeditionHeroList");
        int unchanged=0,approved=0; var problems=new List<string>();
        foreach (var pair in oldMethods)
        {
            MethodDef other;
            if (!newMethods.TryGetValue(pair.Key,out other))
            { if (scoped(pair.Key)) approved++; else problems.Add("removed "+pair.Key); }
            else
            {
                candidate=false; string original=Body(pair.Value);
                candidate=true; string rebuilt=Body(other);
                if (pair.Value.Attributes!=other.Attributes || original!=rebuilt)
                { if (scoped(pair.Key)) approved++; else problems.Add("changed "+pair.Key); }
                else unchanged++;
            }
        }
        foreach (var pair in newMethods) if (!oldMethods.ContainsKey(pair.Key))
        { if (scoped(pair.Key)) approved++; else problems.Add("added "+pair.Key); }
        foreach (string problem in problems) Console.WriteLine(problem);
        if (problems.Count>0) throw new Exception("Unexpected helper method changes");
        var oldResources=before.Resources.OfType<EmbeddedResource>().ToDictionary(r=>r.Name.ToString());
        var newResources=after.Resources.OfType<EmbeddedResource>().ToDictionary(r=>r.Name.ToString());
        if (!oldResources.Keys.OrderBy(x=>x).SequenceEqual(newResources.Keys.OrderBy(x=>x))) throw new Exception("Resource inventory changed");
        foreach (var pair in oldResources)
            if (pair.Value.Attributes!=newResources[pair.Key].Attributes || !pair.Value.CreateReader().ToArray().SequenceEqual(newResources[pair.Key].CreateReader().ToArray()))
                throw new Exception("Helper resource changed: "+pair.Key);
        var encounter=Types(after).Single(t=>t.FullName=="HYJS.DesertV4.FieldRuntime").Methods.Single(m=>m.Name=="StartEncounter");
        var calls=encounter.Body.Instructions.Where(i=>i.OpCode.Code==Code.Call || i.OpCode.Code==Code.Callvirt)
            .Select(i=>i.Operand as IMethod).Where(m=>m!=null).ToArray();
        int loading=Array.FindIndex(calls,m=>m.Name=="ShowLoadingPre"), monster=Array.FindIndex(calls,m=>m.DeclaringType.FullName=="HYJS.DesertV4.FieldMonsterPosition" && m.Name=="Begin"),
            boss=Array.FindIndex(calls,m=>m.Name=="ConfirmPosition");
        if (loading<0 || monster<=loading || boss<=loading || calls.Count(m=>m.Name=="ShowLoadingPre")!=1)
            throw new Exception("Native loading must begin before either position confirmation");
        Console.WriteLine("PASS public ABI entries="+oldApi.Count+" unchanged methods="+unchanged+" scoped differences="+approved+" embedded resources="+oldResources.Count);
        Console.WriteLine("PASS native loading starts before monster and boss position confirmation");
    }
}
