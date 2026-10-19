"""Compose the original formation model-load completion refresh; no APK packaging."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import dnfile

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body,require
SOURCE=Path(__file__).resolve().parent
PINS=json.loads((SOURCE/'target-pins.json').read_text())

def sha(raw):return hashlib.sha256(raw).hexdigest()

def verify(before,after):
    a,b=[dnfile.dnPE(data=x) for x in [before,after]]
    for table in ['TypeDef','Field','MethodDef','AssemblyRef','MemberRef']:
        require(len(getattr(a.net.mdtables,table).rows)==len(getattr(b.net.mdtables,table).rows),'Metadata inventory changed: '+table)
    require(not getattr(b.net.mdtables,'FieldPtr',None),'Unexpected FieldPtr')
    require(a.net.mdtables.Module.rows[0].Mvid.value==b.net.mdtables.Module.rows[0].Mvid.value,'MVID changed')
    require(a.net.struct.Flags==b.net.struct.Flags and a.net.struct.StrongNameSignatureSize==b.net.struct.StrongNameSignatureSize==0,'CLI identity changed')
    left,right=methods(a),methods(b)
    names={rid:name for name,rid,row in left}
    changes=[];engine=unchanged=0
    for (name,rid,old),(nn,nr,new) in zip(left,right):
        require((name,rid)==(nn,nr) and old.Signature.value==new.Signature.value and old.struct.Flags==new.struct.Flags and old.struct.ImplFlags==new.struct.ImplFlags,'Original ABI changed: '+name)
        if not old.Rva:
            require(not new.Rva,'Unexpected body');continue
        x,y=exact_body(a,old),exact_body(b,new)
        if name not in PINS:
            require(x==y,'Non-target raw body changed: '+name)
            unchanged+=1;engine+=name.startswith('Battle.Engine');continue
        require(sha(x) in PINS[name],'Original target pin differs: '+name)
        ob,oc=decoded(a,old);nb,nc=decoded(b,new)
        require(not ob.exception_handlers and not nb.exception_handlers,'Unexpected target EH')
        require(len(nc)==len(oc)+6,'Wrong instruction count')
        sites=[i for i,(op,arg) in enumerate(nc) if op=='call' and arg>>24==6 and names.get(arg&0xffffff)=='UI.UIFormationPopupPage::OnTabClick']
        require(len(sites)==1,'Wrong original refresh callback')
        end=sites[0];drop=set(range(end-5,end+1))
        require([op for op,arg in nc[end-5:end+1]]==['ldarg.0','ldfld','ldarg.0','ldfld','ldfld','call'],'Wrong selected-tab refresh prefix')
        indexes={i:i-sum(x<i for x in drop) for i in range(len(nc)) if i not in drop}
        normal=[]
        for i,(op,arg) in enumerate(nc):
            if i in drop:continue
            if op=='switch':arg=tuple(indexes[v] for v in arg)
            elif op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave')) and isinstance(arg,int):arg=indexes[arg]
            normal.append((op,arg))
        require(normal==oc,'Original coroutine flow changed')
        changes.append(dict(method=name,token=hex(0x6000000|rid),beforeSHA256=sha(x),afterSHA256=sha(y)))
    require(len(changes)==1 and engine==547,'Wrong target/engine count')
    return dict(changes=changes,unrelatedRawMethodBodiesUnchanged=unchanged,originalEngineMethodsUnchanged=engine,originalMetadataABIAndTokensPreserved=True,noNewAssemblyReferences=True)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--input-managed',type=Path,required=True)
    p.add_argument('--input-managed-sha256',required=True)
    p.add_argument('--output-directory',type=Path,required=True)
    args=p.parse_args();before=args.input_managed.read_bytes()
    require(sha(before)==args.input_managed_sha256,'Input SHA differs')
    out=args.output_directory.resolve();require(out.is_relative_to(ROOT/'analysis') and not out.exists(),'Use fresh analysis directory');out.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-formation-visuals-') as temp:
        work=Path(temp);host=work/'Assembly-CSharp.dll';host.write_bytes(before)
        shutil.copy2(SOURCE/'Injector.cs',work/'Program.cs')
        project=work/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        for name,command in [('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']),('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,out/'Assembly-CSharp.dll'])]:
            result=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90)
            (out/(name+'.log')).write_text(result.stdout+result.stderr)
            require(result.returncode==0,name+' failed; inspect saved log')
    after=(out/'Assembly-CSharp.dll').read_bytes()
    manifest=dict(schema='hyjs-client-formation-loaded-tab-refresh-v1',status='STATIC_VERIFIED_NOT_PACKAGED',inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),sourcePins={str(x.relative_to(ROOT)):sha(x.read_bytes()) for x in [SOURCE/'build.py',SOURCE/'Injector.cs',SOURCE/'target-pins.json']},deviceExecuted=False,apkBuilt=False,noNewHelperAssembly=True,replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll'],**verify(before,after))
    (out/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:manifest[k] for k in ['status','inputManagedSHA256','outputManagedSHA256','originalEngineMethodsUnchanged']}))

if __name__=='__main__':main()
