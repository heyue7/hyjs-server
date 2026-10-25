using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.MaterialEligibility {
 public static class Guard {
  static readonly Dictionary<string,string> exclusions=new Dictionary<string,string>();
  static long owner;static string session="";static bool ready,busy;static float observed;
  static Runner runner;static TalentDetail pending,confirmed;static object pendingInfo,confirmedInfo;
  static readonly BindingFlags Flags=BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.Instance;
  static object Field(object value,string name){if(value==null)return null;FieldInfo f=value.GetType().GetField(name,Flags);return f==null?null:f.GetValue(value);}
  static string Session(){return StaticData.sessionKey==null?"":BitConverter.ToString(StaticData.sessionKey).Replace("-","").ToLowerInvariant();}
  static bool Same(long id,string key){return StaticData.playerData!=null&&StaticData.playerData.playerId==id&&key==Session();}
  static bool Fresh(){return ready&&Same(owner,session)&&Time.realtimeSinceStartup-observed<5f;}
  static void Read(){
   if(busy||StaticData.playerData==null)return;
   if(runner==null){GameObject go=new GameObject("TalentMaterialEligibility");UnityEngine.Object.DontDestroyOnLoad(go);runner=go.AddComponent<Runner>();}
   runner.StartCoroutine(ReadState());
  }
  public static List<string> FilterGuids(List<string> source){
   if(!Fresh()){Read();return new List<string>();}
   return source==null?new List<string>():source.FindAll(delegate(string guid){return guid!=null&&!exclusions.ContainsKey(guid);});
  }
  static string MissingReason(TalentDetail detail){
   UITalent talent=Field(detail,"talentInfo")as UITalent;if(talent==null)return "材料状态已变化，请重新选择天赋";
   string heroId=null;foreach(string part in talent.Cost.Split(';')){string[] pair=part.Split(',');if(pair.Length==2&&pair[0].StartsWith("hero_"))heroId=pair[0];}
   if(heroId==null)return null;
   List<string> eligible=Field(detail,"costHeroList")as List<string>;int needed=Convert.ToInt32(Field(detail,"costHeroNum"));
   if(eligible!=null&&eligible.Count>=needed)return null;
   IDictionary heroes=Field(StaticData.playerData.InstPlayerData,"instHeros")as IDictionary;
   if(heroes!=null)foreach(DictionaryEntry entry in heroes){UIHero hero=entry.Value as UIHero;string reason;
    if(hero!=null&&hero.heroId==heroId&&hero.IsZeroHero()&&hero.owner==null&&!hero.Help()&&exclusions.TryGetValue(hero.guid,out reason))return "可用同名材料不足："+reason;
   }
   return null;
  }
  public static bool BeforeTalentActive(object value){
   TalentDetail detail=value as TalentDetail;if(detail==null)return false;
   if(Convert.ToInt32(Field(detail,"costHeroNum"))<=0)return true;
   if(confirmed==detail&&System.Object.ReferenceEquals(confirmedInfo,Field(detail,"talentInfo"))&&Fresh()){
    confirmed=null;confirmedInfo=null;
    string reason=MissingReason(detail);if(reason!=null){BoxManager.CreatePopupTextBox(reason,0f);return false;}return true;
   }
   pending=detail;pendingInfo=Field(detail,"talentInfo");Read();return false;
  }
  static void Refresh(bool continueActivation){
   TalentDetail click=pending;object info=pendingInfo;pending=null;pendingInfo=null;
   foreach(UnityEngine.Object item in UnityEngine.Object.FindObjectsOfType(typeof(TalentDetail))){TalentDetail detail=item as TalentDetail;
    if(detail!=null&&detail.gameObject.activeInHierarchy){UITalent talent=Field(detail,"talentInfo")as UITalent;if(talent!=null)detail.SetTalent(talent);}
   }
   if(continueActivation&&click!=null&&click.gameObject.activeInHierarchy&&System.Object.ReferenceEquals(Field(click,"talentInfo"),info)){confirmed=click;confirmedInfo=info;click.OnActiveBtn();}
  }
  static IEnumerator ReadState(){
   long id=StaticData.playerData.playerId;string key=Session();string origin=null;
   try{origin=new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority);}catch(Exception){}
   if(origin==null||key.Length!=32){pending=null;pendingInfo=null;yield break;}
   busy=true;ready=false;
   string body="{\"playerId\":"+id+",\"sessionHex\":\""+key+"\"}";
   Dictionary<string,string> headers=new Dictionary<string,string>();headers.Add("Content-Type","application/json");
   WWW request=new WWW(origin+"/instance-lock/state",Encoding.UTF8.GetBytes(body),headers);float deadline=Time.realtimeSinceStartup+15f;
   while(!request.isDone&&Time.realtimeSinceStartup<deadline)yield return null;
   bool confirmed=false;string error="材料状态读取失败，请稍后重试";
   try{if(Same(id,key)&&request.isDone&&String.IsNullOrEmpty(request.error)){
    JsonData reply=JsonMapper.ToObject(request.text);
    if((int)reply["code"]==0){IDictionary values=reply["data"]["materialUnavailable"]as IDictionary;
     if(values!=null&&values.Count<=4096){Dictionary<string,string> next=new Dictionary<string,string>();
      foreach(DictionaryEntry pair in values){string guid=(string)pair.Key;string reason=(string)(JsonData)pair.Value;if(guid.Length==0||guid.Length>128||reason.Length>512)throw new InvalidOperationException();next.Add(guid,reason);}
      exclusions.Clear();foreach(KeyValuePair<string,string> pair in next)exclusions.Add(pair.Key,pair.Value);
      owner=id;session=key;observed=Time.realtimeSinceStartup;ready=true;confirmed=true;
     }
    }else if(((IDictionary)reply).Contains("msg"))error=(string)reply["msg"];
   }}catch(Exception){}
   request.Dispose();busy=false;
   if(!Same(id,key)){ready=false;pending=null;pendingInfo=null;yield break;}
   if(confirmed)Refresh(true);else{ready=false;pending=null;pendingInfo=null;BoxManager.CreatePopupTextBox(error,0f);}
  }
 }
 public sealed class Runner:MonoBehaviour{}
}
