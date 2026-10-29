"""Build a camera adapter and optional exact desert loader overlay for a pinned APK.

Does not package/install an APK. Entry selection requires explicit flags. The old camera ray and all
other original method bodies must remain byte-identical.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
import dnfile
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body, require

HERE = Path(__file__).resolve().parent
INJECTOR = HERE / 'Injector78.cs'
TARGET = 'CameraTool::SetTransRender'
PIN = '70552a0caa2d9866c10ab8dbd11b8f086484a0ead965cc1acc3f9978bc663324'
RAY_TARGET = 'CameraTool::Update'
RAY_PIN = '18b7963319d92fbad10e813a67533ea37c8d7b2dbf7c7e056f8b022f345ef54a'
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
BASE_APK_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
BASE_MANAGED_SHA = '8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437'
QUALITY_METHOD = 'UI.UIInstPlayerData::GetAvailableHerosForFormation'
QUALITY_PIN = '8c1cec4f57ef5a748045cf62308deb3e903e375f1a63579f55010c1c804952a5'
LOADER = 'AssetLoad.AssetBundleManager::'
DEPENDENCIES = {
    'LoadAssetBundle': ('f2bb2bd7c5fb3907504016364ab8213cb1b2bbdacbf4f4973136985eed8dea52', 45),
    'IsLoadedAssetBundle': ('f318030670b93760ce6352d467aa296bc374f6e1e55119c6cc86735b0dc515c6', 51),
    'UnloadAssetBundle': ('bfb90ba0769ea4e9478f2ac86b2bb36d2af7f532a29ed82ab433f93fb1acab31', 112),
    'UnloadBundleOnly': ('045ddae8bedfa37990835f84b3a4bf01d532c0906c154961f0813649ad89c0c0', 33),
}
PREFIXES = {
    'IsAssetExist': ('f7f06b73157801388b06b34fd9dcde71e43583400d9a936b92a996ed12833800', 5),
    'GetSize': ('248e26d6fbbbb509b13e70f48e00d794eb5feea01f48349fc653667efb39c597', 5),
    'GetFullPath': ('3318e4d951f3f403e69be79b44e3b005afd3d77a7fdd082fba81f18c225b51d0', 8),
}
WALK_GUARDS = {
    'Room.RoomNetField::SCFieldResourceInfo': '9d8077f762657e7ad09bde99c046ba6c6edadeb2268c80dde90d63e493edb679',
    'Room.RoomNetField::SCSideMonster': '339c4bcea7af7dba48e6c33eb5fcc9fc5646fca19d5c1a718467e539ca6cc778',
    'Room.RoomNetField::SCSideBoss': '9f953c730a13149cc8a0a773380a48dfbc41ad6964f583956fa62a96f0a0900b',
    'Room.RoomNetField::SCSideWorldBoss': 'a93385a71a6b546905600dd8cda117d425a8aec7a43e9c14d6a447bc421b809f',
    'Room.RoomBase::LoadNPCDataFromTable': '61cc89ecd196cdccddc937d0892dee8c2d2131d41c6c7b3607dc815887707d2c',
    'Room.RoomField::LoadSafeArea': '71e85c8c34e1b0940643870b60771905aa23fb460aba67e9861c8dba25224c4c',
}
ENTRY_GUARDS = {
    'UI.WorldMapPage::onMapClick': ('670960e06a6324b0fa0d658868563c15047929bad5797ea14302e042d42bed08',63,'2'),
    'UI.WorldMapPage::onTransfer': ('e9edd387fcf2a3707ac2ec2c76fa537bf3c5eaebbb7122cfc7991e0c245812ab',11,'field_003'),
    'ManagerController::TrySceneTransfer': ('d43df8ad23191cfc5c16a8723ffd10ecd57723d176ee880d8c7e6d49d28979e0',21,'field_003'),
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def sha_file(path):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''): h.update(block)
    return h.hexdigest()


def verify(before, after, bundle_size=None, walk_only=False, desert_entry=False, resume_login=False):
    require(not resume_login or desert_entry, 'Login resume requires desert entry')
    left, right = [dnfile.dnPE(data=x) for x in (before, after)]
    for table in ('TypeDef', 'Field', 'MethodDef'):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows), 'Original inventory changed: ' + table)
    require(not getattr(right.net.mdtables, 'FieldPtr', None), 'Unexpected old-engine FieldPtr')
    old_methods, new_methods = methods(left), methods(right)
    require(len(old_methods) == len(new_methods), 'Method count changed')
    unchanged = engine = 0
    target_verified = False
    quality_verified = False
    ray_verified = False
    loader_changes = []
    walk_changes = []
    entry_changes = []
    resume_changes = []
    def helper_call(instruction, expected):
        require(instruction[0] == 'call', 'Expected static helper call')
        ref = right.net.mdtables.MemberRef.rows[(instruction[1] & 0xffffff) - 1]
        require(str(ref.Name) == expected and str(ref.Class.row.TypeNamespace) == 'HYJS.Desert53' and
                str(ref.Class.row.TypeName) == 'DesertBundle', 'Wrong loader helper')
    def suffix(item, count):
        op, value = item
        if op == 'switch': return op, tuple(v - count for v in value)
        if op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) and isinstance(value, int):
            return op, value - count
        return item
    for (name, rid, old), (new_name, new_rid, new) in zip(old_methods, new_methods):
        require((name, rid) == (new_name, new_rid), 'Original token/name changed')
        require(old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags, 'Original ABI changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'New body: ' + name)
            continue
        a, b = exact_body(left, old), exact_body(right, new)
        if name == QUALITY_METHOD:
            _, quality = decoded(left, old)
            require(sha(a)==QUALITY_PIN and a==b and quality[74]==('ldc.i4.s',11), 'Released quality filter changed')
            quality_verified = True
        if name == RAY_TARGET:
            require(sha(a)==RAY_PIN, 'Unknown original camera ray body')
            ob,x=decoded(left,old)
            nb,y=decoded(right,new)
            require(len(x)==len(y) and [i for i in range(len(x)) if x[i]!=y[i]]==[40], 'Unexpected camera ray edits')
            require(x[40][0]==y[40][0]=='call', 'Camera ray opcode changed')
            original=left.net.mdtables.MemberRef.rows[(x[40][1]&0xffffff)-1]
            revised=right.net.mdtables.MemberRef.rows[(y[40][1]&0xffffff)-1]
            require(str(original.Name)=='RaycastAll' and str(original.Class.row.TypeNamespace)=='UnityEngine' and str(original.Class.row.TypeName)=='Physics' and original.Signature.value.hex()=='00031d11810d11813d0c08', 'Unknown original Ray,float,int query')
            require(str(revised.Name)=='RaycastAll' and str(revised.Class.row.TypeNamespace)=='HYJS.Desert53' and str(revised.Class.row.TypeName)=='DesertOcclusion' and revised.Signature.value==original.Signature.value, 'Wrong camera ray wrapper signature')
            require(getattr(ob.local_var_sig_tok,'value',None)==getattr(nb.local_var_sig_tok,'value',None) and ob.flags.value==nb.flags.value and ob.max_stack==nb.max_stack, 'Camera ray body metadata changed')
            def eh_shape(e): return {key:getattr(value,'value',value) for key,value in vars(e).items()}
            require([eh_shape(e) for e in ob.exception_handlers]==[eh_shape(e) for e in nb.exception_handlers], 'Camera ray EH changed')
            ray_verified=True
            continue
        if resume_login and name == 'Login.LoginManager::LoginScene':
            require(sha(a) == '1683d88a208215f9acc9e2fb645d65df2ed5f323f23ad31b500a7c0f235aa9c7', 'Unknown login dispatch preimage')
            ob, x = decoded(left, old)
            nb, y = decoded(right, new)
            require(len(y) == len(x)+3 and y[0][0]=='call' and y[1]==('brfalse',3) and y[2][0]=='ret', 'Wrong resume prefix')
            ref = right.net.mdtables.MemberRef.rows[(y[0][1]&0xffffff)-1]
            require(str(ref.Name)=='ResumeLogin' and str(ref.Class.row.TypeNamespace)=='HYJS.Desert53' and str(ref.Class.row.TypeName)=='DesertWalkOnly', 'Wrong resume helper')
            require([suffix(i,3) for i in y[3:]] == x, 'Original login control flow changed')
            require(len(ob.exception_handlers)==len(nb.exception_handlers), 'Login EH count changed')
            shift=nb.instructions[3].offset-ob.instructions[0].offset
            for oe,ne in zip(ob.exception_handlers,nb.exception_handlers):
                require(oe.exception_type==ne.exception_type and oe.catch_type==ne.catch_type, 'Login EH type changed')
                for key in ('try_start','try_end','handler_start','handler_end'):
                    require(getattr(ne,key)==getattr(oe,key)+shift, 'Login EH range changed')
                require((ne.filter_start==oe.filter_start+shift) if oe.is_filter() else ne.filter_start==oe.filter_start, 'Login EH filter changed')
            require(ob.local_var_sig_tok==nb.local_var_sig_tok and ob.flags.value==nb.flags.value, 'Login body flags/locals changed')
            resume_changes.append(dict(method=name,beforeSHA256=sha(a),afterSHA256=sha(b)))
            continue
        if desert_entry and (name in ENTRY_GUARDS or name == 'UI.WorldMapPage::isShamoOpen'):
            ob, x = decoded(left, old)
            nb, y = decoded(right, new)
            if name in ENTRY_GUARDS:
                pin,resume,match = ENTRY_GUARDS[name]
                require(sha(a) == pin and len(y) == len(x)+4, 'Entry preimage differs')
                require(y[0][0]=='ldarg.1' and y[1]==('ldstr',match) and y[2][0]=='call' and y[3]==('brtrue',resume+4), 'Wrong target entry prefix')
                ref = right.net.mdtables.MemberRef.rows[(y[2][1]&0xffffff)-1]
                require(str(ref.Name)=='op_Equality' and str(ref.Class.row.TypeName)=='String','Wrong exact-name equality')
                require([suffix(i,4) for i in y[4:]]==x,'Other entry behavior changed')
                require(not ob.exception_handlers and not nb.exception_handlers,'Unexpected entry EH')
                if name == 'UI.WorldMapPage::onMapClick':
                    require(x[resume][0]=='ldarg.0' and x[resume+1][0]=='ldfld', 'Desert entry must begin at native eligibility')
                    field=left.net.mdtables.Field.rows[(x[resume+1][1]&0xffffff)-1]
                    require(str(field.Name)=='shamoFlag' and ('ldstr','field_003') in x[resume:resume+8], 'Desert click must select field003 with the native level gate')
            else:
                require(sha(a)=='8fb471acd1f4b3e72fa44a6e603222dd2b92556d225444f2a96e456084156690','Unknown desert eligibility')
                snow = next(m for n,_,m in old_methods if n=='UI.WorldMapPage::isXueshanOpen')
                require(sha(exact_body(left,snow))=='9bf46aa037e5ac9ddd0d4864e6f1ff2143b306f1d01fcc359e8ca5586472fa46','Unknown original level gate')
                sb, source = decoded(left,snow)
                def mapped(item):
                    op,value=item
                    if op=='ldstr' and value=='field_002': return op,'field_003'
                    if op in ('ldfld','stfld') and (value>>24)==4:
                        f=left.net.mdtables.Field.rows[(value&0xffffff)-1]
                        if str(f.Name).startswith('xueshan'):
                            target='shamo'+str(f.Name)[7:]
                            row=next(i+1 for i,f in enumerate(right.net.mdtables.Field.rows) if str(f.Name)==target)
                            return op,0x04000000|row
                    return item
                require(y==[mapped(i) for i in source], 'Native desert gate differs from mapped original snow gate')
                old_locals=left.net.mdtables.StandAloneSig.rows[(sb.local_var_sig_tok.value&0xffffff)-1].Signature.value
                new_locals=right.net.mdtables.StandAloneSig.rows[(nb.local_var_sig_tok.value&0xffffff)-1].Signature.value
                require(old_locals==new_locals and not nb.exception_handlers, 'Native gate locals differ')
            entry_changes.append(dict(method=name,beforeSHA256=sha(a),afterSHA256=sha(b)))
            continue
        if walk_only and name in WALK_GUARDS:
            require(sha(a) == WALK_GUARDS[name], 'Unknown content handler: ' + name)
            ob, x = decoded(left, old)
            nb, y = decoded(right, new)
            require(len(y) == len(x) + 3 and y[0][0] == 'call' and y[1] == ('brfalse', 3) and y[2][0] == 'ret', 'Unexpected walk-only guard')
            ref = right.net.mdtables.MemberRef.rows[(y[0][1] & 0xffffff) - 1]
            require(str(ref.Name) == 'IsActive' and str(ref.Class.row.TypeNamespace) == 'HYJS.Desert53' and str(ref.Class.row.TypeName) == 'DesertWalkOnly', 'Wrong walk-only helper')
            require([suffix(i, 3) for i in y[3:]] == x, 'Other field content behavior changed')
            require(len(ob.exception_handlers) == len(nb.exception_handlers), 'Content EH count changed')
            shift = nb.instructions[3].offset - ob.instructions[0].offset
            for oe, ne in zip(ob.exception_handlers, nb.exception_handlers):
                for key in ('try_start','try_end','handler_start','handler_end'):
                    require(getattr(ne, key) == getattr(oe, key) + shift, 'Content EH changed')
            walk_changes.append(dict(method=name,beforeSHA256=sha(a),afterSHA256=sha(b)))
            continue
        short = name[len(LOADER):] if name.startswith(LOADER) else ''
        if bundle_size is not None and short in (DEPENDENCIES.keys() | PREFIXES.keys()):
            pin, count = (DEPENDENCIES | PREFIXES)[short]
            require(sha(a) == pin, 'Loader body differs from pinned evidence: ' + name)
            ob, x = decoded(left, old)
            nb, y = decoded(right, new)
            if short == 'LoadAssetBundle':
                # Retain the full v78 pet portrait wrapper and original branches.
                # Only the exact target bundle takes the new five-instruction path.
                require(len(y) == len(x) + 5, 'Unexpected portrait wrapper branch size')
                ref = left.net.mdtables.MemberRef.rows[(x[count][1] & 0xffffff) - 1]
                require(x[count][0] == 'call' and str(ref.Name) == 'Dependencies' and
                        str(ref.Class.row.TypeName) == 'PetPortraitAssets75', 'Wrong inherited portrait dependency query')
                require([i[0] for i in y[count:count+5]] == ['dup','call','brfalse','call','br'] and
                        y[count+2][1] == count+5 and y[count+4][1] == count+6, 'Wrong desert-only dependency branch')
                helper_call(y[count+1], 'IsTarget'); helper_call(y[count+3], 'GetDependencies')
                def shifted(item):
                    op, value = item
                    if op == 'switch': return op, tuple(v + (5 if v >= count else 0) for v in value)
                    if op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) and isinstance(value,int):
                        return op, value + (5 if value >= count else 0)
                    return item
                require([shifted(item) for item in x] == y[:count]+y[count+5:], 'Inherited loader control flow or portrait wrapper changed')
                require(len(ob.exception_handlers)==len(nb.exception_handlers)==0, 'Unexpected LoadAssetBundle exception regions')
            elif short in DEPENDENCIES:
                require(len(x) == len(y) and [i for i in range(len(x)) if x[i] != y[i]] == [count], 'Unexpected dependency edits')
                ref = left.net.mdtables.MemberRef.rows[(x[count][1] & 0xffffff) - 1]
                require(x[count][0] == 'callvirt' and str(ref.Name) == 'GetAllDependencies' and
                        str(ref.Class.row.TypeName) == 'AssetBundleManifest', 'Wrong original dependency query')
                helper_call(y[count], 'GetDependencies')
            else:
                require(len(y) == len(x) + count and [suffix(i, count) for i in y[count:]] == x, 'Original loader control flow changed')
                if short == 'GetFullPath':
                    require([i[0] for i in y[:count]] == ['ldarg.1','ldarg.2','ldarg.3','call','dup','brfalse','ret','pop'] and y[5][1] == 7, 'Wrong path guard')
                    helper_call(y[3], 'ResolvePath')
                else:
                    require([i[0] for i in y[:count]] == ['ldarg.1','call','brfalse','ldc.i4.1' if short == 'IsAssetExist' else 'ldc.i8','ret'] and y[2][1] == count, 'Wrong directory guard')
                    helper_call(y[1], 'IsTarget')
                    if short == 'GetSize': require(y[3][1] == bundle_size, 'Wrong bundle size')
                require(len(ob.exception_handlers) == len(nb.exception_handlers), 'Loader exception scopes changed')
                shift = nb.instructions[count].offset - ob.instructions[0].offset
                for oe, ne in zip(ob.exception_handlers, nb.exception_handlers):
                    for key in ('try_start','try_end','handler_start','handler_end'):
                        require(getattr(ne, key) == getattr(oe, key) + shift, 'Loader EH changed')
            loader_changes.append(dict(method=name, beforeSHA256=sha(a), afterSHA256=sha(b)))
            continue
        if name != TARGET:
            require(a == b, 'Unrelated original method changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(a) == PIN, 'Camera setter body differs from REA-bound input')
        _, x = decoded(left, old)
        _, y = decoded(right, new)
        require(len(x) == len(y), 'Camera instruction count changed')
        changes = [i for i in range(len(x)) if x[i] != y[i]]
        require(changes == [35, 44], 'Unexpected camera changes: ' + repr(changes))
        for index, helper, original in ((35, 'SetShader', 'set_shader'), (44, 'SetColor', 'set_color')):
            require(x[index][0] == 'callvirt' and y[index][0] == 'call', 'Unexpected setter opcode')
            old_ref = left.net.mdtables.MemberRef.rows[(x[index][1] & 0xffffff) - 1]
            new_ref = right.net.mdtables.MemberRef.rows[(y[index][1] & 0xffffff) - 1]
            require(str(old_ref.Name) == original and str(new_ref.Name) == helper, 'Wrong setter helper')
            require(str(new_ref.Class.row.TypeNamespace) == 'HYJS.Desert53' and str(new_ref.Class.row.TypeName) == 'DesertOcclusion', 'Wrong helper type')
        target_verified = True
    require(target_verified and ray_verified and quality_verified and engine == 547, 'Missing camera, v80 quality fix or original engine methods')
    require(len(loader_changes) == (7 if bundle_size is not None else 0), 'Loader target inventory differs')
    require(len(walk_changes) == (6 if walk_only else 0), 'Content guard inventory differs')
    require(len(entry_changes)==(4 if desert_entry else 0),'Entry target inventory differs')
    require(len(resume_changes)==(1 if resume_login else 0),'Login resume target inventory differs')
    return dict(originalEngineMethodsUnchanged=engine, unrelatedRawMethodBodiesUnchanged=unchanged,
                originalTokensAndABI=True, qualityFilterV80Exact=True, cameraInstructionSubstitutions=[35, 44], cameraRaySubstitution=dict(method=RAY_TARGET,instruction=40,originalSignature='RaycastAll(Ray,float,int)',targetExtraLayer=11,targetExtraAncestor='Observed bridge',extraEndpointExclusion=.2,originalMaskPreserved=True), loaderChanges=loader_changes,walkOnlyChanges=walk_changes,desertEntryChanges=entry_changes,desertLoginResumeChanges=resume_changes)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--runtime', type=Path, default=ROOT / '.tools/native-batch')
    parser.add_argument('--apk-sha256', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--bundle', type=Path)
    parser.add_argument('--bundle-sha256')
    parser.add_argument('--walk-only', action='store_true')
    parser.add_argument('--enable-desert-entry',action='store_true')
    parser.add_argument('--resume-desert-login',action='store_true')
    args = parser.parse_args()
    require(args.apk_sha256 == BASE_APK_SHA and sha_file(args.base_apk) == BASE_APK_SHA, 'Wrong pinned v80 APK')
    require(bool(args.bundle) == bool(args.bundle_sha256), 'Bundle and exact SHA must be supplied together')
    require(not args.walk_only or args.bundle is not None, 'Walk-only requires the verified desert bundle')
    require(not args.enable_desert_entry or args.bundle is not None,'Desert entry requires the verified bundle')
    require(not args.resume_desert_login or args.enable_desert_entry,'Login resume requires desert entry')
    bundle_raw = args.bundle.read_bytes() if args.bundle else None
    if bundle_raw is not None:
        require(sha(bundle_raw) == args.bundle_sha256, 'Bundle SHA differs')
        inspection = json.loads(args.bundle.with_name('bundle-inspection.json').read_text())
        require(inspection['passed'] and inspection['sha256'] == args.bundle_sha256 and
                len(inspection['fadeShaders']) == 7, 'Require verified native bundle')
    require(not args.output.exists(), 'Use a fresh output directory')
    args.output.mkdir(parents=True)
    runtime = args.runtime.resolve()
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, text=True, capture_output=True, timeout=90)
        (args.output / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; inspect saved log')
    with tempfile.TemporaryDirectory(prefix='hyjs-desert53-camera-') as temporary:
        work = Path(temporary)
        with zipfile.ZipFile(args.base_apk) as archive:
            before = archive.read(ENTRY)
            references = []
            for name in ('UnityEngine.dll', 'mscorlib.dll', 'System.dll'):
                p = work / name
                p.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
                references.append(p)
        require(sha_file(args.base_apk) == BASE_APK_SHA and sha(before)==BASE_MANAGED_SHA, 'APK or released managed baseline changed')
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        bridge = args.output.resolve() / 'SoulFlowerDesert53.dll'
        sources = [HERE / 'DesertOcclusion.cs']
        if bundle_raw is not None:
            generated = work / 'DesertBundle.cs'
            generated_source=(HERE / 'DesertBundle.cs').read_text().replace('__BUNDLE_SHA256__', args.bundle_sha256).replace('__BUNDLE_SIZE__', str(len(bundle_raw)))
            generated.write_text(generated_source)
            sources.append(generated)
        if args.walk_only or args.resume_desert_login: sources.append(HERE / 'DesertWalkOnly.cs')
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(bridge), *['/reference:' + str(p) for p in references], *sources])
        shutil.copy2(INJECTOR, work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Program.cs"/><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        output = args.output.resolve() / 'Assembly-CSharp.dll'
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', host, output,
                      *([str(len(bundle_raw)), 'loader'] if bundle_raw is not None else []), *(['walk-only'] if args.walk_only else (['live'] if args.enable_desert_entry else [])), *(['entry'] if args.enable_desert_entry else []), *(['resume'] if args.resume_desert_login else [])])
    after = output.read_bytes()
    core_checks=verify(before,after,len(bundle_raw) if bundle_raw is not None else None,args.walk_only,args.enable_desert_entry,args.resume_desert_login)
    report = dict(schema='hyjs-desert53-runtime-adapter-v2', enabled=False, runtimeIntegrated=False,
                  apkSHA256=args.apk_sha256, inputManagedSHA256=sha(before), outputManagedSHA256=sha(after),
                  helperSHA256=sha(bridge.read_bytes()), walkOnly=args.walk_only,desertEntrySelectable=args.enable_desert_entry,desertLoginResume=args.resume_desert_login,qaPhoneObserver=False, **core_checks)
    if bundle_raw is not None:
        physical = 'assets/android/hyjs/desert53/' + args.bundle_sha256 + '/map_desertfield_001'
        with zipfile.ZipFile(args.base_apk) as archive:
            catalog = archive.read('assets/android/smallassetmanifest')
        logical = b'map/map_desertfield_001'
        lines = catalog.splitlines(keepends=True)
        require(not any(line.split(b'\t')[0] == logical for line in lines), 'Existing logical catalog row needs explicit composition')
        line = logical + b'\t' + hashlib.md5(bundle_raw).hexdigest().encode() + b'\t' + str(len(bundle_raw)).encode() + b'\n'
        revised = catalog + (b'\n' if not catalog.endswith(b'\n') else b'') + line
        catalog_path = args.output / 'smallassetmanifest'
        catalog_path.write_bytes(revised)
        report.update(status='STATIC_VERIFIED_OVERLAY_NOT_PACKAGED', bundleSHA256=args.bundle_sha256,
                      bundleBytes=len(bundle_raw), logicalBundleName=logical.decode(), physicalAPKEntry=physical,
                      targetDependencies=[], persistentTargetBundleBypassed=True, targetDirectoryGuard=True,
                      globalManifestChanged=False, gameOrAndroidExecuted=False,
                      apkEntries={ENTRY:str(output.resolve()), 'assets/bin/Data/Managed/SoulFlowerDesert53.dll':str(bridge.resolve()),
                                  'assets/android/smallassetmanifest':str(catalog_path.resolve()), physical:str(args.bundle.resolve())},
                      originalCatalogSHA256=sha(catalog), updatedCatalogSHA256=sha(revised))
    report['sourceSHA256'] = {str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in
                             (Path(__file__), INJECTOR, HERE / 'DesertOcclusion.cs', HERE / 'DesertBundle.cs', HERE / 'DesertWalkOnly.cs')}
    (args.output / 'manifest.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()
