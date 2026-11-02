"""Exercise the actual helper against minimal UI/network doubles, not Unity."""
import argparse
import shutil
import subprocess
import tempfile
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("--tool-root", type=Path, required=True)
args = parser.parse_args()
source = Path(__file__).parent
dotnet = args.tool_root / ".tools/native-batch/dotnet/dotnet"

stubs = r'''
using System; using System.Collections.Generic;
namespace UnityEngine {
 public class Object { public static void DontDestroyOnLoad(Object o){} }
 public class MonoBehaviour:Object {}
 public class GameObject:Object { public GameObject(string n){} public T AddComponent<T>() where T:new(){return new T();} }
 public static class Time { public static float realtimeSinceStartup; }
 public static class PlayerPrefs {
  static Dictionary<string,string> data=new Dictionary<string,string>();
  public static string GetString(string key,string def){string value;return data.TryGetValue(key,out value)?value:def;}
  public static void SetString(string key,string value){data[key]=value;}
  public static void Save(){}
 }
}
namespace NetWork.Packets { public class Packet {public object kBody;} }
namespace ProtoBuf.Message {
 public class CSLuxuryEquipmentInfo {}
 public class SCLuxuryEquipmentInfo {public int id,rewardState;public long endTime,currentTime;}
}
public class Player {public long playerId=5;}
public static class StaticData {public static Player playerData=new Player();public static byte[] sessionKey=new byte[16];public static string httpServer="s1";public static bool isOpeningLuxuryEquipActivity;}
public static class NetAPI {
 public class Call {public int id;public Action<NetWork.Packets.Packet,bool> callback;}
 public static bool available=true; public static List<Call> calls=new List<Call>();
 public static bool SendHttp(int id,object req,Action<NetWork.Packets.Packet,bool> cb){if(!available)return false;calls.Add(new Call{id=id,callback=cb});return true;}
}
namespace UI {
 public class UICopy {}
 public class PageManager {public static PageManager Instance=new PageManager();public object currentPage;public List<string> opened=new List<string>();public void OpenPage(string name,string options){opened.Add(name+"?"+options);} }
}
'''
tests = r'''
using System;using ProtoBuf.Message;using NetWork.Packets;using R=HYJS.LuxuryDragon.Runtime;
class Test {
 static int count;static void A(bool ok,string why){if(!ok)throw new Exception(why);count++;}
 static void Tick(float delta){UnityEngine.Time.realtimeSinceStartup+=delta;R.Tick();}
 static NetAPI.Call Last(){return NetAPI.calls[NetAPI.calls.Count-1];}
 static void Reply(NetAPI.Call c,int id,long end,int state){c.callback(new Packet{kBody=new SCLuxuryEquipmentInfo{id=id,endTime=end,currentTime=1000000,rewardState=state}},true);}
 static void Start(){R.Schedule();Tick(1);}
 static void Main(){
  UI.PageManager.Instance.currentPage=new object();R.Schedule();Tick(1);A(NetAPI.calls.Count==0,"sent while away from story");
  UI.PageManager.Instance.currentPage=new UI.UICopy();NetAPI.available=false;Tick(1);A(NetAPI.calls.Count==0,"busy transport recorded request");
  NetAPI.available=true;Tick(1);A(NetAPI.calls.Count==1&&Last().id==57107,"busy retry did not query");var first=Last();Tick(1);A(NetAPI.calls.Count==1,"duplicate in-flight request");
  Reply(first,2,2000000,0);A(UI.PageManager.Instance.opened.Count==1,"first active offer did not open");A(UI.PageManager.Instance.opened[0]=="LuxuryPage?id=2&endTime=2000000&rewardState=0","original page arguments changed");A(StaticData.isOpeningLuxuryEquipActivity,"activity entry absent");
  Start();Reply(Last(),2,2000000,0);A(UI.PageManager.Instance.opened.Count==1,"same window reopened");
  Start();Reply(Last(),2,3000000,0);A(UI.PageManager.Instance.opened.Count==2,"new round did not open");
  Start();Reply(Last(),2,999999,0);A(UI.PageManager.Instance.opened.Count==2&&!StaticData.isOpeningLuxuryEquipActivity,"expired unpaid offer shown");
  Start();Reply(Last(),2,999999,1);A(UI.PageManager.Instance.opened.Count==3&&StaticData.isOpeningLuxuryEquipActivity,"earned expired offer unavailable");
  Start();Reply(Last(),0,4000000,0);A(UI.PageManager.Instance.opened.Count==3&&!StaticData.isOpeningLuxuryEquipActivity,"missing offer shown");
  Start();Reply(Last(),2,4000000,2);A(UI.PageManager.Instance.opened.Count==3,"claimed offer shown");
  Start();var timed=Last();Tick(5.1f);var retry=Last();A(!Object.ReferenceEquals(timed,retry),"timed out request did not retry");Reply(timed,2,5000000,0);A(UI.PageManager.Instance.opened.Count==3,"stale timed out response shown");Reply(retry,2,5000000,0);A(UI.PageManager.Instance.opened.Count==4,"retry response lost");
  Start();var switched=Last();StaticData.sessionKey[0]=1;Reply(switched,2,6000000,0);A(UI.PageManager.Instance.opened.Count==4,"old session response shown");
  Start();var away=Last();UI.PageManager.Instance.currentPage=new object();Reply(away,2,6000000,0);A(UI.PageManager.Instance.opened.Count==4,"popup interrupted another page");UI.PageManager.Instance.currentPage=new UI.UICopy();Tick(1);Reply(Last(),2,6000000,0);A(UI.PageManager.Instance.opened.Count==5,"return to story did not retry");
  Start();var failure=Last();failure.callback(null,false);Tick(1);A(!Object.ReferenceEquals(failure,Last()),"network failure did not retry");Reply(Last(),2,7000000,0);A(UI.PageManager.Instance.opened.Count==6,"network recovery lost popup");
  Start();var oldRole=Last();StaticData.playerData.playerId=8;Reply(oldRole,2,8000000,0);A(UI.PageManager.Instance.opened.Count==6,"other player response shown");
  Start();var oldServer=Last();StaticData.httpServer="s2";Reply(oldServer,2,8000000,0);A(UI.PageManager.Instance.opened.Count==6,"other server response shown");
  Start();Reply(Last(),2,8000000,0);A(UI.PageManager.Instance.opened.Count==7,"new player/server offer missing");
  Console.WriteLine("PASS "+count+" actual-source assertions (minimal doubles; not Unity/device)");
 }
}
'''

with tempfile.TemporaryDirectory(prefix="hyjs-luxury-logic-") as temp:
    work = Path(temp)
    shutil.copy2(source / "Runtime.cs", work / "Runtime.cs")
    (work / "Stubs.cs").write_text(stubs)
    (work / "Test.cs").write_text(tests)
    (work / "Cases.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><LangVersion>3</LangVersion><GenerateAssemblyInfo>false</GenerateAssemblyInfo><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable></PropertyGroup></Project>')
    result = subprocess.run(["rtk", "proxy", str(dotnet), "run", "--project", str(work / "Cases.csproj"), "-p:NuGetAudit=false"], capture_output=True, text=True, timeout=180)
    print(result.stdout + result.stderr)
    raise SystemExit(result.returncode)
