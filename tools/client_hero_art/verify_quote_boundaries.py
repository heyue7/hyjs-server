"""Exercise actual Quote/Price source with original table rows and minimal doubles."""
import argparse,csv,subprocess,tempfile
from pathlib import Path
SOURCE=Path(__file__).resolve().parent;ROOT=SOURCE.parents[1]
STUB=r"""
using System;using System.Collections.Generic;
namespace Dict.DataModel{public class TableItemShopPrice{public string shopItemId,count,price;}}
namespace UI{public class UIShopProduct{public List<Cost> price=new List<Cost>();public int dayTotalTimes,dayLeftTimes,canBuy;}}
public class Cost{public string id;public int num;}
public class Player{public UI.UIShopProduct p;public UI.UIShopProduct GetItemShopProduct(string id){return p;}}
public static class StaticData{public static Player playerData=new Player();}
public static class BoxManager{public static int popups;public static void CreatePopupTextBox(string s,float x){popups++;}}
namespace Dict.Blo{public static class TableItemShopPriceBlo{public static int calls;public static string[] rows=new string[]{TABLE_ROWS};public static Dict.DataModel.TableItemShopPrice GetShopPrice(string id,int ordinal){calls++;foreach(string row in rows){string[] x=row.Split('|');string[] n=x[0].Split('-');if(id=="itemShopId_4"&&ordinal>=int.Parse(n[0])&&ordinal<=int.Parse(n[n.Length-1]))return new Dict.DataModel.TableItemShopPrice{price=x[1]};}return null;}}}
public static class Program{
const string Q="p4qt:0123456789abcdef:itemShopId_4";
static int tests;static void Check(bool b,string n){tests++;if(!b)throw new Exception(n);}
static UI.UIShopProduct Setup(int left,int total,int can,int price){BoxManager.popups=0;Dict.Blo.TableItemShopPriceBlo.calls=0;var p=new UI.UIShopProduct{dayLeftTimes=left,dayTotalTimes=total,canBuy=can};p.price.Add(new Cost{id="diamond",num=price});StaticData.playerData.p=p;return p;}
public static void Main(){
var p=Setup(0,20,0,50);var result=HYJS.Shop.QuotedShopCost.Price(Q,21);
Check(result.price=="diamond,50"&&BoxManager.popups==0&&Dict.Blo.TableItemShopPriceBlo.calls==0&&p.dayLeftTimes==0&&p.canBuy==0,"soldout50 versus next80 false alert");
int stock=86;for(int i=0;i<5;i++){stock--;HYJS.Shop.QuotedShopCost.Price(Q,21);}Check(stock==81&&p.dayTotalTimes==20&&p.dayLeftTimes==0&&p.canBuy==0&&BoxManager.popups==0&&Dict.Blo.TableItemShopPriceBlo.calls==0,"inventory redraw changed purchase counters");
p=Setup(17,20,17,20);result=HYJS.Shop.QuotedShopCost.Price(Q,4);Check(result.price=="diamond,20"&&BoxManager.popups==0&&p.dayLeftTimes==17,"valid next tier rejected");
p=Setup(17,20,17,10);HYJS.Shop.QuotedShopCost.Price(Q,4);Check(BoxManager.popups==1&&p.dayLeftTimes==0&&p.canBuy==0,"real stale tier not protected");
p=Setup(0,20,0,77);result=HYJS.Shop.QuotedShopCost.Price("p4q:0123456789abcdef:itemShopId_4",100);Check(result.price=="diamond,77"&&BoxManager.popups==0&&Dict.Blo.TableItemShopPriceBlo.calls==0,"fixed p4q changed");
result=HYJS.Shop.QuotedShopCost.Price("s1q:0123456789abcdef:itemShopId_4",100);Check(result.price=="diamond,77"&&BoxManager.popups==0,"fixed s1q changed");
Check(HYJS.Shop.QuotedShopCost.Price("itemShopId_4",1)==null&&BoxManager.popups==0,"unquoted original changed");
p.price.Clear();Check(HYJS.Shop.QuotedShopCost.Price(Q,21)==null&&BoxManager.popups==0,"missing price changed");
StaticData.playerData.p=null;Check(HYJS.Shop.QuotedShopCost.Price(Q,21)==null,"missing product changed");
p=Setup(0,0,0,50);HYJS.Shop.QuotedShopCost.Price(Q,1);Check(BoxManager.popups==1,"invalid total bypassed tier protection");
p=Setup(0,20,0,50);HYJS.Shop.QuotedShopCost.Price(Q,0);Check(BoxManager.popups==1,"invalid ordinal bypassed tier protection");
p=Setup(1,20,1,50);result=HYJS.Shop.QuotedShopCost.Price(Q,20);Check(result.price=="diamond,50"&&p.dayLeftTimes==1&&BoxManager.popups==0,"last available unit changed");
p=Setup(0,20,1,50);HYJS.Shop.QuotedShopCost.Price(Q,21);Check(BoxManager.popups==1,"contradictory soldout canBuy accepted");
Console.WriteLine("PASS "+tests+" actual-Price boundary assertions using original itemShopId_4 table rows; minimal UI doubles, not Unity/item-use integration");}}
"""
def main():
 a=argparse.ArgumentParser();a.add_argument('--tool-root',type=Path,required=True);a.add_argument('--output',type=Path,required=True);args=a.parse_args()
 source=(SOURCE/'QuotedShopCost.cs').read_text();quote=source[source.index('        static bool Quote('):source.index('        public static bool IsQuoted(')];price=source[source.index('        public static TableItemShopPrice Price('):source.index('        public static void CostIcon(')]
 body='using System;using System.Globalization;using Dict.DataModel;namespace HYJS.Shop{public static class QuotedShopCost{'+quote+price+'}}'
 with (ROOT/'analysis/tables/TableItemShopPrice.tsv').open()as f:rows=list(csv.reader(f,delimiter='\t'))
 values=[r[2]+'|'+r[3] for r in rows[5:] if len(r)>3 and r[1]=='itemShopId_4']
 assert '16-20|diamond,50'in values and '21-25|diamond,80'in values
 stub=STUB.replace('TABLE_ROWS',','.join('"'+r+'"'for r in values))
 with tempfile.TemporaryDirectory(prefix='hyjs-quote-boundary-')as tmp:
  w=Path(tmp);(w/'Program.cs').write_text(stub);(w/'ActualPrice.cs').write_text(body);(w/'Check.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>')
  r=subprocess.run(['rtk','proxy',str(args.tool_root/'.tools/native-batch/dotnet/dotnet'),'run','--project',str(w/'Check.csproj'),'--verbosity','quiet','-p:NuGetAudit=false'],capture_output=True,text=True,timeout=120)
  args.output.write_text(r.stdout+r.stderr)
  if r.returncode:raise RuntimeError('Actual Price boundary check failed: '+str(args.output))
  print(r.stdout.strip())
if __name__=='__main__':main()
