"""Rebuild and verify the narrow desert/expedition client overlay."""
import argparse
import ast
import hashlib
import json
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT=Path(__file__).resolve().parents[2]
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body

BASE_APK='9206c18fbb10080d3476a451fb3e9e24d591d0f769086490ea434afc278d4ae1'
BASE_MAIN='d7e48841b355d19332a986355dc88f3a5dce93c722e610a073fe589980429e87'
BASE_HELPER='135341b579796695a88033af183e7d11ae831a7e4f3a6d84a3dcc714cc6aa58f'
BASE_REV='ca2f669fc36c43d77082030d1d246795a5f281f4'
MAIN='assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER='assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
TARGET='UI.SelectPlayerPanel::getHeroList'
REFS=('Assembly-CSharp.dll','Assembly-CSharp-firstpass.dll','UnityEngine.dll','mscorlib.dll','System.dll',
      'System.Core.dll','LitJson.dll','HYJS.Field63.dll','LocalFieldSettlement.dll')

def sha(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''): h.update(chunk)
    return h.hexdigest()

def sources(raw):
    tree=ast.parse(raw)
    return next(ast.literal_eval(n.value) for n in tree.body if isinstance(n,ast.Assign) and
                any(isinstance(t,ast.Name) and t.id=='SOURCES' for t in n.targets))

def run(command):
    subprocess.run(['rtk','proxy',*map(str,command)],check=True,cwd=ROOT,timeout=150)

def git_blob(path):
    return subprocess.check_output(['rtk','proxy','git','show',BASE_REV+':'+str(path)],cwd=ROOT)

