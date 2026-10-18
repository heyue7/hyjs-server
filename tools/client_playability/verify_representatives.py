"""Execute emitted overlay IL branches with bounded API stubs; this is not Unity/device testing."""
import argparse
import json
from pathlib import Path
import sys
import dnfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_playability.build import decoded

def symbol(pe,token):
    table={4:'Field',6:'MethodDef',10:'MemberRef',2:'TypeDef'}[token>>24]
    row=getattr(pe.net.mdtables,table).rows[(token&0xffffff)-1]
    return str(row.TypeName if table=='TypeDef' else row.Name)

def run(pe,code,args,chapter=None,stop_at=None):
    stack=[];events=[];pc=0
    for _ in range(250):
        if pc==stop_at:return events+['original-guide-step']
        op,arg=code[pc];pc+=1
        if op.startswith('ldarg.'):stack.append(args[int(op[-1])])
        elif op in ('ldstr','ldc.r4'):stack.append(arg)
        elif op.startswith('ldc.i4.'):stack.append(int(op[-1]))
        elif op=='ldsfld':stack.append('')
        elif op=='ldfld':stack.append(stack.pop()[symbol(pe,arg)])
        elif op=='stfld':
            value=stack.pop();stack.pop()[symbol(pe,arg)]=value
        elif op in ('conv.i8','conv.r4'):pass
        elif op=='dup':stack.append(stack[-1])
        elif op=='pop':stack.pop()
        elif op=='castclass':pass
        elif op=='isinst':
            value=stack.pop();stack.append(value if value and value.get('_type')==symbol(pe,arg) else None)
        elif op.startswith('brtrue'):
            if stack.pop():pc=arg
        elif op.startswith('brfalse'):
            if not stack.pop():pc=arg
        elif op in ('br','br.s'):pc=arg
        elif op in ('call','callvirt'):
            name=symbol(pe,arg)
            if name=='op_Equality':
                b=stack.pop();a=stack.pop();stack.append(a==b)
            elif name in ('get_transactionData','get_orderId','get_paymentId','get_payStatus'):stack.append(stack.pop()[name[4:]])
            elif name=='GetUIChapterData':
                assert stack.pop()=='1';stack.append(chapter)
            elif name=='NextStep':
                assert stack.pop()==1;stack.pop();events.append('next-original-step')
            elif name=='CreatePopupTextBox':
                assert stack.pop()==0;events.append(stack.pop())
            elif name=='get_Instance':stack.append(None)
            elif name in ('CancelInvoke','TickCheck','TestCharge'):
                owner=stack.pop();events.append(name)
                if name=='TestCharge':owner['orderId']=owner['payMentId']=''
            else:raise AssertionError('Unmodeled call '+name)
        elif op=='ret':
            assert not stack,stack
            return events
        else:raise AssertionError('Unmodeled opcode '+op)
    raise AssertionError('Loop limit')

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--managed',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
    pe=dnfile.dnPE(str(a.managed));bodies={n:decoded(pe,m)[1] for n,_,m in methods(pe) if n in ['GuideManager::StepClick','PlatformSDKCallBack::ChekOrderCallBack']};results=[]
    callback=bodies['PlatformSDKCallBack::ChekOrderCallBack']
    def response(status='PAY_STATUS_SUCCESS',order='order1',product='payment1'):
        return {'kBody':{'_type':'SCCheckOrder','transactionData':{'orderId':order,'paymentId':product,'payStatus':status}}}
    def owner():return dict(orderId='order1',payMentId='payment1',tick=4)
    cases=[('confirmed',True,response(),1),('wait',True,response('PAY_STATUS_WAIT'),0),('failed-status',True,response('PAY_STATUS_FAILED'),0),('transport-failure',False,response(),0),('stale-order',True,response(order='old'),0),('wrong-product',True,response(product='other'),0),('missing-body',True,{'kBody':None},0),('missing-transaction',True,{'kBody':{'_type':'SCCheckOrder','transactionData':None}},0)]
    for name,success,packet,expected in cases:
        actor=owner();events=run(pe,callback,[actor,packet,success]);assert events.count('购买成功')==expected,(name,events)
        if expected:
            assert actor['orderId']==actor['payMentId']==''
            again=run(pe,callback,[actor,packet,success]);assert '购买成功' not in again
        results.append({'case':name,'successNotices':expected,'passed':True})
    guide=bodies['GuideManager::StepClick']
    for name,step,chapter,skip in [('claimed-first-box',{'stepId':'59','guideId':'guide_copy_reward'},{'chapter_inst_get_box_0':True},True),('unclaimed-first-box',{'stepId':'59','guideId':'guide_copy_reward'},{'chapter_inst_get_box_0':False},False),('missing-chapter',{'stepId':'59','guideId':'guide_copy_reward'},None,False),('other-step',{'stepId':'58','guideId':'guide_copy_reward'},{'chapter_inst_get_box_0':True},False),('other-guide',{'stepId':'59','guideId':'other'},{'chapter_inst_get_box_0':True},False),('null-step',None,{'chapter_inst_get_box_0':True},False)]:
        events=run(pe,guide,[{'curStep':step}],chapter,27);assert ('next-original-step' in events)==skip,(name,events)
        results.append({'case':name,'continuesAlreadyCompletedStep':skip,'passed':True})
    result={'scope':'emitted-IL control flow with explicit stubs; not Unity, phone or real payment execution','cases':results,'count':len(results),'passed':True}
    a.output.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'count':len(results),'passed':True}))
if __name__=='__main__':main()
