"""Pinned formal65 -> feedback66 managed overlay; no packaging/device/network."""
import argparse,hashlib,json,re,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
from tools.client_feedback65.build import parsed,normalized
from tools.client_one_key59.build import verify
from tools.client_arena_state.build import exact_body,methods
ROOT=Path(__file__).resolve().parents[2];HERE=Path(__file__).resolve().parent
MANAGED='assets/bin/Data/Managed/';MAIN=MANAGED+'Assembly-CSharp.dll'
BASE='f26942c80a6b39d2b83eeed63a0775dff7e84147db815f553256e09c06026ba7'
MAIN_SHA='5842a437f402d73dd209b85960bde78ce750150bf79719bc766d2b6d6bc6593d'
TRIAL={int(t,16)&0xffffff:h for t,h in re.findall(r'\[0x([0-9a-f]+)\] = \([^\n]+"([0-9a-f]{64})"\)',(HERE/'TrialInjector.cs').read_text())}
COMMERCE={0x533a:'b67b047687648fff5fd1064e114b3d60da4971e996d8267a47cfb1692625431f',0x5341:'e9e8055d7d7a71f9c0687cddca690433b89d8429582be52003c534fe1ebe7d31'}
def sha(b):return hashlib.sha256(b).hexdigest()
def ref(pe,i):
 if i.opcode.name!='call' or i.operand.value>>24!=0x0a:return ''
 r=pe.net.mdtables.MemberRef.rows[(i.operand.value&0xffffff)-1]
 return str(r.Name) if str(getattr(r.Class.row,'TypeNamespace',''))=='HYJS.Feedback66' else ''
def proof(before,after):
 a=dnfile.dnPE(data=before);b=dnfile.dnPE(data=after);targets={**TRIAL,**COMMERCE}
 assert len(TRIAL)==10
 for rid,digest in targets.items():assert sha(exact_body(a,a.net.mdtables.MethodDef.rows[rid-1]))==digest
 report=verify(before,after,[{'token':hex(0x06000000+r)}for r in targets])
 for rid in targets:
  old,new=parsed(a,rid),parsed(b,rid); rows=list(new.instructions)
  assert not new.exception_handlers and getattr(old.local_var_sig_tok,'value',None)==getattr(new.local_var_sig_tok,'value',None)
  if rid in COMMERCE:
   assert [x.opcode.name for x in rows[-4:]]==['ldarg.0','ldarg.1','call','ret'] and ref(b,rows[-2])=='Quota'
   original=rows[:-3]
   expected=normalized(old,old.instructions);actual=normalized(new,original)
   assert actual[-1]==('ldarg.0',None);actual[-1]=('ret',None)
   assert expected==actual,(hex(rid),'commerce original graph')
  else:
   count=4 if rid==0x3ddc else 7 if rid in (0x6cfe,0x5e6c,0x2b55,0x2b5b) else 5
   prefix=rows[:count]; rows=rows[count:]
   assert sum(bool(ref(b,i)) for i in prefix)==1
   if count in (5,7):assert prefix[-2].opcode.name=='brfalse' and prefix[-2].operand==rows[0].offset and prefix[-1].opcode.name=='ret'
   if count==5 and rid != 0x5e8b:
    marker=54501 if rid in (0x5e71,0x5e66) else 54503
    at=next(n for n,i in enumerate(rows)if i.opcode.name=='ldc.i4' and i.operand==marker)
    capture=rows[at-4:at];assert [i.opcode.name for i in capture]==['ldarg.0','ldloc','ldc.i4','call'] and ref(b,capture[-1]) in ('Request','CaptureEnd')
    rows=rows[:at-4]+rows[at:]
    hooks=[i for i in rows if ref(b,i) in ('WrapStart','StartSent','WrapEnd','EndSent')];assert len(hooks)==2
    rows=[i for i in rows if i not in hooks]
   assert normalized(old,old.instructions)==normalized(new,rows),(hex(rid),'trial original graph')
 # Native types defined by Main must never resolve through helper ModuleRefs.
 # A circular helper->Main signature import previously created exactly that
 # shape for HttpHandler; original-only CIL checks cannot detect this failure.
 native={(str(t.TypeNamespace),str(t.TypeName)) for t in a.net.mdtables.TypeDef.rows}
 invalid=[]
 for rid,t in enumerate(b.net.mdtables.TypeRef.rows[len(a.net.mdtables.TypeRef.rows):],len(a.net.mdtables.TypeRef.rows)+1):
  if (str(t.TypeNamespace),str(t.TypeName)) not in native:continue
  scope=t.ResolutionScope
  if scope.table.name!='Module':invalid.append(dict(rid=rid,type=str(t.TypeNamespace)+'.'+str(t.TypeName),scope=scope.table.name,name=str(getattr(scope.row,'Name',''))))
 assert not invalid,('new Main native type has external scope',invalid)
 # Independently decode added hook MemberRef signatures. Every typed callback
 # return/argument must target Main's HttpHandler TypeDef (coded index tag 0).
 def compressed(raw,at):
  x=raw[at]
  if x<128:return x,at+1
  if x<192:return ((x&63)<<8)|raw[at+1],at+2
  return ((x&31)<<24)|(raw[at+1]<<16)|(raw[at+2]<<8)|raw[at+3],at+4
 checked=0
 for m in b.net.mdtables.MemberRef.rows[len(a.net.mdtables.MemberRef.rows):]:
  if str(getattr(m.Class.row,'TypeNamespace',''))!='HYJS.Feedback66':continue
  raw=m.Signature.value;at=1;count,at=compressed(raw,at)
  for unused in range(count+1):
   kind=raw[at];at+=1
   if kind in (0x11,0x12):
    coded,at=compressed(raw,at);tag=coded&3;rid=coded>>2
    assert tag==0,('hook signature must reference Main TypeDef',str(m.Name),tag,rid)
    target=b.net.mdtables.TypeDef.rows[rid-1]
    assert (str(target.TypeNamespace),str(target.TypeName))==('NetWork.Layer','HttpHandler'),str(m.Name)
   else:assert kind in (1,2,8,0x1c),('unreviewed new hook signature',str(m.Name),kind)
  assert at==len(raw),str(m.Name)
  checked+=1
 assert checked==13,('expected all 12 Trial hooks plus Commerce hook signatures',checked)
 for table in ('AssemblyRef','MemberRef','TypeRef'):
  left,right=getattr(a.net.mdtables,table).rows,getattr(b.net.mdtables,table).rows
  assert len(right)>=len(left)
  for x,y in zip(left,right):
   for field in ('Name','TypeName','TypeNamespace','Signature','PublicKey','PublicKeyOrToken','Culture'):
    if hasattr(x,field):
     u,v=getattr(x,field),getattr(y,field);assert (u.value if hasattr(u,'value')else str(u))==(v.value if hasattr(v,'value')else str(v))
   for field in ('Class','ResolutionScope'):
    if hasattr(x,field):
     u,v=getattr(x,field),getattr(y,field);assert (u.table.name,u.row_index)==(v.table.name,v.row_index)
 return dict(report,originalEngineMethodsExact=547,originalMetadataReferencesAppendOnly=True,originalTargetInstructionsBranchesLocalsExact=True,newHookSignaturesNativeTypeDefBound=True,newHookSignatureCount=checked,newNativeHelperScopedTypeRefsAbsent=True)
