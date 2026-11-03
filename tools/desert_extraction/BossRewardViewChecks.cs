using System;
using System.Collections.Generic;
using LitJson;
using ProtoBuf.Message;
using HYJS.DesertV4;

namespace ProtoBuf.Message
{
    public sealed class SimpleItem { public string id; public int num; }
    public sealed class ResultObject
    {
        public List<SimpleItem> simps=new List<SimpleItem>();
        public List<object> equips=new List<object>(),treas=new List<object>(),wars=new List<object>(),pets=new List<object>();
    }
}

static class BossRewardViewChecks
{
    const string Fight="1234567890abcdef12345678", Other="1234567890abcdef12345679";
    static int count;
    static void Check(bool condition,string message) { count++; if (!condition) throw new Exception(message); }
    static BossRewardView Parse(string status,string fields)
    {
        return BossRewardView.FromReceipt(Fight,JsonMapper.ToObject("{\"fightId\":\""+Fight+
            "\",\"rewardStatus\":\""+status+"\""+fields+"}"));
    }
    static void Reject(string status,string fields)
    {
        bool failed=false; try { Parse(status,fields); } catch (Exception) { failed=true; }
        Check(failed,"invalid boss receipt accepted: "+status+fields);
    }
    static int Find(ResultObject data,string id)
    {
        int total=0;
        foreach (SimpleItem item in data.simps) if (item!=null && item.id==id) { Check(total==0,"duplicate "+id); total=item.num; }
        return total;
    }
    static void Main()
    {
        const string resources=",\"rewardResources\":[{\"id\":\"diamond\",\"amount\":700},{\"id\":\"coin\",\"amount\":8000000},{\"id\":\"MineItem_009\",\"amount\":20},{\"id\":\"FishItem_025\",\"amount\":3},{\"id\":\"item_9711\",\"amount\":6}]";
        var native=new ResultObject(); native.simps.Add(new SimpleItem { id="diamond",num=700 }); native.wars.Add(new object());
        var delivered=Parse("delivered",resources+",\"rewardChips\":[1,2,3,4,5,6]");
        Check(Object.ReferenceEquals(native,delivered.Project(Other,native)),"wrong fight shows rewards");
        ResultObject display=delivered.Project(Fight,native);
        Check(!Object.ReferenceEquals(display,native),"display shares native result");
        Check(Find(display,"diamond")==700 && Find(display,"coin")==8000000 && Find(display,"MineItem_009")==20 &&
              Find(display,"FishItem_025")==3 && Find(display,"item_9711")==6,"delivered resources differ");
        for (int i=0;i<6;i++) Check(Find(display,"item_"+(9701+i))==i+1,"chip mapping differs");
        Check(native.simps.Count==1 && native.simps[0].id=="diamond" && native.wars.Count==1 &&
              display.wars.Count==1 && Object.ReferenceEquals(display.wars[0],native.wars[0]),"native result was changed");
        Check(!Object.ReferenceEquals(native.simps[0],display.simps[0]),"native simple item was shared");
        display.simps[0].num=1;
        Check(native.simps[0].num==700,"display changed native item");
        ResultObject again=delivered.Project(Fight,native), repeat=delivered.Project(Fight,again);
        Check(Object.ReferenceEquals(again,repeat) && again.simps.Count==11 && Find(again,"item_9706")==6,
              "repeated render accumulated rewards");
        ResultObject partial=Parse("pending_chip_delivery",resources).Project(Fight,new ResultObject());
        Check(Find(partial,"diamond")==700 && Find(partial,"item_9701")==0,"pending chip status differs");
        ResultObject credit=Parse("delivered",",\"rewardCredit\":150").Project(Fight,new ResultObject());
        Check(Find(credit,"item_9710")==150 && Find(credit,"item_9701")==0,"late credit differs");
        ResultObject combined=Parse("delivered",",\"rewardResources\":[{\"id\":\"item_9710\",\"amount\":25}],\"rewardCredit\":150").Project(Fight,new ResultObject());
        Check(Find(combined,"item_9710")==175,"credit resources were duplicated");
        foreach (string status in new[]{"pending_boss_defeat","pending_delivery","daily_limit","no_contribution"})
            Check(Object.ReferenceEquals(native,Parse(status,"").Project(Fight,native)),status+" must remain empty");
        Check(Object.ReferenceEquals(native,Parse("delivered",",\"rewardChips\":[0,0,0,0,0,0]").Project(Fight,native)),"zero chips should be empty");
        Reject("pending_delivery",resources); Reject("daily_limit",resources);
        Reject("pending_chip_delivery",",\"rewardChips\":[1,0,0,0,0,0]");
        Reject("pending_chip_delivery",",\"rewardCredit\":1");
        Reject("delivered",",\"rewardChips\":[1,0,0,0,0]");
        Reject("delivered",",\"rewardChips\":[1,0,0,0,0,-1]");
        Reject("delivered",",\"rewardChips\":[2147483648,0,0,0,0,0]");
        Reject("delivered",",\"rewardChips\":[\"1\",0,0,0,0,0]");
        Reject("delivered",",\"rewardChips\":[1.0,0,0,0,0,0]");
        Reject("delivered",",\"rewardChips\":[1,0,0,0,0,0],\"rewardCredit\":1");
        Reject("delivered",",\"rewardCredit\":-1"); Reject("delivered",",\"rewardCredit\":2147483648");
        Reject("delivered",",\"rewardResources\":{}");
        Reject("delivered",",\"rewardResources\":[{\"id\":\"coin\",\"amount\":0}]");
        Reject("delivered",",\"rewardResources\":[{\"id\":\"coin\",\"amount\":-1}]");
        Reject("delivered",",\"rewardResources\":[{\"id\":\"coin\",\"amount\":2147483648}]");
        Reject("delivered",",\"rewardResources\":[{\"id\":\"coin\",\"amount\":\"4\"}]");
        Reject("delivered",",\"rewardResources\":[{\"id\":\" coin\",\"amount\":1}]");
        Reject("delivered",",\"rewardResources\":[{\"id\":\"item_9701\",\"amount\":1}]");
        Reject("delivered",",\"rewardResources\":[{\"id\":\"coin\",\"amount\":1},{\"id\":\"coin\",\"amount\":1}]");
        Reject("unknown","");
        bool badFight=false; try { BossRewardView.FromReceipt(Other,JsonMapper.ToObject("{\"fightId\":\""+Fight+"\",\"rewardStatus\":\"delivered\"}")); }
        catch (Exception) { badFight=true; }
        Check(badFight,"mismatched receipt fight accepted");
        Console.WriteLine("PASS "+count+" boss reward projection checks");
    }
}
