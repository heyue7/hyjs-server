"""Produce the fixed batch40 isolated QA APK. Never install or promote it.
Run using .analysis-venv/bin/python; no dependency downloads.
"""
import argparse,copy,fcntl,hashlib,json,os,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash,copy_original_record,require
from tools.client_remote import build as remote
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
from dncil.cil.body.reader import read_method_body_from_bytes
import dnfile
ORIGIN='http://127.0.0.1:28935'
DLL=common.DLL
WALLET='assets/local-wallet.json'

def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--base-apk',type=Path,required=True)
 parser.add_argument('--expected-sha',required=True)
 parser.add_argument('--output-directory',type=Path,required=True)
 parser.add_argument('--record',type=Path,required=True)
 args=parser.parse_args()
 BASE=args.base_apk.resolve();BASE_SHA=args.expected_sha;OUT=args.output_directory.resolve();RECORD=args.record.resolve()
 require(all(p.is_relative_to(ROOT) for p in [BASE,OUT,RECORD]),'QA paths must remain in project')
 require(common.digest(BASE)==BASE_SHA,'Pinned formal base changed')
 require(not OUT.exists() and not RECORD.exists(),'Fresh QA output and record required')
 require(shutil.disk_usage(ROOT).free>BASE.stat().st_size*3+200_000_000,'Insufficient isolated packaging space')
 OUT.mkdir(parents=True)
 sdk=Path.home()/'Library/Android/sdk/build-tools/33.0.0';jdk=Path('/opt/homebrew/opt/openjdk@21')
 env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/'bin')+os.pathsep+os.environ.get('PATH',''))
 def run(cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=120,env=env)
  require(r.returncode==0,'QA command failed: '+r.stderr[-2500:]);return r.stdout+r.stderr
 cert=common._certificate(run([sdk/'apksigner','verify','--print-certs',BASE]))
 with zipfile.ZipFile(BASE) as z:
  before=z.read(DLL);wallet=json.loads(z.read(WALLET));old=remote.validate_origin(wallet['origin'])
 require(old!=ORIGIN,'Base is already QA')
 targets={name:(old+'/sao/' if name=='ServerConfig::GetServerUrl' else old,ORIGIN+'/sao/' if name=='ServerConfig::GetServerUrl' else ORIGIN) for name in remote.targets_for(ORIGIN)}
 # Inspect executable instructions, rather than replacing arbitrary byte strings.
 pe=dnfile.dnPE(data=before)
 for name,_,m in methods(pe):
  if name in targets:
   literals=[pe.net.user_strings.get(i.operand.value&0xffffff).value for i in read_method_body_from_bytes(exact_body(pe,m)).instructions if i.opcode.name=='ldstr']
   require(literals.count(targets[name][0])==1,'Expected inherited executable origin missing: '+name)
 pins={Path(__file__).resolve():common.digest(Path(__file__)),Path(__file__).with_name('QAInjector.cs'):common.digest(Path(__file__).with_name('QAInjector.cs'))}
 with (ROOT/'build/.client-build.lock').open('a') as lock:
  fcntl.flock(lock,fcntl.LOCK_EX)
  with tempfile.TemporaryDirectory(prefix='intermediate-',dir=OUT) as temp:
   folder=Path(temp);current=folder/'input.dll';current.write_bytes(before)
   shutil.copy2(Path(__file__).with_name('QAInjector.cs'),folder/'Program.cs')
   runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
   project=folder/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
   run([dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
   managed=folder/'output.dll';run([dotnet,folder/'bin/Debug/net8.0/Patch.dll',current,old,ORIGIN,managed]);after=managed.read_bytes()
   # Reuse the strict four-method verifier with origins discovered from this base.
   remote.targets_for=lambda _:targets
   checks=remote.verify(before,after,ORIGIN)
   patched=dnfile.dnPE(data=after)
   for _,_,m in methods(patched):
    if not m.Rva:continue
    for i in read_method_body_from_bytes(exact_body(patched,m)).instructions:
     if i.opcode.name=='ldstr':require(old not in patched.net.user_strings.get(i.operand.value&0xffffff).value,'Inherited remote executable origin remains')
   wallet['origin']=ORIGIN;payload={DLL:after,WALLET:(json.dumps(wallet,separators=(',',':'))+'\n').encode()}
   inherited={};compressed={};unsigned=folder/'unsigned.apk';aligned=folder/'aligned.apk';signed=folder/'signed.apk'
   with zipfile.ZipFile(BASE) as z,BASE.open('rb') as stream,zipfile.ZipFile(unsigned,'w') as out:
    out.comment=z.comment
    for info in z.infolist():
     if common._signature(info.filename):continue
     if info.filename in payload:out.writestr(copy.copy(info),payload[info.filename])
     else:
      inherited[info.filename]=common._hash_entry(z,info.filename);compressed[info.filename]=compressed_hash(stream,info);copy_original_record(stream,out,info)
   run([sdk/'zipalign','-f','-p','4',unsigned,aligned]);unsigned.unlink()
   run([sdk/'apksigner','sign','--ks',ROOT/'.local-signing/debug.keystore','--ks-pass','pass:android','--key-pass','pass:android','--out',signed,aligned]);aligned.unlink()
   require(common._certificate(run([sdk/'apksigner','verify','--verbose','--print-certs',signed]))==cert,'QA signer differs')
   run([sdk/'zipalign','-c','-p','4',signed])
   hashes={n:sha(v) for n,v in payload.items()}
   with zipfile.ZipFile(signed) as z,signed.open('rb') as stream:
    require(len(z.namelist())==len(set(z.namelist())) and {n for n in z.namelist() if not common._signature(n)}==set(inherited)|set(payload),'QA entry inventory differs')
    for n,h in {**inherited,**hashes}.items():require(common._hash_entry(z,n)==h,'QA content differs: '+n)
    for n,h in compressed.items():require(compressed_hash(stream,z.getinfo(n))==h,'QA compressed inherited payload differs: '+n)
   require(common.digest(BASE)==BASE_SHA,'Formal base changed during packaging')
   for p,h in pins.items():require(common.digest(p)==h,'QA source changed')
   output=OUT/'hyjs-terminal-recycle-qa.apk';signed.replace(output)
   idsig=signed.with_suffix('.apk.idsig')
   if idsig.exists():idsig.replace(output.with_suffix('.apk.idsig'))
   result=dict(status='PASS_QA_SIGN_ALIGN_STATIC_NOT_INSTALLED',outputAPK=str(output.relative_to(ROOT)),binarySHA256=common.digest(output),baseAPK=str(BASE.relative_to(ROOT)),baseSHA256=BASE_SHA,inheritedOrigin=old,qaOrigin=ORIGIN,replacementEntries=hashes,inheritedEntryCount=len(inherited),allOtherEntriesAndCompressedPayloadsExact=True,javaAllDexExact=True,signatureVerified=True,zipalignVerified=True,phoneExecuted=False,installed=False,sourcePins={str(p.relative_to(ROOT)):h for p,h in pins.items()},**checks)
  result['ownedIntermediatesCleaned']=not folder.exists()
  RECORD.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps({k:result[k] for k in ['status','outputAPK','binarySHA256','inheritedEntryCount','originalEngineMethodsUnchanged','ownedIntermediatesCleaned']}))
if __name__=='__main__':main()
