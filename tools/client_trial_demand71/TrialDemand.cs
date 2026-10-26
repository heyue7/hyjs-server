using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using System.Reflection;
using System.Text;
using LitJson;
using ProtoBuf.Message;
using UI;
using UnityEngine;

namespace HYJS.TrialDemand {
 public static class Bridge {
  public static void Open(object value) { RushEliteBossPage page=value as RushEliteBossPage;if(page==null)return;Controller c=page.GetComponent<Controller>();if(c==null)c=page.gameObject.AddComponent<Controller>();c.Open(page); }
  public static void Refresh(object value) { RushEliteBossPage page=value as RushEliteBossPage;if(page==null)return;Controller c=page.GetComponent<Controller>();if(c==null){Open(page);return;}c.Reopen(page); }
  public static void Close(object value) { RushEliteBossPage page=value as RushEliteBossPage;if(page==null)return;Controller c=page.GetComponent<Controller>();if(c!=null)c.Close(); }
  public static void Select(object value,GameObject icon) { RushEliteBossPage page=value as RushEliteBossPage;if(page==null)return;Controller c=page.GetComponent<Controller>();if(c!=null)c.Select(icon); }
 }
 public sealed class Controller:MonoBehaviour {
  const int PageSize=5;
  static readonly BindingFlags Flags=BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic;
  RushEliteBossPage page;long owner;string session;int generation,nextOffset,total;bool busy,opened;WWW request;GameObject more;
  string selected="",queued="";float lastInput;Vector3 scrollPosition;
  readonly List<RushEliteBossInfo> rows=new List<RushEliteBossInfo>();
  readonly HashSet<string> pending=new HashSet<string>();
  readonly Dictionary<string,GameObject> icons=new Dictionary<string,GameObject>();
  static object Get(object o,string n) { return o==null?null:o.GetType().GetField(n,Flags).GetValue(o); }
  static void Set(object o,string n,object v) { o.GetType().GetField(n,Flags).SetValue(o,v); }
  static string Key() { return StaticData.sessionKey==null?"":BitConverter.ToString(StaticData.sessionKey).Replace("-","").ToLowerInvariant(); }
  bool Current(int stamp) { return opened&&stamp==generation&&page!=null&&page.gameObject.activeInHierarchy&&StaticData.playerData!=null&&StaticData.playerData.playerId==owner&&Key()==session; }
  List<GameObject> Cache { get { return (List<GameObject>)Get(page,"bossIconCache"); } }
  static void Retire(GameObject item) { if(item==null)return;item.SetActive(false);item.transform.SetParent(null);UnityEngine.Object.Destroy(item); }
  public void Open(RushEliteBossPage value) {
   Close();page=value;opened=true;owner=StaticData.playerData.playerId;session=Key();nextOffset=0;total=0;selected="";queued="";rows.Clear();pending.Clear();icons.Clear();
   foreach(GameObject item in Cache)Retire(item);Cache.Clear();Set(page,"remoteData",new SCGetRushEliteBossInfo());Set(page,"currentSelectBossId","");
   foreach(GameObject item in (List<GameObject>)Get(page,"bossItemCache"))Retire(item);((List<GameObject>)Get(page,"bossItemCache")).Clear();
   page.bossDescLabel.text="";page.fightBtn.SetActive(false);page.leftChanllegeTimeLabel.text="…";More("点击加载");StartCoroutine(Read(0,null));
  }
  public void Close() {
   opened=false;generation++;StopAllCoroutines();if(request!=null){request.Dispose();request=null;}busy=false;queued="";
   if(more!=null){Retire(more);more=null;}
  }
  public void Reopen(RushEliteBossPage value) {
   if(page!=value||rows.Count==0||StaticData.playerData==null||StaticData.playerData.playerId!=owner||Key()!=session){Open(value);return;}
   Close();opened=true;foreach(RushEliteBossInfo info in rows){pending.Add(info.eliteTaskId);info.isOpen=0;Paint(info);}if(nextOffset>=0)More("更多首领");page.fightBtn.SetActive(false);StartCoroutine(Read(-1,selected));
  }
  void OnDisable() { Close(); }
  void More(string text) {
   if(more==null){more=UnityEngine.Object.Instantiate(page.bossIconPrefab)as GameObject;Place(more);RushEliteBossIcon icon=more.GetComponent<RushEliteBossIcon>();if(icon.bossIcon.gameObject!=more)icon.bossIcon.gameObject.SetActive(false);icon.bossIconFrame.enabled=false;icon.selector.SetActive(false);icon.closeMask.SetActive(false);icon.nameLabel.text=text;icon.onIconClick=delegate(GameObject ignored){Next();};}
   more.GetComponent<RushEliteBossIcon>().nameLabel.text=text;page.bossIconGrid.repositionNow=true;
  }
  void Place(GameObject obj) { obj.transform.SetParent(page.bossIconGrid.transform);obj.transform.localPosition=Vector3.zero;obj.transform.localScale=Vector3.one;obj.transform.localEulerAngles=Vector3.zero; }
  void Next() { if(opened&&!busy&&nextOffset>=0)StartCoroutine(Read(nextOffset,null)); }
  void Update() {
   if(!opened||page==null||!page.gameObject.activeInHierarchy)return;
   if(StaticData.playerData==null||StaticData.playerData.playerId!=owner||Key()!=session){Close();return;}
   if(Input.GetMouseButton(0)||Input.touchCount>0)lastInput=Time.realtimeSinceStartup;
   Vector3 pos=page.bossIconScrollView.transform.localPosition;bool moved=(pos-scrollPosition).sqrMagnitude>4f;scrollPosition=pos;
   if(moved&&Time.realtimeSinceStartup-lastInput<1.2f&&!busy&&more!=null&&page.bossIconScrollView.panel.IsVisible(more.transform.position))Next();
  }
  void Paint(RushEliteBossInfo info) {
   GameObject obj;bool created=!icons.TryGetValue(info.eliteTaskId,out obj);
   if(created){obj=UnityEngine.Object.Instantiate(page.bossIconPrefab)as GameObject;Place(obj);icons.Add(info.eliteTaskId,obj);Cache.Add(obj);}
   RushEliteBossIcon icon=obj.GetComponent<RushEliteBossIcon>();icon.Init(info,selected==info.eliteTaskId,delegate(GameObject item){Select(item);},page);
   // Keep the original condition label at its exact visual position while the
   // lock mask is hidden for an explicitly unverified entry.
   if(pending.Contains(info.eliteTaskId)){icon.openConditionLabel.transform.SetParent(icon.transform,true);icon.closeMask.SetActive(false);icon.openConditionLabel.gameObject.SetActive(true);icon.openConditionLabel.text="点击查看";}
   else icon.openConditionLabel.gameObject.SetActive(info.isOpen<=0);
  }
  void Header(SCGetRushEliteBossInfo packet) {
   SCGetRushEliteBossInfo merged=new SCGetRushEliteBossInfo();merged.leftTime=packet.leftTime;merged.totalTime=packet.totalTime;merged.diamondLeftTime=packet.diamondLeftTime;merged.diamondLeftTimeCost=packet.diamondLeftTimeCost;merged.bossInfoList.AddRange(rows);
   Set(page,"remoteData",merged);StaticData.ReceiveRushEliteBossInfo(merged);Set(page,"leftChallengeTime",merged.leftTime);Set(page,"diamondChallengeBuyTime",merged.diamondLeftTime);Set(page,"diamondChallengeBuyCost",merged.diamondLeftTimeCost);
   page.leftChanllegeTimeLabel.text=merged.leftTime+"/"+merged.totalTime;Delegate changed=Get(page,"onRushEliteBossTimeValueChange")as Delegate;if(changed!=null)changed.DynamicInvoke(new object[]{merged.leftTime});
  }
  void ShowSelected() {
   foreach(KeyValuePair<string,GameObject> entry in icons){RushEliteBossIcon icon=entry.Value.GetComponent<RushEliteBossIcon>();if(entry.Key==selected)icon.Select();else icon.UnSelect();}
   Set(page,"currentSelectBossId",selected);RushEliteBossInfo info=rows.Find(delegate(RushEliteBossInfo item){return item.eliteTaskId==selected;});
   if(info==null){page.bossDescLabel.text="";page.fightBtn.SetActive(false);return;}
   typeof(RushEliteBossPage).GetMethod("RefreshBossDetail",Flags).Invoke(page,new object[]{selected});page.fightBtn.SetActive(info.isOpen>0&&!pending.Contains(selected));
  }
  public void Select(GameObject obj) {
   if(!opened||obj==null)return;RushEliteBossIcon icon=obj.GetComponent<RushEliteBossIcon>();if(icon==null)return;string id=icon.GetBossId();if(!icons.ContainsKey(id))return;
   selected=id;ShowSelected();if(pending.Contains(id)){page.fightBtn.SetActive(false);if(busy)queued=id;else StartCoroutine(Read(-1,id));}
  }
  IEnumerator Read(int offset,string bossId) {
   if(busy)yield break;busy=true;int stamp=generation;bool detail=bossId!=null;string origin=null;
   try{origin=new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority);}catch(Exception){}
   if(origin==null||session.Length!=32){busy=false;yield break;}
   string body="{\"playerId\":"+owner+",\"sessionHex\":\""+session+"\",\"limit\":"+PageSize+(detail?",\"bossId\":"+JsonMapper.ToJson(bossId):",\"offset\":"+offset+",\"selectedBossId\":"+JsonMapper.ToJson(selected))+"}";
   Dictionary<string,string> headers=new Dictionary<string,string>();headers.Add("Content-Type","application/json");request=new WWW(origin+"/rush/elite-page",Encoding.UTF8.GetBytes(body),headers);float deadline=Time.realtimeSinceStartup+15f;
   while(!request.isDone&&Time.realtimeSinceStartup<deadline)yield return null;
   bool ok=false;string error="首领信息读取失败，请点击重试";
   if(Current(stamp))try{
    if(!request.isDone||!String.IsNullOrEmpty(request.error)||request.text.Length>262144)throw new InvalidOperationException();
    JsonData reply=JsonMapper.ToObject(request.text);if((int)reply["code"]!=0){if(((IDictionary)reply).Contains("msg"))error=(string)reply["msg"];throw new InvalidOperationException();}
    JsonData data=reply["data"];string encoded=(string)data["packet"];byte[] bytes=Convert.FromBase64String(encoded);SCGetRushEliteBossInfo packet;
    using(MemoryStream stream=new MemoryStream(bytes,false))packet=ProtoBuf.Serializer.Deserialize<SCGetRushEliteBossInfo>(stream);
    if(packet==null||packet.bossInfoList.Count>(detail?1:PageSize))throw new InvalidOperationException();
    HashSet<string> received=new HashSet<string>();HashSet<string> unverified=new HashSet<string>();foreach(JsonData id in data["pendingIds"])unverified.Add((string)id);
    foreach(RushEliteBossInfo info in packet.bossInfoList){if(String.IsNullOrEmpty(info.eliteTaskId)||!received.Add(info.eliteTaskId)||detail&&info.eliteTaskId!=bossId)throw new InvalidOperationException();}
    if(!unverified.IsSubsetOf(received))throw new InvalidOperationException();
    if(!detail){if((int)data["offset"]!=offset||offset!=rows.Count)throw new InvalidOperationException();total=(int)data["total"];nextOffset=(int)data["nextOffset"];if(total<rows.Count||total>1024||nextOffset>=0&&nextOffset!=offset+packet.bossInfoList.Count)throw new InvalidOperationException();}
    foreach(RushEliteBossInfo info in packet.bossInfoList){int index=rows.FindIndex(delegate(RushEliteBossInfo item){return item.eliteTaskId==info.eliteTaskId;});if(index<0){if(detail)throw new InvalidOperationException();rows.Add(info);}else rows[index]=info;if(unverified.Contains(info.eliteTaskId))pending.Add(info.eliteTaskId);else pending.Remove(info.eliteTaskId);}
    if(selected=="")selected=(string)data["selectedBossId"];if(selected==""&&rows.Count>0)selected=rows[0].eliteTaskId;
    Header(packet);foreach(RushEliteBossInfo info in packet.bossInfoList)Paint(info);
    if(more!=null){Retire(more);more=null;}if(nextOffset>=0)More("更多首领");page.bossIconGrid.repositionNow=true;ShowSelected();scrollPosition=page.bossIconScrollView.transform.localPosition;ok=true;
   }catch(Exception){}
   if(request!=null){request.Dispose();request=null;}busy=false;if(!Current(stamp))yield break;
   if(!ok){if(!detail)More("点击重试");BoxManager.CreatePopupTextBox(error,0f);}
   string next=queued;queued="";if(next!=""&&next==selected&&pending.Contains(next))StartCoroutine(Read(-1,next));
  }
 }
}
