"""Build an additive Java8/min23 startup overlay against pinned formal62. No APK signing/install."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.client_compat63.axml import patch, require, NEW
from tools.client_compat63.dex_patch import patch as patch_dex
BASE_SHA='fe427f13e8683b434bf7d2756b2dbd4a6f42746d0d6c66c86264182252704580'
SOURCE=Path(__file__).resolve().parent

def sha(data): return hashlib.sha256(data).hexdigest()
def dex_classes(raw):
    require(raw[:8]==b'dex\n035\0','Expected min23 DEX035')
    string_count,string_at,type_count,type_at=struct.unpack_from('<4I',raw,56)
    class_count,class_at=struct.unpack_from('<2I',raw,96)
    def descriptor(index):
        require(index<type_count,'Invalid type')
        si=struct.unpack_from('<I',raw,type_at+index*4)[0]
        require(si<string_count,'Invalid string')
        pos=struct.unpack_from('<I',raw,string_at+si*4)[0]
        while raw[pos]&128: pos+=1
        pos+=1
        return raw[pos:raw.index(0,pos)].decode('utf-8')
    return {descriptor(struct.unpack_from('<I',raw,class_at+i*32)[0]):descriptor(struct.unpack_from('<I',raw,class_at+i*32+8)[0]) for i in range(class_count)}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk',type=Path,required=True)
    parser.add_argument('--output-directory',type=Path,required=True)
    parser.add_argument('--input-manifest',type=Path,help='Optional final composed binary Manifest; preserves its versions/attributes')
    a=parser.parse_args(); out=a.output_directory.resolve()
    require(sha(a.base_apk.read_bytes())==BASE_SHA,'Input must be exact formal62')
    require(not out.exists(),'Use a fresh output directory'); out.mkdir(parents=True)
    sdk=Path(os.environ.get('ANDROID_SDK_ROOT',Path.home()/'Library/Android/sdk'))
    jdk=Path(os.environ.get('HYJS_WALLET_JAVA_HOME','/opt/homebrew/opt/openjdk@21'))
    android=sdk/'platforms/android-33/android.jar'; d8=sdk/'cmdline-tools/latest/bin/d8'
    aapt=sdk/'build-tools/33.0.0/aapt'
    jsonjar=Path.home()/'.m2/repository/com/vaadin/external/google/android-json/0.0.20131108.vaadin1/android-json-0.0.20131108.vaadin1.jar'
    require(all(x.is_file() for x in [android,d8,aapt,jdk/'bin/javac',jsonjar]),'Existing SDK/JDK/host JSON required')
    env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/'bin')+os.pathsep+os.environ.get('PATH',''))
    def run(name,args):
        r=subprocess.run(['rtk','proxy',*map(str,args)],env=env,capture_output=True,text=True,timeout=120)
        (out/(name+'.log')).write_text(r.stdout+r.stderr)
        require(r.returncode==0,name+' failed: see log')
        return r.stdout+r.stderr
    with zipfile.ZipFile(a.base_apk) as z,tempfile.TemporaryDirectory(prefix='hyjs-compat63-') as td:
        temp=Path(td); classes=temp/'classes'; classes.mkdir(); dex=temp/'dex'; dex.mkdir(); stubs=temp/'stubs'; stubs.mkdir()
        stub=temp/'LocalAssetOverlay.java'
        stub.write_text('package com.hyjs.wallet; public final class LocalAssetOverlay { public static void synchronize(android.content.Context c) throws java.io.IOException {} }')
        run('compile-classpath-stub',[jdk/'bin/javac','--release','8','-classpath',android,'-d',stubs,stub])
        run('javac',[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',str(android)+os.pathsep+str(stubs),'-d',classes,*sorted((SOURCE/'src').rglob('*.java'))])
        run('d8',[d8,'--release','--min-api','23','--lib',android,'--classpath',stubs,'--output',dex,*sorted(classes.rglob('*.class'))])
        names=sorted(n for n in z.namelist() if re.fullmatch(r'classes(?:[2-9][0-9]*)?\.dex',n))
        require(names==['classes.dex','classes2.dex','classes3.dex','classes4.dex'],'Formal DEX layout changed')
        original_dex={n:sha(z.read(n)) for n in names}
        original_classes={}
        for n in names:
            for name,superclass in dex_classes(z.read(n)).items():
                require(name not in original_classes,'Duplicate original class');original_classes[name]=superclass
        require(original_classes.get('Lcom/hyjs/wallet/LocalAssetOverlay;')=='Ljava/lang/Object;','Original overlay class missing')
        newdex=(dex/'classes.dex').read_bytes(); added=dex_classes(newdex)
        require(bool(added) and all(n.startswith('Lcom/hyjs/compat/') for n in added),'Unexpected new DEX classes')
        require(not set(added)&set(original_classes),'Class replacement forbidden')
        require(added.get('Lcom/hyjs/compat/StartupActivity;')=='Landroid/app/Activity;','Bootstrap must be plain Activity')
        require(b'com/unity3d' not in newdex and b'com/hyjs/wallet/ClientUpdateActivity' not in newdex,'No Unity/game class type reference allowed')
        # Destination remains a plain Java String, not a class descriptor/initialization dependency.
        (out/'classes5.dex').write_bytes(newdex)
        raw=a.input_manifest.read_bytes() if a.input_manifest else z.read('AndroidManifest.xml')
        manifest,manifest_proof=patch(raw); (out/'AndroidManifest.xml').write_bytes(manifest)
        check=temp/'inspect.apk'
        with zipfile.ZipFile(check,'w') as inspect:
            inspect.writestr('AndroidManifest.xml',manifest)
            inspect.writestr('resources.arsc',z.read('resources.arsc'))
        xml=run('aapt-xmltree',[aapt,'dump','xmltree',check,'AndroidManifest.xml'])
        badging=run('aapt-badging',[aapt,'dump','badging',check])
        require('launchable-activity: name='+repr(NEW) in badging,'Wrong launcher')
        require("uses-feature-not-required: name='android.hardware.microphone'" in badging,'Microphone not optional')
        require("uses-feature: name='android.hardware.microphone'" not in badging,'Microphone still required')
        require('android:screenOrientation(0x0101001e)=(type 0x10)0x0' in xml,'Landscape missing')
        # Host probe compiles the actual original whitelist/hash implementation with isolated Context fixtures.
        host=temp/'host';host.mkdir(); assets=temp/'assets';assets.mkdir()
        config=json.loads(z.read('assets/local-resource-overrides.json'))
        entries=['assets/local-resource-overrides.json']+['assets/android/'+row['path'] for row in config['files']]
        for n in entries:
            target=assets/n.removeprefix('assets/');target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(z.read(n))
        original_overlay=ROOT/'tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java'
        test_sources=[*sorted((SOURCE/'tests').rglob('*.java')),SOURCE/'src/com/hyjs/compat/StartupGate.java',SOURCE/'src/com/hyjs/compat/FailureDiagnostic.java']
        # Reproduce the unchanged formal62 failure with the original literal pins.
        old_source=temp/'old-source/LocalAssetOverlay.java'; old_source.parent.mkdir()
        new_sha=sha(z.read('assets/android/smallassetmanifest'));new_size=len(z.read('assets/android/smallassetmanifest'))
        pin=next(row for row in config['files'] if row['path']=='smallassetmanifest')
        old_literal='new Pin("smallassetmanifest", "'+pin['sha256']+'", '+str(pin['bytes'])+'L)'
        new_literal='new Pin("smallassetmanifest", "'+new_sha+'", '+str(new_size)+'L)'
        source_text=original_overlay.read_text(); require(source_text.count(new_literal)==1,'Source new pin missing')
        old_source.write_text(source_text.replace(new_literal,old_literal)); old_host=temp/'old-host';old_host.mkdir()
        run('compile-formal62-probe',[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',jsonjar,'-d',old_host,*test_sources,old_source])
        old_probe=run('formal62-failure-probe',[jdk/'bin/java','-classpath',str(old_host)+os.pathsep+str(jsonjar),'com.hyjs.compat.StartupProbe',assets,temp/'original-fault-root','original'])
        mismatch={'path':'smallassetmanifest','expectedBytes':pin['bytes'],'expectedSHA256':pin['sha256'],
            'actualBytes':new_size,'actualSHA256':new_sha,'formal62FreshInstallLaunchBlocked':True}
        pin['sha256']=new_sha;pin['bytes']=new_size
        config_bytes=(json.dumps(config,ensure_ascii=False,indent=2)+'\n').encode()
        config_path=out/'assets/local-resource-overrides.json';config_path.parent.mkdir(parents=True);config_path.write_bytes(config_bytes)
        (assets/'local-resource-overrides.json').write_bytes(config_bytes)
        patched_dex,dex_proof=patch_dex(z.read('classes2.dex'),new_sha,new_size)
        require(dex_proof['actualPatchedDEXInitializedPins']==config['files'],'Actual DEX pins/config differ')
        for row in dex_proof['actualPatchedDEXInitializedPins']:
            asset=z.read('assets/android/'+row['path'])
            require(len(asset)==row['bytes'] and sha(asset)==row['sha256'],'Actual DEX pin/APK asset differs')
        (out/'classes2.dex').write_bytes(patched_dex)
        run('dexdump',[sdk/'build-tools/33.0.0/dexdump','-f','-h',out/'classes2.dex'])
        verify_dex=temp/'d8-verify';verify_dex.mkdir()
        run('d8-verify-patched-dex',[d8,'--release','--min-api','23','--lib',android,'--output',verify_dex,out/'classes2.dex'])
        run('compile-probe',[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',jsonjar,'-d',host,*test_sources,original_overlay])
        probe=run('fault-probe',[jdk/'bin/java','-classpath',str(host)+os.pathsep+str(jsonjar),'com.hyjs.compat.StartupProbe',assets,temp/'fault-roots'])
        diagnostic_probe=run('safe-diagnostic-probe',[jdk/'bin/java','-classpath',str(host)+os.pathsep+str(jsonjar),'com.hyjs.compat.DiagnosticProbe'])
        untouched={n:sha(z.read(n)) for n in z.namelist() if n.startswith('lib/') or n.startswith('assets/bin/Data/Managed/')}
    source_files=[*SOURCE.glob('*.py'),*SOURCE.glob('*.md'),*(SOURCE/'src').rglob('*.java'),*(SOURCE/'tests').rglob('*.java')]
    sources={str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in sorted(source_files)}
    sources[str(original_overlay.relative_to(ROOT))]=sha(original_overlay.read_bytes())
    replacements={'AndroidManifest.xml':{'file':'AndroidManifest.xml','sha256':sha(manifest)},'assets/local-resource-overrides.json':{'file':'assets/local-resource-overrides.json','sha256':sha(config_bytes)},'classes2.dex':{'file':'classes2.dex','sha256':sha(patched_dex)}}
    additions={'classes5.dex':{'file':'classes5.dex','sha256':sha(newdex)}}
    art_evidence=ROOT/'analysis/feedback63-20261024/phone-art64-pinprobe-r10.txt'
    art_text=art_evidence.read_text()
    require('PROBE_EXIT-0' in art_text and all(row['sha256'] in art_text for row in dex_proof['actualPatchedDEXInitializedPins']),'Actual ART PinProbe evidence missing/differs')
    require(sha(patched_dex)=='b592412b77a0a0146e8f03d5598b187c95c0d6374d5f616f3191ed672f6f6bbb','Generated DEX differs from actual ART tested DEX')
    art_proof={'path':str(art_evidence.relative_to(ROOT)),'sha256':sha(art_evidence.read_bytes()),'classes2SHA256':sha(patched_dex),'runtime':'ART64 standalone, no Unity/Context/synchronization','exit':0,'allSixPinsMatched':True}
    report={'schema':'hyjs-client-compat63-overlay-v1','status':'STATIC_DEX_AND_HOST_PROBE_PASS_NOT_PACKAGED_NOT_DEVICE_TESTED',
        'baseAPKSHA256':BASE_SHA,'baseAPK':str(a.base_apk.resolve()),'manifestProof':manifest_proof,
        'replacements':replacements,'additions':additions,'newDexClasses':added,'originalDexSHA256':original_dex,
        'originalDexPolicy':'classes.dex/classes3.dex/classes4.dex byte-exact; classes2 only clinit manifest Pin semantics + necessary absolute offset relocation; all other method instructions/EH/debug and all APIs/flags exact',
        'unchangedNativeAndManagedEntrySHA256':untouched,'sourcePins':sources,
        'actualARTPinProbe':art_proof,'startupDiagnostics':{'tag':'HYJSStartup','exceptionMessages':False,'hostProbe':diagnostic_probe.strip()},'dexProof':dex_proof,'probe':probe.strip(),'formal62FailureReproduced':old_probe.strip(),'existingFormal62Blocker':mismatch,'probeOriginalOverlaySourceSHA256':sha(original_overlay.read_bytes()),
        'probeScope':'Actual original Java overlay source + production StartupGate; host Context fixtures. New source pin/config uses true formal62 APK assets; unchanged formal62 fresh-cache failure is independently asserted. Android Activity lifecycle/UI/native crash recovery not exercised.',
        'androidJavaRelease':8,'minAPI':23,'noOriginalDexRebuild':True,'originalDEXReassembly':False,'noUnityClassReferenceInNewDex':True,
        'unverified':['Device cold start/error page/retry/duplicate launcher/process restore/background lifecycle','No-microphone device installation/voice behavior','64-bit-only OS and 16KiB engine compatibility remain unsupported/unproven']}
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'status':report['status'],'newClasses':len(added),'dexSHA256':sha(newdex),'manifestSHA256':sha(manifest)}))
if __name__=='__main__': main()
