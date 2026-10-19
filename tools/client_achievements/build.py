"""Build the three pinned client bridges without packaging an APK."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body, require
SOURCE = Path(__file__).resolve().parent
TARGETS = json.loads((SOURCE/'target-pins.json').read_text())
BASE = '50c9781ac6bf61984a74cd4e7517519792bdb73a0778abcce48d8815f747c35e'

def sha(raw): return hashlib.sha256(raw).hexdigest()

def helper_call(pe, token, expected):
    require(token >> 24 == 10, 'Expected bridge MemberRef')
    row = pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1]
    require(str(row.Class.row.TypeNamespace) == 'HYJS.Achievements' and str(row.Class.row.TypeName) == 'AchievementsBridge' and str(row.Name) == expected, 'Wrong bridge call')

def normalize(code, indexes):
    def destination(value):
        require(value in indexes, 'Branch enters new bridge instructions')
        return indexes[value]
    result = []
    for i,(op,value) in enumerate(code):
        if i not in indexes: continue
        if op == 'switch': value = tuple(destination(v) for v in value)
        elif op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave')) and isinstance(value,int): value = destination(value)
        result.append((op,value))
    return result

def verify(before, after):
    a,b = (dnfile.dnPE(data=raw) for raw in (before,after))
    require(sha(before) == BASE, 'Exact managed input required')
    for table in ['TypeDef','Field','MethodDef']:
        require(len(getattr(a.net.mdtables,table).rows) == len(getattr(b.net.mdtables,table).rows), 'Inventory changed: '+table)
    require(not getattr(b.net.mdtables,'FieldPtr',None), 'Unexpected FieldPtr')
    for old,new in zip(a.net.mdtables.Field.rows,b.net.mdtables.Field.rows):
        require(str(old.Name)==str(new.Name) and old.Signature.value==new.Signature.value and old.struct.Flags==new.struct.Flags, 'Original field ABI changed')
    require(a.net.mdtables.Module.rows[0].Mvid.value == b.net.mdtables.Module.rows[0].Mvid.value, 'MVID changed')
    def identity(row):
        return (str(row.Name),str(row.Culture),row.struct.MajorVersion,row.struct.MinorVersion,row.struct.BuildNumber,row.struct.RevisionNumber,row.struct.Flags,row.PublicKey.value)
    require([identity(r) for r in a.net.mdtables.Assembly.rows] == [identity(r) for r in b.net.mdtables.Assembly.rows], 'Assembly identity changed')
    ar,br = a.net.mdtables.AssemblyRef.rows,b.net.mdtables.AssemblyRef.rows
    require([identity(r) for r in ar] == [identity(r) for r in br[:len(ar)]], 'Original references changed')
    require(len(br)==len(ar)+1 and str(br[-1].Name)=='SoulFlowerAchievements' and br[-1].struct.Flags==0 and br[-1].PublicKey.value==b'', 'Wrong helper assembly reference')
    require(all(not(r.struct.Flags&1) or bool(r.PublicKey.value) for r in br), 'Empty full public key')
    require(a.net.struct.Flags==b.net.struct.Flags and a.net.struct.StrongNameSignatureSize==b.net.struct.StrongNameSignatureSize==0, 'CLI identity changed')
    changes=[]; unchanged=engine=0
    for (name,rid,old),(nn,nrid,new) in zip(methods(a),methods(b)):
        require((name,rid)==(nn,nrid) and old.Signature.value==new.Signature.value and old.struct.Flags==new.struct.Flags and old.struct.ImplFlags==new.struct.ImplFlags, 'Method ABI changed: '+name)
        if not old.Rva:
            require(not new.Rva,'New body added');continue
        x,y=exact_body(a,old),exact_body(b,new)
        if name not in TARGETS:
            require(x==y,'Unrelated raw method changed: '+name)
            unchanged+=1;engine+=name.startswith('Battle.Engine');continue
        require(sha(x)==TARGETS[name], 'Target source pin differs: '+name)
        ob,oc=decoded(a,old);nb,nc=decoded(b,new)
        require(len(ob.exception_handlers)==len(nb.exception_handlers),'Target EH count changed')
        if name.endswith('::GetItemConfig'):
            require(len(nc)==len(oc)+17,'Wrong config prefix size')
            require(nc[0:2]==[('ldarg.1',None),('ldc.i4',37)] and nc[2]==('blt',17) and nc[3:5]==[('ldarg.1',None),('ldc.i4',41)] and nc[5]==('bgt',17),'Wrong type bounds')
            helper_call(b,nc[11][1],'Page')
            require(nc[16][0]=='ret','Missing new-type return')
            require(normalize(nc,{i:i-17 for i in range(17,len(nc))})==oc,'Original config altered')
            indexes={i:i-17 for i in range(17,len(nc))}
        elif name.endswith('::InitItemWithInfo'):
            require(oc[-1][0]=='ret' and sum(op=='ret' for op,v in oc)==1,'Unexpected Init returns')
            require(len(nc)==len(oc)+3 and [op for op,v in nc[-4:]]==['ldarg.0','ldarg.1','call','ret'],'Wrong display binding suffix')
            helper_call(b,nc[-2][1],'Bind')
            indexes={i:i for i in range(len(oc)-1)};indexes[len(nc)-4]=len(oc)-1
            require(normalize(nc,indexes)[:-1]==oc[:-1],'Original Init altered')
        else:
            require(len(oc)==len(nc),'Pet pool control flow changed')
            differences=[i for i in range(len(oc)) if oc[i]!=nc[i]]
            require(len(differences)==4 and all(oc[i][0]==nc[i][0]=='callvirt' for i in differences),'Expected four PetIcon substitutions')
            for i in differences:
                token=nc[i][1]
                if token>>24==0x2b:
                    spec=b.net.mdtables.MethodSpec.rows[(token&0xffffff)-1]
                    require(spec.Instantiation.value!=a.net.mdtables.MethodSpec.rows[(oc[i][1]&0xffffff)-1].Instantiation.value,'Pet getter did not change')
                else:
                    require(token>>24==6 and any(rid2==(token&0xffffff) and n.startswith('UI.PetIcon::') for n,rid2,m in methods(b)),'Wrong pet API')
            indexes={i:i for i in range(len(nc))}
        # dncil instruction offsets include the method header; EH boundaries
        # are code-relative. Preserve the original protected instruction ranges.
        oi={instruction.offset-ob.header_size:i for i,instruction in enumerate(ob.instructions)}
        ni={instruction.offset-nb.header_size:indexes[i] for i,instruction in enumerate(nb.instructions) if i in indexes}
        oi[ob.instructions[-1].offset+ob.instructions[-1].size-ob.header_size]=len(oc)
        ni[nb.instructions[-1].offset+nb.instructions[-1].size-nb.header_size]=len(oc)
        for oe,ne in zip(ob.exception_handlers,nb.exception_handlers):
            require(oe.exception_type==ne.exception_type and oe.catch_type==ne.catch_type,'Target EH type changed')
            for key in ['try_start','try_end','handler_start','handler_end']:
                require(oi[getattr(oe,key)]==ni[getattr(ne,key)],'Target EH boundary changed')
        changes.append(dict(method=name,token=hex(0x6000000|rid),beforeSHA256=sha(x),afterSHA256=sha(y)))
    require(engine==547 and len(changes)==3,'Wrong target/engine count')
    return dict(changes=changes,unrelatedRawMethodBodiesUnchanged=unchanged,originalEngineMethodsUnchanged=engine,originalMetadataABIAndTokensPreserved=True,originalAssemblyIdentityAndReferencesPreserved=True,addedAssemblyReferencePublicKeyFlags=0)

def write_manifest(before, out, checks):
    after=(out/'Assembly-CSharp.dll').read_bytes();bridge=out/'SoulFlowerAchievements.dll'
    report=dict(schema='hyjs-client-achievements-native-fragments-v1',status='STATIC_VERIFIED_NOT_PACKAGED',inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),bridgeSHA256=sha(bridge.read_bytes()),oldProfileCompilerAcceptedOutputAsReference=True,achievementTypes=[37,38,39,40,41],type41ID='canonicalID~loginDays~level~stages',originalRewardIDAndEligibilityPreserved=True,originalTargetEHBoundariesPreserved=True,deviceExecuted=False,replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll','assets/bin/Data/Managed/SoulFlowerAchievements.dll'],sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in [SOURCE/'build.py',SOURCE/'Injector.cs',SOURCE/'Achievements.cs',SOURCE/'target-pins.json']},**checks)
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    return report

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,default=ROOT/'apks/build/hyjs-local.apk')
    p.add_argument('--input-managed',type=Path,required=True)
    p.add_argument('--output-directory',type=Path,required=True)
    args=p.parse_args();before=args.input_managed.read_bytes();require(sha(before)==BASE,'Wrong input')
    out=args.output_directory.resolve();require(not out.exists(),'Fresh output required');out.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name,command):
        result=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90)
        (out/(name+'.log')).write_text(result.stdout+result.stderr);require(result.returncode==0,name+' failed; inspect log')
    with tempfile.TemporaryDirectory(prefix='hyjs-client-achievements-') as temporary:
        work=Path(temporary);host=work/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
        with zipfile.ZipFile(args.base_apk) as archive:
            require(sha(archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll'))==BASE,'APK main changed')
            for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
                target=work/name;target.write_bytes(archive.read('assets/bin/Data/Managed/'+name));refs.append(target)
        bridge=out/'SoulFlowerAchievements.dll'
        run('compile-bridge',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(bridge),*['/reference:'+str(r) for r in refs],SOURCE/'Achievements.cs'])
        shutil.copy2(SOURCE/'Injector.cs',work/'Program.cs');project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,bridge,out/'Assembly-CSharp.dll'])
        consumer=work/'Consumer.cs';consumer.write_text('public static class Consumer { public static AchievementItem Keep(AchievementItem item) { return item; } }')
        run('compile-metadata-consumer',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/out:'+str(work/'Consumer.dll'),'/reference:'+str(out/'Assembly-CSharp.dll'),*['/reference:'+str(r) for r in refs[1:]],consumer])
    after=(out/'Assembly-CSharp.dll').read_bytes();checks=verify(before,after)
    report=write_manifest(before,out,checks)
    print(json.dumps({k:report[k] for k in ['status','outputManagedSHA256','bridgeSHA256','unrelatedRawMethodBodiesUnchanged','originalEngineMethodsUnchanged']}))

if __name__=='__main__': main()
