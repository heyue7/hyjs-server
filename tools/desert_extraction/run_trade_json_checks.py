"""Check trade JSON with the pinned game LitJson and built QA helper."""
import argparse,hashlib,json,os,subprocess,tempfile,zipfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
HERE=Path(__file__).resolve().parent
FIXTURE=ROOT/'analysis/desert-extraction-v4-client/trade-none-redacted.json'
BASE_SHA='25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
LIT_SHA='151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230'
FIXTURE_SHA='a97d0471b8bfb06c62d105ea353310e6a38206bd9ddaa6bcf900fda691e1edfd'

def file_sha(path):
 h=hashlib.sha256()
 with Path(path).open('rb') as f:
  for data in iter(lambda:f.read(1024*1024),b''):h.update(data)
 return h.hexdigest()
def require(condition,message):
 if not condition:raise ValueError(message)
def run(cmd,env):
 r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,env=env,text=True,capture_output=True,timeout=30)
 if r.returncode:raise RuntimeError('Trade regression failed: '+r.stderr[-1000:]+r.stdout[-1000:])
 if r.stdout.strip():print(r.stdout.strip())

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--qa-input',type=Path,required=True)
 ap.add_argument('--runtime',type=Path,default=Path('/Users/lirui/LR/hyjs/.tools/native-batch'))
 a=ap.parse_args();qa=a.qa_input.resolve();base=a.base_apk.resolve();rt=a.runtime.resolve()/'dotnet'
 require(file_sha(base)==BASE_SHA and file_sha(FIXTURE)==FIXTURE_SHA,'Pinned base or fixture differs')
 m=json.loads((qa/'manifest.json').read_text())
 require(m.get('qaOnly') is True and m.get('neverPromote') is True and m.get('baseAPKSHA256')==BASE_SHA,'QA input differs')
 for name in ('NestedArrayJson.cs','TradeController.cs','FieldWire.cs'):
  rel='tools/desert_extraction/'+name
  require(m['sourcePins'][rel]==file_sha(ROOT/rel),'Source differs from built helper')
 def payload(entry):
  path=(ROOT/m['payloadFiles'][entry]).resolve()
  require(path.is_relative_to(qa) and file_sha(path)==m['replacementEntries'][entry],'QA payload differs')
  return path
 managed=payload('assets/bin/Data/Managed/Assembly-CSharp.dll')
 helper=payload('assets/bin/Data/Managed/SoulFlowerDesertV4.dll')
 dotnet=rt/'dotnet';csc=next((rt/'sdk').glob('*/Roslyn/bincore/csc.dll'))
 ref=next((rt/'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
 env=dict(os.environ,DOTNET_PROCESSOR_COUNT='2',DOTNET_CLI_TELEMETRY_OPTOUT='1')
 sample={'code':0,'msg':'success','data':{'status':'open','tradeId':'123e4567-e89b-12d3-a456-426614174000','initiatorId':2,'receiverId':3,'version':7,'expiresAtMs':1793206264009,'serverTimeMs':1793206064009,'active':True,'previewValid':True,'offers':[[9223372036854775807,1234567890123,0,0,0,0],[7,0,0,0,0,9]],'confirmed':[False,True],'offerCreditValues':['9223372036854775807','100'],'terminal':{'chips':[0,0,0,0,0,0],'protected':[0,0,0,0,0,0],'creditBalance':9223372036854775807,'revision':4},'afterChips':[0,0,0,0,0,0],'afterProtected':[0,0,0,0,0,0],'note':'brackets [ ] and quote " and slash \\ preserved'}}
 with tempfile.TemporaryDirectory(prefix='hyjs-v4-trade-check-') as td:
  p=Path(td)
  with zipfile.ZipFile(base) as z:
   for name in ('LitJson.dll','UnityEngine.dll','Assembly-CSharp-firstpass.dll','HYJS.Field63.dll','LocalFieldSettlement.dll'):
    (p/name).write_bytes(z.read('assets/bin/Data/Managed/'+name))
  require(file_sha(p/'LitJson.dll')==LIT_SHA,'Original LitJson differs')
  (p/'Assembly-CSharp.dll').write_bytes(managed.read_bytes());(p/'SoulFlowerDesertV4.dll').write_bytes(helper.read_bytes())
  (p/'open.json').write_text(json.dumps(sample,separators=(',',':')))
  for name in ('negative','wrongsize','badprotect'):
   x=json.loads(json.dumps(sample))
   if name=='negative':x['data']['offers'][0][0]=-1
   elif name=='wrongsize':x['data']['offers'][1].pop()
   else:x['data']['terminal']['protected'][0]=1
   (p/(name+'.json')).write_text(json.dumps(x,separators=(',',':')))
  runtime=json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':ref.parents[1].name}}})
  refs=['/reference:'+str(x) for x in sorted(ref.glob('*.dll'))]+['/reference:'+str(p/'LitJson.dll')]
  for name,sources,fixtures in (
    ('JsonChecks',[HERE/'NestedArrayJson.cs',HERE/'TradeJsonChecks.cs'],[FIXTURE,p/'open.json']),
    ('ParserChecks',[HERE/'TradeControllerParseChecks.cs'],[FIXTURE,p/'open.json',p/'negative.json',p/'wrongsize.json',p/'badprotect.json'])):
   (p/(name+'.runtimeconfig.json')).write_text(runtime)
   run([dotnet,csc,'/nologo','/target:exe','/nostdlib+','/langversion:3','/out:'+str(p/(name+'.dll')),*refs,*sources],env)
   run([dotnet,p/(name+'.dll'),*fixtures],env)
 print('Temporary trade check directory cleaned')
if __name__=='__main__':main()
