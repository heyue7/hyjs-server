using System;
using System.IO;
using System.Text;
using System.Globalization;
using System.Reflection;
using UnityEngine;
using Room;
using ProtoBuf.Message;

// Temporary hot-load QA caller. Never fabricates packets/results or touches
// battle/reward state. Legal UI/movement actions require an explicit command.
public static class NativeBatchRunner {
 static GameObject host;
 public static string Folder {get{return Path.Combine(Application.persistentDataPath,"hyjs65-qa");}}
 static void Attach(string source) {
  if(host!=null)return;
  Directory.CreateDirectory(Folder);
  GameObject stale=GameObject.Find("HYJS65ReadOnlyProbe");if(stale!=null)UnityEngine.Object.Destroy(stale);
  host=new GameObject("HYJS65ReadOnlyProbe");UnityEngine.Object.DontDestroyOnLoad(host);
  host.AddComponent<HYJS65Probe>();
  Write("{\"kind\":\"attached\",\"source\":"+Q(source)+",\"t\":"+N(Time.realtimeSinceStartup)+"}");
 }
 static Type original;
 static string originalDigest;
 static bool originalMissing;
 static object Pass(string name,object[] args) {
  string path=Path.Combine(Application.persistentDataPath,"native-runner-original65.dll");
  if(!File.Exists(path)) {
   if(!originalMissing){originalMissing=true;Write("{\"kind\":\"original-caller-missing\"}");}
   return null;
  }
  if(original==null) {
  byte[] bytes=File.ReadAllBytes(path);string digest;
  using(System.Security.Cryptography.SHA256 hash=System.Security.Cryptography.SHA256.Create())digest=Convert.ToBase64String(hash.ComputeHash(bytes));
  {
   Type next=Assembly.Load(bytes).GetType("NativeBatchRunner",true);
   if(next.Assembly==typeof(NativeBatchRunner).Assembly)throw new InvalidOperationException("Original caller resolves to current probe");
   // A mistakenly copied QA probe must never recursively load another probe.
   if(next.GetMethod("Pass",BindingFlags.Static|BindingFlags.NonPublic)!=null || next.GetMethod("Load",BindingFlags.Static|BindingFlags.NonPublic)!=null)throw new InvalidOperationException("Original caller is a QA probe");
   original=next;originalDigest=digest;originalMissing=false;
   Write("{\"kind\":\"original-caller-loaded\"}");
  }
  }
  MethodInfo method=original.GetMethod(name,BindingFlags.Static|BindingFlags.Public);
  if(method==null){Write("{\"kind\":\"original-method-missing\",\"method\":"+Q(name)+"}");return null;}
  return method.Invoke(null,args);
 }
 public static bool TryStart(object candidate){Attach("TryStart");object result=Pass("TryStart",new object[]{candidate});return result is bool?(bool)result:true;}
 public static void LogRoomPosition(object net,object response,int phase){Attach("LogRoomPosition");Pass("LogRoomPosition",new object[]{net,response,phase});Write("{\"kind\":\"position-hook\",\"phase\":"+phase+",\"t\":"+N(Time.realtimeSinceStartup)+"}");}
 public static void AfterPVEUIUpdate(object manager){Attach("AfterPVEUIUpdate");Pass("AfterPVEUIUpdate",new object[]{manager});}
 public static void ObservePVEUIPress(int camp,string actorUid,int skillIndex){Attach("ObservePVEUIPress");Pass("ObservePVEUIPress",new object[]{camp,actorUid,skillIndex});}
 public static void ObservePVEUIBegin(object skill){Attach("ObservePVEUIBegin");Pass("ObservePVEUIBegin",new object[]{skill});}
 public static void ApplyActivityInformation(object panel,object packet,bool success){Attach("ApplyActivityInformation");Pass("ApplyActivityInformation",new object[]{panel,packet,success});}
 public static void ApplyExpeditionPreview(object panel){Attach("ApplyExpeditionPreview");Pass("ApplyExpeditionPreview",new object[]{panel});}
 internal static object F(object value,string name) {
  if(value==null)return null;
  for(Type t=value.GetType();t!=null;t=t.BaseType) {
   FieldInfo field=t.GetField(name,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.DeclaredOnly);
   if(field!=null)return field.GetValue(value);
  }
  return null;
 }
 internal static object SF(string type,string name) {
  Type t=typeof(RoomBase).Assembly.GetType(type,false);if(t==null)return null;
  FieldInfo f=t.GetField(name,BindingFlags.Static|BindingFlags.Public|BindingFlags.NonPublic);return f==null?null:f.GetValue(null);
 }
 internal static string Q(string s){if(s==null)return "null";return "\""+s.Replace("\\","\\\\").Replace("\"","\\\"").Replace("\r","\\r").Replace("\n","\\n")+"\"";}
 internal static string N(float n){return float.IsNaN(n)||float.IsInfinity(n)?"null":n.ToString("R",CultureInfo.InvariantCulture);}
 internal static string V(Vector3 p){return "["+N(p.x)+","+N(p.y)+","+N(p.z)+"]";}
 internal static string Active(object value) {
  Component c=value as Component;GameObject g=value as GameObject;if(c!=null)g=c.gameObject;
  return g==null?"null":"{\"self\":"+(g.activeSelf?"true":"false")+",\"hierarchy\":"+(g.activeInHierarchy?"true":"false")+"}";
 }
 internal static bool ObservationLimitReached;
 internal static void Write(string line) {
  string path=Path.Combine(Folder,"observations.jsonl");
  long length=File.Exists(path)?new FileInfo(path).Length:0;
  if(length+Encoding.UTF8.GetByteCount(line+"\n")>8*1024*1024){ObservationLimitReached=true;return;}
  File.AppendAllText(path,line+"\n");
 }
 internal static string Snapshot() {
  RoomBase room=RoomManager.currentRoom;
  RoomCharactorManager manager=room==null?null:room.charactorManager;
  RoomCharactor self=manager==null?null:manager.GetSelfCharactor();
  object loading=SF("LoadingPanel","instance"); // Never call lazy Instance getter.
  StringBuilder s=new StringBuilder("{\"kind\":\"sample\",\"t\":"+N(Time.realtimeSinceStartup)+",\"timeScale\":"+N(Time.timeScale));
  s.Append(",\"room\":").Append(Q(room==null?null:room.GetType().FullName));
  s.Append(",\"scene\":").Append(Q(StaticData.fieldId));
  s.Append(",\"playerPosition\":").Append(self==null?"null":V(self.transform.position));
  AppendSelfNavigation(s,self);
  s.Append(",\"netMask\":").Append(Active(SF("BoxManager","netMask")));
  s.Append(",\"netBox\":").Append(Active(SF("BoxManager","netBox")));
  s.Append(",\"loadingRoot\":").Append(Active(loading));
  s.Append(",\"loadingVisual\":").Append(Active(F(loading,"loading")));
  s.Append(",\"masking\":").Append(Active(F(loading,"masking")));
  s.Append(",\"bottomLoading\":").Append(Active(F(loading,"bottomLoading")));
  s.Append(",\"loadingFlag\":").Append(F(loading,"isLoading") is bool && (bool)F(loading,"isLoading")?"true":"false");
  s.Append(",\"monsters\":[");bool first=true;int count=0;
  if(room is RoomField && manager!=null && StaticData.fieldMon!=null) {
   foreach(System.Collections.Generic.KeyValuePair<string,SideMonster> entry in StaticData.fieldMon) {
    if(count++>=64)break;
    SideMonster m=entry.Value;RoomCharactor owner=manager.GetCharactor(entry.Key);
    if(!first)s.Append(',');first=false;
    s.Append("{\"uuid\":").Append(Q(entry.Key)).Append(",\"monId\":").Append(Q(m.monId)).Append(",\"state\":").Append(m.state);
    s.Append(",\"transform\":").Append(owner==null?"null":V(owner.transform.position));
    s.Append(",\"serverPoint\":").Append(m.currentPoint==null?"null":"["+N(m.currentPoint.x)+","+N(m.currentPoint.y)+","+N(m.currentPoint.z)+"]");
    object motion=null;
    if(owner!=null)foreach(Component item in owner.GetComponents<Component>())if(item!=null && item.GetType().FullName=="HYJS.Field63.FieldMotion"){motion=item;break;}
    object anchor=F(motion,"anchor");s.Append(",\"patrolAnchor\":").Append(anchor is Vector3?V((Vector3)anchor):"null");
    s.Append(",\"ownerRunning\":").Append(owner==null?"null":owner.Running?"true":"false");
    object state=F(F(owner,"ai"),"curState");s.Append(",\"aiState\":").Append(Integer(state));
    foreach(string name in new string[]{"PatrolAttempts","PatrolStarts","PatrolArrivals","PatrolFailures"})s.Append(',').Append(Q(name)).Append(':').Append(Integer(F(motion,name)));
    s.Append(",\"PatrolStatus\":").Append(Q(F(motion,"PatrolStatus") as string));
    foreach(string name in new string[]{"LastSource","LastTarget"}){object point=F(motion,name);s.Append(',').Append(Q(name)).Append(':').Append(point is Vector3?V((Vector3)point):"null");}
    s.Append(",\"moving\":").Append(owner!=null && owner.IsMoving()?"true":"false").Append('}');
   }
  }
  s.Append(']');AppendBossSnapshot(s,room,manager,self);return s.Append('}').ToString();
 }


