"""Build independent v80 desert+trial83 static inputs and a loopback-only QA overlay.

No APK is signed, installed, or published. Static output is not a package manifest.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import resource
import shutil
import subprocess
import sys
import tempfile
import threading
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_trial_refresh82 import build as trial
from tools.desert_extraction import build_v78_adapter, build_field_patch, package_resources
from tools.local_asset_ready.remove_gate import exact_body, require

ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
MAP_HELPER = 'assets/bin/Data/Managed/SoulFlowerDesert53.dll'
V4_HELPER = 'assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
SHOP = 'assets/bin/Data/Managed/SoulFlowerGGOShop.dll'
R8J_MAIN = 'ed7c7dd45800b50bc97930b77976e4786a1119c51259c048f1457896fe2e9e7e'
R8J_HELPER = 'acff84fe406f31ca86de7069fe60d18ac568aa3583a806f6b7f516777fc4790b'
QA_ORIGIN = 'http://127.0.0.1:28979'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def file_sha(path):
    return trial.digest(Path(path))


def source_pins(entries):
    require(isinstance(entries, dict) and entries, 'Source pins missing')
    for name, expected in entries.items():
        f = (ROOT / name).resolve()
        require(f.is_relative_to(ROOT) and f.is_file() and file_sha(f) == expected, 'Source pin differs: ' + name)


def add_pin(pins, path):
    p = Path(path).resolve()
    require(p.is_relative_to(ROOT) and p.is_file(), 'Pinned source must be inside project')
    key, value = str(p.relative_to(ROOT)), file_sha(p)
    require(key not in pins or pins[key] == value, 'Conflicting pin: ' + key)
    pins[key] = value


def audit_shop(raw):
    """Audit the actual compiled shop payload, including both origin entry points."""
    pe = dnfile.dnPE(data=raw)
    required = {'HYJS.GGO.GGOShop::Origin', 'HYJS.GGO.TrialShopRefresh75::Current'}
    hits = set()
    register = None
    for name, _, method in methods(pe):
        if name == 'HYJS.GGO.GGOShop::RegisterIcons':
            require(register is None, 'Duplicate icon registration method')
            register = method
        if not method.Rva:
            continue
        names = []
        for inst in read_method_body_from_bytes(exact_body(pe, method)).instructions:
            if inst.opcode.name == 'ldstr':
                value = pe.net.user_strings.get(inst.operand.value & 0xffffff).value
                require('://' not in value and '127.0.0.1:28979' not in value,
                        'Absolute URL in compiled shop helper: ' + name)
            if inst.opcode.name not in ('call', 'callvirt', 'newobj'):
                continue
            token = getattr(inst.operand, 'value', 0)
            if token >> 24 != 10:
                continue
            ref = pe.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
            owner = str(getattr(ref.Class.row, 'TypeName', ''))
            names.append((owner, str(ref.Name)))
        if name in required:
            require(names.count(('ServerConfig', 'GetServerUrl')) == 1
                    and ('Uri', '.ctor') in names and ('Uri', 'GetLeftPart') in names,
                    'Shop origin is not derived from ServerConfig: ' + name)
            hits.add(name)
    require(hits == required and register is not None and register.Flags.mdPrivate and register.Flags.mdStatic,
            'Shop origin methods or private icon registration missing')
    return {'sha256': sha(raw), 'absoluteUrlsAbsent': True, 'serverConfigOriginMethods': sorted(hits),
            'desertPrivateIconRegistrationPresent': True}


def composite_check(original, after, map_check, field_check):
    before, current = dnfile.dnPE(data=original), dnfile.dnPE(data=after)
    old, new = methods(before), methods(current)
    require(len(old) == len(new), 'Managed method inventory differs')
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param'):
        require(len(getattr(before.net.mdtables, table).rows) == len(getattr(current.net.mdtables, table).rows),
                'Managed metadata inventory differs: ' + table)
    require(not getattr(current.net.mdtables, 'FieldPtr', None) and not getattr(current.net.mdtables, 'MethodPtr', None),
            'Unity pointer tables are forbidden')
    changed, engine, quality = [], 0, False
    for (name, rid, a), (other, rid2, b) in zip(old, new):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags)
                == (other, rid2, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), 'Managed ABI differs: ' + name)
        if not a.Rva:
            require(not b.Rva, 'Managed body unexpectedly added: ' + name)
            continue
        left, right = exact_body(before, a), exact_body(current, b)
        if name == build_v78_adapter.QUALITY_METHOD:
            require(sha(left) == build_v78_adapter.QUALITY_PIN and left == right, 'v80 quality fix differs')
            quality = True
        if left != right:
            changed.append(name)
        elif name.startswith('Battle.Engine'):
            engine += 1
        for inst in read_method_body_from_bytes(right).instructions:
            if inst.opcode.name == 'ldstr':
                value = current.net.user_strings.get(inst.operand.value & 0xffffff).value
                require('127.0.0.1:28979' not in value, 'QA origin in independent managed source: ' + name)
    allowed = {build_v78_adapter.TARGET, build_v78_adapter.RAY_TARGET}
    allowed.update(x['method'] for k in ('loaderChanges', 'desertEntryChanges', 'desertLoginResumeChanges') for x in map_check[k])
    allowed.update(x['method'] for x in field_check['fieldHooks'])
    require(not (allowed & set(trial.TARGETS)), 'Trial hooks overlap desert hooks')
    allowed.update(trial.TARGETS)
    require(set(changed) == allowed and engine == 547 and quality, 'Composite method scope, engine or quality differs')
    return sorted(changed)


def run(label, command, logs):
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, env=env,
                            capture_output=True, text=True, timeout=180)
    (logs / (label + '.log')).write_text(result.stdout + result.stderr)
    require(result.returncode == 0, label + ' failed; inspect ' + str(logs / (label + '.log')))
    return result.stdout.strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', required=True, type=Path)
    parser.add_argument('--map-overlay', required=True, type=Path)
    parser.add_argument('--preview', required=True, type=Path)
    parser.add_argument('--bundle', required=True, type=Path)
    parser.add_argument('--runtime', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    base, map_dir, field_dir, bundle, runtime, out = (p.resolve() for p in
        (args.base_apk, args.map_overlay, args.preview, args.bundle, args.runtime, args.output))
    require(out.is_relative_to(ROOT / 'analysis/desert-extraction-v4-client')
            and out.name.startswith('combined83') and not out.exists(), 'Fresh owned combined83 output required')
    require(file_sha(base) == trial.BASE == package_resources.BASE_SHA, 'Wrong v80 base')
    map_raw, field_raw = (map_dir / 'manifest.json').read_bytes(), (field_dir / 'manifest.json').read_bytes()
    m, v = json.loads(map_raw), json.loads(field_raw)
    for item in (m, v):
        require(not item.get('qaOnly') and not item.get('neverPromote') and not item.get('diagnosticOnly')
                and not item.get('qaOrigin'), 'QA/diagnostic input cannot be combined')
    require(m.get('schema') == 'hyjs-desert53-runtime-adapter-v2' and m.get('status') == 'STATIC_VERIFIED_OVERLAY_NOT_PACKAGED'
            and m.get('apkSHA256') == trial.BASE and m.get('desertEntrySelectable') is True
            and m.get('desertLoginResume') is True and m.get('walkOnly') is False, 'Map input differs')
    require(v.get('schema') == 'hyjs-desert-v4-scene-overlay-v2' and v.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE'
            and v.get('apkSHA256') == trial.BASE, 'Field input differs')
    pins = {}
    for record in (m, v):
        entries = record.get('sourceSHA256'); source_pins(entries)
        for name, value in entries.items():
            require(name not in pins or pins[name] == value, 'Conflicting source pin: ' + name)
            pins[name] = value
    extra = list(trial.SOURCES) + [Path(__file__), ROOT/'tools/client_trial_refresh82/build.py',
        ROOT/'tools/client_trial_refresh82/Injector.cs', ROOT/'tools/client_trial_refresh82/Verify.cs',
        ROOT/'tools/ggo_items/assets/ggo_credit_hud-source.png', ROOT/'tools/ggo_items/build_client_overlay.py',
        ROOT/'tools/awake_diff_rows/build.py', ROOT/'tools/local_asset_ready/remove_gate.py',
        ROOT/'tools/client_activity/build.py', ROOT/'tools/desert_extraction/package_resources.py',
        ROOT/'tools/desert_extraction/prepare_qa_package.py', ROOT/'tools/client_remote/build.py',
        ROOT/'tools/client_remote/config.py', ROOT/'tools/client_remote/Injector.cs']
    for path in extra:
        add_pin(pins, path)
    source_pins(pins)

    native_map = (map_dir/'Assembly-CSharp.dll').read_bytes()
    native_a = (field_dir/'Assembly-CSharp.dll').read_bytes()
    map_helper = (map_dir/'SoulFlowerDesert53.dll').read_bytes()
    field_helper = (field_dir/'SoulFlowerDesertV4.dll').read_bytes()
    map_bundle = bundle.read_bytes()
    require(sha(native_map) == m['outputManagedSHA256'] == v['inputManagedSHA256']
            and sha(native_a) == v['outputManagedSHA256'] == R8J_MAIN
            and sha(map_helper) == m['helperSHA256'] and sha(field_helper) == v['helperSHA256'] == R8J_HELPER,
            'Raw r8j static payload differs')
    with zipfile.ZipFile(base) as z:
        names = z.namelist()
        require(len(names) == len(set(names)) and all(x in names for x in (ENTRY, SHOP, 'classes2.dex', package_resources.CATALOG_ENTRY))
                and all(x not in names for x in (MAP_HELPER, V4_HELPER, package_resources.BUNDLE_ENTRY)), 'Base entries differ')
        original, original_shop, dex = z.read(ENTRY), z.read(SHOP), z.read('classes2.dex')
        catalog = z.read(package_resources.CATALOG_ENTRY)
        production_origin = json.loads(z.read('assets/local-wallet.json'))['origin'].rstrip('/')
        require(production_origin and production_origin != QA_ORIGIN, 'Unexpected v80 origin')
        expected_catalog = package_resources.compose_catalog(catalog, map_bundle)
        require(sha(original) == m['inputManagedSHA256'] == trial.MAIN_SHA
                and sha(catalog) == m['originalCatalogSHA256']
                and sha(expected_catalog) == m['updatedCatalogSHA256']
                and expected_catalog == (map_dir/'smallassetmanifest').read_bytes(), 'Base or resource identity differs')
        inherited = {n: sha(z.read(n)) for n in names if n.startswith('lib/') or (n.startswith('classes') and n.endswith('.dex'))}
    map_check = build_v78_adapter.verify(original, native_map, len(map_bundle), False, True, True)
    field_check = build_field_patch.verify(native_map, native_a)
    require(map_check['originalEngineMethodsUnchanged'] == field_check['originalEngineMethodsUnchanged'] == 547
            and map_check['qualityFilterV80Exact'] and field_check['qualityFilterV80Exact'], 'Desert v80 engine/quality differs')
    dot = runtime/'dotnet/dotnet'; compiler = next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    require(dot.is_file() and dnlib.is_file(), 'Pinned runtime unavailable')
    before_free = shutil.disk_usage(ROOT).free
    min_free = [before_free]; stop = threading.Event()
    def sample_disk():
        while not stop.wait(.25):
            min_free[0] = min(min_free[0], shutil.disk_usage(ROOT).free)
    sampler = threading.Thread(target=sample_disk, daemon=True); sampler.start()
    created = False
    try:
        out.mkdir(parents=True); created = True
        static, prod, qa = out/'static', out/'qa-production', out/'qa-loopback'
        static.mkdir(); prod.mkdir()
        (static/'map-input-manifest.json').write_bytes(map_raw); (static/'field-input-manifest.json').write_bytes(field_raw)
        add_pin(pins, static/'map-input-manifest.json'); add_pin(pins, static/'field-input-manifest.json')
        with tempfile.TemporaryDirectory(prefix='hyjs-combined83-build-') as temporary:
            work = Path(temporary); refs = work/'refs'; refs.mkdir()
            with zipfile.ZipFile(base) as z:
                for name in z.namelist():
                    if name.startswith(trial.MANAGED) and name.endswith('.dll'):
                        (refs/Path(name).name).write_bytes(z.read(name))
            (refs/'Assembly-CSharp.dll').write_bytes(native_a)
            (refs/'SoulFlowerDesert53.dll').write_bytes(map_helper)
            (refs/'SoulFlowerDesertV4.dll').write_bytes(field_helper)
            shop = static/'SoulFlowerGGOShop.dll'
            run('compile-shop-helper', [dot, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                '/deterministic+', '/codepage:65001', '/out:'+str(shop),
                '/resource:'+str(ROOT/'tools/ggo_items/assets/ggo_credit_hud-source.png')+',credit_hud.png',
                *['/r:'+str(p) for p in sorted(refs.glob('*.dll')) if p.name != shop.name], *trial.SOURCES], static)
            verify = work/'verify'; verify.mkdir()
            (verify/'Program.cs').write_text((ROOT/'tools/client_trial_refresh82/Verify.cs').read_text())
            (verify/'Verify.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
            run('compile-api-verifier', [dot, 'build', verify/'Verify.csproj', '--verbosity', 'quiet', '-p:NuGetAudit=false'], static)
            api = run('verify-shop-api', [dot, verify/'bin/Debug/net8.0/Verify.dll', refs/shop.name, shop], static)
            require(api.startswith('PASS inherited public/protected API '), 'Shop inherited API verification absent')
            inject = work/'inject'; inject.mkdir()
            (inject/'Program.cs').write_text((ROOT/'tools/client_trial_refresh82/Injector.cs').read_text())
            (inject/'Inject.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
            run('compile-trial-injector', [dot, 'build', inject/'Inject.csproj', '--verbosity', 'quiet', '-p:NuGetAudit=false'], static)
            combined = static/'Assembly-CSharp.dll'
            run('inject-trial', [dot, inject/'bin/Debug/net8.0/Inject.dll', refs/'Assembly-CSharp.dll', combined], static)
            native_b, shop_raw = combined.read_bytes(), shop.read_bytes()
            two_hooks = trial.check_main(native_a, native_b)
            allowed = composite_check(original, native_b, map_check, field_check)
            shop_audit = audit_shop(shop_raw)
            source_pins(pins)
            require(file_sha(base) == trial.BASE, 'Base changed while building')
            proof = {'status':'STATIC_PREVIEW_NOT_FOR_RELEASE', 'baseAPKSHA256':trial.BASE,
                     'desertInputManagedSHA256':sha(native_a), 'combinedManagedSHA256':sha(native_b),
                     'shopHelperSHA256':sha(shop_raw), 'trialHookCheck':two_hooks,
                     'allChangedMethods':allowed, 'qualityFilterV80Exact':True, 'originalEngineMethodsExact':547,
                     'shopInheritedPublicProtectedApi':api, 'shopOriginAudit':shop_audit,
                     'sourcePins':pins, 'deviceExecuted':False, 'serverExecuted':False}
            (static/'build-proof.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2)+'\n')
            add_pin(pins, static/'build-proof.json')
            files = {ENTRY:native_b, MAP_HELPER:map_helper, V4_HELPER:field_helper, SHOP:shop_raw}
            paths = {}
            for entry, raw in files.items():
                target = prod/Path(entry).name; target.write_bytes(raw); paths[entry] = str(target.relative_to(ROOT))
            (prod/'smallassetmanifest').write_bytes(expected_catalog); (prod/'map_desertfield_001').write_bytes(map_bundle)
            report = {'schema':'hyjs-desert-trial83-qa-managed-v1', 'status':'STATIC_VERIFIED_QA_NOT_RELEASED',
                      'qaOnly':True, 'neverPromote':True, 'managedOnly':True, 'baseAPKSHA256':trial.BASE,
                      'inputManagedSHA256':sha(original), 'outputManagedSHA256':sha(native_b), 'inputDexSHA256':sha(dex),
                      'inheritedJavaByteExact':True, 'inheritedJavaPublicProtectedABIExact':True,
                      'inheritedJavaEntrySHA256':inherited, 'payloadFiles':paths,
                      'replacementEntries':{name:sha(raw) for name,raw in files.items()},
                      'allowedChangedMethods':allowed, 'sourcePins':pins, 'originalEngineMethodsExact':547,
                      'qualityFilterV80Exact':True, 'deviceExecuted':False, 'serverExecuted':False,
                      'shopOriginAudit':shop_audit}
            (prod/'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
            resource_files = {package_resources.CATALOG_ENTRY:'smallassetmanifest', package_resources.BUNDLE_ENTRY:'map_desertfield_001'}
            resources = {'schema':'hyjs-desert-v4-map-resource-overlay-v1', 'baseAPKSHA256':trial.BASE,
                         'bundleSHA256':package_resources.BUNDLE_SHA, 'bundleBytes':len(map_bundle),
                         'originalCatalogSHA256':sha(catalog), 'updatedCatalogSHA256':sha(expected_catalog),
                         'payloadFiles':{n:str((prod/f).relative_to(ROOT)) for n,f in resource_files.items()},
                         'replacementEntries':{n:file_sha(prod/f) for n,f in resource_files.items()},
                         'sourcePins':pins, 'releaseReady':False}
            (prod/'resource-manifest.json').write_text(json.dumps(resources, ensure_ascii=False, indent=2)+'\n')
            run('prepare-loopback-qa', [sys.executable, '-B', ROOT/'tools/desert_extraction/prepare_qa_package.py',
                '--base-apk', base, '--source', prod, '--runtime', runtime, '--output', qa], static)
            qa_manifest_path = qa/'manifest.json'; qa_manifest = json.loads(qa_manifest_path.read_text())
            qa_shop = (qa/'SoulFlowerGGOShop.dll').read_bytes()
            qa_shop_audit = audit_shop(qa_shop)
            require(qa_shop == shop_raw and qa_manifest['replacementEntries'][SHOP] == sha(shop_raw), 'QA shop payload differs')
            require(qa_manifest.get('qaOnly') is True and qa_manifest.get('neverPromote') is True
                    and qa_manifest.get('qaOrigin') == QA_ORIGIN, 'QA isolation markers differ')
            # Existing QA preparer audits the base shop. Replace that one reported hash
            # only after independently auditing the actual newly copied shop payload.
            qa_manifest['qaOriginProof']['helperNoAbsoluteOrigin']['shop'] = sha(shop_raw)
            qa_manifest['qaOriginProof']['shopActualPayloadAudit'] = qa_shop_audit
            qa_manifest_path.write_text(json.dumps(qa_manifest, ensure_ascii=False, indent=2)+'\n')
            source_pins(qa_manifest['sourcePins'])
            require(file_sha(base) == trial.BASE, 'Base changed during QA preparation')
        stop.set(); sampler.join(); min_free[0] = min(min_free[0], shutil.disk_usage(ROOT).free)
        size = sum(p.stat().st_size for p in out.rglob('*') if p.is_file())
        usage = {'diskFreeBeforeBytes':before_free, 'minimumFreeBytesObserved':min_free[0],
                 'diskFreeAfterBytes':shutil.disk_usage(ROOT).free, 'ownedOutputLogicalBytes':size,
                 'samplingSeconds':0.25,
                 'selfPeakRssPlatformUnits':resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                 'maxChildPeakRssPlatformUnits':resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss,
                 'rssUnits':'bytes on macOS; maximum single process, not aggregate',
                 'diskNote':'Shared volume; minimum sampled includes concurrent activity.'}
        (out/'resource-usage.json').write_text(json.dumps(usage, indent=2)+'\n')
        print(json.dumps({'status':'STATIC_ONLY_QA_NOT_SIGNED','qaInput':str(qa),
            'qaManifestSHA256':file_sha(qa/'manifest.json'),'qaResourcesSHA256':file_sha(qa/'resource-manifest.json'),
            'combinedManagedSHA256':sha(native_b),'shopSHA256':sha(shop_raw),
            'qaManagedSHA256':qa_manifest['outputManagedSHA256'],'sourcePins':len(qa_manifest['sourcePins']),
            'originalEngineMethodsExact':547,'qualityFilterV80Exact':True,'usage':usage}))
    except Exception:
        stop.set(); sampler.join()
        if created: shutil.rmtree(out)
        raise


if __name__ == '__main__':
    main()
