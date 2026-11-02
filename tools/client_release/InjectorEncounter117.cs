using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

var module=ModuleDefMD.Load(args[0]);
var helper=module.GetAssemblyRefs().Single(x=>x.Name=="SoulFlowerDesertV4");
var hooks=new TypeRefUser(module,"HYJS.DesertV4","FieldHooks",helper);
var drop=new MemberRefUser(module,"DropFightStartResponse",MethodSig.CreateStatic(module.CorLibTypes.Boolean,module.CorLibTypes.Int32,module.CorLibTypes.Object,module.CorLibTypes.Object),hooks);
var awake=new TypeRefUser(module,"HYJS.DesertV4","AwakeStartRecovery",helper);
foreach (var name in new[]{"SCSidePveFightStart","SCSidePveBossFightStart"})
{
    var method=module.GetTypes().Single(t=>t.FullName=="Room.RoomNetField").Methods.Single(m=>m.Name==name);
    var body=method.Body; var ins=body.Instructions;
    if (body.ExceptionHandlers.Count!=0 || ins.Count<4 || ins[0].OpCode!=OpCodes.Ldc_I4 ||
        ins[1].OpCode!=OpCodes.Ldarg_1 || ins[2].OpCode!=OpCodes.Call ||
        ins[2].Operand is not IMethod old || old.FullName!="System.Void HYJS.DesertV4.FieldHooks::FightStartResponse(System.Int32,System.Object)" ||
        ins[0].GetLdcI4Value()!=(name=="SCSidePveFightStart" ? 6 : 12))
        throw new InvalidOperationException("Shipped encounter prefix differs: "+name);
    var first=ins[3];
    ins.RemoveAt(2); ins.RemoveAt(1);
    ins.Insert(1,Instruction.Create(OpCodes.Ldarg_0));
    ins.Insert(2,Instruction.Create(OpCodes.Ldarg_1));
    ins.Insert(3,Instruction.Create(OpCodes.Call,drop));
    ins.Insert(4,Instruction.Create(OpCodes.Brfalse,first));
    ins.Insert(5,Instruction.Create(OpCodes.Ret));
    body.MaxStack=(ushort)Math.Max(8,(int)body.MaxStack);
}
{
    var send=module.GetTypes().Single(t=>t.FullName=="NetAPI").Methods.Single(m=>m.Name=="SendHttp");
    if (send.MethodSig.RetType.ElementType!=ElementType.Boolean || send.MethodSig.Params.Count!=3)
        throw new InvalidOperationException("HTTP ABI differs");
    var replacement=new MemberRefUser(module,"SendHttp",send.MethodSig,awake);
    var copy=module.GetTypes().Single(t=>t.FullName=="Copy.CopyManager").Methods.Single(m=>m.Name=="AwakeCopyToBattleScene");
    var call=copy.Body.Instructions.Single(i=>i.OpCode==OpCodes.Call && i.Operand is IMethod m && m.FullName==send.FullName);
    call.Operand=replacement;
    var manager=module.GetTypes().Single(t=>t.FullName=="ManagerController");
    var start=manager.Methods.Single(m=>m.Name=="AwakeCopyToBattle");
    var first=start.Body.Instructions[0];
    var before=new MemberRefUser(module,"BeforeStart",MethodSig.CreateStatic(module.CorLibTypes.Boolean),awake);
    start.Body.Instructions.Insert(0,Instruction.Create(OpCodes.Call,before));
    start.Body.Instructions.Insert(1,Instruction.Create(OpCodes.Brfalse,first));
    start.Body.Instructions.Insert(2,Instruction.Create(OpCodes.Ret));
    var wait=start.Body.Instructions.Single(i=>i.OpCode==OpCodes.Call && i.Operand is IMethod m && m.Name=="WaitAwakeCopyBattleLoad");
    var factory=(IMethod)wait.Operand;
    var guard=new MemberRefUser(module,"Guard",MethodSig.CreateStatic(factory.MethodSig.RetType,factory.MethodSig.RetType),awake);
    start.Body.Instructions.Insert(start.Body.Instructions.IndexOf(wait)+1,Instruction.Create(OpCodes.Call,guard));
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
