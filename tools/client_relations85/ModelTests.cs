using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using dnlib.DotNet;
using dnlib.DotNet.Emit;

// Execute the emitted selection, callback, circle-refresh, assist guard and
// original assist-send CIL against small Unity/network stand-ins. These tests
// do not execute the Unity player or make a network request.
class Hero { public string id, guid; public Hero(string id, string guid) { this.id=id; this.guid=guid; } }
class Camp { public Hero hero; }
class Label { public string Text = "old"; }
class Button { public bool Active = true; }
class Panel { public bool Active = true; public string Center; }
class Page {
    public int Index; public string Guid = "old"; public string Detail = "old";
    public Label Name = new(), Count = new(), Total = new();
    public Label[] HelpNames = Enumerable.Range(0, 6).Select(_ => new Label()).ToArray();
    public Button[] Adds = Enumerable.Range(0, 6).Select(_ => new Button()).ToArray();
    public Button[] Masks = Enumerable.Range(0, 5).Select(_ => new Button()).ToArray();
    public Panel Panel = new(); public int ClearCalls, StopCalls, RefreshCalls;
}
class Capture { public Page Page; public string Guid; }
class Pair { public Camp Value; }
class Iter { public List<Camp> Values; public int Position = -1; }
class RefLocal { public int Index; public RefLocal(int i) { Index=i; } }
class Icon { public string ArtHeroId; }
class Replace { public int Position; }
class HelperCapture { public string Guid; public Replace Page; }
class HelperRequest { public string Guid; public int Pos; }
class World {
    public Dictionary<int,Camp> Camps = new(); public int SentCommand; public HelperRequest Sent;
    public int DeniedHelp, AllowedHelp;
    public World(params (int slot,string id,string guid)[] occupied) {
        for (int n=1;n<=5;n++) Camps.Add(n,new Camp());
        foreach (var p in occupied) Camps[p.slot].hero = new Hero(p.id,p.guid);
    }
}

