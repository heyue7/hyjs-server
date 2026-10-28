using System;
using System.IO;
using System.Reflection;
using System.Runtime.Loader;
using System.Runtime.Serialization;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
class CandidateProbe {
 static BindingFlags F=BindingFlags.Instance|BindingFlags.Static|BindingFlags.Public|BindingFlags.NonPublic;
 static object Empty(Type t){return FormatterServices.GetUninitializedObject(t);}
 static void Set(object o,string field,object value){o.GetType().GetField(field,F).SetValue(o,value);}
 static void Dictionary(object o,string field){var f=o.GetType().GetField(field,F);f.SetValue(o,Activator.CreateInstance(f.FieldType));}
 static IDictionary Dict(object o,string field){return (IDictionary)o.GetType().GetField(field,F).GetValue(o);}
 static object Hero(Assembly game,string id,int quality,int heroClass){var h=Empty(game.GetType("UI.UIHero",true));var table=Empty(game.GetType("Dict.DataModel.TableHero",true));Set(table,"id",id);Set(table,"quality",quality);Set(table,"heroClass",heroClass);Set(h,"heroId",id);Set(h,"guid","owned-"+id);Set(h,"m_level",1);Set(h,"heroData",table);return h;}
 static void Main(string[] args){
  string managed=Path.GetFullPath(args[0]),refs=Path.GetFullPath(args[1]);AssemblyLoadContext.Default.Resolving+=(_,name)=>{var p=Path.Combine(refs,name.Name+".dll");return File.Exists(p)?AssemblyLoadContext.Default.LoadFromAssemblyPath(p):null;};
  var game=AssemblyLoadContext.Default.LoadFromAssemblyPath(managed);var instance=Empty(game.GetType("UI.UIInstPlayerData",true));foreach(var n in new[]{"instHeros","instBattleCamps","instHelpCamps"})Dictionary(instance,n);
  var player=Empty(game.GetType("UI.UIPlayerData",true));Set(player,"instPlayerData",instance);Set(player,"heroWarUID","main-outside-fixture");game.GetType("StaticData",true).GetField("playerData",F).SetValue(null,player);
  var heroes=Dict(instance,"instHeros");foreach(var x in JsonDocument.Parse(File.ReadAllText(args[2])).RootElement.EnumerateArray()){string id=x.GetProperty("id").GetString();heroes.Add(id,Hero(game,id,x.GetProperty("quality").GetInt32(),x.GetProperty("heroClass").GetInt32()));}
  int threshold=int.Parse(args[3]);var select=instance.GetType().GetMethod("GetAvailableHerosForFormation",F);
  Func<HashSet<string>> selected=()=>{var list=(IEnumerable)select.Invoke(instance,new object[]{2});var result=new HashSet<string>();foreach(var h in list)result.Add((string)h.GetType().GetField("heroId",F).GetValue(h));return result;};
  var expected=new HashSet<string>();foreach(DictionaryEntry x in heroes){var h=x.Value;var t=h.GetType().GetField("heroData",F).GetValue(h);int q=(int)t.GetType().GetField("quality",F).GetValue(t),c=(int)t.GetType().GetField("heroClass",F).GetValue(t);if(q>threshold&&c!=99)expected.Add((string)x.Key);}
  var initial=selected();if(!initial.SetEquals(expected))throw new Exception("candidate quality set differs");
  var formationType=game.GetType("UI.UIHeroFormation",true);var mounted=Empty(formationType);Set(heroes["hero_056"],"owner",mounted);expected.Remove("hero_056");
  var helps=Dict(instance,"instHelpCamps");var helpType=instance.GetType().GetField("instHelpCamps",F).FieldType.GetGenericArguments()[1];var help=Empty(helpType);game.ManifestModule.ResolveField(0x040031b9).SetValue(help,"owned-hero_057");helps.Add(1,help);expected.Remove("hero_057");
  var other=Empty(formationType);Set(other,"hero",Hero(game,"hero_058",12,0));var getter=formationType.GetProperty("campPos",F).GetGetMethod(true);var il=getter.GetMethodBody().GetILAsByteArray();game.ManifestModule.ResolveField(BitConverter.ToInt32(il,2)).SetValue(other,1);Dict(instance,"instBattleCamps").Add(1,other);expected.Remove("hero_058");
  var protectedSet=selected();if(!protectedSet.SetEquals(expected))throw new Exception("owner/helper/duplicate exclusions differ");
  heroes.Clear();if(selected().Count!=0)throw new Exception("empty bag differs");
  Console.WriteLine(JsonSerializer.Serialize(new {threshold,initialCandidates=initial.Count,green=initial.Count(x=>String.CompareOrdinal(x,"hero_056")>=0&&String.CompareOrdinal(x,"hero_061")<=0),blue=initial.Count(x=>!x.StartsWith("hero_exp")&&String.CompareOrdinal(x,"hero_056")<0),protectedCandidates=protectedSet.Count,materialsExcluded=true,ownerHelperDuplicateProtected=true,emptyBagVerified=true,physicalDevice=false}));
 }
}
