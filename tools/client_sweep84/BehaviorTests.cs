using System;
using System.Collections.Generic;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using HYJS.Stamina;

// This executes the actual emitted iterator instruction stream with modeled
// Unity fields and calls. It also tests the unchanged StaminaSweep source with
// explicit engine/network stubs. It does not run Unity, a device, or a server.
public sealed class ArenaPage
{
    public int sweepCount, sweepTotalCount, sends;
    public Button confirmButton = new Button();
}
public sealed class Iter
{
    public int PC, current;
    public object page, wait;
}
public static class Program
{
    static void Check(bool x, string m) { if (!x) throw new Exception(m); }
    static int Integer(object x) => Convert.ToInt32(x);
    static bool True(object x) => x is bool b ? b : Integer(x) != 0;
    static object Field(object target, string name) => name switch {
        "$PC" => ((Iter)target).PC,
        "$current" => ((Iter)target).wait,
        "<>f__this" => ((Iter)target).page,
        "current" => ((Iter)target).current,
        "sweepCount" => ((ArenaPage)target).sweepCount,
        "sweepTotalCount" => ((ArenaPage)target).sweepTotalCount,
        "confirmButton" => ((ArenaPage)target).confirmButton,
        "needStop" => ((UI.UICopySweep)target).needStop,
        "count" => ((UI.UICopySweep)target).count,
        "titleAlpha" => ((UI.UICopySweep)target).titleAlpha,
        "bodyScale" => ((UI.UICopySweep)target).bodyScale,
        "duration" => ((Tween)target).duration,
        _ => throw new Exception("unmodeled field " + name)
    };
    static bool Move(MethodDef method, Iter iterator)
    {
        var body = method.Body.Instructions;
        var index = body.Select((x, i) => (x, i)).ToDictionary(x => x.x, x => x.i);
        var stack = new Stack<object>();
        object[] local = new object[method.Body.Variables.Count];
        int pc = 0;
        for (int step = 0; step < 500; step++)
        {
            var i = body[pc++];
            switch (i.OpCode.Code)
            {
                case Code.Ldarg_0: stack.Push(iterator); break;
                case Code.Ldloc_0: stack.Push(local[0]); break;
                case Code.Ldloc_1: stack.Push(local[1]); break;
                case Code.Stloc_0: local[0] = stack.Pop(); break;
                case Code.Stloc_1: local[1] = stack.Pop(); break;
                case Code.Ldc_I4_M1: stack.Push(-1); break;
                case Code.Ldc_I4_0: stack.Push(0); break;
                case Code.Ldc_I4_1: stack.Push(1); break;
                case Code.Ldc_R4: stack.Push((float)i.Operand); break;
                case Code.Pop: stack.Pop(); break;
                case Code.Add: { float b = Convert.ToSingle(stack.Pop()), a = Convert.ToSingle(stack.Pop()); stack.Push(a + b); break; }
                case Code.Ldfld: stack.Push(Field(stack.Pop(), ((IField)i.Operand).Name)); break;
                case Code.Stfld:
                {
                    object value = stack.Pop(), target = stack.Pop();
                    string field = ((IField)i.Operand).Name;
                    if (field == "$PC") ((Iter)target).PC = Integer(value);
                    else if (field == "$current") ((Iter)target).wait = value;
                    else throw new Exception("unmodeled stfld " + field);
                    break;
                }
                case Code.Switch:
                {
                    int value = Integer(stack.Pop()); var targets = (Instruction[])i.Operand;
                    if (value >= 0 && value < targets.Length) pc = index[targets[value]];
                    break;
                }
                case Code.Br: case Code.Br_S: pc = index[(Instruction)i.Operand]; break;
                case Code.Brfalse: case Code.Brfalse_S: if (!True(stack.Pop())) pc = index[(Instruction)i.Operand]; break;
                case Code.Brtrue: case Code.Brtrue_S: if (True(stack.Pop())) pc = index[(Instruction)i.Operand]; break;
                case Code.Blt: case Code.Blt_S: { int b = Integer(stack.Pop()), a = Integer(stack.Pop()); if (a < b) pc = index[(Instruction)i.Operand]; break; }
                case Code.Bge: case Code.Bge_S: { int b = Integer(stack.Pop()), a = Integer(stack.Pop()); if (a >= b) pc = index[(Instruction)i.Operand]; break; }
                case Code.Newobj:
                    Check(((IMethod)i.Operand).DeclaringType.FullName == "UnityEngine.WaitForSeconds", "unexpected constructor");
                    stack.Push(Convert.ToSingle(stack.Pop())); break;
                case Code.Call: case Code.Callvirt:
                {
                    string name = ((IMethod)i.Operand).Name;
                    switch (name)
                    {
                        case "CanContinue": stack.Push(StaminaSweep.CanContinue((UI.UICopySweep)stack.Pop())); break;
                        case "_ShowBtn": stack.Pop(); ((UI.UICopySweep)stack.Pop()).showCalls++; break;
                        case "OnSweepCopyAuto":
                        {
                            stack.Pop(); var page = (UI.UICopySweep)stack.Pop();
                            if (StaminaSweep.BeginSend(page)) page.sends++;
                            break;
                        }
                        case "AskFight": ((ArenaPage)stack.Pop()).sends++; break;
                        case "SetActive": { bool active = True(stack.Pop()); ((Button)stack.Pop()).SetActive(active); break; }
                        default: throw new Exception("unmodeled call " + name);
                    }
                    break;
                }
                case Code.Ret: { bool result = True(stack.Pop()); Check(stack.Count == 0, "unbalanced iterator stack"); return result; }
                default: throw new Exception("unmodeled opcode " + i.OpCode.Name);
            }
        }
        throw new Exception("iterator step bound exceeded");
    }
    static void IteratorCases(ModuleDefMD module)
    {
        MethodDef Method(string nested, string parent) => module.GetTypes().Single(t => t.Name == nested && t.DeclaringType.FullName == parent).Methods.Single(m => m.Name == "MoveNext");
        var arena = Method("<SweepNext>c__IteratorF4", "UI.PVPSweepPage");
        var copy = Method("<_StartSweep>c__Iterator141", "UI.UICopySweep");
        var a = new ArenaPage { sweepCount = 1, sweepTotalCount = 5 };
        var ai = new Iter { page = a };
        Check(Move(arena, ai) && (float)ai.wait == .05f && a.sends == 0, "arena first pass should yield .05s before a send");
        Check(!Move(arena, ai) && a.sends == 1 && !a.confirmButton.Active, "arena resume should send exactly once");
        a.sweepCount = 5; ai = new Iter { page = a };
        Check(Move(arena, ai) && !Move(arena, ai) && a.sends == 1 && a.confirmButton.Active, "arena final result should confirm without next send");
        var c = new UI.UICopySweep { count = 5 };
        c.titleAlpha.duration = 1.2f; c.bodyScale.duration = 2f;
        StaminaSweep.Opened(c);
        var ci = new Iter { page = c, current = 1 };
        Check(Move(copy, ci) && (float)ci.wait == .05f && c.sends == 0, "copy first pass should yield .05s");
        Check(!Move(copy, ci) && c.sends == 1 && c.showCalls == 1, "copy resume should send once");
        ci = new Iter { page = c, current = 5 };
        Check(!Move(copy, ci) && c.sends == 1, "copy count limit should finish before waiting");
        ci = new Iter { page = c, current = 2 };
        Check(Move(copy, ci), "copy stop test must enter wait");
        StaminaSweep.Stop(c);
        Check(!Move(copy, ci) && c.sends == 1 && c.stopCalls == 1, "stop during wait must block next send");
        StaminaSweep.Opened(c); ci = new Iter { page = c, current = 2 };
        Check(Move(copy, ci), "copy close test must enter wait");
        StaminaSweep.Closed(c);
        Check(!Move(copy, ci) && c.sends == 1, "close during wait must block next send");
        StaminaSweep.Opened(c); c.needStop = true; ci = new Iter { page = c, current = 2 };
        Check(!Move(copy, ci) && c.sends == 1, "original needStop guard must finish before waiting");
        Console.WriteLine("PASS emitted iterators: .05 yield, one send after resume, count limit, stop and close");
    }
    static void GuardCases()
    {
        var c = new UI.UICopySweep(); var token = new object();
        StaminaSweep.Opened(c);
        Check(StaminaSweep.BeginSend(c), "open page permits a send");
        StaminaSweep.Capture(token, c);
        StaminaSweep.Stop(c);
        Check(!StaminaSweep.CanStartNext(c) && !StaminaSweep.BeginSend(c), "stop must deny continuation");
        Check(!c.stopBtn.Active && c.submitBtn.Active, "stop must restore original button pair");
        var packet = new NetWork.Packets.Packet { kBody = new ProtoBuf.Message.SCCopySweep() };
        Check(StaminaSweep.Accept(token, c, packet, true), "same-generation in-flight reply remains accepted after stop");
        Check(!StaminaSweep.Accept(token, c, packet, true), "reply token consumed once");
        StaminaSweep.Opened(c); c.levelId = "old-level"; token = new object(); StaminaSweep.Capture(token, c);
        StaminaSweep.Closed(c); StaminaSweep.Opened(c); c.levelId = "new-level";
        var result = (ProtoBuf.Message.SCCopySweep)packet.kBody; result.wantedId = "wanted";
        Check(!StaminaSweep.Accept(token, c, packet, true), "old generation cannot refresh reopened page");
        Check(StaticData.playerData.gainCount == 1 && StaticData.playerData.petCount == 1 && StaticData.playerData.Stamina == 93 && UI.UICopyData.LastLevel == "old-level" && UI.UICopyData.Added == 1 && BoxManager.boxes == 1, "stale success accounting must retain captured level");
        Check(!StaminaSweep.Accept(token, c, packet, true) && StaticData.playerData.gainCount == 1, "stale token consumed once");
        token = new object(); StaminaSweep.Capture(token, c); StaminaSweep.Closed(c); StaminaSweep.Opened(c);
        Check(!StaminaSweep.Accept(token, c, packet, false) && StaticData.playerData.gainCount == 1, "stale failure must not invent reward");
        Console.WriteLine("PASS original guard source: stop, same-generation reply, reopen, stale success/failure, duplicate");
    }
    static void StructureCases(ModuleDefMD module)
    {
        MethodDef Method(string owner, string name) => module.GetTypes().Single(t => t.FullName == owner).Methods.Single(m => m.Name == name);
        int Call(MethodDef method, string name) => method.Body.Instructions.Select((x, i) => (x, i)).Single(x =>
            (x.x.OpCode.Code == Code.Call || x.x.OpCode.Code == Code.Callvirt) && x.x.Operand is IMethod m && m.Name == name).i;
        var callback = Method("UI.PVPSweepPage", "AskFightCallback");
        Check(callback.Body.Instructions[0].OpCode.Code == Code.Ldarg_2 && callback.Body.Instructions[1].OpCode.Code == Code.Brfalse,
              "arena callback success gate changed");
        int create = Call(callback, "CreateItem"), next = Call(callback, "SweepNext"), start = Call(callback, "StartCoroutine");
        Check(create < next && next < start, "arena next coroutine must be created after a successful result");
        Check(callback.Body.Instructions.IndexOf((Instruction)callback.Body.Instructions[1].Operand) > start,
              "arena failure must skip next coroutine");
        var copyClosure = module.GetTypes().Single(t => t.Name == "<OnSweepCopyAuto>c__AnonStorey19B" && t.DeclaringType.FullName == "UI.UICopySweep").Methods.Single(m => m.Name == "<>m__1E7");
        int accept = Call(copyClosure, "Accept"), refresh = Call(copyClosure, "Refresh"), guard = Call(copyClosure, "CanStartNext"), nextCopy = Call(copyClosure, "_StartSweep");
        Check(accept < refresh && refresh < guard && guard < nextCopy, "copy callback/guard ordering changed");
        Check(copyClosure.Body.Instructions[accept+1].OpCode.Code == Code.Brtrue && copyClosure.Body.Instructions[accept+2].OpCode.Code == Code.Ret,
              "copy stale/duplicate response gate changed");
        var auto = Method("UI.UICopySweep", "OnSweepCopyAuto");
        Check(Call(auto, "BeginSend") < Call(auto, "SendHttp"), "copy send guard must precede actual request");
        var tween = module.GetTypes().Single(t => t.FullName == "TweenPosition").Methods.Single(m => m.MDToken.Raw == 0x0600263f);
        int sample = Call(tween, "Sample"), disable = Call(tween, "set_enabled");
        Check(sample < disable && tween.Body.Instructions.Any(x => x.OpCode.Code == Code.Bgt_Un), "zero-duration NGUI finish branch missing");
        Console.WriteLine("PASS inherited callback ordering, send gate and original zero-duration NGUI branch");
    }
    public static void Main(string[] args)
    {
        Check(args.Length == 1, "patched managed path required");
        using var module = ModuleDefMD.Load(args[0]);
        StructureCases(module);
        IteratorCases(module);
        GuardCases();
    }
}
