"""Run the actual guard source with minimal UI doubles; no Unity/device claim."""
import argparse,json,subprocess,tempfile
from pathlib import Path
SOURCE=Path(__file__).resolve().parent
STUB=r"""
using System;using System.Collections.Generic;
namespace UnityEngine{public class Object{public static int destroyed;public static void Destroy(Object o){destroyed++;}}public class GameObject:Object{public bool active=true;public void SetActive(bool b){active=b;}}}
namespace ProtoBuf.Message{public class LoginRewardItem{public int state;}public class SCLoginRewardInfo{public int isShow;public List<LoginRewardItem> items;}}
namespace NetWork.Packets{public class Packet{public object kBody;}}
namespace UI{public class Page{}public class TownPage:Page{public UnityEngine.GameObject tenDaySignBtn=new UnityEngine.GameObject();public T GetComponentInChildren<T>()where T:new(){return new T();}}public class ShowRight{public void ResetIconPos(int i){}}public class PageManager{public static PageManager Instance=new PageManager();public Page currentPage;}public class TenDaySignPage:Page{public SimpleGrid simpleGrid=new SimpleGrid();public UnityEngine.GameObject itemPref=new UnityEngine.GameObject();public int closed;public void Close(){closed++;}}}
public class SimpleGrid{public List<UnityEngine.GameObject> childs=new List<UnityEngine.GameObject>();public void Reposition(){}}
public static class BoxManager{public static string last;public static void CreatePopupTextBox(string s,float f){last=s;}}
public static class Program{static void Check(bool b,string n){if(!b)throw new Exception(n);}public static void Main(){
 var town=new UI.TownPage();UI.PageManager.Instance.currentPage=town;
 var valid=new ProtoBuf.Message.SCLoginRewardInfo{isShow=1,items=new List<ProtoBuf.Message.LoginRewardItem>{new ProtoBuf.Message.LoginRewardItem{state=1}}};
 var packet=new NetWork.Packets.Packet{kBody=valid};
 Check(HYJS.NewbieSign.NewbieSignGuard.CanOpen(packet,true),"live actual list denied");
 Check(!HYJS.NewbieSign.NewbieSignGuard.CanOpen(packet,false)&&BoxManager.last==null,"failure fabricated notice");
 valid.isShow=0;valid.items[0].state=2;
 Check(!HYJS.NewbieSign.NewbieSignGuard.CanOpen(packet,true)&&BoxManager.last.Contains("全部领取")&&!town.tenDaySignBtn.active,"completion not guarded");
 valid.items.Clear();Check(!HYJS.NewbieSign.NewbieSignGuard.CanOpen(packet,true)&&BoxManager.last.Contains("未开放"),"closed empty page not guarded");
 Check(!HYJS.NewbieSign.NewbieSignGuard.CanOpen(null,true),"null packet accepted");
 var page=new UI.TenDaySignPage();var real=new List<ProtoBuf.Message.LoginRewardItem>{new ProtoBuf.Message.LoginRewardItem{state=2}};
 Check(HYJS.NewbieSign.NewbieSignGuard.CanRender(page,real)&&page.closed==0,"real claimed items made unreadable");
 var child=new UnityEngine.GameObject();page.simpleGrid.childs.Add(child);
 Check(!HYJS.NewbieSign.NewbieSignGuard.CanRender(page,new List<ProtoBuf.Message.LoginRewardItem>())&&page.closed==1&&page.simpleGrid.childs.Count==0&&!child.active&&!page.itemPref.active&&UnityEngine.Object.destroyed==1,"empty prefab cleanup/close failed");
 Check(!HYJS.NewbieSign.NewbieSignGuard.CanRender(null,null),"null page accepted");
 Console.WriteLine("PASS: 8 actual-helper guard cases; UI doubles, no Unity/device execution");}}
"""
def main():
 ap=argparse.ArgumentParser();ap.add_argument("--tool-root",type=Path,required=True);ap.add_argument("--output",type=Path,required=True);a=ap.parse_args()
 with tempfile.TemporaryDirectory(prefix="hyjs-newbie-guard-check-")as tmp:
  w=Path(tmp);(w/"Program.cs").write_text(STUB);(w/"NewbieSign.cs").write_bytes((SOURCE/"NewbieSign.cs").read_bytes());(w/"Check.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>')
  r=subprocess.run(["rtk","proxy",str(a.tool_root/".tools/native-batch/dotnet/dotnet"),"run","--project",str(w/"Check.csproj"),"--verbosity","quiet","-p:NuGetAudit=false"],capture_output=True,text=True,timeout=120)
  a.output.write_text(r.stdout+r.stderr)
  if r.returncode:raise RuntimeError("Actual guard source check failed: "+str(a.output))
  print(r.stdout.strip())
if __name__=="__main__":main()
