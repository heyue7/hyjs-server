using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using UnityEngine;
using Room;
using ProtoBuf.Message;

namespace HYJS.Field63 {
// Local visual policy approved for batch63: one second rest, at most eight
// NavMesh samples. Authoritative uuid/state, starts, qualification and rewards
// continue through the original protocol and server.
public static class Runtime {
 static readonly MethodInfo click=typeof(RCUIServerMonster).GetMethod("OnClick",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
 static readonly Dictionary<string,FieldInfo> fields=new Dictionary<string,FieldInfo>();
 internal static object Get(object value,string name) {
  if(value==null)return null;
  Type type=value.GetType();string key=type.FullName+":"+name;FieldInfo field;
  if(!fields.TryGetValue(key,out field)) {
   for(Type t=type;t!=null;t=t.BaseType) {field=t.GetField(name,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.DeclaredOnly);if(field!=null)break;}
   fields[key]=field;
  }
  return field==null?null:field.GetValue(value);
 }
 internal static FieldMotion Motion(RoomCharactor owner) {
  if(owner==null)return null;
  FieldMotion motion=owner.gameObject.GetComponent<FieldMotion>();
  if(motion==null)motion=owner.gameObject.AddComponent<FieldMotion>();
  return motion;
 }
 public static void CollisionTick(object value) {
  RCUIServerMonster ui=value as RCUIServerMonster;if(ui==null)return;
  if(!(RoomManager.currentRoom is RoomField) || Time.timeScale<=0)return;
  RoomCharactor owner=Get(ui,"owner") as RoomCharactor;
  FieldMotion motion=Motion(owner);if(motion==null)return;
  float now=Time.unscaledTime;
  if(now<motion.NextCollision)return;
  motion.NextCollision=now+0.1f;
  if(now<motion.NextRetry)return;
  RoomCharactor self=owner.charactorManager.GetSelfCharactor();
  if(self==null)return;
  RCAISelf ai=self.ai as RCAISelf;
  if(ai==null || ai.AutoPathFinding)return;
  if(Vector3.Distance(owner.transform.position,self.transform.position)>=1.3f)return;
  // Original OnClick retains normal/death/invincibility and fight checks.
  // Retry at the original one-second cadence while a response is in flight.
  motion.NextRetry=now+1f;
  Debug.Log("[HYJS63 field] contact-check t="+now);
  if(click!=null)click.Invoke(ui,null);
 }
 public static void PatrolTick(object value) {
  RCAIServerMonster ai=value as RCAIServerMonster;if(ai==null)return;
  RoomCharactor owner=Get(ai,"owner") as RoomCharactor;
  if(owner==null || !(RoomManager.currentRoom is RoomField))return;
  // Initial scene/reconnect loaders can create the original UI/AI before this
  // module sees a push. Bind from its original UUID and freshest server cache.
  SideMonster original=Get(owner.ui,"monster") as SideMonster;
  if(original==null)return;
  SideMonster latest;
  if(!StaticData.fieldMon.TryGetValue(original.uuid,out latest))latest=original;
  FieldMotion motion=Motion(owner);
  motion.Bind(owner,latest);
  motion.Advance(ai);
 }
 public static void Monsters(object net,object message) {
  SCSideMonster packet=message as SCSideMonster;
  RoomBase room=Get(net,"roomBase") as RoomBase;
  if(packet==null || room==null || !(room is RoomField))return;
  RoomCharactorManager manager=room.charactorManager;
  Debug.Log("[HYJS63 field] monster-push count="+packet.monsters.Count+" t="+Time.unscaledTime);
  // Apply only received monsters. The original handler revisited every cached
  // monster and stopped each path even for an unrelated spawn refresh.
  foreach(SideMonster monster in packet.monsters) {
   RoomCharactor owner=manager.GetCharactor(monster.uuid);
   bool created=owner==null;
   if(owner==null && monster.state!=0 && monster.state!=3) {
    manager.CreateMonster(monster);owner=manager.GetCharactor(monster.uuid);
    if(owner!=null)manager.SetPosition(monster.uuid,Point(monster));
   }
   StaticData.fieldMon[monster.uuid]=monster;
   if(owner==null) {if(monster.state==3)StaticData.fieldMon.Remove(monster.uuid);continue;}
   RCAIServerMonster ai=owner.ai as RCAIServerMonster;
   if(ai==null)continue;
   if(ai.curState!=monster.state)ai.SwitchState(monster.state);ai.SetHp((float)monster.curHp);
   FieldMotion motion=Motion(owner);motion.Bind(owner,monster);
   if(monster.state==3) {
    motion.Halt();manager.DestroyCharactor(monster.uuid);StaticData.fieldMon.Remove(monster.uuid);
   } else if(monster.state!=1)motion.Halt();
   if(created) {owner.ui.CreateBoxCollider();owner.ui.CreateRigidbody();}
  }
 }
 internal static Vector3 Point(SideMonster monster) {return new Vector3(monster.currentPoint.x,monster.currentPoint.y,monster.currentPoint.z);}
 internal static bool Config(SideMonster monster,out float radius,out float speed) {
  radius=0;speed=0;
  foreach(Dict.DataModel.TableFieldMonster row in Dict.DictDataManager.Instance.tableFieldMonsterDao.CacheList) {
   if(row.MonId==monster.monId && row.TeamLeader==1) {
    radius=(float)row.WanderR;speed=(float)row.WanderSpeed;
    return radius>0 && speed>0 && !float.IsNaN(radius) && !float.IsNaN(speed) && !float.IsInfinity(radius) && !float.IsInfinity(speed);
   }
  }
  return false;
 }
}
public sealed class FieldMotion:MonoBehaviour {
 public float NextCollision,NextRetry;
 RoomCharactor owner;
 Vector3 anchor;
 string uuid,monId;
 float radius,speed,nextMove;
 bool configured,moving;
 public void Bind(RoomCharactor next,SideMonster monster) {
  Vector3 point=Runtime.Point(monster);
  bool corrected=uuid==monster.uuid && (point-anchor).sqrMagnitude>0.0001f;
  bool changed=uuid!=monster.uuid || monId!=monster.monId || corrected;
  owner=next;
  if(!changed)return;
  Halt();uuid=monster.uuid;monId=monster.monId;anchor=point;
  configured=Runtime.Config(monster,out radius,out speed);
  nextMove=Time.unscaledTime+1f;
  NextRetry=0;
  // Keep authoritative position correction. Running navigation queues the
  // original full path; paused navigation uses its original SamplePosition.
  // Same UUID + same anchor does not touch a path.
  if(corrected) {owner.StopTo(point);moving=owner.IsMoving();}
 }
 public void Halt() {
  if(owner!=null && moving)owner.StopMove();
  moving=false;nextMove=Time.unscaledTime+1f;
 }
 public void Advance(RCAIServerMonster ai) {
  if(!configured || owner==null)return;
  // Running also covers room pause/background; keep original animations and
  // state transitions. No positional changes are made during fights/death.
  if(!(RoomManager.currentRoom is RoomField) || !owner.Running || ai.curState!=1 || Time.timeScale<=0) {Halt();return;}
  float now=Time.unscaledTime;
  if(moving) {
   if(owner.IsMoving())return;
   moving=false;nextMove=now+1f;return;
  }
  if(now<nextMove)return;
  nextMove=now+1f;
  // Field move type 2 is RCMoveNavigation. Its agent is lazy and may be
  // absent/disabled while the original corner queue still handles movement.
  // Follow the original navigation's all-area sampling, without requiring it.
  if(!(owner.move is RCMoveNavigation))return;
  for(int i=0;i<8;i++) {
   Vector2 disk=UnityEngine.Random.insideUnitCircle*radius;
   Vector3 wanted=anchor+new Vector3(disk.x,0,disk.y);NavMeshHit hit;
   if(!NavMesh.SamplePosition(wanted,out hit,1f,-1))continue;
   Vector3 delta=hit.position-anchor;delta.y=0;
   if(delta.sqrMagnitude>radius*radius || (hit.position-owner.transform.position).sqrMagnitude<0.04f)continue;
   NavMeshPath path=new NavMeshPath();
   if(!NavMesh.CalculatePath(owner.transform.position,hit.position,-1,path) || path.status!=NavMeshPathStatus.PathComplete)continue;
   bool inside=true;
   foreach(Vector3 corner in path.corners) {Vector3 d=corner-anchor;d.y=0;if(d.sqrMagnitude>radius*radius){inside=false;break;}}
   if(!inside)continue;
   // Original MonsterAIStateRest uses StopTo; for a running navigation
   // component it calculates/enqueues the full route and Tick walks it.
   owner.SetSpeed(speed);owner.StopTo(hit.position);moving=owner.IsMoving();return;
  }
 }
 void Update() {
  if(owner!=null && (!(RoomManager.currentRoom is RoomField) || !owner.Running || Time.timeScale<=0))Halt();
 }
 void OnDisable(){Halt();}
}
}
