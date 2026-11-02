"""Reproduce the shipped helper, then build the scoped encounter recovery overlay."""
import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.desert_extraction.rebuild_clientfix import sources,verify_main as unused
from tools.awake_diff_rows.build import methods,operand,symbol
from tools.local_asset_ready.remove_gate import exact_body,require
from dncil.cil.body.reader import read_method_body_from_bytes
import dnfile

BASE='cac89c00b7ad79e0a098a3108229cee94fd693bbf155fc64076c83ae25c36a47'
BASE_REV='ef0eaac'
MAIN='assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER='assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
ENCOUNTER_TARGETS={'Room.RoomNetField::SCSidePveFightStart','Room.RoomNetField::SCSidePveBossFightStart'}
TARGETS=ENCOUNTER_TARGETS|{'Copy.CopyManager::AwakeCopyToBattleScene','ManagerController::AwakeCopyToBattle'}
REFS=('Assembly-CSharp.dll','Assembly-CSharp-firstpass.dll','UnityEngine.dll','mscorlib.dll','System.dll','System.Core.dll','LitJson.dll','HYJS.Field63.dll','LocalFieldSettlement.dll')

def sha(raw): return hashlib.sha256(raw).hexdigest()
def file_sha(path):
    with Path(path).open('rb') as f: return hashlib.file_digest(f,'sha256').hexdigest()
def run(argv): subprocess.run(['rtk','proxy',*map(str,argv)],cwd=ROOT,check=True,timeout=150)
def blob(path): return subprocess.check_output(['rtk','proxy','git','show',BASE_REV+':'+str(path)],cwd=ROOT)

