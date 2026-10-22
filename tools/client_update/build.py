"""Build isolated update dex and compose four appVersion call sites: two login checks plus the login screen version label."""
import argparse
import hashlib
import json
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_branding.branding import chunks, elements, strings_from_pool
from tools.local_asset_ready.remove_gate import exact_body, require
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
ALLOWED = {'Login.LoginCheckVersion::CheckVersion', 'Login.LoginUserLogin::UserLogin',
           'UI.LoginNormalUI::Start', 'UI.LoginNormalUI::Show'}
def sha(value): return hashlib.sha256(value).hexdigest()

def manifest_versions(raw):
    """Read the base APK identity so the packaged manifest keeps increasing."""
    parts = chunks(raw)
    pools = [part for part in parts if part[0] == 1]
    require(len(pools) == 1, 'Expected one manifest string pool')
    values, _ = strings_from_pool(pools[0][2])
    for tag, attributes in elements(parts, values):
        if tag != 'manifest': continue
        fields = {key: value for _, key, _, value in attributes}
        return int(fields['versionCode']), str(fields['versionName'])
    raise ValueError('Manifest identity missing')

def build_java(out,build_code,version_name):
    import os
    sdk=Path(os.environ.get('ANDROID_SDK_ROOT',Path.home()/'Library/Android/sdk'))
    jdk=Path(os.environ.get('HYJS_WALLET_JAVA_HOME','/opt/homebrew/opt/openjdk@21'))
    android=sdk/'platforms/android-33/android.jar';d8=sdk/'cmdline-tools/latest/bin/d8'
    require(android.is_file() and d8.is_file() and (jdk/'bin/javac').is_file(),'Existing Android SDK/JDK required')
    env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/'bin')+os.pathsep+os.environ.get('PATH',''))
    with tempfile.TemporaryDirectory(prefix='hyjs-client-update-java-') as temp:
        temp=Path(temp);classes=temp/'classes';classes.mkdir();dex=temp/'dex';dex.mkdir()
        source=(Path(__file__).resolve().parent/'src/com/hyjs/wallet/ClientUpdate.java').read_text()
        # Compatibility for older sources that pinned BUILD/NAME; current Java reads
        # PackageManager, so these patterns normally match nothing.
        source=re.sub(r'(private static final long BUILD\s*=\s*)\d+L',lambda m:m.group(1)+str(build_code)+'L',source)
        source=re.sub(r'(private static final String NAME\s*=\s*)"[^"]*"',lambda m:m.group(1)+json.dumps(version_name),source)
        java=temp/'ClientUpdate.java';java.write_text(source)
        stub=temp/'WalletActivity.java';stub.write_text('package com.hyjs.wallet; public class WalletActivity extends android.app.Activity {}')
        stubs=temp/'stubs';stubs.mkdir()
        result=subprocess.run(['rtk','proxy',str(jdk/'bin/javac'),'--release','8','-classpath',str(android),'-d',str(stubs),str(stub)],env=env,capture_output=True,text=True,timeout=120)
        require(result.returncode==0,'Wallet classpath stub compile failed')
        activity=Path(__file__).resolve().parent/'src/com/hyjs/wallet/ClientUpdateActivity.java'
        for name,cmd in [('javac',[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',str(android)+os.pathsep+str(stubs),'-d',classes,java,activity]),('d8',[d8,'--release','--min-api','23','--lib',android,'--classpath',stubs,'--output',dex])]:
            if name=='d8':cmd.extend(sorted(classes.rglob('*.class')))
            result=subprocess.run(['rtk','proxy',*map(str,cmd)],env=env,capture_output=True,text=True,timeout=120)
            (out/(name+'.log')).write_text(result.stdout+result.stderr);require(result.returncode==0,name+' failed')
        require(sorted(x.name for x in dex.iterdir())==['classes.dex'],'Expected isolated dex')
        shutil.copy2(dex/'classes.dex',out/'classes.dex')

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--base-sha256',required=True)
    p.add_argument('--input-managed',type=Path);p.add_argument('--output-directory',type=Path,required=True)
    p.add_argument('--build-code',type=int,required=True);p.add_argument('--version-name',required=True)
    a=p.parse_args();out=a.output_directory.resolve();require(not out.exists(),'Fresh directory required')
    require(0<a.build_code<2147483648,'Android build code must fit signed int')
    require(len(a.version_name.strip())>0,'Android version name required')
    allowed=ALLOWED
    require(sha(a.base_apk.read_bytes())==a.base_sha256,'APK identity changed')
    with zipfile.ZipFile(a.base_apk) as base_zip: base_code,base_name=manifest_versions(base_zip.read('AndroidManifest.xml'))
    require(a.build_code>base_code,'Android build code must exceed the base manifest')
    out.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    source=Path(__file__).resolve().parent;dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name,cmd):
        r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=120)
        (out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed; see saved log')
    with zipfile.ZipFile(a.base_apk) as z, tempfile.TemporaryDirectory(prefix='hyjs-native-account-') as t:
        work=Path(t);before=a.input_managed.read_bytes() if a.input_managed else z.read(ENTRY)
        host=work/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
        for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
            f=work/name;f.write_bytes(z.read('assets/bin/Data/Managed/'+name));refs.append(f)
        helper=out/'HYJSClientUpdate.dll'
        run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(x) for x in refs],source/'ClientUpdate.cs'])
        shutil.copy2(source/'Injector.cs',work/'Program.cs');project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,out/'Assembly-CSharp.dll'])
    build_java(out,a.build_code,a.version_name)
    after=(out/'Assembly-CSharp.dll').read_bytes();left,right=[dnfile.dnPE(data=b)for b in [before,after]]
    changes=[];engine=0
    require(len(list(methods(left)))==len(list(methods(right))),'Method count changed')
    for (n,i,x),(m,j,y) in zip(methods(left),methods(right)):
        require((n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags),'ABI changed: '+n)
        if not x.Rva:continue
        old,new=exact_body(left,x),exact_body(right,y)
        if old!=new:require(n in allowed,'Unrelated body changed: '+n);changes.append(n)
        else:engine+=n.startswith('Battle.Engine')
    require(set(changes).issubset(allowed) and engine==547,'Expected scoped method changes')
    require(not getattr(right.net.mdtables,'FieldPtr',None),'Unsupported FieldPtr')
    require(len(left.net.mdtables.ModuleRef.rows)==len(right.net.mdtables.ModuleRef.rows),'Unexpected ModuleRef')
    inputs=[source/'ClientUpdate.cs',source/'Injector.cs',source/'build.py',*sorted((source/'src').rglob('*.java'))]
    report={'status':'STATIC_VERIFIED_NOT_PACKAGED','buildCode':a.build_code,'versionName':a.version_name,'versionSource':'PackageManager','baseVersionCode':base_code,'baseVersionName':base_name,'baseAPKSHA256':a.base_sha256,'inputManagedSHA256':sha(before),'outputManagedSHA256':sha(after),'allowedChangedMethods':changes,'originalEngineMethodsExact':engine,'sourcePins':{str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in inputs if f.is_file()}}
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))
if __name__=='__main__':main()
