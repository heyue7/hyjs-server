"""Build a static, non-release 1.0.78 desert resource hook overlay.

Requires the pinned v78 desert map adapter; does not create or install an APK.
Server implementation, all other V4 gameplay and device acceptance remain gates.
"""
import argparse
import os
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT=Path(os.environ.get('HYJS_REPO_ROOT',str(Path(__file__).resolve().parents[2])))
sys.path.insert(0,str(ROOT))
import dnfile
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body,require

HERE=Path(__file__).resolve().parent
APK_SHA='fe67aea7d584199119a04f10edcd8baa54207a70717e295d377ce3dd67b8affc'
MAP_MANAGED_SHA='4eaa1d47b1d822f579e0c7bb3bfa2d9cb5b08f75a32f194be7420fc44a1a1c97'
# Exact map overlay is pinned above; every original method outside this list
# must remain byte-identical, including all original Battle.Engine methods.
HEADS={
 'Room.RoomNetField::SCFieldResourceInfo':('ResourceChanged',['call','brfalse','ret']),
 'Room.RoomNetField::SCFieldGetResource':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SCFieldResourceStartGather':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SCSideResourceCheck':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SendResourceCheck':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SendGetFieldRes':('Gather',['ldarg.1','ldarg.2','call','brfalse','ret']),
 'Room.RoomField::OnMainCharactorLoaded':('ResourceChanged',['call','pop']),
 'Room.RoomNetField::SCSideMonster':('MonsterChanged',['call','brfalse','ret']),
 'Room.RCUINormalNPC::OpenPage':('OpenNpc',['ldarg.0','call','brfalse','ret']),
 'Room.RoomNetField::SendPve':('BeforeEncounter',['ldc.i4','ldarg.1','call','brfalse','ret']),
 'Room.RoomNetField::SendPveBoss':('BeforeEncounter',['ldc.i4','ldarg.1','call','brfalse','ret']),
 'Room.RoomNetField::SCSidePveFightStart':('FightStartResponse',['ldc.i4','ldarg.1','call']),
 'Room.RoomNetField::SCSidePveBossFightStart':('FightStartResponse',['ldc.i4','ldarg.1','call']),
 'NetAPI::Reconnect':('TcpConnecting',['call']),
 'NetAPI::SendSocketLogin':('TcpConnecting',['call']),
 'NetAPI::Disconnect':('TcpConnecting',['call']),
 'NetAPI::SCLoginTcpCallback':('TcpLoginResponse',['ldarg.0','call']),
 'Battle.FieldPVEBattleOver::FieldPVEFightResultHandler':('FightEndResponse',['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret']),
 'Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler':('FightEndResponse',['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret']),
}
REPLACEMENTS={
 'Room.RCAIServerMonster::Tick':{'MonsterPatrolTick':'PatrolTick'},
 'Battle.FieldPVEBattleOver::AskFieldPVEFightResult':{'FightEndSend':'SendSocket'},
 'Battle.FieldPVEBossBattleOver::AskFieldPVEFightResult':{'FightEndSend':'SendSocket','LegacyBossObserve':'Observe'},
 'Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler':{'LegacyBossCompleted':'Completed'},
}
EXTRAS={'BoxManager::CreateSocketReconnect'} | set(REPLACEMENTS)
SOURCES=('GatherAttempt.cs','FieldWire.cs','GatherJournalStore.cs','FieldResources.cs','GatherPreload.cs','NativeGatherUI.cs','GatherCoordinator.cs','ProtectionRelease.cs','FieldRuntime.cs','PendingRequest.cs','TerminalController.cs','FieldNpcs.cs','FieldMonsters.cs','FightEndJournal.cs','BattleRecovery.cs')

def sha(raw): return hashlib.sha256(raw).hexdigest()
def name_for(pe,token,old_methods):
    if not isinstance(token,int): return ''
    table=token>>24; index=(token&0xffffff)-1
    if table==6: return old_methods.get(index+1,'').split('::')[-1]
    if table==10:
        try: return str(pe.net.mdtables.MemberRef.rows[index].Name)
        except Exception: return ''
    return ''
def hook_for(pe,item):
    op,token=item
    if op!='call' or not isinstance(token,int) or token>>24!=10: return ''
    ref=pe.net.mdtables.MemberRef.rows[(token&0xffffff)-1]
    cls=ref.Class.row
    if str(getattr(cls,'TypeNamespace',''))!='HYJS.DesertV4' or str(getattr(cls,'TypeName',''))!='FieldHooks': return ''
    return str(ref.Name)
