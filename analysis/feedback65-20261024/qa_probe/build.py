import argparse,hashlib,json,subprocess,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
HERE=Path(__file__).resolve().parent
ap=argparse.ArgumentParser();ap.add_argument('--main',type=Path,default=ROOT/'build/field64-overlay/Assembly-CSharp.dll');ap.add_argument('--apk',type=Path,default=Path('/Users/lirui/LR/hyjs/build/hyjs-local.apk'));ap.add_argument('--expected-main-sha256',default='57f12e1c1aaa0ee46bc00d12be6b06bba5fda16533b47cc50a02c19c5e5e4f22');ap.add_argument('--expected-apk-sha256',default='4a85ed009e86c649b5954e8d1f02165e9d1951ec89256196f3dccd9076e6183d');a=ap.parse_args()
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
assert a.main.is_file() and a.apk.is_file(),'Select current field64 Main and published63 reference APK'
assert sha(a.main)==a.expected_main_sha256,'QA reference Main drift'
assert sha(a.apk)==a.expected_apk_sha256,'QA reference APK drift'
source=HERE/'NativeBatchRunner.cs';identity=sha(source)[:16];out=HERE/('HYJS65QA_'+identity+'.dll')
tools=ROOT/'.tools/native-batch';dotnet=tools/'dotnet/dotnet';csc=next((tools/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
with tempfile.TemporaryDirectory(prefix='refs-',dir=HERE) as tmp:
 refs=Path(tmp);(refs/'Assembly-CSharp.dll').write_bytes(a.main.read_bytes())
 with zipfile.ZipFile(a.apk) as z:
  for name in ['mscorlib.dll','System.dll','System.Core.dll','UnityEngine.dll','Assembly-CSharp-firstpass.dll']:
   (refs/name).write_bytes(z.read('assets/bin/Data/Managed/'+name))
 cmd=['rtk','proxy',str(dotnet),str(csc),'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(out),*['/r:'+str(p)for p in sorted(refs.glob('*.dll'))],str(source)]
 r=subprocess.run(cmd,capture_output=True,text=True);(HERE/'compile.log').write_text(r.stdout+r.stderr)
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 references={p.name:sha(p)for p in refs.glob('*.dll')}
(HERE/'manifest.json').write_text(json.dumps(dict(sourceSHA256=sha(source),mainSHA256=sha(a.main),referenceSHA256=references,caller=out.name,callerSHA256=sha(out),csharp3Compiled=True,referenceMainExpectedSHA256=a.expected_main_sha256,referenceAPKSHA256=sha(a.apk),mainChanged=False,phoneOperated=False,runtimeTested=False,commandMode='explicit-whitelist-legal-ui-and-movement-only'),indent=2)+'\n')
print(out)
