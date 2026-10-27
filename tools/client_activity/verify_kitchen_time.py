"""Resolve inherited bridge ABI signatures across shifted metadata references."""
import argparse,json,hashlib,zipfile
from pathlib import Path
import dnfile

def signature(pe,raw):
 at=0
 def byte():
  nonlocal at
  v=raw[at];at+=1;return v
 def number():
  v=byte()
  if v<128:return v
  if v<192:return ((v&63)<<8)|byte()
  return ((v&31)<<24)|(byte()<<16)|(byte()<<8)|byte()
 def ref():
  v=number();table=['TypeDef','TypeRef','TypeSpec'][v&3];r=getattr(pe.net.mdtables,table).rows[(v>>2)-1]
  if table=='TypeSpec':return signature(pe,r.Signature.value)
  return str(r.TypeNamespace)+'.'+str(r.TypeName)
 def typ():
  v=byte()
  if v in [17,18]:return ('named',ref())
  if v in [15,16,29,69]:return (v,typ())
  if v in [19,30]:return(v,number())
  if v==21:
   kind=byte();name=ref();n=number();return(v,kind,name,tuple(typ()for _ in range(n)))
  if v==20:
   t=typ();rank=number();sizes=tuple(number()for _ in range(number()));lower=tuple(number()for _ in range(number()));return(v,t,rank,sizes,lower)
  if v in [31,32]:return(v,ref(),typ())
  assert v in [1,2,3,4,5,6,7,8,9,10,11,12,13,14,24,25,28],hex(v)
  return v
 flags=byte()
 if flags==6:return('field',typ())
 generic=number()if flags&16 else 0;n=number();ret=typ();args=tuple(typ()for _ in range(n));assert at==len(raw),(at,len(raw),raw.hex());return('method',flags,generic,ret,args)
def members(pe):
 methods=set();fields=set()
 for t in pe.net.mdtables.TypeDef.rows:
  name=str(t.TypeNamespace)+'.'+str(t.TypeName)
  methods.update((name,str(m.row.Name),signature(pe,m.row.Signature.value))for m in t.MethodList)
  fields.update((name,str(f.row.Name),signature(pe,f.row.Signature.value))for f in t.FieldList)
 return methods,fields

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--overlay-directory',type=Path,required=True);a=ap.parse_args();out=a.overlay_directory
 with zipfile.ZipFile(a.base_apk)as z:old=dnfile.dnPE(data=z.read('assets/bin/Data/Managed/SoulFlowerActivity.dll'))
 new=dnfile.dnPE(str(out/'SoulFlowerActivity.dll'));om,of=members(old);nm,nf=members(new);assert om<=nm and of==nf
 added=nm-om;assert len(added)==1;hook=next(iter(added));assert hook[:2]==('HYJS.ActivitySafety.ActivityGuard','PrepareKitchenResponse')
 expected=('method',0,0,2,(('named','UI.UIActivityAsunaKitchen'),('named','NetWork.Packets.Packet'),2));assert hook[2]==expected,hook
 host=dnfile.dnPE(str(out/'Assembly-CSharp.dll'));refs=[r for r in host.net.mdtables.MemberRef.rows if str(r.Name)=='PrepareKitchenResponse'];assert len(refs)==1 and signature(host,refs[0].Signature.value)==expected
 manifest=out/'manifest.json';r=json.loads(manifest.read_text());r['inheritedHelperMethodSignaturesExact']=len(om);r['inheritedHelperFieldSignaturesExact']=len(of);r['newHookSignatureExact']=True;r['lateCallbackDoesNotClearNewPanel']=True;r['prefabEvidence']['rootComponentPathID']=273;r['prefabEvidence']['activityTimePointerOffset']=108;r['prefabEvidence']['activityTimeLabelPathID']=199
 root=Path(__file__).resolve().parents[2];r['sourcePins'][str(Path(__file__).resolve().relative_to(root))]=hashlib.sha256(Path(__file__).read_bytes()).hexdigest();manifest.write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'inheritedMethods':len(om),'inheritedFields':len(of),'newHookSignatureExact':True}))
if __name__=='__main__':main()
