"""Original-frame/game-font announcement overlay; no APK or phone mutation."""
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
TARGETS = json.loads((SOURCE / 'target-pins.json').read_text())
BASE = '5b6ba3af357c36f7e81d7ad82f4ba31157eda6d84dc6ec8f18a1314a3604c8f4'


def sha(raw): return hashlib.sha256(raw).hexdigest()


def bridge_name(pe, token):
    require(token >> 24 == 10, 'Expected helper MemberRef')
    row = pe.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
    require(str(row.Class.row.TypeNamespace) == 'HYJS.Notices' and str(row.Class.row.TypeName) == 'OriginalNotices', 'Wrong helper')
    return str(row.Name)


def verify(before, after, input_sha):
    require(sha(before) == input_sha, 'Exact input required')
    a, b = [dnfile.dnPE(data=raw) for raw in [before, after]]
    old_methods, new_methods = methods(a), methods(b)
    require(len(old_methods) == len(new_methods), 'Original method inventory changed')
    for name in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, name).rows) == len(getattr(b.net.mdtables, name).rows), 'Original inventory changed: ' + name)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Unexpected FieldPtr')
    def identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion, row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)
    require([identity(r) for r in a.net.mdtables.Assembly.rows] == [identity(r) for r in b.net.mdtables.Assembly.rows], 'Original assembly identity changed')
    old_refs, new_refs = a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows
    require([identity(r) for r in old_refs] == [identity(r) for r in new_refs[:len(old_refs)]], 'Original references changed')
    require(len(new_refs) == len(old_refs) + 1 and str(new_refs[-1].Name) == 'SoulFlowerNotices' and new_refs[-1].struct.Flags == 0 and new_refs[-1].PublicKey.value == b'', 'Wrong helper assembly token')
    for r in new_refs:
        require(not(r.struct.Flags & 1) or bool(r.PublicKey.value), 'Empty full PublicKey: ' + str(r.Name))
    require(a.net.struct.Flags == b.net.struct.Flags and a.net.struct.StrongNameSignatureSize == b.net.struct.StrongNameSignatureSize == 0, 'CLI/strong-name flags changed')
    changes = []
    engine = unchanged = 0
    for (name, rid, old), (nn, nrid, new) in zip(old_methods, new_methods):
        require((name, rid) == (nn, nrid) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags, 'Original ABI/token changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Unexpected method body')
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name not in TARGETS:
            require(x == y, 'Non-target raw method changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(x) == TARGETS[name], 'Original target pin differs: ' + name)
        ob, oc = decoded(a, old)
        nb, nc = decoded(b, new)
        dropped = set()
        if name.endswith('::DoClose') or name.endswith('::Close'):
            require(len(nc) == len(oc) + 2 and nc[0][0] == 'ldarg.0' and nc[1][0] == 'call', 'Wrong close prefix')
            require(bridge_name(b, nc[1][1]) == ('HideAnnouncement' if name.startswith('UI.') else 'HideSystem'), 'Wrong close helper')
            dropped = {0, 1}
        elif name == 'SystemAnnouncePage::Show':
            require(len(nc) == len(oc) + 1, 'Wrong system show instruction count')
            indexes = [i for i, (op, token) in enumerate(nc) if op == 'call' and token >> 24 == 10 and str(b.net.mdtables.MemberRef.rows[(token & 0xffffff)-1].Class.row.TypeName) == 'OriginalNotices']
            require(len(indexes) == 1 and bridge_name(b, nc[indexes[0]][1]) == 'ShowSystem' and nc[indexes[0]-1][0] == 'ldarg.0', 'Wrong system show helper')
            dropped = {indexes[0] - 1}
        else:
            require(len(nc) == len(oc), 'Callback instruction count changed')
        def index(value): return value - sum(v < value for v in dropped)
        normalized = []
        for i, (op, value) in enumerate(nc):
            if i in dropped: continue
            if op == 'switch': value = tuple(index(v) for v in value)
            elif op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave')) and isinstance(value,int): value = index(value)
            normalized.append((op,value))
        differences = [i for i in range(len(oc)) if oc[i] != normalized[i]]
        if dropped == {0, 1}:
            require(not differences, 'Original close flow changed')
        else:
            require(len(differences) == 1, 'Expected one webview call substitution')
            i = differences[0]
            require(oc[i][0] == normalized[i][0] == 'call', 'Wrong webview opcode')
            require(bridge_name(b, normalized[i][1]) == ('ShowSystem' if name == 'SystemAnnouncePage::Show' else 'ShowAnnouncement'), 'Wrong webview replacement')
        require(len(ob.exception_handlers) == len(nb.exception_handlers), 'Original EH count changed')
        oi = {v.offset:i for i,v in enumerate(ob.instructions)}
        ni = {v.offset:index(i) for i,v in enumerate(nb.instructions)}
        oi[ob.instructions[-1].offset+ob.instructions[-1].size] = len(oc)
        ni[nb.instructions[-1].offset+nb.instructions[-1].size] = len(oc)
        for oe,ne in zip(ob.exception_handlers,nb.exception_handlers):
            for key in ['try_start','try_end','handler_start','handler_end']:
                require(oi[getattr(oe,key)] == ni[getattr(ne,key)], 'EH boundary changed')
        changes.append(dict(method=name,beforeSHA256=sha(x),afterSHA256=sha(y)))
    require(engine == 547 and len(changes) == 4, 'Engine/target count differs')
    return dict(changes=changes,originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,originalMetadataABIAndTokensPreserved=True,originalAssemblyIdentityAndReferencesPreserved=True,addedAssemblyReferencePublicKeyFlags=0)


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,default=ROOT/'apks/build/hyjs-local.apk')
    p.add_argument('--input-managed',type=Path,required=True)
    p.add_argument('--input-managed-sha256',default=BASE)
    p.add_argument('--output-directory',type=Path,required=True)
    args=p.parse_args(); before=args.input_managed.read_bytes()
    require(sha(before)==args.input_managed_sha256,'Exact input required')
    out=args.output_directory.resolve(); require(not out.exists(),'Use fresh output');out.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name,command):
        result=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90)
        (out/(name+'.log')).write_text(result.stdout+result.stderr)
        require(result.returncode==0,name+' failed; inspect saved log')
    with tempfile.TemporaryDirectory(prefix='hyjs-original-notices-') as temporary:
        work=Path(temporary);host=work/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
        with zipfile.ZipFile(args.base_apk) as archive:
            for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
                target=work/name;target.write_bytes(archive.read('assets/bin/Data/Managed/'+name));refs.append(target)
        bridge=out/'SoulFlowerNotices.dll'
        run('compile-bridge',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(bridge),*['/reference:'+str(r) for r in refs],SOURCE/'OriginalNotices.cs'])
        shutil.copy2(SOURCE/'Injector.cs',work/'Program.cs');project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,bridge,out/'Assembly-CSharp.dll'])
        consumer=work/'MetadataConsumer.cs';consumer.write_text('public static class MetadataConsumer { public static UI.AnnouncePage Keep(UI.AnnouncePage page) { return page; } }')
        run('compile-metadata-consumer',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/out:'+str(work/'Consumer.dll'),'/reference:'+str(out/'Assembly-CSharp.dll'),*['/reference:'+str(r) for r in refs[1:]],consumer])
    after=(out/'Assembly-CSharp.dll').read_bytes();checks=verify(before,after,args.input_managed_sha256)
    bridge_metadata=dnfile.dnPE(data=bridge.read_bytes())
    bridge_calls=[str(row.Class.row.TypeName)+'::'+str(row.Name) for row in bridge_metadata.net.mdtables.MemberRef.rows if hasattr(row.Class.row,'TypeName')]
    require('UILabel::get_bitmapFont' in bridge_calls and 'UILabel::set_bitmapFont' in bridge_calls and 'UILabel::set_trueTypeFont' in bridge_calls,'Original NGUI font clone contract missing')
    require(not any(name.startswith(('GUI::','GUIStyle::','AndroidJava')) for name in bridge_calls),'Unexpected IMGUI or Android content renderer')
    report=dict(schema='hyjs-original-frame-ngui-font-notices-v1',status='STATIC_VERIFIED_NOT_PACKAGED',inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),bridgeSHA256=sha(bridge.read_bytes()),oldProfileCompilerAcceptedOutputAsReference=True,gameFontSource='original announcement UILabel bitmapFont/trueTypeFont references',nguiFontCloneCallsVerified=True,imguiOrAndroidContentRenderer=False,originalFramesAndCloseFlowPreserved=True,androidJavaModified=False,deviceExecuted=False,
        replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll','assets/bin/Data/Managed/SoulFlowerNotices.dll'],sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in [SOURCE/'build.py',SOURCE/'Injector.cs',SOURCE/'OriginalNotices.cs',SOURCE/'target-pins.json']},**checks)
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({k:report[k] for k in ['status','outputManagedSHA256','bridgeSHA256','unrelatedRawMethodBodiesUnchanged','originalEngineMethodsUnchanged']}))


if __name__=='__main__':main()
