"""Recompile only additive classes5; copy/re-pin an already verified compatibility payload."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.client_compat63.axml import require
from tools.client_compat63.build import dex_classes
SOURCE=Path(__file__).resolve().parent

def sha(raw):return hashlib.sha256(raw).hexdigest()
def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--input-overlay',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args()
    old=a.input_overlay.resolve();out=a.output_directory.resolve();require(not out.exists(),'Fresh output required')
    report=json.loads((old/'manifest.json').read_text());require(report['status']=='STATIC_DEX_AND_HOST_PROBE_PASS_NOT_PACKAGED_NOT_DEVICE_TESTED','Qualified input required')
    out.mkdir(parents=True)
    sdk=Path.home()/'Library/Android/sdk';jdk=Path('/opt/homebrew/opt/openjdk@21');android=sdk/'platforms/android-33/android.jar';d8=sdk/'cmdline-tools/latest/bin/d8'
    env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/'bin')+os.pathsep+os.environ.get('PATH',''))
    def run(name,args):
        r=subprocess.run(['rtk','proxy',*map(str,args)],env=env,capture_output=True,text=True,timeout=120)
        (out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed')
        return r.stdout+r.stderr
    with tempfile.TemporaryDirectory(prefix='hyjs-startup-diagnostic-') as td:
        temp=Path(td);classes=temp/'classes';classes.mkdir();stubs=temp/'stubs';stubs.mkdir();dex=temp/'dex';dex.mkdir()
        stub=temp/'LocalAssetOverlay.java';stub.write_text('package com.hyjs.wallet; public final class LocalAssetOverlay { public static void synchronize(android.content.Context c) throws java.io.IOException {} }')
        run('compile-stub',[jdk/'bin/javac','--release','8','-classpath',android,'-d',stubs,stub])
        run('compile-diagnostic',[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',str(android)+os.pathsep+str(stubs),'-d',classes,*sorted((SOURCE/'src').rglob('*.java'))])
        run('d8-diagnostic',[d8,'--release','--min-api','23','--lib',android,'--classpath',stubs,'--output',dex,*sorted(classes.rglob('*.class'))])
        shutil.copy2(dex/'classes.dex',out/'classes5.dex')
        probe=temp/'DiagnosticProbe.java';probe.write_text('''import com.hyjs.compat.FailureDiagnostic;
public class DiagnosticProbe { public static void main(String[] ignored) {
Throwable cause=new java.io.IOException("SECRET_CREDENTIAL /private/account/file");
Throwable error=new ExceptionInInitializerError(cause);
String log=FailureDiagnostic.format("prepare",error);
if(log.contains("SECRET") || log.contains("/private") || !log.contains("java.io.IOException") || !log.contains("ExceptionInInitializerError")) throw new AssertionError(log);
System.out.println("PASS safe exception/cause types and stack locations; no exception messages/paths/credentials");
}}''')
        run('compile-diagnostic-probe',[jdk/'bin/javac','--release','8','-classpath',classes,'-d',classes,probe])
        run('diagnostic-probe',[jdk/'bin/java','-classpath',classes,'DiagnosticProbe'])
    for name,entry in report['replacements'].items():
        raw=(old/entry['file']).read_bytes();require(sha(raw)==entry['sha256'],'Input replacement pin differs')
        target=out/entry['file'];target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(raw)
    raw=(out/'classes5.dex').read_bytes();added=dex_classes(raw)
    require(all(name.startswith('Lcom/hyjs/compat/') for name in added),'Unexpected additive class')
    report['additions']['classes5.dex']={'file':'classes5.dex','sha256':sha(raw)};report['newDexClasses']=added
    files=[*SOURCE.glob('*.py'),*SOURCE.glob('*.md'),*(SOURCE/'src').rglob('*.java'),*(SOURCE/'tests').rglob('*.java'),ROOT/'tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java']
    report['sourcePins']={str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in sorted(files)}
    report['startupDiagnostics']={'tag':'HYJSStartup','phases':['prepare','handoff'],'fields':['exception class','cause class','stack class/method/line'],'exceptionMessages':False,'deviceTested':False,'onlyAdditiveDEXRecompiled':True,'priorOverlayManifestSHA256':sha((old/'manifest.json').read_bytes())}
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'classes5SHA256':sha(raw),'newClasses':len(added),'allThreeReplacementPayloadsByteExact':True}))
if __name__=='__main__':main()
