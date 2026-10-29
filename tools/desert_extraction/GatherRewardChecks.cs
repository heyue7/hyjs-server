using System;
using System.Collections;
using System.IO;
using System.Reflection;
using LitJson;

// Reads the parser from the compiled QA helper, not a test copy.
static class GatherRewardChecks
{
    static int count;
    static MethodInfo parse;
    static void Check(bool ok,string why) { count++; if (!ok) throw new Exception(why); }
    static object Parse(string json) { return parse.Invoke(null,new object[]{JsonMapper.ToObject(json)}); }
    static IList Rewards(object view) { return (IList)view.GetType().GetField("rewards").GetValue(view); }
    static string Id(object row) { return (string)row.GetType().GetProperty("id").GetValue(row,null); }
    static int Amount(object row) { return (int)row.GetType().GetProperty("num").GetValue(row,null); }
    static string Text(object view,string name) { return (string)view.GetType().GetField(name).GetValue(view); }
    static bool Flag(object view,string name) { return (bool)view.GetType().GetField(name).GetValue(view); }
    static void Reject(string json)
    {
        bool failed=false; try { Parse(json); } catch (TargetInvocationException) { failed=true; }
        Check(failed,"invalid reward must fail");
    }
    static void Main()
    {
        Assembly helper=Assembly.LoadFrom(Path.Combine(AppContext.BaseDirectory,"SoulFlowerDesertV4.dll"));
        Type runtime=helper.GetType("HYJS.DesertV4.FieldRuntime",true);
        parse=runtime.GetMethod("ParseGatherRewards",BindingFlags.Static|BindingFlags.NonPublic);
        Check(parse!=null,"actual helper parser");
        FieldInfo tiredField=runtime.GetField("SetNativeTired",BindingFlags.Static|BindingFlags.NonPublic);
        MethodInfo tired=tiredField==null ? null : tiredField.GetValue(null) as MethodInfo;
        Check(tired!=null && tired.IsPrivate && tired.Name=="SetHeroTired" && tired.GetParameters().Length==0,"original fatigue HUD method binding");
        object q=Parse("{\"resources\":[{\"id\":\"MineItem_010\",\"amount\":7}],\"chips\":[0,0,0,0,0,0]}");
        IList r=Rewards(q);Check(r.Count==1 && Id(r[0])=="MineItem_010" && Amount(r[0])==7,"actual O11 reward shape");
        Check(!Flag(q,"chips") && Text(q,"largeChips")=="","zero chip reward");
        q=Parse("{\"resources\":[{\"id\":\"FishItem_009\",\"amount\":4}],\"chips\":[0,1,0,0,0,2]}");r=Rewards(q);
        Check(r.Count==3 && Id(r[0])=="FishItem_009" && Amount(r[0])==4,"fish reward");
        Check(Id(r[1])=="item_9702" && Amount(r[1])==1 && Id(r[2])=="item_9706" && Amount(r[2])==2 && Flag(q,"chips"),"chip colors and amounts");
        q=Parse("{\"resources\":[],\"chips\":[0,0,0,0,0,0]}");Check(Rewards(q).Count==0 && !Flag(q,"chips"),"empty reward");
        q=Parse("{\"resources\":[],\"chips\":[9223372036854775807,0,0,0,0,0]}");
        Check(Rewards(q).Count==0 && Text(q,"largeChips").Contains("9223372036854775807"),"large amount kept exact");
        Reject("{\"resources\":[],\"chips\":[0,0,0,0,0]}");
        Reject("{\"resources\":[],\"chips\":[-1,0,0,0,0,0]}");
        Reject("{\"resources\":[{\"id\":\"MineItem_010\",\"amount\":-1}],\"chips\":[0,0,0,0,0,0]}");
        Reject("{\"resources\":[{\"id\":\"unknown\",\"amount\":1}],\"chips\":[0,0,0,0,0,0]}");
        Reject("{\"resources\":[{\"id\":\"MineItem_010\",\"amount\":2147483648}],\"chips\":[0,0,0,0,0,0]}");
        Console.WriteLine("PASS "+count+" actual compiled gather reward checks");
    }
}
