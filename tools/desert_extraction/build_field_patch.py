"""Build a static, non-release 1.0.79 desert resource hook overlay.

Requires the pinned v80 desert map adapter; does not create or install an APK.
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
from tools.desert_extraction.build_v78_adapter import QUALITY_METHOD,QUALITY_PIN

HERE=Path(__file__).resolve().parent
APK_SHA='25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
MAP_MANAGED_SHA='ba402ea01a9cd4fb3db03e57f6a9335a738a8a48a212cb83cd7935fbce66668f'
# Exact map overlay is pinned above; every original method outside this list
# must remain byte-identical, including all original Battle.Engine methods.
HEADS={
 'UI.MiniMapComponent::OnClick':('Open',['ldarg.0','call','brfalse','ret']),
 'Room.RoomNetField::SCQueryTeam':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCQueryTeamTick':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCJoinTeam':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCExitTeam':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCTeamKick':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCDisbandTeam':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCTeamCreateAndInvite':('Observe',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SendCSQueryTeamTick':('TickSent',['ldarg.0','call']),
 'Room.RoomNetBase::SCPlayerAppear':('ObservePosition',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetBase::SCMovePush':('ObservePosition',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetBase::SCPlayerDisappear':('ObservePosition',['ldarg.0','ldarg.1','call']),
 'Room.RoomNetField::SCFieldResourceInfo':('ResourceNotification',['ldarg.1','call','brfalse','ret']),
 'Room.RCUIPlayer::OnClick':('OpenPlayer',['ldarg.0','call','brfalse','ret']),
 'Room.RoomNetField::SCSidePvpFightResult':('PvpResponse',['ldarg.1','call','brfalse','ret']),
 'Room.RoomNetField::SCFieldGetResource':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SCFieldResourceStartGather':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SCSideResourceCheck':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SendResourceCheck':('ResourceCheck',['call','brfalse','ret']),
 'Room.RoomNetField::SendGetFieldRes':('Gather',['ldarg.1','ldarg.2','call','brfalse','ret']),
 'Room.RoomField::OnMainCharactorLoaded':('ResourceChanged',['call','pop']),
 'Room.RoomNetField::SCSideMonster':('MonsterChanged',['call','brfalse','ret']),
 'Room.RoomNetField::SCSideBoss':('MonsterChanged',['call','brfalse','ret']),
 'Room.RoomNetField::SCSideWorldBoss':('MonsterChanged',['call','brfalse','ret']),
 'Room.RoomNetField::SCSideBossCountdown':('MonsterChanged',['call','brfalse','ret']),
 'Room.RCUINormalNPC::OpenPage':('OpenNpc',['ldarg.0','call','brfalse','ret']),
 'Room.RoomNetField::SendPve':('BeforeEncounter',['ldc.i4','ldarg.1','call','brfalse','ret']),
 'Room.RoomNetField::SendPveBoss':('BeforeEncounter',['ldc.i4','ldarg.1','call','brfalse','ret']),
 'Room.RoomNetField::SCSidePveFightStart':('FightStartResponse',['ldc.i4','ldarg.1','call']),
 'Room.RoomNetField::SCSidePveBossFightStart':('FightStartResponse',['ldc.i4','ldarg.1','call']),
 'NetAPI::Reconnect':('TcpConnecting',['call']),
 'NetAPI::SendSocketLogin':('TcpConnecting',['call']),
 'NetAPI::Disconnect':('TcpConnecting',['call']),
 'NetAPI::SCLoginTcpCallback':('TcpLoginResponse',['ldarg.0','call']),
 'NetAPI::SendSocket':('TransferSend',['ldarg.0','ldarg.1','call','brfalse','ret','ldarg.0','call']),
 'ManagerController::SceneTransferCallback':('TransferResponse',['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret']),
 'Room.RoomNetField::SCTransferLine':('TransferResponse',['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret']),
 'NetWork.Sockets.Event.EventManger::AddPacket':('Queue',['ldarg.1','call','ldarg.1','call']),
 'NetWork.Layer.SocketTool::ReceiveHandler':('DropOldSocket',['ldarg.0','ldarg.1','call','brfalse','ret']),
 'Room.RoomCharactor::MoveTo':('BlockMove',['ldarg.0','call','brfalse','ret']),
 'Battle.FieldPVEBattleOver::FieldPVEFightResultHandler':('FightEndResponse',['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret']),
 'Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler':('FightEndResponse',['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret']),
}
REPLACEMENTS={
 'Room.RCAIServerMonster::Tick':{'MonsterPatrolTick':'PatrolTick','MonsterMoving':'IsMoving'},
 'Battle.FieldPVEBattleOver::AskFieldPVEFightResult':{'FightEndSend':'SendSocket'},
 'Battle.FieldPVEBossBattleOver::AskFieldPVEFightResult':{'FightEndSend':'SendSocket','LegacyBossObserve':'Observe'},
 'Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler':{'LegacyBossCompleted':'Completed'},
}
EXTRAS={'BoxManager::CreateSocketReconnect','NetWork.Sockets.Event.EventManger::Tick','Room.RoomNetBase::SCCurrentPosition','Room.RoomNetField::SCCurrentPosition','Room.RoomField::OnLoadComplete','StaticData::InitPlayerData','Battle.CopyBalance::Refresh'} | set(REPLACEMENTS)
SOURCES=('GatherAttempt.cs','NestedArrayJson.cs','FieldWire.cs','GatherJournalStore.cs','FieldResources.cs','GatherPreload.cs','NativeGatherUI.cs','GatherCoordinator.cs','ProtectionRelease.cs','FieldRuntime.cs','PendingRequest.cs','TerminalController.cs','FieldNpcs.cs','FieldMonsters.cs','FieldBoss.cs','PositionSync.cs','TravelController.cs','LeaveController.cs','TradeController.cs','TradePanel.cs','FightEndJournal.cs','MonsterRewardView.cs','BattleRecovery.cs','ColdBattleRecovery.cs','PvpBridge.cs','MapViewer86.cs','TeamMap86.cs')
MAP_ICON=HERE/'assets/evacuation-npc.png'

def sha(raw): return hashlib.sha256(raw).hexdigest()
def sha_file(path):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''): h.update(block)
    return h.hexdigest()
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
    if str(getattr(cls,'TypeNamespace',''))!='HYJS.DesertV4': return ''
    typ=str(getattr(cls,'TypeName',''))
    if typ!='FieldHooks' and not (typ=='MapViewer86' and str(ref.Name)=='Open') and not (typ=='TeamMap86' and str(ref.Name) in ('Queue','Observe','TickSent','InspectDispatch','ObservePosition','ScopeStarting')): return ''
    return str(ref.Name)
def hook_owner(pe,item):
    op,token=item
    if op!='call' or not isinstance(token,int) or token>>24!=10: return ''
    cls=pe.net.mdtables.MemberRef.rows[(token&0xffffff)-1].Class.row
    return str(getattr(cls,'TypeNamespace',''))+'.'+str(getattr(cls,'TypeName',''))
def require_field_hook(pe,item,name,signature):
    require(hook_for(pe,item)==name and hook_owner(pe,item)=='HYJS.DesertV4.FieldHooks',
            'Wrong native readiness hook: '+name)
    ref=pe.net.mdtables.MemberRef.rows[(item[1]&0xffffff)-1]
    scope=getattr(getattr(ref.Class.row,'ResolutionScope',None),'row',None)
    require(type(scope).__name__=='AssemblyRefRow' and str(scope.Name)=='SoulFlowerDesertV4' and
            (scope.MajorVersion,scope.MinorVersion,scope.BuildNumber,scope.RevisionNumber)==(0,0,0,0),
            'Wrong readiness helper assembly: '+name)
    require(ref.Signature.value==bytes.fromhex(signature),'Wrong native readiness hook signature: '+name)
def verify_readiness_helper(raw):
    pe=dnfile.dnPE(data=raw)
    assemblies=pe.net.mdtables.Assembly.rows
    require(len(assemblies)==1 and str(assemblies[0].Name)=='SoulFlowerDesertV4' and
            (assemblies[0].MajorVersion,assemblies[0].MinorVersion,assemblies[0].BuildNumber,assemblies[0].RevisionNumber)==(0,0,0,0),
            'Wrong readiness helper assembly identity')
    types=[t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeNamespace)=='HYJS.DesertV4' and str(t.TypeName)=='FieldHooks']
    require(len(types)==1 and types[0].struct.Flags&7==1,'FieldHooks is not a public type')
    for name,signature in (('RoomLoaded','0001011c'),('FieldPositionReady','0002011c1c'),('PlayerDataReady','0001011c')):
        found=[x.row for x in types[0].MethodList if str(x.row.Name)==name]
        require(len(found)==1,'Missing or ambiguous readiness helper: '+name)
        row=found[0]
        require(row.struct.Flags&7==6 and row.struct.Flags&0x10 and row.Rva and
                row.Signature.value==bytes.fromhex(signature),'Wrong readiness helper ABI: '+name)
def branch(op): return op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave'))
def require_unbypassed_tail(body,items,name):
    require(items and items[-1][0]=='ret' and sum(op=='ret' for op,_ in items)==1 and not body.exception_handlers,
            'Unexpected original readiness tail: '+name)
    tail=len(items)-1
    require(all(not (branch(op) and value==tail) and not (op=='switch' and tail in value)
                for op,value in items),'Readiness tail can be bypassed: '+name)
def verify(before,after):
    left,right=(dnfile.dnPE(data=x) for x in (before,after))
    for table in ('TypeDef','Field','MethodDef'):
        require(len(getattr(left.net.mdtables,table).rows)==len(getattr(right.net.mdtables,table).rows),'Original inventory changed: '+table)
    require(not getattr(right.net.mdtables,'FieldPtr',None),'Unexpected FieldPtr')
    old,new=methods(left),methods(right); old_names={rid:name for name,rid,row in old}
    require(len(old)==len(new),'Method inventory differs')
    changed=[]; unchanged=engine=0; quality=False; expected=set(HEADS)|EXTRAS
    for (name,rid,a),(name2,rid2,b) in zip(old,new):
        require((name,rid)==(name2,rid2),'Method identity differs')
        require(a.Signature.value==b.Signature.value and a.struct.Flags==b.struct.Flags and a.struct.ImplFlags==b.struct.ImplFlags,'Original ABI changed: '+name)
        if not a.Rva: require(not b.Rva,'Unexpected new body: '+name); continue
        raw_a,raw_b=exact_body(left,a),exact_body(right,b)
        if name==QUALITY_METHOD:
            _, instructions=decoded(left,a)
            require(sha(raw_a)==QUALITY_PIN and raw_a==raw_b and instructions[74]==('ldc.i4.s',11),'Released quality filter changed')
            quality=True
        if name not in expected:
            require(raw_a==raw_b,'Unexpected body change: '+name)
            unchanged+=1; engine+=name.startswith('Battle.Engine'); continue
        require(raw_a!=raw_b,'Expected hook not written: '+name)
        ob,x=decoded(left,a); nb,y=decoded(right,b)
        remove=set(); prefix=HEADS.get(name)
        if prefix:
            hook,ops=prefix; require([q[0] for q in y[:len(ops)]]==ops,'Incorrect hook prefix: '+name)
            ci=ops.index('call'); require(hook_for(right,y[ci])==hook,'Wrong hook target: '+name)
            if hook in ('Queue','Observe','TickSent','ObservePosition'):
                require(hook_owner(right,y[ci])=='HYJS.DesertV4.TeamMap86','Wrong team hook owner: '+name)
            if name=='NetAPI::SendSocket':
                require(hook_for(right,y[6])=='ScopeStarting' and hook_owner(right,y[6])=='HYJS.DesertV4.TeamMap86',
                        'Wrong team scope invalidation')
                ref=right.net.mdtables.MemberRef.rows[(y[6][1]&0xffffff)-1]
                require(ref.Signature.value==bytes.fromhex('00010108'),'Wrong team scope invalidation signature')
            if name=='NetWork.Sockets.Event.EventManger::AddPacket':
                require(hook_for(right,y[3])=='TrackPositionPacket','Wrong position packet observation: '+name)
                require(hook_owner(right,y[3])=='HYJS.DesertV4.FieldHooks','Wrong position hook owner: '+name)
            for j,op in enumerate(ops):
                if op=='brfalse': require(y[j][1]==(5 if name=='NetAPI::SendSocket' else len(ops)),'Guard does not branch to original: '+name)
            if name=='ManagerController::SceneTransferCallback': require(y[0][1]==30033,'Wrong scene response opcode')
            if name=='Room.RoomNetField::SCTransferLine': require(y[0][1]==30062,'Wrong line response opcode')
            if name in ('Room.RoomNetField::SendPve','Room.RoomNetField::SCSidePveFightStart','Battle.FieldPVEBattleOver::FieldPVEFightResultHandler'): require(y[0][1]==6,'Wrong fight type')
            if name in ('Room.RoomNetField::SendPveBoss','Room.RoomNetField::SCSidePveBossFightStart','Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler'): require(y[0][1]==12,'Wrong boss type')
            remove.update(range(len(ops)))
        if name=='BoxManager::CreateSocketReconnect':
            found=[j for j,q in enumerate(y) if hook_for(right,q)=='PreserveBattle']
            require(len(found)==1,'Missing battle preservation guard'); j=found[0]
            require(j>0 and name_for(right,y[j-1][1],old_names)=='ClearSocketBox' and y[j+1][0]=='brtrue','Wrong reconnect guard placement')
            target=y[j+1][1]; require(y[target]==('ldstr','UI/MessageBox/NetError'),'Wrong original reconnect view')
            remove.update((j,j+1))
        if name=='Battle.CopyBalance::Refresh':
            found=[j for j,q in enumerate(y) if hook_for(right,q)=='MonsterRewardDisplay']
            require(len(found)==1,'Missing monster reward projection'); j=found[0]
            require(j>0 and j+1<len(y) and name_for(right,y[j-1][1],old_names)=='get_getn' and
                    name_for(right,y[j+1][1],old_names)=='GetResultItem' and y[j+1][0]=='call',
                    'Monster reward projection is not on the original display-only path')
            require(hook_owner(right,y[j])=='HYJS.DesertV4.FieldHooks','Wrong monster reward projection owner')
            remove.add(j)
        if name=='NetWork.Sockets.Event.EventManger::Tick':
            found=[j for j,q in enumerate(y) if hook_for(right,q)=='DropPositionPacket']
            require(len(found)==1,'Missing position event epoch guard'); j=found[0]
            require(j>0 and y[j-1][0]=='ldloc.0' and y[j+1][0]=='brtrue' and y[j+1][1]==y[0][1],'Wrong event guard')
            require([q[0] for q in y[j+2:j+5]]==['ldarg.0','ldloc.0','call'] and
                    hook_for(right,y[j+4])=='InspectDispatch' and hook_owner(right,y[j+4])=='HYJS.DesertV4.TeamMap86',
                    'Wrong team dispatch observation')
            require(sum(hook_for(right,q)=='InspectDispatch' for q in y)==1,'Unexpected team dispatch observations')
            remove.update((j-1,j,j+1,j+2,j+3,j+4))
        if name=='StaticData::InitPlayerData':
            require(a.struct.Flags&0x10 and not a.struct.Flags&0x0400 and
                    a.Signature.value==bytes.fromhex('000a011283f412843812844012844812845012845812846812847812849812895c'),
                    'Original ten-argument static login ABI changed')
            require_unbypassed_tail(ob,x,name)
            found=[j for j,q in enumerate(y) if hook_for(right,q)=='PlayerDataReady']
            require(len(found)==1,'Missing login player observation'); j=found[0]
            require(j==len(y)-2 and [q[0] for q in y[j-1:]]==['ldarg.0','call','ret'],
                    'Wrong login player observation tail')
            require_field_hook(right,y[j],'PlayerDataReady','0001011c')
            require(ob.max_stack==nb.max_stack,'Original login max stack changed')
            remove.update((j-1,j))
        if name=='Room.RoomField::OnLoadComplete':
            require_unbypassed_tail(ob,x,name)
            found=[j for j,q in enumerate(y) if hook_for(right,q)=='RoomLoaded']
            require(len(found)==1,'Missing room completion observation'); j=found[0]
            require(j==len(y)-2 and [q[0] for q in y[j-1:]]==['ldarg.0','call','ret'],
                    'Wrong room completion tail')
            require_field_hook(right,y[j],'RoomLoaded','0001011c')
            remove.update((j-1,j))
        if name in ('Room.RoomNetBase::SCCurrentPosition','Room.RoomNetField::SCCurrentPosition'):
            expected_hook='TrackPositionBase' if name=='Room.RoomNetBase::SCCurrentPosition' else 'TrackPositionField'
            found=[j for j,q in enumerate(y) if hook_for(right,q)==expected_hook]
            require(len(found)==1,'Missing position response observation'); j=found[0]
            ops=['ldarg.0','ldarg.1','call'] if expected_hook=='TrackPositionBase' else ['ldarg.1','call']
            start=j-len(ops)+1
            require([q[0] for q in y[start:j+1]]==ops,'Wrong position response observation')
            remove.update(range(start,j+1))
            if expected_hook=='TrackPositionBase':
                require(j+1==len(y)-1 and y[-1][0]=='ret','Wrong base position response tail')
            else:
                require_unbypassed_tail(ob,x,name)
                require([q[0] for q in y[j+1:]]==['ldarg.0','ldarg.1','call','ret'],
                        'Wrong field position readiness tail')
                require(sum(hook_for(right,q)=='FieldPositionReady' for q in y)==1,
                        'Unexpected field position readiness observations')
                require_field_hook(right,y[j+3],'FieldPositionReady','0002011c1c')
                remove.update((j+1,j+2,j+3))
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
            h=hook_for(right,y[j])
            require(old_op==new_op or (h=='MonsterMoving' and old_op=='callvirt' and new_op=='call'),'Original opcode changed: '+name)
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
            require(oe.exception_type==ne.exception_type and getattr(oe.catch_type,'value',None)==getattr(ne.catch_type,'value',None),'Original exception handler type changed: '+name)
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
    require(len(changed)==len(expected) and engine==547 and quality,'Missing field hook, released quality fix or changed combat engine')
    return dict(originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,originalTokensAndABI=True,qualityFilterV80Exact=True,fieldHooks=changed)

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--base-apk',type=Path,required=True)
    ap.add_argument('--map-managed',type=Path,required=True)
    ap.add_argument('--runtime',type=Path,required=True)
    ap.add_argument('--output',type=Path,required=True)
    args=ap.parse_args()
    require(sha_file(args.base_apk)==APK_SHA,'Wrong pinned APK')
    before=args.map_managed.read_bytes(); require(sha(before)==MAP_MANAGED_SHA,'Wrong pinned v80 map overlay')
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
        require(MAP_ICON.is_file(),'Missing evacuation icon')
        run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),'/resource:'+str(MAP_ICON)+',HYJS.DesertV4.Evacuation.png',*['/reference:'+str(x) for x in refs],*[HERE/x for x in SOURCES]])
        verify_readiness_helper(helper.read_bytes())
        shutil.copy2(HERE/'InjectorField.cs',work/'Program.cs')
        project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Program.cs"/><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        output=args.output.resolve()/'Assembly-CSharp.dll'
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,output])
    result=verify(before,output.read_bytes())
    report=dict(schema='hyjs-desert-v4-scene-overlay-v2',status='STATIC_PREVIEW_NOT_FOR_RELEASE',runtimeIntegrated=False,apkSHA256=APK_SHA,inputManagedSHA256=MAP_MANAGED_SHA,outputManagedSHA256=sha(output.read_bytes()),helperSHA256=sha(helper.read_bytes()),deviceExecuted=False,serverExecuted=False,**result)
    report['sourceSHA256']={'tools/desert_extraction/'+x:sha((HERE/x).read_bytes()) for x in (*SOURCES,'InjectorField.cs','build_field_patch.py')}
    report['sourceSHA256']['tools/desert_extraction/assets/evacuation-npc.png']=sha(MAP_ICON.read_bytes())
    (args.output/'manifest.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(report))
if __name__=='__main__': main()