def main():
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);args=p.parse_args()
 assert sha(args.base_apk.read_bytes())==BASE
 out=args.output_directory.resolve();assert out.is_relative_to(ROOT) and not out.exists();out.mkdir(parents=True)
 sources=sorted(HERE.glob('*.cs'))+[HERE/'build.py',HERE/'README.md'];pins={str(x.relative_to(ROOT)):sha(x.read_bytes())for x in sources}
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';csc=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
 def run(label,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=180);(out/(label+'.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,label
 with zipfile.ZipFile(args.base_apk)as apk,tempfile.TemporaryDirectory(prefix='hyjs66-')as tmp:
  work=Path(tmp);refs=work/'refs';refs.mkdir();before=apk.read(MAIN);assert sha(before)==MAIN_SHA
  for n in apk.namelist():
   if n.startswith(MANAGED)and n.endswith('.dll'):(refs/Path(n).name).write_bytes(apk.read(n))
  for name,source in [('HYJS.Trial66.dll','TrialRuntime.cs'),('HYJS.Commerce66.dll','RuntimeCommerce.cs')]:
   run('compile-'+name,[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/codepage:65001','/out:'+str(out/name),*['/r:'+str(x)for x in sorted(refs.glob('*.dll'))],HERE/source])
  current=refs/'Assembly-CSharp.dll'
  for source,helper,output in [('TrialInjector.cs','HYJS.Trial66.dll',work/'trial.dll'),('CommerceInjector.cs','HYJS.Commerce66.dll',out/'Assembly-CSharp.dll')]:
   project=work/source.removesuffix('.cs');project.mkdir();shutil.copy2(HERE/source,project/'Program.cs')
   proj=project/'Inject.csproj';proj.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(runtime/'injector/bin/Debug/net8.0/dnlib.dll')+'</HintPath></Reference></ItemGroup></Project>')
   run('compile-'+source,[dotnet,'build',proj,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
   run('inject-'+source,[dotnet,project/'bin/Debug/net8.0/Inject.dll',current,out/helper,output]);current=output
  verified=proof(before,current.read_bytes())
  payload={MANAGED+n:str((out/n).relative_to(ROOT))for n in ('Assembly-CSharp.dll','HYJS.Trial66.dll','HYJS.Commerce66.dll')}
  report=dict(schema='hyjs-feedback66-managed-v1',baseAPKSHA256=BASE,inputManagedSHA256=MAIN_SHA,outputManagedSHA256=sha(current.read_bytes()),inputDexSHA256=sha(apk.read('classes2.dex')),payloadFiles=payload,replacementEntries={n:sha((ROOT/f).read_bytes())for n,f in payload.items()},sourcePins=pins,managedOnly=True,inheritedJavaByteExact=True,inheritedJavaPublicProtectedABIExact=True,allowedChangedMethods=[name for name,rid,_ in methods(dnfile.dnPE(data=before)) if rid in {**TRIAL,**COMMERCE}],targetPins={hex(0x06000000+r):h for r,h in {**TRIAL,**COMMERCE}.items()},apkBuilt=False,deviceTested=False,**verified)
  assert pins=={str(x.relative_to(ROOT)):sha(x.read_bytes())for x in sources};(out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(verified))
if __name__=='__main__':main()