def branch(op): return op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave'))
def verify(before,after):
    left,right=(dnfile.dnPE(data=x) for x in (before,after))
    for table in ('TypeDef','Field','MethodDef'):
        require(len(getattr(left.net.mdtables,table).rows)==len(getattr(right.net.mdtables,table).rows),'Original inventory changed: '+table)
    require(not getattr(right.net.mdtables,'FieldPtr',None),'Unexpected FieldPtr')
    old,new=methods(left),methods(right); old_names={rid:name for name,rid,row in old}
    require(len(old)==len(new),'Method inventory differs')
    changed=[]; unchanged=engine=0; expected=set(HEADS)|EXTRAS
    for (name,rid,a),(name2,rid2,b) in zip(old,new):
        require((name,rid)==(name2,rid2),'Method identity differs')
        require(a.Signature.value==b.Signature.value and a.struct.Flags==b.struct.Flags and a.struct.ImplFlags==b.struct.ImplFlags,'Original ABI changed: '+name)
        if not a.Rva: require(not b.Rva,'Unexpected new body: '+name); continue
        raw_a,raw_b=exact_body(left,a),exact_body(right,b)
        if name not in expected:
            require(raw_a==raw_b,'Unexpected body change: '+name)
            unchanged+=1; engine+=name.startswith('Battle.Engine'); continue
        require(raw_a!=raw_b,'Expected hook not written: '+name)
        ob,x=decoded(left,a); nb,y=decoded(right,b)
        remove=set(); prefix=HEADS.get(name)
        if prefix:
            hook,ops=prefix; require([q[0] for q in y[:len(ops)]]==ops,'Incorrect hook prefix: '+name)
            ci=ops.index('call'); require(hook_for(right,y[ci])==hook,'Wrong hook target: '+name)
            for j,op in enumerate(ops):
                if op=='brfalse': require(y[j][1]==len(ops),'Guard does not branch to original: '+name)
            if name in ('Room.RoomNetField::SendPve','Room.RoomNetField::SCSidePveFightStart','Battle.FieldPVEBattleOver::FieldPVEFightResultHandler'): require(y[0][1]==6,'Wrong fight type')
            if name in ('Room.RoomNetField::SendPveBoss','Room.RoomNetField::SCSidePveBossFightStart','Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler'): require(y[0][1]==12,'Wrong boss type')
            remove.update(range(len(ops)))
        if name=='BoxManager::CreateSocketReconnect':
            found=[j for j,q in enumerate(y) if hook_for(right,q)=='PreserveBattle']
            require(len(found)==1,'Missing battle preservation guard'); j=found[0]
            require(j>0 and name_for(right,y[j-1][1],old_names)=='ClearSocketBox' and y[j+1][0]=='brtrue','Wrong reconnect guard placement')
            target=y[j+1][1]; require(y[target]==('ldstr','UI/MessageBox/NetError'),'Wrong original reconnect view')
            remove.update((j,j+1))
        replacements=REPLACEMENTS.get(name,{})
        hit={k:0 for k in replacements}
        for j,q in enumerate(y):
            h=hook_for(right,q)
            if h in replacements:
                hit[h]+=1
                if h=='FightEndSend':
                    require(j>0 and y[j-1][0]=='ldarg.0','Fight end owner not passed')
                    remove.add(j-1)
        require(all(n==1 for n in hit.values()),'Missing replacement call: '+name)
        kept=[j for j in range(len(y)) if j not in remove]
        require(len(kept)==len(x),'Changed instruction inventory: '+name)
        after_to_old={j:k for k,j in enumerate(kept)}
        for k,j in enumerate(kept):
            old_op,old_value=x[k]; new_op,new_value=y[j]
            require(old_op==new_op,'Original opcode changed: '+name)
            h=hook_for(right,y[j])
            if h in replacements:
                require(name_for(left,old_value,old_names)==replacements[h],'Unexpected original call: '+name)
                continue
            if branch(old_op):
                require(new_value in after_to_old and old_value==after_to_old[new_value],'Original branch changed: '+name)
            elif old_op=='switch':
                require(tuple(after_to_old[q] for q in new_value)==old_value,'Original switch changed: '+name)
            else: require(old_value==new_value,'Original instruction operand changed: '+name)
        require(getattr(ob.local_var_sig_tok,'value',None)==getattr(nb.local_var_sig_tok,'value',None),'Original locals changed: '+name)
        require(len(ob.exception_handlers)==len(nb.exception_handlers),'Original exception handler count changed: '+name)
        old_offsets={q.offset-ob.header_size:k for k,q in enumerate(ob.instructions)}
        for oe,ne in zip(ob.exception_handlers,nb.exception_handlers):
            require(oe.exception_type==ne.exception_type and oe.catch_type==ne.catch_type,'Original exception handler type changed: '+name)
            for key in ('try_start','try_end','handler_start','handler_end'):
                off=getattr(oe,key)
                if off==ob.code_size: mapped=nb.code_size
                else:
                    require(off in old_offsets,'Exception boundary missing: '+name)
                    mapped=nb.instructions[kept[old_offsets[off]]].offset-nb.header_size
                require(getattr(ne,key)==mapped,'Original exception boundary changed: '+name+' '+key)
            if oe.is_filter():
                off=oe.filter_start; require(off in old_offsets,'Filter boundary missing: '+name)
                mapped=nb.instructions[kept[old_offsets[off]]].offset-nb.header_size
                require(ne.filter_start==mapped,'Original filter changed: '+name)
            else: require(ne.filter_start==oe.filter_start,'Original filter changed: '+name)
        changed.append(dict(method=name,beforeSHA256=sha(raw_a),afterSHA256=sha(raw_b)))
    require(len(changed)==len(expected) and engine==547,'Missing field hook or changed combat engine')
    return dict(originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,originalTokensAndABI=True,fieldHooks=changed)

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--base-apk',type=Path,required=True)
    ap.add_argument('--map-managed',type=Path,required=True)
    ap.add_argument('--runtime',type=Path,required=True)
    ap.add_argument('--output',type=Path,required=True)
    args=ap.parse_args()
    require(sha(args.base_apk.read_bytes())==APK_SHA,'Wrong pinned APK')
    before=args.map_managed.read_bytes(); require(sha(before)==MAP_MANAGED_SHA,'Wrong pinned v78 map overlay')
    require(not args.output.exists(),'Use a fresh output directory'); args.output.mkdir(parents=True)
    runtime=args.runtime.resolve(); dotnet=runtime/'dotnet/dotnet'; compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    def run(label,command):
        result=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,text=True,capture_output=True,timeout=120)
        (args.output/(label+'.log')).write_text(result.stdout+result.stderr)
        require(result.returncode==0,label+' failed; inspect output log')
    with tempfile.TemporaryDirectory(prefix='hyjs-desert-v4-field-') as tmp:
        work=Path(tmp); refs=[]
        with zipfile.ZipFile(args.base_apk) as apk:
            for name in ('Assembly-CSharp-firstpass.dll','UnityEngine.dll','mscorlib.dll','System.dll','System.Core.dll','LitJson.dll','HYJS.Field63.dll','LocalFieldSettlement.dll'):
                target=work/name; target.write_bytes(apk.read('assets/bin/Data/Managed/'+name)); refs.append(target)
        host=work/'Assembly-CSharp.dll'; host.write_bytes(before); refs.insert(0,host)
        helper=args.output.resolve()/'SoulFlowerDesertV4.dll'
        run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(x) for x in refs],*[HERE/x for x in SOURCES]])
        shutil.copy2(HERE/'InjectorField.cs',work/'Program.cs')
        project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Program.cs"/><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        output=args.output.resolve()/'Assembly-CSharp.dll'
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,output])
    result=verify(before,output.read_bytes())
    report=dict(schema='hyjs-desert-v4-scene-overlay-v2',status='STATIC_PREVIEW_NOT_FOR_RELEASE',runtimeIntegrated=False,apkSHA256=APK_SHA,inputManagedSHA256=MAP_MANAGED_SHA,outputManagedSHA256=sha(output.read_bytes()),helperSHA256=sha(helper.read_bytes()),deviceExecuted=False,serverExecuted=False,**result)
    report['sourceSHA256']={'tools/desert_extraction/'+x:sha((HERE/x).read_bytes()) for x in (*SOURCES,'InjectorField.cs','build_field_patch.py')}
    (args.output/'manifest.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(report))
if __name__=='__main__': main()