 internal static string B(object value){return value is bool?((bool)value?"true":"false"):"null";}
 internal static void AppendSelfNavigation(StringBuilder s,RoomCharactor self) {
  object ai=F(self,"ai"),move=F(self,"move");
  s.Append(",\"selfNavigation\":{\"aiType\":").Append(Q(ai==null?null:ai.GetType().FullName));
  s.Append(",\"running\":").Append(self==null?"null":self.Running?"true":"false").Append(",\"isMoving\":").Append(self==null?"null":self.IsMoving()?"true":"false");
  s.Append(",\"pressing\":").Append(B(F(ai,"pressing"))).Append(",\"stoped\":").Append(B(F(ai,"stoped"))).Append(",\"autoPathFinding\":").Append(B(F(ai,"autoPathFinding")));
  object direction=F(ai,"moveDirection"),state=F(ai,"curState");
  s.Append(",\"moveDirection\":").Append(direction is Vector3?V((Vector3)direction):"null").Append(",\"actorState\":").Append(state==null?"null":Convert.ToInt32(state,CultureInfo.InvariantCulture).ToString(CultureInfo.InvariantCulture));
  s.Append(",\"moveType\":").Append(Q(move==null?null:move.GetType().FullName)).Append(",\"navigationRunning\":").Append(B(F(move,"running")));
  s.Append(",\"first\":").Append(B(F(move,"first"))).Append(",\"usingNavAgent\":").Append(B(F(move,"_usingNavAgent")));
  NavMeshAgent agent=F(move,"_agent") as NavMeshAgent;
  s.Append(",\"agentPresent\":").Append(agent==null?"false":"true").Append(",\"agentEnabled\":").Append(Enabled(agent));
  s.Append(",\"agentOnNavMesh\":").Append(agent==null?"null":agent.isOnNavMesh?"true":"false");
  System.Collections.Generic.Queue<Vector3> corners=F(move,"corners") as System.Collections.Generic.Queue<Vector3>;
  s.Append(",\"nativeCornerCount\":").Append(corners==null?"null":corners.Count.ToString(CultureInfo.InvariantCulture));
  s.Append(",\"nativeNextCorner\":").Append(corners==null||corners.Count==0?"null":V(corners.Peek()));
  Vector3[] points=corners!=null&&corners.Count<=128?corners.ToArray():null;
  s.Append(",\"nativeGoal\":").Append(points==null||points.Length==0?"null":V(points[points.Length-1])).Append('}');
 }
 internal static string Enabled(object value) {
  Behaviour b=value as Behaviour;if(b!=null)return b.enabled?"true":"false";
  Collider c=value as Collider;return c==null?"null":c.enabled?"true":"false";
 }
 internal static string Integer(object value){return value==null?"null":Convert.ToInt32(value,CultureInfo.InvariantCulture).ToString(CultureInfo.InvariantCulture);}
 internal static void AppendBossSnapshot(StringBuilder s,RoomBase room,RoomCharactorManager manager,RoomCharactor self) {
  // GetCurrState/InvincibleTimer IL reads these backing fields directly.
  // Use the already-created normal state, without invoking either getter.
  System.Collections.IDictionary states=F(F(self,"ai"),"allStates") as System.Collections.IDictionary;
  object normal=states!=null&&states.Contains(0)?states[0]:null;
  object invincible=F(normal,"invincibleTimer");
  s.Append(",\"selfInvincibleTimer\":").Append(invincible is float?N((float)invincible):"null");
  s.Append(",\"boss\":[");bool first=true;
  System.Collections.Generic.Dictionary<string,RoomCharactor> owners=new System.Collections.Generic.Dictionary<string,RoomCharactor>();
  System.Collections.Generic.Dictionary<string,SideBoss> packets=new System.Collections.Generic.Dictionary<string,SideBoss>();
  // SCSideBoss creates owners without populating StaticData.fieldBoss.
  // Enumerate already-created characters first and read their bound packet.
  // No owner creation, lazy getter, OnClick, send, or movement is invoked.
  if(room is RoomField && manager!=null) {
   foreach(var entry in manager.GetCharactors()) {
    RoomCharactor owner=entry.Value;object ui=F(owner,"ui"),ai=F(owner,"ai");
    if(!((ui!=null&&ui.GetType().FullName=="Room.RCUIFieldBoss")||(ai!=null&&ai.GetType().FullName=="Room.RCAIFieldBoss")))continue;
    SideBoss bound=F(ui,"boss") as SideBoss;
    string uuid=bound!=null&&!String.IsNullOrEmpty(bound.uuid)?bound.uuid:entry.Key;
    if(String.IsNullOrEmpty(uuid)||owners.ContainsKey(uuid))continue;
    owners.Add(uuid,owner);packets.Add(uuid,bound);
    if(owners.Count>=8)break;
   }
   // Cache is supplemental only. Preserve live packet/owner precedence and
   // deduplicate by packet UUID, since the cache key may be stale.
   System.Collections.IDictionary bosses=SF("StaticData","fieldBoss") as System.Collections.IDictionary;
   if(bosses!=null)foreach(System.Collections.DictionaryEntry entry in bosses) {
    if(owners.Count>=8)break;
    SideBoss packet=entry.Value as SideBoss;
    string uuid=packet!=null&&!String.IsNullOrEmpty(packet.uuid)?packet.uuid:entry.Key as string;
    if(String.IsNullOrEmpty(uuid)||owners.ContainsKey(uuid))continue;
    owners.Add(uuid,manager.GetCharactor(uuid));packets.Add(uuid,packet);
   }
  }
  foreach(System.Collections.Generic.KeyValuePair<string,RoomCharactor> entry in owners) {
   RoomCharactor owner=entry.Value;object ui=F(owner,"ui"),ai=F(owner,"ai");
   SideBoss bound=F(ui,"boss") as SideBoss,server=packets[entry.Key];
   if(!first)s.Append(',');first=false;
   s.Append("{\"uuid\":").Append(Q(entry.Key)).Append(",\"uiType\":").Append(Q(ui==null?null:ui.GetType().FullName));
   s.Append(",\"uiBossUuid\":").Append(Q(bound==null?null:bound.uuid));
   s.Append(",\"serverUuid\":").Append(Q(server==null?null:server.uuid));
   s.Append(",\"ownerTransform\":").Append(owner==null?"null":V(owner.transform.position));
   s.Append(",\"aiType\":").Append(Q(ai==null?null:ai.GetType().FullName)).Append(",\"aiCurState\":").Append(Integer(F(ai,"curState")));
   s.Append(",\"selfDistance\":").Append(owner==null||self==null?"null":N(Vector3.Distance(owner.transform.position,self.transform.position))).Append('}');
  }
  s.Append(']');
 }
 internal static string PathOf(Transform value) {
  if(value==null)return null;string p=value.name;
  for(Transform t=value.parent;t!=null;t=t.parent)p=t.name+"/"+p;
  return p;
 }
 static Transform TransformOf(object value) {
  GameObject g=value as GameObject;if(g!=null)return g.transform;
  Component c=value as Component;return c==null?null:c.transform;
 }
 static void UITree(Transform t,string file,ref int count,int depth) {
  if(t==null)return;
  if(count++>=4096 || depth>64){File.AppendAllText(file,"{\"kind\":\"truncated\"}\n");return;}
  UIWidget[] widgets=t.GetComponents<UIWidget>();
  StringBuilder row=new StringBuilder("{\"kind\":\"ui-node\",\"t\":"+N(Time.realtimeSinceStartup)+",\"name\":"+Q(t.name)+",\"path\":"+Q(PathOf(t)));
  row.Append(",\"active\":").Append(Active(t)).Append(",\"localPosition\":").Append(V(t.localPosition)).Append(",\"worldPosition\":").Append(V(t.position));
  row.Append(",\"widgetCount\":").Append(widgets.Length).Append(",\"widgets\":[");bool first=true;
  foreach(UIWidget w in widgets) {
   if(!first)row.Append(',');first=false;
   row.Append("{\"type\":").Append(Q(w.GetType().FullName)).Append(",\"width\":").Append(w.width).Append(",\"height\":").Append(w.height).Append(",\"worldCorners\":[");
   bool cornerFirst=true;foreach(Vector3 point in w.worldCorners){if(!cornerFirst)row.Append(',');cornerFirst=false;row.Append(V(point));}row.Append("]}");
  }
  File.AppendAllText(file,row.Append("]}\n").ToString());
  foreach(Transform child in t)UITree(child,file,ref count,depth+1);
 }
 internal static void DumpUI(string typeName,string filename) {
  Type type=typeof(RoomBase).Assembly.GetType(typeName,false);if(type==null)throw new InvalidOperationException("UI type unavailable");
  string file=Path.Combine(Folder,filename);
  UnityEngine.Object[] found=Resources.FindObjectsOfTypeAll(type);
  File.AppendAllText(file,"{\"kind\":\"ui-dump\",\"component\":"+Q(typeName)+",\"instances\":"+found.Length+",\"t\":"+N(Time.realtimeSinceStartup)+"}\n");
  foreach(UnityEngine.Object item in found) {
   Component component=item as Component;if(component==null)continue;
   if(typeName=="UI.UIActivityEveryDaySign") {
    foreach(string field in new string[]{"normalSignTab","superSignTab","limitSignTab","normalTip","superTip","limitSignTip"}) {
     object value=F(component,field);Transform target=TransformOf(value);
     File.AppendAllText(file,"{\"kind\":\"ui-field\",\"name\":"+Q(field)+",\"path\":"+Q(PathOf(target))+",\"active\":"+Active(value)+"}\n");
    }
   } else {
    for(Type t=type;t!=null && t!=typeof(MonoBehaviour);t=t.BaseType)foreach(FieldInfo field in t.GetFields(BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.DeclaredOnly)) {
     Transform target=TransformOf(field.GetValue(component));
     File.AppendAllText(file,"{\"kind\":\"ui-field\",\"name\":"+Q(field.Name)+",\"fieldType\":"+Q(field.FieldType.FullName)+",\"objectPath\":"+Q(PathOf(target))+"}\n");
    }
   }
   int count=0;UITree(component.transform,file,ref count,0);
  }
 }
 internal static void Command(string text) {
  string[] parts=text.Trim().Split(new char[]{' ','\t','\r','\n'},StringSplitOptions.RemoveEmptyEntries);
  if(parts.Length==0)return;
  if(parts[0]=="inventory" && parts.Length==2) {
   string id=parts[1];long count=0;UI.UIPlayerData player=StaticData.playerData;
   if(player==null || player.playerId!=1)throw new InvalidOperationException("Owned QA player required");
   if(id=="item_005" || id=="item_9710") {UI.UISimpleItem item;if(player.SimpleGoods.TryGetValue(id,out item)&&item!=null)count=item.num;}
   else if(id=="hero_exp_004") {foreach(UI.UIHero hero in player.InstPlayerData.InstHeros.Values)if(hero!=null&&hero.heroId==id)count++;}
   else if(id.StartsWith("frag_hero_",StringComparison.Ordinal)) {object dict=F(player,"fragHeros");System.Collections.IDictionary values=dict as System.Collections.IDictionary;if(values==null)throw new InvalidOperationException("Fragment collection missing");UI.UIFragItem frag=values[id] as UI.UIFragItem;if(frag!=null)count=frag.num;}
   else throw new InvalidOperationException("Unsupported inventory observation");
   Write("{\"kind\":\"inventory\",\"resource\":"+Q(id)+",\"amount\":"+count+",\"t\":"+N(Time.realtimeSinceStartup)+"}");return;
  }
  if(parts[0]=="sign" && parts.Length==1){DumpUI("UI.UIActivityEveryDaySign","sign-ui.jsonl");return;}
  if(parts[0]=="ui" && parts.Length==2 && (parts[1]=="sign" || parts[1]=="topbar")){DumpUI(parts[1]=="sign"?"UI.UIActivityEveryDaySign":"UI.TopBarItem",parts[1]=="sign"?"sign-ui.jsonl":"topbar-ui.jsonl");return;}
  if(parts[0]=="open" && parts.Length==4 && parts[1]=="copy") {
   if(!((parts[2]=="1" && parts[3]=="3") || (parts[2]=="11" && parts[3]=="60")))throw new InvalidOperationException("Unsupported QA copy selection");
   // Native page options select a page only. Do not invoke OnChallengeClick/Enter.
   UI.PageManager.Instance.OpenPage("UICopy","chapterId="+parts[2]+"&levelId="+parts[3]);return;
  }
  if(parts[0]=="open" && parts.Length==2) {
   string page=parts[1]=="profile"?"PlayerInfoPage":parts[1]=="wanted"?"MercenaryTaskPage":parts[1]=="credit"?"ShopPage":parts[1]=="activity"?"UIActivity":parts[1]=="trial"?"RushPage":parts[1]=="map"?"WorldMapPage":parts[1]=="copy"?"UICopy":null;
   if(page==null)throw new InvalidOperationException("Unsupported page");
   UI.PageManager.Instance.OpenPage(page,parts[1]=="credit"?"type=11":parts[1]=="wanted"?"tab=1":String.Empty);return;
  }
  if(parts[0]=="ui" && parts.Length==2 && (parts[1]=="profile" || parts[1]=="choice" || parts[1]=="wanted" || parts[1]=="trial")) {
   string type=parts[1]=="profile"?"UI.PlayerInfoPage":parts[1]=="choice"?"UI.SelectReward":parts[1]=="wanted"?"UI.WantedMission":"UI.RushPage";
   DumpUI(type,parts[1]+"-ui.jsonl");return;
  }
  if(parts[0]=="field" && parts.Length==2 && (parts[1]=="0" || parts[1]=="1")) {
   UI.WorldMapPage map=UI.PageManager.Instance.GetDictPage("WorldMapPage") as UI.WorldMapPage;
   if(map==null || !map.gameObject.activeInHierarchy)throw new InvalidOperationException("Open map and wait for initialization first");
   typeof(UI.WorldMapPage).GetMethod("onMapClick",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic).Invoke(map,new object[]{parts[1]});return;
  }

  if(parts[0]=="navsample" && parts.Length==4) {
   RoomBase room=RoomManager.currentRoom;
   if(!(room is RoomField))throw new InvalidOperationException("Navsample requires an active field");
   RoomCharactor self=room.charactorManager==null?null:room.charactorManager.GetSelfCharactor();
   if(self==null)throw new InvalidOperationException("Navsample requires native self");
   float[] p=new float[3];for(int i=0;i<3;i++){p[i]=float.Parse(parts[i+1],CultureInfo.InvariantCulture);if(float.IsNaN(p[i])||float.IsInfinity(p[i])||Mathf.Abs(p[i])>1000f)throw new InvalidOperationException("Navsample coordinate outside diagnostic bounds");}
   Vector3 target=new Vector3(p[0],p[1],p[2]);NavMeshHit hit;
   bool sampled=NavMesh.SamplePosition(target,out hit,100f,NavMesh.AllAreas);
   NavMeshPath path=new NavMeshPath();
   bool calculated=sampled&&NavMesh.CalculatePath(self.transform.position,hit.position,NavMesh.AllAreas,path);
   Vector3 delta=sampled?hit.position-target:Vector3.zero;delta.y=0;
   StringBuilder cornerPoints=new StringBuilder("[");for(int c=0;c<path.corners.Length && c<128;c++){if(c>0)cornerPoints.Append(',');cornerPoints.Append(V(path.corners[c]));}cornerPoints.Append(']');
   Write("{\"kind\":\"navsample\",\"requested\":"+V(target)+",\"sampled\":"+(sampled?"true":"false")+",\"projected\":"+(sampled?V(hit.position):"null")+",\"xzOffset\":"+(sampled?N(delta.magnitude):"null")+",\"selfPosition\":"+V(self.transform.position)+",\"calculated\":"+(calculated?"true":"false")+",\"status\":"+(sampled?Q(path.status.ToString()):"null")+",\"corners\":"+path.corners.Length+",\"cornerPoints\":"+cornerPoints.ToString()+",\"t\":"+N(Time.realtimeSinceStartup)+"}");return;
  }
  if(parts[0]=="goto" && parts.Length==4) {
   RoomBase room=RoomManager.currentRoom;
   if(!(room is RoomField)&&!(room is RoomCopy))throw new InvalidOperationException("Goto requires an active field or copy");
   RoomCharactor self=room.charactorManager==null?null:room.charactorManager.GetSelfCharactor();
   object ai=F(self,"ai"),move=F(self,"move"),state=F(ai,"curState");
   if(self==null||!self.Running||ai==null||ai.GetType().FullName!="Room.RCAISelf"||move==null||move.GetType().FullName!="Room.RCMoveNavigation")throw new InvalidOperationException("Native self navigation unavailable");
   if(!(F(move,"running") is bool)||!(bool)F(move,"running")||!(F(move,"first") is bool)||(bool)F(move,"first"))throw new InvalidOperationException("Native navigation not initialized and running");
   if(!(F(ai,"pressing") is bool)||(bool)F(ai,"pressing")||!(F(ai,"stoped") is bool)||!(bool)F(ai,"stoped")||state==null||Convert.ToInt32(state,CultureInfo.InvariantCulture)!=0)throw new InvalidOperationException("Release native joystick and wait for idle actor state");
   NavMeshAgent agent=F(move,"_agent") as NavMeshAgent;
   if(agent!=null&&(!agent.enabled||!agent.isOnNavMesh))throw new InvalidOperationException("Existing native agent not ready");
   float[] p=new float[3];for(int i=0;i<3;i++){p[i]=float.Parse(parts[i+1],CultureInfo.InvariantCulture);if(float.IsNaN(p[i])||float.IsInfinity(p[i])||Mathf.Abs(p[i])>1000f)throw new InvalidOperationException("Goto coordinate outside diagnostic bounds");}
   Vector3 target=new Vector3(p[0],p[1],p[2]);const float originalMineStopDistance=0.8f;
   Vector3 endpoint=Vector3.MoveTowards(target,self.transform.position,originalMineStopDistance);
   NavMeshPath path=new NavMeshPath();
   bool calculated=NavMesh.CalculatePath(self.transform.position,endpoint,NavMesh.AllAreas,path);
   Write("{\"kind\":\"native-goto-path-check\",\"calculated\":"+(calculated?"true":"false")+",\"status\":"+Q(path.status.ToString())+",\"corners\":"+path.corners.Length+",\"requested\":"+V(target)+",\"checkedEndpoint\":"+V(endpoint)+",\"t\":"+N(Time.realtimeSinceStartup)+"}");
   if(!calculated||path.status!=NavMeshPathStatus.PathComplete||path.corners.Length==0)throw new InvalidOperationException("Requested native goto has no complete path");
   MethodInfo method=ai.GetType().GetMethod("AutoGoto",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
   if(method==null)throw new InvalidOperationException("Original AutoGoto missing");
   method.Invoke(ai,new object[]{target,null,originalMineStopDistance});
   Write("{\"kind\":\"native-goto\",\"requested\":"+V(target)+",\"checkedEndpoint\":"+V(endpoint)+",\"stopDistance\":0.8,\"pathCorners\":"+path.corners.Length+",\"t\":"+N(Time.realtimeSinceStartup)+"}");return;
  }
  if((parts[0]=="move" || parts[0]=="stopto") && parts.Length==4) {
   RoomBase room=RoomManager.currentRoom;
   if(!(room is RoomField) && !(room is RoomTown) && !(room is RoomCopy))throw new InvalidOperationException("Movement requires an active town/field/copy room");
   RoomCharactor self=room.charactorManager.GetSelfCharactor();if(self==null || !self.Running)throw new InvalidOperationException("Player is not running");
   float[] p=new float[3];for(int i=0;i<3;i++){p[i]=float.Parse(parts[i+1],CultureInfo.InvariantCulture);if(float.IsNaN(p[i]) || float.IsInfinity(p[i]) || Mathf.Abs(p[i])>1000)throw new InvalidOperationException("Coordinate outside diagnostic bounds");}
   Vector3 point=new Vector3(p[0],p[1],p[2]);if(parts[0]=="move")self.MoveTo(point);else self.StopTo(point);return;
  }
  if(parts[0]=="sample" && parts.Length==1){if(host==null)throw new InvalidOperationException("Probe unavailable");host.GetComponent<HYJS65Probe>().BeginSampling();return;}
  if(parts[0]=="stop" && parts.Length==1){if(host!=null)UnityEngine.Object.Destroy(host);host=null;return;}
  throw new InvalidOperationException("Unsupported command");
 }
}
public sealed class HYJS65Probe:MonoBehaviour {
 float began,nextSample,nextCommand;bool failed,samplingStopped;
 void Start(){BeginSampling();}
 public void BeginSampling(){began=Time.realtimeSinceStartup;nextSample=began;samplingStopped=false;NativeBatchRunner.ObservationLimitReached=false;}
 void Update() {
  if(failed)return;
  float now=Time.realtimeSinceStartup;
  try {
   string path=Path.Combine(NativeBatchRunner.Folder,"observations.jsonl");
   if(!samplingStopped && (now-began>180f || NativeBatchRunner.ObservationLimitReached || File.Exists(path)&&new FileInfo(path).Length>=8*1024*1024)){samplingStopped=true;NativeBatchRunner.Write("{\"kind\":\"sampling-stopped-commands-active\"}");}
   if(!samplingStopped && now>=nextSample){nextSample=now+0.1f;NativeBatchRunner.Write(NativeBatchRunner.Snapshot());}
   if(now>=nextCommand) {
    nextCommand=now+0.2f;string command=Path.Combine(NativeBatchRunner.Folder,"command.txt");
    if(File.Exists(command)) {
     string value=File.ReadAllText(command);File.Delete(command);
     if(value.Length>128)throw new InvalidOperationException("Command exceeds bound");
     try {NativeBatchRunner.Command(value);NativeBatchRunner.Write("{\"kind\":\"command-ok\",\"command\":"+NativeBatchRunner.Q(value.Trim())+",\"t\":"+NativeBatchRunner.N(now)+"}");}
     catch(Exception e){string message=e.Message??String.Empty;if(message.Length>256)message=message.Substring(0,256);NativeBatchRunner.Write("{\"kind\":\"command-failed\",\"errorType\":"+NativeBatchRunner.Q(e.GetType().Name)+",\"message\":"+NativeBatchRunner.Q(message)+",\"t\":"+NativeBatchRunner.N(now)+"}");}
    }
   }
  }catch(Exception e){failed=true;try{NativeBatchRunner.Write("{\"kind\":\"probe-error\",\"errorType\":"+NativeBatchRunner.Q(e.GetType().Name)+"}");}catch(Exception){}}
 }
}