class Machine {
    readonly ModuleDefMD module;
    readonly World world;
    readonly MethodDef update, init;
    public Machine(ModuleDefMD module, World world) {
        this.module=module; this.world=world;
        update=Method("UI.UILuck","UpdateCirclePanel"); init=Method("UI.UILuck","Init");
    }
    public MethodDef Method(string owner,string name) => module.GetTypes().Single(t=>t.FullName==owner).Methods.Single(m=>m.Name==name);
    static bool Call(Instruction i,string owner,string name) => i.Operand is IMethod m && m.DeclaringType.FullName==owner && m.Name==name;
    static bool Field(Instruction i,string owner,string name) => i.Operand is IField f && f.DeclaringType.FullName==owner && f.Name==name;
    static int Int(object v) => Convert.ToInt32(v);
    static bool Truth(object v) => v != null && (!(v is int n) || n!=0) && (!(v is bool b) || b);
    static object Deref(object value, object[] locals) => value is RefLocal r ? locals[r.Index] : value;
    static int Index(Instruction i) => ((Local)i.Operand).Index;
    static object GetField(object target,string name) {
        if (target == null) throw new NullReferenceException("CIL field read: " + name);
        if (target is Camp c && name=="hero") return c.hero;
        if (target is Hero h && name=="guid") return h.guid;
        if (target is Hero h2 && name=="heroId") return h2.id;
        if (target is Page p) return name switch {
            "currIndex" => p.Index, "guid" => p.Guid, "panel" => p.Panel,
            "luckHeroName" => p.Name, "luckCount" => p.Count, "luckTotalCount" => p.Total,
            "helpNamePos" => p.HelpNames, "sixAdds" => p.Adds,
            "mask0" => p.Masks[0], "mask1" => p.Masks[1], "mask2" => p.Masks[2],
            "mask3" => p.Masks[3], "mask4" => p.Masks[4], _ => throw new Exception("unmodeled page field " + name)
        };
        if (target is Capture cap) return name switch { "heroId" => cap.Guid, "<>f__this" => cap.Page, _ => throw new Exception("capture field " + name) };
        if (target is HelperCapture hc) return name switch { "guid" => hc.Guid, "<>f__this" => hc.Page, _ => throw new Exception("helper capture field " + name) };
        if (target is Replace repl && name=="positionIndex") return repl.Position;
        throw new Exception("unmodeled field " + name + " on " + target.GetType().Name);
    }
    static void SetField(object target,string name,object value) {
        if (target is Page p) { if(name=="currIndex") p.Index=Int(value); else if(name=="guid") p.Guid=(string)value; else throw new Exception("page set " + name); return; }
        if (target is HelperCapture h) { if(name=="guid") h.Guid=(string)value; else if(name=="<>f__this") h.Page=(Replace)value; else throw new Exception("capture set " + name); return; }
        throw new Exception("unmodeled field write " + name);
    }
    static bool IsOriginalSelectionStart(IList<Instruction> code,int n) =>
        n+5<code.Count && Field(code[n],"StaticData","playerData") && Call(code[n+1],"UI.UIPlayerData","get_InstPlayerData") &&
        Call(code[n+2],"UI.UIInstPlayerData","get_BattleCamps") && code[n+3].OpCode.Code==Code.Ldarg_0 &&
        Field(code[n+4],"UI.UILuck","currIndex") && code[n+5].Operand is IMethod m && m.Name=="get_Item";
    public void Resolve(Page page) {
        var c=init.Body.Instructions;
        int start=Enumerable.Range(0,c.Count-3).Single(n=>c[n].OpCode.Code==Code.Ldarg_0 && Field(c[n+1],"UI.UILuck","currIndex") &&
            c[n+2].OpCode.Code==Code.Ldc_I4_1 && c[n+3].OpCode.Code==Code.Blt);
        Run(init,page,null,start,stopAfterCenter:true);
    }
    public void Click(Page page,string guid) {
        var type=module.GetTypes().Single(t=>t.DeclaringType?.FullName=="UI.UILuck" && t.Name=="<SetInBattleHeroIcon>c__AnonStorey1A3");
        Run(type.Methods.Single(m=>m.Name=="<>m__20B"),new Capture { Page=page, Guid=guid },null);
    }
    public void Circle(Page page) => Run(update,page,null);
    public bool HelpGate() {
        var method=Method("UI.UILuckEvent","OpenHelpPage");
        return Run(method,new object(),null,stopAtOriginalHelp:true);
    }
    public string ArtId(string guid) {
        var method=Method("UI.UILuck","SetInBattleHeroIcon"); var c=method.Body.Instructions;
        int call=c.Select((x,n)=>(x,n)).Single(x=>Call(x.x,"UI.HeroIcon","SetHeroWithId")).n;
        int start=call-6;
        if(c[start].OpCode.Code!=Code.Ldloc_1) throw new Exception("icon display start differs");
        var icon=new Icon(); object[] initial=new object[method.Body.Variables.Count];
        initial[1]=icon; initial[2]=new Capture { Guid=guid };
        Run(method,new Page(),null,start,stopAfterIcon:true,initial:initial);
        return icon.ArtHeroId;
    }
    public void SendHelp(int slot,string guid) => Run(Method("UI.HeroReplacePage","OnHelpBattle"),new Replace { Position=slot },guid);
    public void AssertStructure() {
        var c=init.Body.Instructions;
        int draw=c.Select((x,n)=>(x,n)).Single(x=>Call(x.x,"UI.UILuck","SetInBattleHeroIcon")).n;
        if(!c.Skip(draw-12).Take(12).Any(i=>Field(i,"UI.UIHero","guid"))) throw new Exception("paint must capture GUID");
        var original=Method("UI.UILuck","SetCenterHero").Body.Instructions;
        if(!original.Any(i=>Field(i,"UI.UIHero","guid")) || !original.Any(i=>Call(i,"UI.UILuck","UpdateCirclePanel")))
            throw new Exception("center target/refresh contract changed");
        if(!c.Select((x,n)=>n).Any(n=>IsOriginalSelectionStart(c,n))) throw new Exception("original slot access missing");
        var help=Method("UI.UILuckEvent","OpenHelpPage").Body.Instructions;
        int opening=help.Select((x,n)=>(x,n)).Single(x=>Call(x.x,"UI.PageManager","OpenPage")).n;
        int eligible=help.Select((x,n)=>(x,n)).Single(x=>Call(x.x,"CommonMethod","GetModuleOpen")).n;
        if(eligible>=opening) throw new Exception("help eligibility order changed");
    }
    bool Run(MethodDef method,object arg0,object arg1,int start=0,bool stopAfterCenter=false,
             bool stopAtOriginalHelp=false,bool stopAfterIcon=false,object[] initial=null) {
        var code=method.Body.Instructions; var target=code.Select((x,n)=>(x,n)).ToDictionary(x=>x.x,x=>x.n);
        var locals=initial ?? new object[method.Body.Variables.Count]; var stack=new Stack<object>(); int pc=start;
        object Pop() => stack.Pop(); void Push(object value) => stack.Push(value);
        for (int steps=0;steps<3000;steps++) {
            var i=code[pc++];
            if(stopAtOriginalHelp && i.OpCode.Code==Code.Ldstr && (string)i.Operand=="array_help_pos") return true;
            switch(i.OpCode.Code) {
                case Code.Nop: break;
                case Code.Ldarg_0: Push(arg0); break; case Code.Ldarg_1: Push(arg1); break;
                case Code.Ldnull: Push(null); break; case Code.Ldstr: Push((string)i.Operand); break;
                case Code.Ldc_I4_0: Push(0); break; case Code.Ldc_I4_1: Push(1); break; case Code.Ldc_I4_2: Push(2); break;
                case Code.Ldc_I4_3: Push(3); break; case Code.Ldc_I4_4: Push(4); break; case Code.Ldc_I4_5: Push(5); break;
                case Code.Ldc_I4: Push((int)i.Operand); break; case Code.Ldc_I4_S: Push(Convert.ToInt32(i.Operand)); break;
                case Code.Ldloc_0: Push(locals[0]); break; case Code.Ldloc_1: Push(locals[1]); break;
                case Code.Ldloc_2: Push(locals[2]); break; case Code.Ldloc_3: Push(locals[3]); break;
                case Code.Ldloc: case Code.Ldloc_S: Push(locals[Index(i)]); break;
                case Code.Stloc_0: locals[0]=Pop(); break; case Code.Stloc_1: locals[1]=Pop(); break;
                case Code.Stloc_2: locals[2]=Pop(); break; case Code.Stloc_3: locals[3]=Pop(); break;
                case Code.Stloc: case Code.Stloc_S: locals[Index(i)]=Pop(); break;
                case Code.Ldloca: case Code.Ldloca_S: Push(new RefLocal(Index(i))); break;
                case Code.Ldsfld: if(((IField)i.Operand).Name!="playerData") throw new Exception("static field"); Push(world); break;
                case Code.Ldfld: Push(GetField(Pop(),((IField)i.Operand).Name)); break;
                case Code.Stfld: { var value=Pop(); SetField(Pop(),((IField)i.Operand).Name,value); break; }
                case Code.Ldelem_Ref: { int n=Int(Pop()); Push(((Array)Pop()).GetValue(n)); break; }
                case Code.Br: case Code.Br_S: case Code.Leave: case Code.Leave_S: pc=target[(Instruction)i.Operand]; break;
                case Code.Brfalse: case Code.Brfalse_S: if(!Truth(Pop())) pc=target[(Instruction)i.Operand]; break;
                case Code.Brtrue: case Code.Brtrue_S: if(Truth(Pop())) pc=target[(Instruction)i.Operand]; break;
                case Code.Blt: case Code.Blt_S: { int b=Int(Pop()),a=Int(Pop()); if(a<b) pc=target[(Instruction)i.Operand]; break; }
                case Code.Bgt: case Code.Bgt_S: { int b=Int(Pop()),a=Int(Pop()); if(a>b) pc=target[(Instruction)i.Operand]; break; }
                case Code.Box: break;
                case Code.Pop: Pop(); break;
                case Code.Ldftn: Push(i.Operand); break;
                case Code.Newobj: {
                    var m=(IMethod)i.Operand; string n=m.DeclaringType.Name;
                    if(n=="<OnHelpBattle>c__AnonStorey185") Push(new HelperCapture());
                    else if(n=="CSHelperOn") Push(new HelperRequest());
                    else if(n=="HttpHandler") { Pop(); Pop(); Push(new object()); }
                    else throw new Exception("unmodeled constructor " + m.FullName); break;
                }
                case Code.Call: case Code.Callvirt: {
                    var m=(IMethod)i.Operand; string n=m.Name, owner=m.DeclaringType.FullName;
                    if(n=="get_InstPlayerData" || n=="get_BattleCamps") { Pop(); Push(world); }
                    else if(n=="get_Item") { int slot=Int(Pop()); Pop(); Push(world.Camps[slot]); }
                    else if(n=="GetEnumerator") { Pop(); Push(new Iter { Values=world.Camps.OrderBy(x=>x.Key).Select(x=>x.Value).ToList() }); }
                    else if(n=="get_Current") { var it=(Iter)Deref(Pop(),locals); Push(new Pair { Value=it.Values[it.Position] }); }
                    else if(n=="get_Value") Push(((Pair)Deref(Pop(),locals)).Value);
                    else if(n=="MoveNext") { var it=(Iter)Deref(Pop(),locals); it.Position++; Push(it.Position<it.Values.Count); }
                    else if(n=="IsNullOrEmpty") Push(string.IsNullOrEmpty((string)Pop()));
                    else if(n=="op_Equality") { var b=(string)Pop(); var a=(string)Pop(); Push(a==b); }
                    else if(n=="get_gameObject") { var panel=(Panel)Pop(); Push(panel); }
                    else if(n=="SetActive") { bool v=Truth(Pop()); var obj=Pop(); if(obj is Panel panel) panel.Active=v; else ((Button)obj).Active=v; }
                    else if(n=="set_text") { var value=(string)Pop(); ((Label)Pop()).Text=value; }
                    else if(n=="StopAllCoroutines") ((Page)Pop()).StopCalls++;
                    else if(n=="Uninit") { var p=(Page)Pop(); p.ClearCalls++; p.Index=1; p.Detail=""; }
                    else if(owner=="UI.UILuckCirclePanel" && n=="Init") { var value=(string)Pop(); ((Panel)Pop()).Center=value; }
                    else if(n=="UpdateLuckCount") { var p=(Page)Pop(); p.Detail=p.Guid; p.RefreshCalls++; }
                    else if(n=="SetCenterHero") { var f=(Camp)Pop(); var p=(Page)Pop(); p.Guid=f.hero.guid; Circle(p); if(stopAfterCenter) return true; }
                    else if(owner=="UI.UILuck" && n=="Init") { Pop(); var p=(Page)Pop(); Resolve(p); }
                    else if(n=="GetUIHero") { string g=(string)Pop(); Pop(); Push(world.Camps.Values.Select(c=>c.hero).FirstOrDefault(h=>h!=null && h.guid==g)); }
                    else if(n=="SetHeroWithId") { var id=(string)Pop(); ((Icon)Pop()).ArtHeroId=id; if(stopAfterIcon) return true; }
                    else if(n=="set_guid") { var g=(string)Pop(); ((HelperRequest)Pop()).Guid=g; }
                    else if(n=="set_pos") { var pos=Int(Pop()); ((HelperRequest)Pop()).Pos=pos; }
                    else if(n=="SendHttp") { Pop(); var req=(HelperRequest)Pop(); int command=Int(Pop()); world.Sent=req; world.SentCommand=command; Push(null); }
                    else throw new Exception("unmodeled call " + m.FullName);
                    break;
                }
                case Code.Ret: return false;
                default: throw new Exception("unmodeled opcode " + i.OpCode + " in " + method.FullName);
            }
        }
        throw new Exception("CIL step limit exceeded in " + method.FullName);
    }
}