def verify_main(before,after):
    a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
    old,new=methods(a),methods(b)
    require(len(old)==len(new),'Method count changed')
    unchanged=engine=0; changed=[]
    for (name,rid,x),(nn,nr,y) in zip(old,new):
        require((name,rid,x.Signature.value,x.struct.Flags,x.struct.ImplFlags)==(nn,nr,y.Signature.value,y.struct.Flags,y.struct.ImplFlags),'Method ABI changed: '+name)
        if not x.Rva: require(not y.Rva,'Added native body'); continue
        left,right=exact_body(a,x),exact_body(b,y)
        if name not in TARGETS:
            require(left==right,'Unscoped original body change: '+name)
            unchanged+=1; engine+=name.startswith('Battle.Engine'); continue
        ox,ny=read_method_body_from_bytes(left),read_method_body_from_bytes(right)
        require(not ox.exception_handlers and not ny.exception_handlers,'Unexpected handler EH')
        if name=='Copy.CopyManager::AwakeCopyToBattleScene':
            require(len(ox.instructions)==len(ny.instructions),'Awake send shape differs')
            substitutions=0
            for u,v in zip(ox.instructions,ny.instructions):
                require(u.opcode.name==v.opcode.name,'Awake send opcode changed')
                if operand(u.operand)!=operand(v.operand):
                    require(u.opcode.name=='call' and symbol(a,u.operand)==symbol(b,v.operand)=='SendHttp','Unexpected Awake send change')
                    substitutions+=1
            require(substitutions==1,'Expected one Awake send wrapper'); changed.append(name); continue
        if name=='ManagerController::AwakeCopyToBattle':
            require([i.opcode.name for i in ny.instructions[:3]]==['call','brfalse','ret'] and symbol(b,ny.instructions[0].operand)=='BeforeStart','Awake duplicate guard differs')
            suffix=ny.instructions[3:]
            guards=[i for i in suffix if i.opcode.name=='call' and symbol(b,i.operand)=='Guard']
            require(len(guards)==1,'Expected original iterator wrapper')
            suffix=[i for i in suffix if i is not guards[0]]; original=ox.instructions
            require(ny.instructions[1].operand==suffix[0].offset,'Awake original entry differs')
        else:
            require([i.opcode.name for i in ny.instructions[:6]]==['ldc.i4','ldarg.0','ldarg.1','call','brfalse','ret'],'Unexpected encounter guard')
            require(symbol(b,ny.instructions[3].operand)=='DropFightStartResponse','Unexpected guard call')
            suffix=ny.instructions[6:]; original=ox.instructions[3:]
            require(ny.instructions[4].operand==suffix[0].offset,'Original handler entry changed')
        require(len(original)==len(suffix),'Original handler suffix changed')
        oi={i.offset:k for k,i in enumerate(original)}; ni={i.offset:k for k,i in enumerate(suffix)}
        for u,v in zip(original,suffix):
            require(u.opcode.name==v.opcode.name,'Original opcode changed')
            if u.opcode.flow_control.name in {'Branch','Cond_Branch'}: require(oi[u.operand]==ni[v.operand],'Original branch changed')
            else: require(operand(u.operand)==operand(v.operand),'Original operand changed')
        changed.append(name)
    require(engine==547 and set(changed)==TARGETS,'Engine/target scope differs')
    return {'unchangedOriginalBodies':unchanged,'originalEngineMethodsExact':engine,'changedMethods':sorted(changed),'originalHandlerSuffixesPreserved':True}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True); p.add_argument('--output-directory',type=Path,required=True)
    p.add_argument('--runtime',type=Path,default=Path('/Users/lirui/LR/hyjs/.tools/native-batch'))
    args=p.parse_args(); out=args.output_directory.resolve(); base=args.base_apk.resolve(); runtime=args.runtime.resolve()
    require(out.is_relative_to(ROOT) and not out.exists(),'Fresh project output required')
    require(file_sha(base)==BASE,'1.1.6 APK differs')
    here=ROOT/'tools/desert_extraction'
    names=sources(blob(Path('tools/desert_extraction/build_field_patch.py')).decode())
    changed_sources={'FieldRuntime.cs','ColdBattleRecovery.cs','BattleRecovery.cs'}
    for name in names:
        if name not in changed_sources: require((here/name).read_bytes()==blob(Path('tools/desert_extraction')/name),'Unscoped helper source change: '+name)
    out.mkdir(parents=True)
    dotnet=runtime/'dotnet/dotnet'; csc=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs=next((runtime/'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    with zipfile.ZipFile(base) as archive,tempfile.TemporaryDirectory(prefix='hyjs-performance117-') as temp:
        work=Path(temp); baseline=work/'baseline'; baseline.mkdir()
        references=[]
        for name in REFS:
            f=work/name; f.write_bytes(archive.read('assets/bin/Data/Managed/'+name)); references.append('/reference:'+str(f))
        old_main=archive.read(MAIN); old_helper=archive.read(HELPER)
        for name in names: (baseline/name).write_bytes(blob(Path('tools/desert_extraction')/name))
        def compile_helper(dest,paths):
            run([dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+',
                '/out:'+str(dest),'/resource:'+str(here/'assets/evacuation-npc.png')+',HYJS.DesertV4.Evacuation.png',*references,*paths])
        shipped=baseline/'SoulFlowerDesertV4.dll'
        compile_helper(shipped,[baseline/name for name in names])
        require(shipped.read_bytes()==old_helper,'Source does not reproduce shipped helper')
        compile_helper(out/'SoulFlowerDesertV4.dll',[here/name for name in names]+[here/'EncounterStartRecovery.cs',here/'AwakeStartRecovery.cs'])
        folder=work/'injector'; folder.mkdir()
        (folder/'Program.cs').write_bytes(Path(__file__).with_name('InjectorEncounter117.cs').read_bytes())
        (folder/'Patch.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run([dotnet,'build',folder/'Patch.csproj','--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run([dotnet,folder/'bin/Debug/net8.0/Patch.dll',work/'Assembly-CSharp.dll',out/'Assembly-CSharp.dll'])
        checks=verify_main(old_main,(out/'Assembly-CSharp.dll').read_bytes())
        dex=sha(archive.read('classes2.dex'))
    source_paths=[Path(__file__).resolve(),Path(__file__).with_name('InjectorEncounter117.cs'),here/'build_field_patch.py',here/'EncounterStartRecovery.cs',here/'AwakeStartRecovery.cs',here/'assets/evacuation-npc.png',*[here/name for name in names]]
    payload={MAIN:str((out/'Assembly-CSharp.dll').relative_to(ROOT)),HELPER:str((out/'SoulFlowerDesertV4.dll').relative_to(ROOT))}
    proof={'schema':'hyjs-performance117-managed-v1','baseAPKSHA256':BASE,'inputManagedSHA256':sha(old_main),'inputDexSHA256':dex,'managedOnly':True,
        'inheritedJavaByteExact':True,'inheritedJavaPublicProtectedABIExact':True,'managedAssemblyByteExact':False,
        'allowedChangedMethods':sorted(TARGETS),'payloadFiles':payload,'replacementEntries':{k:file_sha(ROOT/v) for k,v in payload.items()},
        'sourcePins':{str(f.relative_to(ROOT)):file_sha(f) for f in source_paths},'baselineReproducedByteForByte':True,'shippedHelperSHA256':sha(old_helper),'checks':checks,'deviceVerified':False}
    (out/'manifest.json').write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'status':'STATIC_VERIFIED_NOT_INSTALLED','checks':checks,'output':str(out)}))

if __name__=='__main__': main()
