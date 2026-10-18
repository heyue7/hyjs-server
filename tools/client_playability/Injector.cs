using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if(args.Length!=2) throw new ArgumentException("input and output managed DLL required");
using var module=ModuleDefMD.Load(args[0]);
MethodDef Method(string type,string name)=>module.GetTypes().Single(t=>t.FullName==type).Methods.Single(m=>m.Name==name);
FieldDef Field(string type,string name)=>module.GetTypes().Single(t=>t.FullName==type).Fields.Single(f=>f.Name==name);
{
    var target=Method("GuideManager","Awake");
    var bridge=new AssemblyRefUser(new AssemblyNameInfo("LocalPlayability, Version=0.0.0.0, Culture=neutral, PublicKeyToken=null"));
    var type=new TypeRefUser(module,"HYJS.Playability","NoviceSkipUI",bridge);
    var ensure=new MemberRefUser(module,"Ensure",MethodSig.CreateStatic(module.CorLibTypes.Void,new ClassSig(target.DeclaringType)),type);
    target.Body.Instructions.Insert(0,Instruction.Create(OpCodes.Ldarg_0));
    target.Body.Instructions.Insert(1,Instruction.Create(OpCodes.Call,ensure));
    target.Body.MaxStack=Math.Max(target.Body.MaxStack,(ushort)1);target.Body.KeepOldMaxStack=true;
}
{
    var target=Method("PlatformSDKCallBack","ChekOrderCallBack");
    var il=target.Body.Instructions;
    // The inherited callback has already checked success, body, transaction,
    // pending order/product identity and PAY_STATUS_SUCCESS before this clear.
    var clear=il.Single(i=>i.OpCode==OpCodes.Stfld && i.Operand is IField f && f.Name=="payMentId");
    if(il.Count(i=>i.OpCode==OpCodes.Ldstr && (string)i.Operand=="购买成功")!=0)throw new InvalidOperationException("Success notice already installed");
    var notice=new[]{Instruction.Create(OpCodes.Ldstr,"购买成功"),Instruction.Create(OpCodes.Ldc_R4,0f),Instruction.Create(OpCodes.Call,Method("BoxManager","CreatePopupTextBox"))};
    int index=il.IndexOf(clear)+1;
    foreach(var instruction in notice)il.Insert(index++,instruction);
    target.Body.MaxStack=Math.Max(target.Body.MaxStack,(ushort)3);target.Body.KeepOldMaxStack=true;
}
{
    var target=Method("GuideManager","StepClick");
    var original=target.Body.Instructions[0];
    var curStep=Field("GuideManager","curStep");
    var step=curStep.FieldSig.Type.ToTypeDefOrRef().ResolveTypeDef();
    var stepID=step.Fields.Single(f=>f.Name=="stepId");
    var guideID=step.Fields.Single(f=>f.Name=="guideId");
    var equality=module.GetMemberRefs().Single(m=>m.Name=="op_Equality" && m.Class.FullName=="System.String");
    var getChapter=Method("UI.UICopyData","GetUIChapterData");
    if(!getChapter.IsStatic||getChapter.MethodSig.Params.Count!=1)throw new InvalidOperationException("Chapter reader contract changed");
    var claimed=Field("UI.UIChapterData","chapter_inst_get_box_0");
    if(claimed.FieldSig.Type.ElementType!=ElementType.Boolean)throw new InvalidOperationException("Expected original bool claim cache");
    var hasChapter=Instruction.Create(OpCodes.Ldfld,claimed);
    var prefix=new[]{
        Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldfld,curStep),Instruction.Create(OpCodes.Brfalse,original),
        Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldfld,curStep),Instruction.Create(OpCodes.Ldfld,stepID),Instruction.Create(OpCodes.Ldstr,"59"),Instruction.Create(OpCodes.Call,equality),Instruction.Create(OpCodes.Brfalse,original),
        Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldfld,curStep),Instruction.Create(OpCodes.Ldfld,guideID),Instruction.Create(OpCodes.Ldstr,"guide_copy_reward"),Instruction.Create(OpCodes.Call,equality),Instruction.Create(OpCodes.Brfalse,original),
        Instruction.Create(OpCodes.Ldstr,"1"),Instruction.Create(OpCodes.Call,getChapter),Instruction.Create(OpCodes.Dup),Instruction.Create(OpCodes.Brtrue,hasChapter),Instruction.Create(OpCodes.Pop),Instruction.Create(OpCodes.Br,original),
        hasChapter,Instruction.Create(OpCodes.Brfalse,original),
        // Continue only this already completed original step, with no re-claim.
        Instruction.Create(OpCodes.Ldarg_0),Instruction.Create(OpCodes.Ldc_I4_1),Instruction.Create(OpCodes.Call,Method("GuideManager","NextStep")),Instruction.Create(OpCodes.Ret)
    };
    for(int i=0;i<prefix.Length;i++)target.Body.Instructions.Insert(i,prefix[i]);
    target.Body.MaxStack=Math.Max(target.Body.MaxStack,(ushort)3);target.Body.KeepOldMaxStack=true;
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[1],options);
Console.WriteLine("Added confirmed purchase notice and one already-claimed tutorial-step continuation");
