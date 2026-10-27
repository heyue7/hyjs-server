"""Execute the actual preview helper against bounded native API stand-ins."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent
RUNTIME = Path(os.environ.get('HYJS_NATIVE_RUNTIME', '/Users/lirui/LR/hyjs/.tools/native-batch'))

HARNESS = r'''
using System;
using System.Collections.Generic;
namespace ProtoBuf.Message {
 public class BaseResource { public string id; public int num; }
 public class ShopItem { public List<BaseResource> item = new List<BaseResource>(); }
}
namespace Dict.DataModel { public class TableItemUse { public string result; } }
namespace Dict.Blo {
 public static class DictItemBlo {
  public static bool Fail;
  public static Dict.DataModel.TableItemUse Box;
  public static Dict.DataModel.TableItemUse GetTableItemUse(string id) { if(Fail) throw new Exception(); return Box; }
 }
}
namespace UI {
 public class PageManager {
  public static PageManager Instance = new PageManager();
  public static string Page, Options; public static bool Fail;
  public void OpenPage(string page, string options) { if(Fail) throw new Exception(); Page=page; Options=options; }
 }
}
public static class Harness {
 const string Contents="item_050,2;item_009,50;item_012,50;item_001,10;coin,1000000";
 static void Check(bool value,string label){if(!value)throw new Exception(label);}
 static ProtoBuf.Message.ShopItem Item(string box,string book,int count) {
  var item=new ProtoBuf.Message.ShopItem();
  item.item.Add(new ProtoBuf.Message.BaseResource{id=box,num=count});
  if(book!=null)item.item.Add(new ProtoBuf.Message.BaseResource{id=book,num=1});
  return item;
 }
 static void Reset(){Dict.Blo.DictItemBlo.Box=new Dict.DataModel.TableItemUse{result=Contents};Dict.Blo.DictItemBlo.Fail=false;UI.PageManager.Fail=false;UI.PageManager.Page=null;UI.PageManager.Options=null;}
 static void Reject(ProtoBuf.Message.ShopItem item,string label){UI.PageManager.Page=null;Check(!HYJS.VipShop.VipGiftPreview.TryOpen(item),label);Check(UI.PageManager.Page==null,label+" opened preview");}
 public static void Main(){
  Reset();var item=Item("item_041","item_063",1);var original=item.item;var box=item.item[0];var book=item.item[1];
  Check(HYJS.VipShop.VipGiftPreview.TryOpen(item),"qualified VIP7");
  Check(UI.PageManager.Page=="ShopGiftPreview" && UI.PageManager.Options=="type=3&activityGetGiftId="+Contents+";item_063,1","exact native type3 contents");
  Check(item.item==original && item.item.Count==2 && item.item[0]==box && item.item[1]==book && box.id=="item_041" && box.num==1 && book.num==1,"reward data changed");
  Reset();item=Item("item_041","item_063",2);item.item.Add(new ProtoBuf.Message.BaseResource{id="coin",num=3});
  Check(HYJS.VipShop.VipGiftPreview.TryOpen(item),"counted parcel plus direct rewards");
  Check(UI.PageManager.Options=="type=3&activityGetGiftId=item_050,4;item_009,100;item_012,100;item_001,20;coin,2000000;item_063,1;coin,3","count/direct semantics");
  Reset();Reject(null,"null item");Reject(Item("item_041",null,1),"single parcel");Reject(Item("item_044","item_063",1),"other parcel");Reject(Item("item_041","item_064",1),"other book");Reject(Item("item_041","item_063",0),"zero parcel");Reject(Item("item_041","item_063",Int32.MaxValue),"overflow");
  item=Item("item_041","item_063",1);item.item.Add(null);Reject(item,"null reward");
  item=Item("item_041","item_063",1);item.item[1].num=-1;Reject(item,"negative reward");
  item=Item("item_041","item_063",1);item.item.Add(new ProtoBuf.Message.BaseResource{id="coin&type=1",num=1});Reject(item,"query injection");
  item=Item("item_041","item_063",1);item.item=null;Reject(item,"null resource list");
  Reset();Dict.Blo.DictItemBlo.Box=null;Reject(Item("item_041","item_063",1),"missing table");
  Reset();Dict.Blo.DictItemBlo.Box.result="item_063,1";Reject(Item("item_041","item_063",1),"changed original table");
  Reset();Dict.Blo.DictItemBlo.Fail=true;Reject(Item("item_041","item_063",1),"table exception");
  Reset();UI.PageManager.Fail=true;Reject(Item("item_041","item_063",1),"page exception");
  Console.WriteLine("PASS: native type3 payload, five original resources, direct book, counts, no mutation, and 14 fallback scenarios");
 }
}
'''


class PreviewTests(unittest.TestCase):
    def test_qualified_payload_and_original_fallback(self):
        dotnet = RUNTIME / 'dotnet/dotnet'
        compiler = next((RUNTIME / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
        refs = next((RUNTIME / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
        with tempfile.TemporaryDirectory(prefix='hyjs-vip-preview-tests-') as temporary:
            work = Path(temporary)
            source = work / 'Harness.cs'
            source.write_text(HARNESS)
            output = work / 'Harness.dll'
            cmd = [str(dotnet), str(compiler), '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
                   '/out:' + str(output), *['/reference:' + str(p) for p in refs.glob('*.dll')],
                   str(source), str(SOURCE / 'VipGiftPreview.cs')]
            result = subprocess.run(['rtk', 'proxy', *cmd], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            (work / 'Harness.runtimeconfig.json').write_text('{"runtimeOptions":{"tfm":"net8.0","framework":{"name":"Microsoft.NETCore.App","version":"8.0.0"}}}')
            result = subprocess.run(['rtk', 'proxy', str(dotnet), str(output)], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('PASS:', result.stdout)
            print(result.stdout.strip())


if __name__ == '__main__':
    unittest.main()