static class Program {
    static int assertions;
    static void Check(bool value,string message) { assertions++; if(!value) throw new Exception(message); }
    static void Selection(ModuleDefMD module,World w,int wanted,int expected,string guid) {
        var m=new Machine(module,w); var p=new Page { Index=wanted }; m.Resolve(p);
        Check(p.Index==expected && p.Guid==guid && p.Detail==guid && p.Panel.Center==guid && p.Panel.Active,
              "initial slot/GUID/detail mismatch");
        Check(m.HelpGate(), "help guard rejected a valid formation");
    }
    static void VerifyStacks(ModuleDefMD module) {
        var type=typeof(ModuleDefMD).Assembly.GetType("dnlib.DotNet.Writer.MaxStackCalculator",true);
        var calc=type.GetMethods(BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.Static)
            .Single(x=>x.Name=="GetMaxStack" && x.GetParameters().Length==3);
        var targets=new[] {
            module.GetTypes().Single(t=>t.FullName=="UI.UILuck").Methods.Single(m=>m.Name=="Init"),
            module.GetTypes().Single(t=>t.FullName=="UI.UILuck").Methods.Single(m=>m.Name=="SetInBattleHeroIcon"),
            module.GetTypes().Single(t=>t.FullName=="UI.UILuck").Methods.Single(m=>m.Name=="UpdateCirclePanel"),
            module.GetTypes().Single(t=>t.FullName=="UI.UILuckEvent").Methods.Single(m=>m.Name=="OpenHelpPage"),
            module.GetTypes().Single(t=>t.DeclaringType?.FullName=="UI.UILuck" && t.Name=="<SetInBattleHeroIcon>c__AnonStorey1A3")
                .Methods.Single(m=>m.Name=="<>m__20B")
        };
        foreach(var method in targets) {
            object[] arguments={method.Body.Instructions,method.Body.ExceptionHandlers,(uint)0};
            bool valid=(bool)calc.Invoke(null,arguments); uint peak=(uint)arguments[2];
            Check(valid && peak<=method.Body.MaxStack,"invalid stack or MaxStack in " + method.FullName + ": " + peak);
        }
    }
    public static void Main(string[] args) {
        if(args.Length!=2) throw new ArgumentException("baseline and patched mains required");
        using var baseline=ModuleDefMD.Load(args[0]); using var patched=ModuleDefMD.Load(args[1]);
        VerifyStacks(patched);
        var sparse=new World((1,"hero_a","guid_a"),(3,"hero_b","guid_b"));
        var original=new Machine(baseline,sparse); bool threw=false;
        try { original.Circle(new Page { Index=3, Guid="guid_b" }); } catch(NullReferenceException) { threw=true; }
        Check(threw,"original sparse-slot null dereference was not reproduced");
        var m=new Machine(patched,sparse); m.AssertStructure();
        Selection(patched,sparse,1,1,"guid_a"); Selection(patched,sparse,3,3,"guid_b");
        Selection(patched,sparse,2,1,"guid_a");
        Selection(patched,new World((1,"hero_a","guid_a"),(5,"hero_e","guid_e")),5,5,"guid_e");
        Selection(patched,new World((3,"hero_c","guid_c")),1,3,"guid_c");
        Selection(patched,new World((3,"hero_c","guid_c")),0,3,"guid_c");
        Selection(patched,new World((3,"hero_c","guid_c")),6,3,"guid_c");
        Selection(patched,new World((1,"hero_a","guid_a"),(2,"hero_b","guid_b")),2,2,"guid_b");
        Selection(patched,new World((1,"a","1"),(2,"b","2"),(3,"c","3"),(4,"d","4"),(5,"e","5")),5,5,"5");
        var p=new Page { Index=1, Guid="guid_a" }; m.Click(p,"guid_b");
        Check(p.Index==3 && p.Guid=="guid_b" && p.Detail=="guid_b" && p.Panel.Center=="guid_b", "click across hole chose old target");
        // Returning after cancelling a slot change resolves the still-occupied
        // selection again, without inventing a new formation or stale GUID.
        m.Resolve(p); Check(p.Index==3 && p.Guid=="guid_b" && p.Detail=="guid_b", "return after cancel lost the valid target");
        Check(m.ArtId("guid_b")=="hero_b", "GUID-bound icon art changed");
        var duplicates=new World((1,"same_hero","instance_a"),(3,"same_hero","instance_b"));
        var dm=new Machine(patched,duplicates); var dp=new Page { Index=1, Guid="instance_a" };
        dm.Click(dp,"instance_b"); Check(dp.Index==3 && dp.Guid=="instance_b" && dp.Detail=="instance_b", "duplicate hero ID selected wrong GUID");
        Check(dm.ArtId("instance_b")=="same_hero", "duplicate hero ID art mismatch");
        dm.SendHelp(4,"helper_guid_selected");
        Check(duplicates.SentCommand==21109 && duplicates.Sent.Guid=="helper_guid_selected" && duplicates.Sent.Pos==4,
              "original assist request did not retain selected helper GUID and slot");
        duplicates.Camps[3].hero=null; dm.Click(dp,"instance_b");
        Check(dp.Index==1 && dp.Guid=="instance_a" && dp.Detail=="instance_a", "removed selection did not refresh to current hero");
        dp.Index=3; dp.Guid="instance_b"; dp.Detail="instance_b"; dm.Resolve(dp);
        Check(dp.Index==1 && dp.Guid=="instance_a" && dp.Detail=="instance_a", "reopen kept a deleted target");
        duplicates.Camps[1].hero=null; dm.Click(dp,"instance_a");
        Check(dp.Index==0 && dp.Guid==null && dp.Detail=="" && !dp.Panel.Active && dp.ClearCalls==1 && dp.StopCalls==1,
              "all-empty stale detail was not cleared");
        Check(dp.Name.Text=="" && dp.Count.Text=="" && dp.Total.Text=="" && dp.HelpNames.All(x=>x.Text=="") &&
              dp.Adds.All(x=>!x.Active) && dp.Masks.All(x=>!x.Active), "all-empty controls retained stale state");
        Check(!dm.HelpGate(), "all-empty formation permitted help action");
        var emptyGuid=new World((3,"hero_c","")); Check(!new Machine(patched,emptyGuid).HelpGate(), "empty GUID permitted help action");
        bool propagated=false; var malformed=new World((1,"a","1")); malformed.Camps.Remove(2);
        try { new Machine(patched,malformed).Resolve(new Page { Index=2 }); } catch(KeyNotFoundException) { propagated=true; }
        Check(propagated,"unrelated data failure was swallowed");
        Console.WriteLine("PASS " + assertions + " checks: actual emitted CIL; sparse 1/3 and 1/5, first empty, continuous, full, duplicate ID, stale/empty and helper GUID/position");
    }
}
