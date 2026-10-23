"""Runs actual PaymentFeedback.cs with minimal Unity/network doubles, not Unity QA."""
import argparse,subprocess,tempfile,shutil
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--tool-root',type=Path,required=True);a=p.parse_args()
S=Path(__file__).parent;dotnet=a.tool_root/'.tools/native-batch/dotnet/dotnet'
stubs=r'''
using System;using System.Collections.Generic;
namespace UnityEngine {
 public class Object{public static void DontDestroyOnLoad(Object o){}}
 public class MonoBehaviour:Object {public T GetComponent<T>() where T:new(){return new T();}public void CancelInvoke(){}}
 public class GameObject:Object{public GameObject(string n){}public T AddComponent<T>()where T:new(){return new T();}}
 public static class Time{public static float realtimeSinceStartup;}
 public class AndroidJavaClass:IDisposable{public AndroidJavaClass(string n){}public void CallStatic(string n,params object[] a){Native.id=(string)a[0];Native.cents=(long)a[1];}public void Dispose(){}}
}
public static class Native{public static string id;public static long cents;}
public class PlatformSDKCallBack:UnityEngine.MonoBehaviour{public static PlatformSDKCallBack Instance=new PlatformSDKCallBack();public string orderId,payMentId;public long tick;}
public static class BoxManager{public static List<string> notices=new List<string>();public static void CreatePopupTextBox(string s,float f){notices.Add(s);}}
public static class StaticData{public static UI.UIPlayerData playerData=new UI.UIPlayerData();public static byte[] sessionKey=new byte[16];}
public class GameConfig{public static GameConfig Instance=new GameConfig();public string GetChannelId="local";}
namespace NetWork.Packets{public class Packet{public object kBody;}}
namespace ProtoBuf.Message{
 public class CSGenerateOrder{public string paymentId,moneyType,channelId,shopId;public double money;}
 public class CSCheckOrder{public string orderId,paymentId;}
 public class TransactionData{public string orderId,paymentId,payStatus;}
 public class SCGenerateOrder{public string orderId;public TransactionData transactionData;}
 public class SCCheckOrder{public TransactionData transactionData;}
}
public static class NetAPI{
 public class Call{public int id;public object req;public Action<NetWork.Packets.Packet,bool> callback;}
 public static List<Call> calls=new List<Call>();public static object SendHttp(int id,object req,Action<NetWork.Packets.Packet,bool> cb){calls.Add(new Call{id=id,req=req,callback=cb});return null;}
}
namespace UI{
 public class UIPlayerData{public long playerId=2;}
 public class ChargeData:UnityEngine.MonoBehaviour{public void InitPayMent(){}}
 public class ChargePage:UnityEngine.MonoBehaviour{public ChargeData chargeData=new ChargeData();public void InitTop(){}}
 public class PageManager{public static PageManager Instance=new PageManager();public object currentPage;}
}
'''
tests=r'''
using System;using System.Reflection;using ProtoBuf.Message;using NetWork.Packets;using PF=HYJS.PaymentFeedback.PaymentFeedback;
class Test{
 static int count;static void Assert(bool b,string s){if(!b)throw new Exception(s);count++;}
 static PF host{get{return (PF)typeof(PF).GetField("instance",BindingFlags.Static|BindingFlags.NonPublic).GetValue(null);}}
 static void Tick(){typeof(PF).GetMethod("Update",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(host,null);}
 static string Begin(string product){PF.Begin(new UI.ChargeData(),"name","30",product);return Native.id;}
 static void Ready(string id){host.OnRechargeBalance(id+"|ready");}
 static void Generated(NetAPI.Call c,string order,string product,bool ok){c.callback(new Packet{kBody=new SCGenerateOrder{orderId=order,transactionData=new TransactionData{orderId=order,paymentId=product}}},ok);}
 static void Generated(NetAPI.Call c,string order,string product){Generated(c,order,product,true);}
 static void Checked(NetAPI.Call c,string order,string product,string status){c.callback(new Packet{kBody=new SCCheckOrder{transactionData=new TransactionData{orderId=order,paymentId=product,payStatus=status}}},true);}
 static void Main(){
 string a=Begin("A");host.OnRechargeBalance(a+"|insufficient");Assert(NetAPI.calls.Count==0,"insufficient created order");Assert(BoxManager.notices[0].Contains("不足"),"insufficient notice");
 a=Begin("A");host.OnRechargeBalance(a+"|error");Assert(NetAPI.calls.Count==0,"error treated ready");
 a=Begin("A");PF.Begin(new UI.ChargeData(),"n","30","B");Assert(Native.id==a,"double click replaced attempt");Ready(a);Assert(NetAPI.calls.Count==1,"one generate");var old=NetAPI.calls[0];
 UnityEngine.Time.realtimeSinceStartup=31;Tick();Assert(BoxManager.notices[BoxManager.notices.Count-1].Contains("尚未确认"),"timeout message");
 string b=Begin("A");Ready(b);var next=NetAPI.calls[1];Generated(old,"old","A");Assert(NetAPI.calls.Count==2,"stale same product generated query");old.callback(null,false);Assert(BoxManager.notices[BoxManager.notices.Count-1].Contains("尚未确认"),"stale failure notice");
 Generated(next,"new","A");var query=NetAPI.calls[2];Checked(query,"new","A","PAY_STATUS_WAIT");Assert(BoxManager.notices[BoxManager.notices.Count-1].Contains("尚未支付"),"WAIT message");
 string c=Begin("B");Ready(c);var third=NetAPI.calls[3];Generated(third,"third","B");var q3=NetAPI.calls[4];Checked(query,"new","A","PAY_STATUS_SUCCESS");Assert(!BoxManager.notices.Contains("购买成功"),"stale query success accepted");
 Checked(q3,"third","WRONG","PAY_STATUS_SUCCESS");Assert(!BoxManager.notices.Contains("购买成功"),"wrong product accepted");
 c=Begin("B");Ready(c);third=NetAPI.calls[5];Generated(third,"fourth","B");q3=NetAPI.calls[6];Checked(q3,"fourth","B","PAY_STATUS_SUCCESS");Checked(q3,"fourth","B","PAY_STATUS_SUCCESS");Assert(BoxManager.notices.FindAll(x=>x=="购买成功").Count==1,"success not once");
 c=Begin("C");Ready(c);third=NetAPI.calls[7];StaticData.sessionKey[0]=1;Tick();Generated(third,"switch","C");Assert(NetAPI.calls.Count==8,"account switch accepted");
 c=Begin("D");Ready(c);third=NetAPI.calls[8];third.callback(new Packet{kBody=new SCGenerateOrder{orderId="one",transactionData=new TransactionData{orderId="two",paymentId="D"}}},true);Assert(NetAPI.calls.Count==9,"mismatch generated order accepted");
 c=Begin("E");UnityEngine.Time.realtimeSinceStartup=62;host.OnRechargeBalance(c+"|ready");Assert(NetAPI.calls.Count==9,"expired preflight created order");
 c=Begin("F");Ready(c);third=NetAPI.calls[9];Generated(third,"f","F");q3=NetAPI.calls[10];Checked(q3,"WRONG","F","PAY_STATUS_SUCCESS");Assert(BoxManager.notices.FindAll(x=>x=="购买成功").Count==1,"wrong order accepted");
 c=Begin("G");Ready(c);third=NetAPI.calls[11];Generated(third,"g","G");q3=NetAPI.calls[12];Checked(q3,"g","G","SUCCESS");Assert(BoxManager.notices.FindAll(x=>x=="购买成功").Count==1,"fake success accepted");
 c=Begin("H");host.OnRechargeBalance("old|ready");Assert(NetAPI.calls.Count==13,"old balance callback accepted");host.OnRechargeBalance(c+"|insufficient");
 Console.WriteLine("PASS "+count+" actual-source boundary assertions (minimal doubles; not Unity)");
 }
}
'''
with tempfile.TemporaryDirectory(prefix='hyjs-payment-logic-')as temp:
 w=Path(temp);shutil.copy2(S/'PaymentFeedback.cs',w/'PaymentFeedback.cs');(w/'Stubs.cs').write_text(stubs);(w/'Test.cs').write_text(tests)
 (w/'Cases.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><LangVersion>3</LangVersion><GenerateAssemblyInfo>false</GenerateAssemblyInfo><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable></PropertyGroup></Project>')
 result=subprocess.run(['rtk','proxy',str(dotnet),'run','--project',str(w/'Cases.csproj'),'-p:NuGetAudit=false'],capture_output=True,text=True,timeout=180);print(result.stdout+result.stderr);raise SystemExit(result.returncode)
