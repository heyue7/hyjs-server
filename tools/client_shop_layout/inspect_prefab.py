"""Read original card prefab geometry using the established binary decoder."""
import argparse,contextlib,hashlib,io,json
from pathlib import Path
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[1]
def main():
 p=argparse.ArgumentParser();p.add_argument("--apk",type=Path,required=True);a=p.parse_args();result={}
 decoder=ROOT/"tools/client_stamina_sweep/decode_buyitempage.py"
 for resource in ("shopinnerpage/popularityshop","shopinnerpage/pointshop"):
  code=decoder.read_text().replace("APK = ROOT / "+repr("build/hyjs-local.apk"),"APK = Path("+repr(str(a.apk.resolve()))+")").replace("pages/buyitempage",resource)
  buffer=io.StringIO()
  with contextlib.redirect_stdout(buffer):exec(compile(code,str(decoder),"exec"),{"__file__":str(decoder),"__name__":"__main__"})
  data=json.loads(buffer.getvalue());rows=[]
  for o in data["objects"]:
   if o["object"] in ("ShopItem","IconRoot","Name","Condition","BtnBuy") or any(c.get("class","").startswith("UI.Shop") for c in o["components"]):
    rows.append({k:v for k,v in o.items() if k!="components"}|{"components":[{k:v for k,v in c.items() if k in ("class","text","width","height","pivot","fontSize")}for c in o["components"]]})
  result[resource]={"entry":data["entry"],"geometry":rows}
 print(json.dumps({"apkSHA256":hashlib.sha256(a.apk.read_bytes()).hexdigest(),"resources":result},ensure_ascii=False,indent=2))
if __name__=="__main__":main()
