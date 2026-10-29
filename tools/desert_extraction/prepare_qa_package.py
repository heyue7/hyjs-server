"""Create a loopback-only derivative of one pinned desert QA overlay."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body,require
from tools.client_remote import build as remote
from tools.desert_extraction import package_resources,build_v78_adapter

ORIGIN='http://127.0.0.1:28979'
ENTRY='assets/bin/Data/Managed/Assembly-CSharp.dll'
def sha(raw): return hashlib.sha256(raw).hexdigest()
def run(command):
    r=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,text=True,capture_output=True,timeout=120)
    require(r.returncode==0,'QA origin patch failed: '+r.stderr[-1500:]);return r.stdout

def check_sources(pins):
    for name,digest in pins.items():
        f=(ROOT/name).resolve();require(f.is_relative_to(ROOT) and f.is_file() and sha(f.read_bytes())==digest,'Source pin changed: '+name)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--source',type=Path,required=True)
    p.add_argument('--runtime',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    a=p.parse_args();base,source,runtime,out=(x.resolve() for x in (a.base_apk,a.source,a.runtime,a.output))
    require(package_resources.sha_file(base)==package_resources.BASE_SHA,'QA base differs')
    require(source.is_relative_to(ROOT/'analysis/desert-extraction-v4-client') and out.is_relative_to(ROOT/'analysis/desert-extraction-v4-client') and not out.exists(),'Fresh owned QA output required')
    manifest=json.loads((source/'manifest.json').read_text());resource=json.loads((source/'resource-manifest.json').read_text())
    require(manifest.get('managedOnly') is True and manifest.get('qaOnly') is True and manifest.get('neverPromote') is True and manifest.get('qualityFilterV80Exact') is True and manifest.get('baseAPKSHA256')==package_resources.BASE_SHA,'Source QA candidate differs')
    check_sources(manifest['sourcePins']); oldpayload={}; oldpins={}; package_resources.apply_overlay(base,source,oldpayload,oldpins)
    with zipfile.ZipFile(base) as z:
        original=z.read(ENTRY); old_origin=json.loads(z.read('assets/local-wallet.json'))['origin'].rstrip('/'); shop_helper=z.read('assets/bin/Data/Managed/SoulFlowerGGOShop.dll')
    require(old_origin and old_origin!=ORIGIN,'Unexpected base origin')
    candidate=(ROOT/manifest['payloadFiles'][ENTRY]).resolve();before=candidate.read_bytes()
    require(candidate.is_relative_to(source) and sha(before)==manifest['outputManagedSHA256'],'Field candidate managed SHA differs')
    specs={name:(old.replace('http://127.0.0.1:8088',old_origin),new) for name,(old,new) in remote.targets_for(ORIGIN).items()}
    injector=ROOT/'tools/client_remote/Injector.cs';runtime_dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-desert-qa-origin-') as temporary:
        work=Path(temporary);(work/'before.dll').write_bytes(before)
        (work/'Program.cs').write_text(injector.read_text().replace('http://127.0.0.1:8088',old_origin))
        project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run([runtime_dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run([runtime_dotnet,work/'bin/Debug/net8.0/Patch.dll',work/'before.dll',ORIGIN,work/'after.dll'])
        after=(work/'after.dll').read_bytes()
    remote.targets_for=lambda value:specs
    checked=remote.verify(before,after,ORIGIN)
    require(checked['originalEngineMethodsUnchanged']==547 and checked['onlyFourHTTPOriginLiteralsChanged'],'QA origin scope differs')
    left,right=dnfile.dnPE(data=original),dnfile.dnPE(data=after);old,new=methods(left),methods(right);changed=[];engine=0;quality=False
    require(len(old)==len(new),'QA method inventory differs')
    for (name,rid,m),(other,rid2,n) in zip(old,new):
        require((name,rid,m.Signature.value,m.struct.Flags,m.struct.ImplFlags)==(other,rid2,n.Signature.value,n.struct.Flags,n.struct.ImplFlags),'QA method ABI differs')
        if not m.Rva:continue
        if name==build_v78_adapter.QUALITY_METHOD:
            require(sha(exact_body(left,m))==build_v78_adapter.QUALITY_PIN and exact_body(left,m)==exact_body(right,n),'Released v80 quality filter differs')
            quality=True
        if exact_body(left,m)!=exact_body(right,n): changed.append(name)
        elif name.startswith('Battle.Engine'):engine+=1
    require(set(changed)==set(manifest['allowedChangedMethods'])|set(specs) and engine==547 and quality,'Combined QA method boundary or quality differs')
    for name,_,method in new:
        if not method.Rva:continue
        for inst in read_method_body_from_bytes(exact_body(right,method)).instructions:
            if inst.opcode.name=='ldstr':
                value=right.net.user_strings.get(inst.operand.value&0xffffff).value
                require(old_origin not in value,'Executable production origin remains in '+name)
    helper_audit={}
    for key,raw in [('map',(source/'SoulFlowerDesert53.dll').read_bytes()),('v4',(source/'SoulFlowerDesertV4.dll').read_bytes()),('shop',shop_helper)]:
        pe=dnfile.dnPE(data=raw)
        for name,_,method in methods(pe):
            if not method.Rva:continue
            for inst in read_method_body_from_bytes(exact_body(pe,method)).instructions:
                if inst.opcode.name!='ldstr':continue
                value=pe.net.user_strings.get(inst.operand.value&0xffffff).value
                require(old_origin not in value and '://' not in value,'Executable helper has absolute origin: '+key+' '+name)
        helper_audit[key]=sha(raw)
    field_wire=(ROOT/'tools/desert_extraction/FieldWire.cs').read_text()
    pvp=(ROOT/'tools/desert_extraction/PvpBridge.cs').read_text()
    require('ServerConfig.GetServerUrl()' in field_wire and 'new WWW(session.origin + path' in field_wire and 'new WWW(owner.origin+"/sao/"' in pvp,'Desert transport origin binding differs')
    sources=dict(manifest['sourcePins'])
    for f in (Path(__file__).resolve(),injector,ROOT/'tools/client_remote/build.py',ROOT/'tools/client_remote/config.py'):
        sources[str(f.relative_to(ROOT))]=sha(f.read_bytes())
    check_sources(sources);out.mkdir(parents=True)
    files={}
    for entry,path in manifest['payloadFiles'].items():
        f=(ROOT/path).resolve();require(f.is_relative_to(source) and sha(f.read_bytes())==manifest['replacementEntries'][entry],'QA managed source changed')
        raw=after if entry==ENTRY else f.read_bytes();dest=out/Path(entry).name;dest.write_bytes(raw);files[entry]=str(dest.relative_to(ROOT))
    resources={}
    for entry,path in resource['payloadFiles'].items():
        f=(ROOT/path).resolve();require(f.is_relative_to(source) and sha(f.read_bytes())==resource['replacementEntries'][entry],'QA resource source changed')
        dest=out/Path(entry).name;dest.write_bytes(f.read_bytes());resources[entry]=str(dest.relative_to(ROOT))
    proof=dict(manifest);proof.update(outputManagedSHA256=sha(after),payloadFiles=files,allowedChangedMethods=sorted(changed),sourcePins=sources,
        replacementEntries={entry:sha((ROOT/path).read_bytes()) for entry,path in files.items()},qaOrigin=ORIGIN,qaOnly=True,neverPromote=True,
        qaOriginProof={'onlyFourHTTPOriginLiteralsChanged':True,'noExecutableProductionOriginRemaining':True,'originalEngineMethodsUnchanged':engine,
                       'desertHelperUsesServerConfigOrigin':True,'helperNoAbsoluteOrigin':helper_audit,'qualityFilterV80Exact':quality,'changedMethods':sorted(specs)})
    (out/'manifest.json').write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n')
    resource.update(payloadFiles=resources,sourcePins=sources)
    (out/'resource-manifest.json').write_text(json.dumps(resource,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'output':str(out),'managedSHA256':sha(after),'qaOrigin':ORIGIN,'allowedMethods':len(changed),'originalEngineMethods':engine,'qaOnly':True}))
if __name__=='__main__':main()