def verify_main(old_raw,new_raw):
    old,new=(dnfile.dnPE(data=b) for b in (old_raw,new_raw))
    assert not getattr(new.net.mdtables,'FieldPtr',None) and not getattr(new.net.mdtables,'MethodPtr',None)
    oldrows,newrows=methods(old),methods(new)
    assert len(oldrows)==len(newrows)
    unchanged=engine=0; targets=0
    for (name,rid,a),(other,newrid,b) in zip(oldrows,newrows):
        assert (name,rid,a.Signature.value,a.struct.Flags,a.struct.ImplFlags)==(other,newrid,b.Signature.value,b.struct.Flags,b.struct.ImplFlags),name
        if not a.Rva:
            assert not b.Rva
            continue
        left,right=exact_body(old,a),exact_body(new,b)
        if name!=TARGET:
            assert left==right,'Unexpected original method change: '+name
            unchanged+=1; engine+=name.startswith('Battle.Engine')
            continue
        targets+=1
        before,after=read_method_body_from_bytes(left),read_method_body_from_bytes(right)
        assert not before.exception_handlers and not after.exception_handlers and len(before.instructions)==len(after.instructions)
        old_call=next(i for i in before.instructions if i.opcode.name=='call' and getattr(i.operand,'value',0)==0x06003df4)
        index=before.instructions.index(old_call); new_call=after.instructions[index]
        assert new_call.opcode.name=='call'
        token=new_call.operand.value
        assert token>>24==10
        ref=new.net.mdtables.MemberRef.rows[(token&0xffffff)-1]
        assert str(ref.Name)=='Send' and str(ref.Class.row.TypeNamespace)=='HYJS.DesertV4' and str(ref.Class.row.TypeName)=='ExpeditionHeroList'
        needle=b'\x28'+struct.pack('<I',0x06003df4)
        assert left.count(needle)==1
        at=left.index(needle)
        assert right==left[:at]+b'\x28'+struct.pack('<I',token)+left[at+5:]
    assert targets==1 and engine==547
    return {'oneOriginalCallOperandChanged':True,'unchangedMethods':unchanged,'unchangedEngineMethods':engine}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk',type=Path,required=True)
    parser.add_argument('--output-directory',type=Path,required=True)
    parser.add_argument('--runtime',type=Path,default=Path('/Users/lirui/LR/hyjs/.tools/native-batch'))
    args=parser.parse_args()
    base=args.base_apk.resolve(); out=args.output_directory.resolve(); runtime=args.runtime.resolve()
    assert out.is_relative_to(ROOT/'analysis') and not out.exists()
    assert sha(base)==BASE_APK
    out.mkdir(parents=True)
    dotnet=runtime/'dotnet/dotnet'; csc=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs=next((runtime/'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    modern=['/reference:'+str(p) for p in sorted(refs.glob('*.dll'))]
    current=sources((HERE/'build_field_patch.py').read_text())
    original=sources(git_blob(Path('tools/desert_extraction/build_field_patch.py')).decode())
    assert current[:len(original)]==original and current[len(original):]==('TerminalCreditQuote.cs','FieldMonsterPosition.cs','ExpeditionHeroList.cs')
    with zipfile.ZipFile(base) as apk,tempfile.TemporaryDirectory(prefix='hyjs-clientfix-build-') as temporary:
        work=Path(temporary); old_src=work/'old-src'; old_src.mkdir(); references=[]
        old_main=apk.read(MAIN); old_helper=apk.read(HELPER); dex=apk.read('classes2.dex')
        assert hashlib.sha256(old_main).hexdigest()==BASE_MAIN and hashlib.sha256(old_helper).hexdigest()==BASE_HELPER
        for name in REFS:
            p=work/name; p.write_bytes(apk.read('assets/bin/Data/Managed/'+name)); references.append('/reference:'+str(p))
        for name in original:
            (old_src/name).write_bytes(git_blob(Path('tools/desert_extraction')/name))
        previous=work/'baseline'; previous.mkdir(); old_dll=previous/'SoulFlowerDesertV4.dll'
        def compile_helper(dest, paths):
            run([dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+',
                 '/out:'+str(dest),'/resource:'+str(HERE/'assets/evacuation-npc.png')+',HYJS.DesertV4.Evacuation.png',
                 *references,*paths])
        compile_helper(old_dll,[old_src/name for name in original])
        assert sha(old_dll)==BASE_HELPER,'Baseline source did not reproduce shipped helper'
        candidate=out/'SoulFlowerDesertV4.dll'
        compile_helper(candidate,[HERE/name for name in current])
        shutil.copy2(dnlib,work/'dnlib.dll')
        config=json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':refs.parents[1].name}}})
        scope=work/'Scope.dll'; (work/'Scope.runtimeconfig.json').write_text(config)
        run([dotnet,csc,'/nologo','/target:exe','/nostdlib+','/out:'+str(scope),*modern,'/reference:'+str(dnlib),HERE/'BridgeScopeChecks.cs'])
        run([dotnet,scope,old_dll,candidate])
        injector=work/'Injector.dll'; (work/'Injector.runtimeconfig.json').write_text(config)
        run([dotnet,csc,'/nologo','/target:exe','/nostdlib+','/out:'+str(injector),*modern,'/reference:'+str(dnlib),HERE/'ExpeditionInjector.cs'])
        output_main=out/'Assembly-CSharp.dll'
        run([dotnet,injector,work/'Assembly-CSharp.dll',output_main])
        proof=verify_main(old_main,output_main.read_bytes())
    payload={MAIN:str(output_main.relative_to(ROOT)),HELPER:str(candidate.relative_to(ROOT))}
    pinned=[HERE/'build_field_patch.py',HERE/'BridgeScopeChecks.cs',HERE/'ExpeditionInjector.cs',Path(__file__),HERE/'assets/evacuation-npc.png',
            HERE/'TerminalCreditQuoteChecks.cs',HERE/'FieldMonsterPositionChecks.cs',HERE/'ExpeditionHeroListChecks.cs',
            HERE/'run_monster_position_checks.py',HERE/'run_expedition_hero_list_checks.py',*[HERE/name for name in current]]
    manifest={'schema':'hyjs-clientfix-20261101-managed-v1','status':'STATIC_VERIFIED_NOT_INSTALLED',
        'baseAPKSHA256':BASE_APK,'inputManagedSHA256':BASE_MAIN,'inputDexSHA256':hashlib.sha256(dex).hexdigest(),
        'inheritedJavaPublicProtectedABIExact':True,'inheritedJavaByteExact':True,'managedOnly':True,
        'managedAssemblyByteExact':False,'allowedChangedMethods':[TARGET],
        'payloadFiles':payload,'replacementEntries':{MAIN:sha(output_main),HELPER:sha(candidate)},
        'oldBridgeSHA256':BASE_HELPER,'baselineReproducedByteForByte':True,'existingHelperPublicABIExact':True,
        'mainProof':proof,'sourcePins':{str(p.relative_to(ROOT)):sha(p) for p in pinned},'deviceExecuted':False}
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(json.dumps({'mainSHA256':sha(output_main),'helperSHA256':sha(candidate),'mainProof':proof,'deviceExecuted':False}))

if __name__=='__main__': main()
