"""Build a native login/register classes2 overlay and single-method managed prefix; no APK/device actions."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods,operand
from tools.local_asset_ready.remove_gate import exact_body,require
from tools.local_wallet.verify import dex_classes
ENTRY='assets/bin/Data/Managed/Assembly-CSharp.dll'
TARGET='UserInput::OnLoginButton'
def sha(raw):return hashlib.sha256(raw).hexdigest()

def dex_abi(raw):
    """Read actual DEX class data and public/protected method/field descriptors."""
    def uleb(at):
        value=shift=0
        while True:
            b=raw[at];at+=1;value|=(b&127)<<shift
            if not b&128:return value,at
            shift+=7
    ns,so,nt,to,np,po,nf,fo,nm,mo,nc,co=struct.unpack_from('<12I',raw,56)
    strings=[]
    for n in range(ns):
        at=struct.unpack_from('<I',raw,so+n*4)[0];_,at=uleb(at)
        strings.append(raw[at:raw.index(b'\0',at)].decode('utf-8',errors='replace'))
    types=[strings[struct.unpack_from('<I',raw,to+n*4)[0]] for n in range(nt)]
    protos=[]
    for n in range(np):
        _,ret,params=struct.unpack_from('<3I',raw,po+n*12)
        args=[]
        if params:
            count=struct.unpack_from('<I',raw,params)[0]
            args=[types[x] for x in struct.unpack_from('<'+'H'*count,raw,params+4)]
        protos.append('('+''.join(args)+')'+types[ret])
    fields=[(types[c],strings[name],types[t]) for c,t,name in (struct.unpack_from('<HHI',raw,fo+n*8) for n in range(nf))]
    meths=[(types[c],strings[name],protos[p]) for c,p,name in (struct.unpack_from('<HHI',raw,mo+n*8) for n in range(nm))]
    result={}
    for n in range(nc):
        cls,flags,_,_,_,_,data,_=struct.unpack_from('<8I',raw,co+n*32)
        if not flags&1 or flags&0x1000 or not data:continue
        counts=[]
        for _ in range(4):v,data=uleb(data);counts.append(v)
        api=[]
        for category,count in zip(['static-field','instance-field','direct-method','virtual-method'],counts):
            index=0
            for _ in range(count):
                delta,data=uleb(data);index+=delta;access,data=uleb(data)
                if 'method' in category:_,data=uleb(data)
                if access&5:
                    owner,name,descriptor=(meths if 'method' in category else fields)[index]
                    api.append([category,name,descriptor,access&0x1f])
        result[types[cls]]=sorted(api)
    return result

def verify_managed(before,after):
    a,b=[dnfile.dnPE(data=x) for x in [before,after]];old,new=methods(a),methods(b)
    require(len(old)==len(new),'Original method inventory changed')
    unchanged=engine=0;changed=[]
    for (name,rid,row),(other,oid,modified) in zip(old,new):
        require((name,rid)==(other,oid) and row.Signature.value==modified.Signature.value and row.struct.Flags==modified.struct.Flags and row.struct.ImplFlags==modified.struct.ImplFlags,'Original ABI changed: '+name)
        if not row.Rva:require(not modified.Rva,'Unexpected body');continue
        x,y=exact_body(a,row),exact_body(b,modified)
        if name!=TARGET:
            require(x==y,'Non-target raw method changed: '+name);unchanged+=1;engine+=name.startswith('Battle.Engine');continue
        ob,nb=[read_method_body_from_bytes(v) for v in [x,y]]
        require(not ob.exception_handlers and not nb.exception_handlers,'Unexpected login EH')
        count=len(nb.instructions)-len(ob.instructions);require(count==29,'Unexpected native form prefix length')
        suffix=nb.instructions[count:];ix={i.offset:n for n,i in enumerate(ob.instructions)};iy={i.offset:n for n,i in enumerate(suffix)}
        for first,second in zip(ob.instructions,suffix):
            require(first.opcode.name==second.opcode.name,'Original login opcode changed')
            if first.opcode.flow_control.name in {'Branch','Cond_Branch'}:require(ix[first.operand]==iy[second.operand],'Original login branch changed')
            else:require(operand(first.operand)==operand(second.operand),'Original login operand changed')
        strings=[b.net.user_strings.get(i.operand.value&0xffffff).value for i in nb.instructions[:count] if i.opcode.name=='ldstr']
        require(strings==['com.hyjs.wallet.LoginCache','hasForm','formUserName','formPassWord'],'Unexpected form interface')
        changed.append({'method':name,'token':hex(0x6000000|rid),'prefixInstructions':count,'beforeSHA256':sha(x),'afterSHA256':sha(y),'originalValidationAndTransportSuffixPreserved':True})
    require(engine==547 and len(changed)==1,'Engine/target count differs')
    for table in ['TypeDef','MethodDef','Field']:
        require(len(getattr(a.net.mdtables,table).rows)==len(getattr(b.net.mdtables,table).rows),'Original '+table+' count changed')
    require(not getattr(b.net.mdtables,'FieldPtr',None),'Old Unity must not gain FieldPtr')
    return {'changes':changed,'unrelatedInheritedRawMethodBodiesExact':unchanged,'originalEngineMethodsExact':engine,'originalCacheAttemptCallbacksAndKeystoreManagedHooksExact':True}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--base-sha256',required=True)
    p.add_argument('--input-managed',type=Path);p.add_argument('--input-managed-sha256')
    p.add_argument('--output-directory',type=Path,required=True)
    p.add_argument('--reuse-managed',type=Path,help='Reuse a previously emitted same-source managed prefix after a fresh strict comparison')
    args=p.parse_args();out=args.output_directory.resolve();require(not out.exists(),'Use fresh output directory')
    require(sha(args.base_apk.read_bytes())==args.base_sha256,'Base APK SHA differs')
    with zipfile.ZipFile(args.base_apk) as z:
        base=z.read(ENTRY);before_dex=z.read('classes2.dex')
        original_dex={name:sha(z.read(name)) for name in ['classes.dex','classes3.dex'] if name in z.namelist()}
    before=args.input_managed.read_bytes() if args.input_managed else base
    if args.input_managed:require(args.input_managed_sha256 and sha(before)==args.input_managed_sha256,'Explicit managed input SHA required')
    out.mkdir(parents=True);runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    java=[*sorted((ROOT/'tools/local_wallet/src').rglob('*.java')),ROOT/'tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java',ROOT/'tools/client_ux/java/com/hyjs/wallet/ClientFeedback.java',ROOT/'tools/login_cache/java/com/hyjs/wallet/LoginCache.java',ROOT/'tools/game_registration/java/com/hyjs/wallet/GameRegistration.java',*sorted((ROOT/'tools/login_ui/java').rglob('*.java'))]
    stubs=sorted((ROOT/'tools/local_wallet/stubs').rglob('*.java'));injector=Path(__file__).with_name('Injector.cs')
    sdk=Path.home()/'Library/Android/sdk';jdk=Path('/opt/homebrew/opt/openjdk@21');android=sdk/'platforms/android-33/android.jar'
    env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/'bin')+os.pathsep+os.environ.get('PATH',''))
    def run(name,command,environment=None):
        r=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,capture_output=True,text=True,env=environment,timeout=120)
        (out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed; see saved log')
    with tempfile.TemporaryDirectory(prefix='hyjs-native-login-ui-') as tmp:
        folder=Path(tmp);current=folder/'Assembly-CSharp.dll';current.write_bytes(before)
        shutil.copy2(injector,folder/'Program.cs');project=folder/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        if args.reuse_managed:
            verify_managed(before,args.reuse_managed.read_bytes())
            shutil.copy2(args.reuse_managed,out/'Assembly-CSharp.dll')
        else:
            run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
            shutil.copytree(folder/'bin/Debug/net8.0',out/'injector')
            run('patch',[dotnet,out/'injector/Patch.dll',current,out/'Assembly-CSharp.dll'])
        stubclasses=folder/'stubs';classes=folder/'classes';dex=folder/'dex'
        for d in [stubclasses,classes,dex]:d.mkdir()
        common=[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',str(android)]
        run('compile-stubs',[*common,'-d',stubclasses,*stubs],env)
        run('compile-java',[jdk/'bin/javac','-encoding','UTF-8','--release','8','-classpath',str(android)+os.pathsep+str(stubclasses),'-d',classes,*java],env)
        run('dex',[sdk/'cmdline-tools/latest/bin/d8','--release','--min-api','23','--lib',android,'--classpath',stubclasses,'--output',dex,*sorted(classes.rglob('*.class'))],env)
        require(sorted(d.name for d in dex.iterdir())==['classes.dex'],'Expected one classes2 module')
        shutil.copy2(dex/'classes.dex',out/'classes2.dex')
    after=(out/'Assembly-CSharp.dll').read_bytes();checks=verify_managed(before,after)
    new_dex=(out/'classes2.dex').read_bytes();old_api,new_api=dex_abi(before_dex),dex_abi(new_dex)
    missing=[]
    for cls,abi in old_api.items():
        if cls not in new_api:missing.append([cls,'CLASS']);continue
        for member in abi:
            if member not in new_api[cls]:missing.append([cls,member])
    require(not missing,'Inherited public/protected Java ABI changed: '+repr(missing))
    new_classes=set(dex_classes(new_dex));require(all(n.startswith('Lcom/hyjs/wallet/') for n in new_classes),'Stubs/unrelated DEX class present')
    report={'schema':'native-login-ui-overlay-v1','status':'STATIC_VERIFIED_NOT_PACKAGED','baseAPKSHA256':args.base_sha256,'inputManagedSHA256':sha(before),'outputManagedSHA256':sha(after),'inputDexSHA256':sha(before_dex),'outputDexSHA256':sha(new_dex),'originalDexEntriesMustRemainExact':original_dex,'inheritedJavaPublicProtectedABIExact':True,'inheritedPublicClasses':sorted(old_api),'newPublicClasses':sorted(set(new_api)-set(old_api)),'removedInternalClasses':sorted(set(dex_classes(before_dex))-new_classes),'newClasses':sorted(new_classes-set(dex_classes(before_dex))),'sourcePins':{str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in [Path(__file__).resolve(),injector,*java,*stubs]},'assetRequired':'assets/login-ui/panel.png','artReady':(ROOT/'tools/login_ui/assets/panel.png').is_file(),'artSHA256':sha((ROOT/'tools/login_ui/assets/panel.png').read_bytes()) if (ROOT/'tools/login_ui/assets/panel.png').is_file() else None,'managedReused':bool(args.reuse_managed),'systemAutofillUsesAndroidDefault':True,'apkPackaged':False,'phoneVerified':False,**checks}
    (out/'abi-before.json').write_text(json.dumps(old_api,ensure_ascii=False,indent=2)+'\n');(out/'abi-after.json').write_text(json.dumps(new_api,ensure_ascii=False,indent=2)+'\n');(out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','outputManagedSHA256','outputDexSHA256','inheritedJavaPublicProtectedABIExact','artReady','originalEngineMethodsExact']},ensure_ascii=False))
if __name__=='__main__':main()
