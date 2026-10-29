"""Exercise the built QA gather reward parser with the fixed game LitJson."""
import argparse,json,os,subprocess,tempfile,zipfile
from pathlib import Path
from run_trade_json_checks import BASE_SHA,LIT_SHA,file_sha,require
ROOT=Path(__file__).resolve().parents[2]
HERE=Path(__file__).resolve().parent

def run(command,env):
 r=subprocess.run(['rtk','proxy',*map(str,command)],cwd=ROOT,env=env,text=True,capture_output=True,timeout=30)
 if r.returncode:raise RuntimeError('Gather reward check failed: '+r.stderr[-1500:]+r.stdout[-1500:])
 if r.stdout.strip():print(r.stdout.strip())

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--qa-input',type=Path,required=True)
 ap.add_argument('--runtime',type=Path,default=Path('/Users/lirui/LR/hyjs/.tools/native-batch'));a=ap.parse_args()
 base=a.base_apk.resolve();qa=a.qa_input.resolve();rt=a.runtime.resolve()/'dotnet'
 require(file_sha(base)==BASE_SHA,'Base APK differs');m=json.loads((qa/'manifest.json').read_text())
 require(m.get('qaOnly') is True and m.get('neverPromote') is True and m.get('baseAPKSHA256')==BASE_SHA,'Not the pinned QA overlay')
 require(m['sourcePins']['tools/desert_extraction/FieldRuntime.cs']==file_sha(HERE/'FieldRuntime.cs'),'Built parser source differs')
 def payload(entry):
  f=(ROOT/m['payloadFiles'][entry]).resolve();require(f.is_relative_to(qa) and file_sha(f)==m['replacementEntries'][entry],'QA payload differs');return f
 managed=payload('assets/bin/Data/Managed/Assembly-CSharp.dll');helper=payload('assets/bin/Data/Managed/SoulFlowerDesertV4.dll')
 dotnet=rt/'dotnet';csc=next((rt/'sdk').glob('*/Roslyn/bincore/csc.dll'));ref=next((rt/'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
 env=dict(os.environ,DOTNET_PROCESSOR_COUNT='2',DOTNET_CLI_TELEMETRY_OPTOUT='1')
 with tempfile.TemporaryDirectory(prefix='hyjs-v4-gather-reward-check-') as td:
  p=Path(td)
  with zipfile.ZipFile(base) as z:
   for name in ('LitJson.dll','UnityEngine.dll','Assembly-CSharp-firstpass.dll','HYJS.Field63.dll','LocalFieldSettlement.dll'):
    (p/name).write_bytes(z.read('assets/bin/Data/Managed/'+name))
  require(file_sha(p/'LitJson.dll')==LIT_SHA,'Original LitJson differs')
  (p/'Assembly-CSharp.dll').write_bytes(managed.read_bytes());(p/'SoulFlowerDesertV4.dll').write_bytes(helper.read_bytes())
  (p/'Checks.runtimeconfig.json').write_text(json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':ref.parents[1].name}}}))
  refs=['/reference:'+str(x) for x in sorted(ref.glob('*.dll'))]+['/reference:'+str(p/'LitJson.dll')]
  run([dotnet,csc,'/nologo','/target:exe','/nostdlib+','/langversion:3','/out:'+str(p/'Checks.dll'),*refs,HERE/'GatherRewardChecks.cs'],env)
  run([dotnet,p/'Checks.dll'],env)
 print('Temporary gather reward check directory cleaned')
if __name__=='__main__':main()
