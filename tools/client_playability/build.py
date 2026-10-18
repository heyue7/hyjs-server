"""Build three-method playability overlay, skip UI and formation bridge, without packaging or device actions."""
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
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require

ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
BASE = 'b9825732e3aa82714483645b0b2d87431bc56a58992d32c1c0ca637e01d941af'
TARGETS = {
    'GuideManager::Awake': '17128e84794f429c175d02da177baca9520a18917e050ae0f9cd3f54c5eb3dfe',
    'GuideManager::StepClick': '4fdb453022b3d5dc16472b45fa96c11d3c6ccb808d73e3e71c73695825b8d1ad',
    'PlatformSDKCallBack::ChekOrderCallBack': '36d8ded770c5898a84fcdf9affc24e2c55a85104fe65bfb466cc0d67ac2de0d1',
}

def sha(raw): return hashlib.sha256(raw).hexdigest()

def decoded(pe, row):
    body = read_method_body_from_bytes(exact_body(pe, row))
    indexes = {i.offset: n for n, i in enumerate(body.instructions)}
    def value(i):
        if i.opcode.name == 'ldstr': return pe.net.user_strings.get(i.operand.value & 0xffffff).value
        if i.opcode.flow_control.name in {'Branch', 'Cond_Branch'}: return indexes[i.operand]
        return operand(i.operand)
    return body, [(i.opcode.name, value(i)) for i in body.instructions]

def verify(before, after):
    a, b = [dnfile.dnPE(data=x) for x in (before, after)]
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Inherited method inventory changed')
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Inherited metadata inventory changed: '+table)
    require(not getattr(b.net.mdtables,'FieldPtr',None), 'Old Unity must not gain FieldPtr')
    changed = []; unchanged = engine = 0
    for (name, rid, old), (newname, newrid, new) in zip(left, right):
        require((name, rid) == (newname, newrid) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags, 'Inherited ABI changed')
        if not old.Rva: require(not new.Rva, 'Bodyless method changed'); continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name not in TARGETS:
            require(x == y, 'Unrelated raw IL/header/EH changed: '+name)
            unchanged += 1; engine += name.startswith('Battle.Engine'); continue
        require(sha(x) == TARGETS[name], 'Target input hash mismatch: '+name)
        ob, oc = decoded(a, old); nb, nc = decoded(b, new)
        require(not ob.exception_handlers and not nb.exception_handlers, 'Unexpected target EH')
        if name == 'GuideManager::Awake':
            require(nc[0][0] == 'ldarg.0' and nc[1][0] == 'call' and nc[2:] == oc, 'Original Awake suffix changed')
        elif name == 'GuideManager::StepClick':
            count = len(nc)-len(oc)
            require(count == 27 and nc[count:] == oc, 'Original guide suffix changed')
            require([i for i in nc[:count] if i[0]=='ldstr'] == [('ldstr','59'),('ldstr','guide_copy_reward'),('ldstr','1')], 'Guide scope differs')
        else:
            # Deleting only the three popup instructions must preserve original
            # instruction order, operands, and all original branch destinations.
            start = next(n for n,i in enumerate(nc) if i == ('ldstr','购买成功'))
            require(nc[start+1][0]=='ldc.r4' and nc[start+1][1]==0 and nc[start+2][0]=='call', 'Unexpected notice sequence')
            filtered = nc[:start]+nc[start+3:]
            filtered = [(op, arg-3 if op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave')) and isinstance(arg,int) and arg>=start+3 else arg) for op,arg in filtered]
            require(filtered == oc, 'Original payment callback control flow changed')
        changed.append(dict(method=name, token=hex(0x06000000|rid), beforeSHA256=sha(x), afterSHA256=sha(y), addedInstructions=len(nc)-len(oc)))
    require(engine == 547 and len(changed)==3, 'Engine or target count differs')
    return dict(changes=changed, originalEngineMethodsUnchanged=engine, unrelatedOriginalRawMethodBodiesUnchanged=unchanged, noOriginalTypesMethodsOrFieldsAdded=True)

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=ROOT/'apks/build/hyjs-local.apk')
    parser.add_argument('--output-directory', type=Path, required=True)
    args=parser.parse_args(); out=args.output_directory.resolve()
    require(not out.exists(), 'Use fresh output directory')
    require(sha(args.base_apk.read_bytes())==BASE, 'Exact qualified remote base required')
    with zipfile.ZipFile(args.base_apk) as z: before=z.read(ENTRY)
    out.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch'; dotnet=runtime/'dotnet/dotnet'
    compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    injector=Path(__file__).with_name('Injector.cs')
    def run(name, command):
        r=subprocess.run(['rtk','proxy',*map(str,command)], cwd=ROOT, capture_output=True,text=True,timeout=90)
        (out/(name+'.log')).write_text(r.stdout+r.stderr)
        require(r.returncode==0, name+' failed; inspect saved log')
    with tempfile.TemporaryDirectory(prefix='hyjs-playability-') as tmp:
        folder=Path(tmp); current=folder/'Assembly-CSharp.dll'; current.write_bytes(before)
        shutil.copy2(injector,folder/'Program.cs')
        project=folder/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        output=out/'Assembly-CSharp.dll'
        run('patch',[dotnet,folder/'bin/Debug/net8.0/Patch.dll',current,output])
        references=[current if p.name=='Assembly-CSharp.dll' else p for p in sorted((ROOT/'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))]
        bridge=out/'LocalPlayability.dll'
        run('compile-skip-ui',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(bridge),*['/reference:'+str(p) for p in references],Path(__file__).with_name('NoviceSkipUI.cs')])
        formation=out/'LocalFormationUI.dll'
        run('compile-formation',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(formation),*['/reference:'+str(p) for p in references],ROOT/'tools/local_formation_ui/LocalFormationUI.cs'])
    checks=verify(before,output.read_bytes())
    result=dict(schema='hyjs-client-playability-overlay-v1',status='STATIC_VERIFIED_NOT_PACKAGED',baseAPKSHA256=BASE,inputManagedSHA256=sha(before),outputManagedSHA256=sha(output.read_bytes()),replacementEntries={ENTRY:sha(output.read_bytes()),'assets/bin/Data/Managed/LocalFormationUI.dll':sha(formation.read_bytes()),'assets/bin/Data/Managed/LocalPlayability.dll':sha(bridge.read_bytes())},**checks,paymentNoticeOnlyAfterPendingOrderAndProductAndServerSuccess=True,paymentSuccessStillClearsPendingIdentityBeforeNotice=True,noAdditionalPaymentRequests=True,skipRequestOnlyOnExplicitClick=True,guideOnlyStep59AfterFirstBoxAlreadyClaimed=True,formationMainRoleCanLeaveWithAtLeastOneRemaining=True,formationUnbindsWearablesWithoutDeletingInventory=True,skipEndpoint='/game-guide/skip',skipVisibleTopRight=True,skipOnlyStopsInitialTeachingAfterServerCommit=True,apkBuilt=False,deviceVerified=False,sourcePins={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),injector,Path(__file__).with_name('NoviceSkipUI.cs'),ROOT/'tools/local_formation_ui/LocalFormationUI.cs']})
    (out/'manifest.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(result,ensure_ascii=False,indent=2))

if __name__=='__main__': main()
