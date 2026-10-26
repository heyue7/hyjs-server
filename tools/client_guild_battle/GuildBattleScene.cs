// Re1.1.11 procedural scenery adapted for the original Unity5.3 client.
// Battle rules, account transport and result authority remain on the server.
// Artifact: OfflineBootstrap.dll; SHA256: b26c6b453e5a534bcdfa7da5d302f9266c9ade88e813129d6ecd321ebe59179b
// Type: OfflineGuildBattleAssets310; ILSpy 8.2.0.7535; target code was not executed.
// Scope: complete selected type

using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using System.Text;
using LitJson;
using ProtoBuf.Message;
using System.Runtime.CompilerServices;
using AssetLoad;
using Dict;
using Dict.Blo;
using Dict.DataModel;
using Room;
using UI;
using UnityEngine;
using UnityEngine.SceneManagement;

namespace HYJS.GuildBattle {
public sealed class OwnedSceneLifetime : MonoBehaviour {
 private void Update() { GuildBattleScene.GuardTick(); }
 private void OnDestroy() { GuildBattleScene.Release(gameObject); }
}
public sealed class GuildEntryProbe : MonoBehaviour {
 internal UnionFightSignUpItem Item;
 internal WWW Request;
 private bool busy;
 internal void Begin(UnionFightSignUpItem item) {
  if (busy) return;
  Item = item; busy = true;
  StartCoroutine(GuildBattleScene.ProbeEntry(this, item));
 }
 internal void Complete() {
  if (Request != null) Request.Dispose();
  Request = null; Item = null; busy = false;
 }
 private void OnDisable() { StopAllCoroutines(); Complete(); }
}
public static class GuildBattleScene
{
 public static void ObserveTown(TownPage page) { GuildWarAvailability.EnsureTownAvailability(page); }
 public static bool BlockTownEntry(TownPage page) { return !GuildWarAvailability.CanEnterFromTown(page); }
	private static GameObject root;
    private static RoomUnionFight sceneRoom;
    private static long scenePlayerID;
    private static string sceneSession;
    private static int sceneGeneration;
    private static float particleDeadline;

	private static Material stone;

	private static Material blue;

	private static Material red;

	private static Material wood;

	private static Material road;

	private static Texture2D map;

	private static bool prepared;

 private sealed class Projection {
  public object Owner; public FieldInfo Field; public PropertyInfo Property; public string Original;
  public void Restore() { if (Field != null) Field.SetValue(Owner, Original); else Property.SetValue(Owner, Original, null); }
 }
 private static readonly List<Projection> projections = new List<Projection>();
 private static readonly List<Texture2D> textures = new List<Texture2D>();
 private static readonly List<Mesh> meshes = new List<Mesh>();
 private static readonly List<MiniMapRestore> miniMaps = new List<MiniMapRestore>();
 private sealed class MiniMapRestore {
  public MiniMapComponent Component; public UITexture Texture; public Texture OriginalTexture;
  public object Scale, Offset, Angle;
 }
 private static void Project(object owner, FieldInfo field, PropertyInfo property) {
  string original = field != null ? (string)field.GetValue(owner) : (string)property.GetValue(owner, null);
  if (string.IsNullOrEmpty(original)) return;
  string projected = Flatten(original);
  if (projected == original) return;
  projections.Add(new Projection { Owner = owner, Field = field, Property = property, Original = original });
  if (field != null) field.SetValue(owner, projected); else property.SetValue(owner, projected, null);
 }
 public static void Release(GameObject owner) {
  if (!object.ReferenceEquals(root, owner)) return;
  CancelGuard(true);
  ResetPlayerRequests();
  root = null; sceneRoom = null; scenePlayerID = 0; sceneSession = null;
  for (int i = projections.Count - 1; i >= 0; i--) projections[i].Restore();
  projections.Clear(); prepared = false;
  foreach (MiniMapRestore restore in miniMaps) {
   if (restore.Component == null || restore.Texture == null || restore.Texture.mainTexture != map) continue;
   restore.Texture.mainTexture = restore.OriginalTexture;
   typeof(MiniMapComponent).GetField("scale",Flags).SetValue(restore.Component,restore.Scale);
   typeof(MiniMapComponent).GetField("offset",Flags).SetValue(restore.Component,restore.Offset);
   typeof(MiniMapComponent).GetField("angle",Flags).SetValue(restore.Component,restore.Angle);
  }
  miniMaps.Clear();
  foreach (Material material in new Material[] { stone, blue, red, wood, road }) if (material != null) UnityEngine.Object.Destroy(material);
  stone = blue = red = wood = road = null;
  foreach (Texture2D texture in textures) if (texture != null) UnityEngine.Object.Destroy(texture);
  textures.Clear(); if (map != null) UnityEngine.Object.Destroy(map); map = null;
  foreach (Mesh mesh in meshes) if (mesh != null) UnityEngine.Object.Destroy(mesh);
  meshes.Clear();
 }
	private static readonly BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;

	public static bool Active {
		get {
			return CanLoad() && root != null && sceneRoom != null && object.ReferenceEquals(RoomManager.currentRoom,sceneRoom) && StaticData.playerData != null && StaticData.playerData.playerId == scenePlayerID && SessionHex() == sceneSession;
		}
	}

	public static Vector3 Ground (Vector3 p)
	{
		return new Vector3 (Mathf.Clamp (p.x, -35f, 115f), 0f, Mathf.Clamp (p.z, -50f, 50f));
	}

	private static Vector3 Vec (string text)
	{
		string[] array = text.Split (',');
		return Ground (new Vector3 (float.Parse (array [0], CultureInfo.InvariantCulture), float.Parse (array [1], CultureInfo.InvariantCulture), float.Parse (array [2], CultureInfo.InvariantCulture)));
	}

	private static string Flatten (string text)
	{
		string[] array = text.Split (';');
		for (int i = 0; i < array.Length; i++) {
			string[] array2 = array [i].Split (',');
			float result;
			float result2;
			if (array2.Length == 3 && float.TryParse (array2 [0], NumberStyles.Float, CultureInfo.InvariantCulture, out result) && float.TryParse (array2 [2], NumberStyles.Float, CultureInfo.InvariantCulture, out result2)) {
				array [i] = array2 [0] + ",0," + array2 [2];
			}
		}
		return string.Join (";", array);
	}

	public static void Prepare ()
	{
		if (prepared) {
			return;
		}
		prepared = true;
		DictDataManager instance = DictDataManager.Instance;
		FieldInfo[] fields = instance.GetType ().GetFields (Flags);
		foreach (FieldInfo fieldInfo in fields) {
			if (!fieldInfo.Name.StartsWith ("tableUnionFight", StringComparison.Ordinal)) {
				continue;
			}
			object value = fieldInfo.GetValue (instance);
			if (value == null) {
				continue;
			}
			PropertyInfo property = value.GetType ().GetProperty ("CacheList");
			if (property == null) {
				continue;
			}
			IList list = property.GetValue (value, null) as IList;
			if (list == null) {
				continue;
			}
			foreach (object item in list) {
				FieldInfo[] fields2 = item.GetType ().GetFields (Flags);
				foreach (FieldInfo fieldInfo2 in fields2) {
					if (fieldInfo2.FieldType == typeof(string) && (fieldInfo2.Name == "Position" || fieldInfo2.Name == "TransferStart" || fieldInfo2.Name == "TransferEnd" || fieldInfo2.Name == "CariotBornPos" || fieldInfo2.Name == "CariotMoveNavInfo")) {
						Project(item, fieldInfo2, null);
					}
				}
				PropertyInfo[] properties = item.GetType ().GetProperties ();
				foreach (PropertyInfo propertyInfo in properties) {
					if (propertyInfo.CanWrite && propertyInfo.PropertyType == typeof(string) && (propertyInfo.Name == "Position" || propertyInfo.Name == "TransferStart" || propertyInfo.Name == "TransferEnd" || propertyInfo.Name == "CariotBornPos" || propertyInfo.Name == "CariotMoveNavInfo")) {
						Project(item, null, propertyInfo);
					}
				}
			}
		}
	}

	private static Material Mat (Color color)
	{
		Material material = new Material (Shader.Find ("DjUnlit/Texture") ?? Shader.Find ("Unlit/Texture"));
		Texture2D texture2D = new Texture2D (1, 1, TextureFormat.ARGB32, false);
        textures.Add(texture2D);
		texture2D.SetPixel (0, 0, color);
		texture2D.Apply ();
		material.mainTexture = texture2D;
		return material;
	}

	private static void Materials ()
	{
		if (!(stone != null)) {
			stone = Mat (new Color (0.38f, 0.43f, 0.46f));
			blue = Mat (new Color (0.23f, 0.57f, 0.77f));
			red = Mat (new Color (0.74f, 0.3f, 0.23f));
			wood = Mat (new Color (0.43f, 0.3f, 0.19f));
			road = Mat (new Color (0.64f, 0.63f, 0.54f));
		}
	}

	private static GameObject Shape (Transform parent, string name, PrimitiveType type, Vector3 at, Vector3 size, Material material, bool collider)
	{
		GameObject gameObject = GameObject.CreatePrimitive (type);
		gameObject.name = name;
		gameObject.layer = 8;
		gameObject.transform.SetParent (parent, false);
		gameObject.transform.localPosition = at;
		gameObject.transform.localScale = size;
		gameObject.GetComponent<Renderer> ().sharedMaterial = material;
		if (!collider) {
			UnityEngine.Object.Destroy (gameObject.GetComponent<Collider> ());
		}
		return gameObject;
	}

 private static GameObject Shape(Transform parent, string name, PrimitiveType type, Vector3 at, Vector3 size, Material material) {
  return Shape(parent, name, type, at, size, material, false);
 }
	private static Transform Node (string path, Vector3 at)
	{
		string[] array = path.Split ('/');
        if (array.Length < 2 || array[0] != root.name) throw new InvalidOperationException("Guild scene node must name a child of the battlefield");
		Transform transform = root.transform;
		for (int i = 1; i < array.Length; i++) {
			Transform transform2 = transform.Find (array [i]);
			if (transform2 == null) {
				GameObject gameObject = new GameObject (array [i]);
				gameObject.layer = 8;
				gameObject.transform.SetParent (transform, false);
				transform2 = gameObject.transform;
			}
			transform = transform2;
		}
		transform.position = Ground (at);
		return transform;
	}

	private static void Tower (string path, Vector3 at, int camp)
	{
		Transform parent = Node (path, at);
		Shape (parent, "plinth", PrimitiveType.Cube, new Vector3 (0f, 0.4f, 0f), new Vector3 (3f, 0.8f, 3f), stone);
		Shape (parent, "column", PrimitiveType.Cylinder, new Vector3 (0f, 2.2f, 0f), new Vector3 (1.7f, 1.6f, 1.7f), (camp == 0) ? blue : red);
		Shape (parent, "cap", PrimitiveType.Cube, new Vector3 (0f, 3.9f, 0f), new Vector3 (2.6f, 0.5f, 2.6f), stone);
	}

	private static void Build ()
	{
		Materials ();
		root = new GameObject ("map_battle_001");
		root.layer = 8;
        root.AddComponent<OwnedSceneLifetime>();
		GameObject ground = Shape (root.transform, "BattlefieldGround", PrimitiveType.Cube, new Vector3 (40f, -0.25f, 0f), new Vector3 (155f, 0.5f, 108f), stone, true);
        ground.layer = 11; // Static Scene terrain must stay outside the original Actor selection mask.
		Shape (root.transform, "MainRoad", PrimitiveType.Cube, new Vector3 (38f, 0.01f, 0f), new Vector3 (130f, 0.04f, 13f), road);
		Shape (root.transform, "UpperRoad", PrimitiveType.Cube, new Vector3 (38f, 0.01f, 25f), new Vector3 (100f, 0.04f, 8f), road);
		Shape (root.transform, "LowerRoad", PrimitiveType.Cube, new Vector3 (38f, 0.01f, -25f), new Vector3 (100f, 0.04f, 8f), road);
		DictDataManager instance = DictDataManager.Instance;
		foreach (TableUnionFightBaseCamp cache in instance.tableUnionFightBaseCampDao.CacheList) {
			Tower (cache.CampUrl, Vec (cache.Position), cache.Camp);
			if (cache.DieEffectUrl != "") {
				Node (cache.CampUrl + "/" + cache.DieEffectUrl, Vec (cache.Position));
			}
			if (cache.NormalEffectUrl != "") {
				Node (cache.CampUrl + "/" + cache.NormalEffectUrl, Vec (cache.Position));
			}
		}
		foreach (TableUnionFightTower cache2 in instance.tableUnionFightTowerDao.CacheList) {
			Tower (cache2.ModelUrl, Vec (cache2.Position), cache2.Camp);
		}
		foreach (TableUnionFightFactory cache3 in instance.tableUnionFightFactoryDao.CacheList) {
			Transform parent = Node (cache3.ModelUrl, Vec (cache3.Position));
			Shape (parent, "Workshop", PrimitiveType.Cube, new Vector3 (0f, 1.6f, 0f), new Vector3 (4f, 3.2f, 4f), stone);
			Shape (parent, "Roof", PrimitiveType.Cube, new Vector3 (0f, 3.5f, 0f), new Vector3 (4.8f, 0.7f, 4.8f), (cache3.Camp == 0) ? blue : red);
		}
		foreach (TableUnionFightGate cache4 in instance.tableUnionFightGateDao.CacheList) {
			Transform transform = Node (cache4.GateModel, Vec (cache4.Position));
			int num = 0;
			string[] array = cache4.GateState.Split (';');
			for (int i = 0; i < array.Length; i++) {
				GameObject gameObject = new GameObject (array [i]);
				gameObject.transform.SetParent (transform, false);
				float num2 = Math.Max (0.2f, 4 - num);
				Shape (gameObject.transform, "Gate", PrimitiveType.Cube, new Vector3 (0f, num2 / 2f, 0f), new Vector3 (5f, num2, 0.6f), (cache4.Camp == 0) ? blue : red);
				num++;
			}
			transform.rotation = Quaternion.Euler (0f, (float)cache4.GateRotation, 0f);
		}
		foreach (TableUnionFightRevivePoint cache5 in instance.tableUnionFightRevivePointDao.CacheList) {
            // Original points 6/7 are hidden safety locations with no scene model.
            // Passing their empty path to Node would move the entire battlefield.
            if (string.IsNullOrEmpty(cache5.ModelUrl)) continue;
			Shape (Node (cache5.ModelUrl, Vec (cache5.Position)), "CapturePoint", PrimitiveType.Cylinder, new Vector3 (0f, 0.12f, 0f), new Vector3 (4f, 0.12f, 4f), (cache5.InitCamp == 0) ? blue : ((cache5.InitCamp == 1) ? red : road));
		}
		GameObject gameObject2 = new GameObject ("BattlefieldLight");
		gameObject2.transform.SetParent (root.transform, false);
		Light light = gameObject2.AddComponent<Light> ();
		light.type = LightType.Directional;
		light.intensity = 0.8f;
		gameObject2.transform.rotation = Quaternion.Euler (50f, 30f, 0f);
		RenderSettings.ambientLight = new Color (0.65f, 0.65f, 0.68f);
		RenderSettings.fog = false;
	}

 // One selected guard action per owned room; no timeout can create a battle result.
 private sealed class GuardAction {
  public int Generation; public int SceneGeneration; public RoomUnionFight Room;
  public long PlayerID; public string Session; public RoomCharactor Target;
  public RCAIGuard Guard; public RCAIUnionSelf AI; public long GUID;
  public Action Arrival; public int Phase; public bool Sent; public float SentAt;
 }
 private static GuardAction guardAction;
 private static int guardGeneration;
 private static bool GuardOwner(GuardAction action) {
  return action != null && object.ReferenceEquals(guardAction, action) && action.Generation == guardGeneration && action.SceneGeneration == sceneGeneration && Active && object.ReferenceEquals(action.Room,sceneRoom) && action.PlayerID == scenePlayerID && action.Session == sceneSession;
 }
 private static bool GuardAvailable(GuardAction action) {
  return action.Target != null && action.Guard != null && action.Target.ai == action.Guard && action.Guard.instData != null && action.Guard.instData.guid == action.GUID && (int)action.Guard.Camp != (int)NetUnionFightBattle.camp && (int)action.Guard.CurrState == 0;
 }
 private static void CancelGuard(bool navigation) {
  GuardAction previous = guardAction; guardAction = null; ++guardGeneration;
  if (navigation && previous != null && previous.AI != null) {
   try {
    FieldInfo callback = typeof(RCAIUnionSelf).GetField("arrivedCallback", Flags);
    if (previous.AI.owner != null && object.ReferenceEquals(callback.GetValue(previous.AI),previous.Arrival)) previous.AI.StopAutoGo();
   } catch (Exception) { } // Cleanup never replaces original scene/packet handling.
  }
 }
 public static bool TryEnemyGuardClick(RCUIUnionGuard click) {
  if (!Active) return false;
  RCAIGuard guard = click == null || click.owner == null ? null : click.owner.ai as RCAIGuard;
  if (guard == null || (int)guard.Camp == (int)NetUnionFightBattle.camp) {
   if (guardAction != null && guardAction.Phase == 0) CancelGuard(false);
   return false; // The original friendly 0.8 approach and Say callback stays intact.
  }
  RoomCharactor player = click.owner.charactorManager.GetSelfCharactor();
  RCAIUnionSelf ai = player == null ? null : player.ai as RCAIUnionSelf;
  if (ai == null || !ai.IdleState() || RoomUnionFight.IsOnHook || (int)NetUnionFightBattle.state != 2) return true;
  GuardAction previous = guardAction;
  if (previous != null && GuardOwner(previous)) {
   if (previous.Phase == 2) return true;
   if (previous.Phase == 1 && (Time.realtimeSinceStartup - previous.SentAt < 3f || !previous.AI.IdleState() || !GuardAvailable(previous))) return true;
   if (previous.Phase == 0 && previous.Target == click.owner && ai.AutoPathFinding) return true;
  }
  CancelGuard(true);
  if (guard.instData == null || guard.instData.guid <= 0 || (int)guard.CurrState != 0) return true;
  GuardAction action = new GuardAction { Generation = guardGeneration, SceneGeneration = sceneGeneration, Room = sceneRoom, PlayerID = scenePlayerID, Session = sceneSession, Target = click.owner, Guard = guard, AI = ai, GUID = guard.instData.guid };
  action.Arrival = delegate { GuardArrived(action); };
  guardAction = action;
  try { ai.AutoGoto(action.Target.transform.position, action.Arrival, .8f); }
  catch { CancelGuard(false); throw; }
  return true;
 }
 private static void GuardArrived(GuardAction action) {
  if (!GuardOwner(action) || action.Sent) return;
  if (!GuardAvailable(action) || !action.AI.IdleState() || RoomUnionFight.IsOnHook || (int)NetUnionFightBattle.state != 2) { CancelGuard(false); return; }
  Vector3 delta = action.Target.transform.position - action.AI.owner.transform.position;
  // AutoGoto uses float coordinates; absorb only endpoint rounding. Server retains its exact range gate.
  if (delta.x * delta.x + delta.z * delta.z > .8001f * .8001f) { CancelGuard(false); return; }
  RoomNetUnionFight net = action.Room.netManager as RoomNetUnionFight;
  if (net == null) { CancelGuard(false); return; }
  action.Sent = true; action.Phase = 1; action.SentAt = Time.realtimeSinceStartup;
  try { net.SendCSUFPlayerAskFight(action.GUID); }
  catch { CancelGuard(false); throw; } // Original sender feedback/exception propagation remains.
 }
 public static void GuardTick() {
  TickPlayerRequests();
  try {
   GuardAction action = guardAction;
   if (action == null) return;
   if (!GuardOwner(action) || action.Target == null || action.AI == null || action.Guard == null || (int)action.Guard.CurrState == 2) { CancelGuard(true); return; }
   if (action.Phase == 0 && (!action.AI.AutoPathFinding || !object.ReferenceEquals(typeof(RCAIUnionSelf).GetField("arrivedCallback",Flags).GetValue(action.AI),action.Arrival))) CancelGuard(false);
   if (action.Phase == 2 && action.AI.IdleState()) CancelGuard(false);
  } catch (Exception) { guardAction = null; ++guardGeneration; }
 }
 public static void ObservePlayerBegin(RoomNetUnionFight net, object value) {
  ObservePlayerAdmission(net,value,true);
  try {
   GuardAction action = guardAction; SCUFPlayerFightBegin frame = value as SCUFPlayerFightBegin;
   if (!GuardOwner(action) || frame == null || !object.ReferenceEquals(action.Room.netManager,net) || frame.guid != action.PlayerID) return;
   if (frame.targetGuid == action.GUID && action.Sent) action.Phase = 2;
   else CancelGuard(false);
  } catch (Exception) { guardAction = null; ++guardGeneration; }
 }
 public static void ObservePlayerEnd(RoomNetUnionFight net, object value) {
  ObservePlayerAdmission(net,value,false);
  try {
   GuardAction action = guardAction; SCUFPlayerFightEnd frame = value as SCUFPlayerFightEnd;
   if (GuardOwner(action) && action.Phase == 2 && frame != null && object.ReferenceEquals(action.Room.netManager,net) && frame.guid == action.PlayerID) CancelGuard(false);
  } catch (Exception) { guardAction = null; ++guardGeneration; }
 }
 private static readonly PlayerRequestGate playerRequests = new PlayerRequestGate();
 private static float nextPlayerRequestCheck;
 private static void ResetPlayerRequests() { playerRequests.Reset(); }
 private static void TickPlayerRequests() {
  if (!playerRequests.HasIntent || Time.realtimeSinceStartup < nextPlayerRequestCheck) return;
  nextPlayerRequestCheck = Time.realtimeSinceStartup + .25f;
  if (!Active || !NetAPI.SocketIsConnected()) { ResetPlayerRequests(); return; }
  RCAIUnionActor self = SelfPlayerActor();
  if (self == null || self.instPlayer == null || (int)self.CurrState == 3 || (int)self.CurrState == 8) { ResetPlayerRequests(); return; }
  playerRequests.ObserveFightState(sceneGeneration,self.instPlayer.deadNum,(int)self.CurrState == 1 || (int)self.CurrState == 6);
  long targetID = playerRequests.Target;
  if (targetID <= 0) return;
  RoomCharactor target = sceneRoom.charactorManager.GetCharactor(targetID.ToString(CultureInfo.InvariantCulture));
  float range = (float)DictDataManager.Instance.tableGameConfigureDao.currentData.UnionFightArriveOther;
  Vector3 delta = target == null ? Vector3.zero : target.transform.position - self.owner.transform.position;
  if (target == null || delta.x * delta.x + delta.z * delta.z > range * range)
   playerRequests.OutsideRange(sceneGeneration,self.instPlayer.deadNum,targetID,Time.realtimeSinceStartup);
 }
 private static RCAIUnionActor SelfPlayerActor() {
  RoomCharactor self = sceneRoom == null ? null : sceneRoom.charactorManager.GetSelfCharactor();
  return self == null ? null : self.ai as RCAIUnionActor;
 }
 private static bool PlayerStateAvailable(RCAIUnionActor actor) {
  if (actor == null || actor.instPlayer == null) return false;
  int state = (int)actor.CurrState;
  return state != 1 && state != 3 && state != 6 && state != 8;
 }
 // Only the original ordinary-player scanner calls this adapter. Its original
 // timer and candidate cleanup still run, and guard sends keep their own path.
 public static void SendScannedPlayerAsk(RoomNetUnionFight net, long targetID, RoomUnionFightCheckPvp scan) {
  if (AllowScannedPlayerAsk(net,targetID,scan)) {
   SyncArrivalPosition(sceneRoom.charactorManager.GetSelfCharactor());
   net.SendCSUFPlayerAskFight(targetID);
  }
 }
 private static bool AllowScannedPlayerAsk(RoomNetUnionFight net, long targetID, RoomUnionFightCheckPvp scan) {
  if (!Active) { ResetPlayerRequests(); return root == null; }
  if (scan == null || !object.ReferenceEquals(scan.roomBase,sceneRoom) || !object.ReferenceEquals(scan.net,net) || !object.ReferenceEquals(sceneRoom.netManager,net)) return false;
  if (!NetAPI.SocketIsConnected() || (int)NetUnionFightBattle.state != 2 || RoomUnionFight.IsOnHook) { ResetPlayerRequests(); return false; }
  RoomCharactor target;
  RCAIUnionActor self = SelfPlayerActor();
  if (self == null || self.instPlayer == null || self.instPlayer.guid != scenePlayerID) { ResetPlayerRequests(); return false; }
  if ((int)self.CurrState == 3 || (int)self.CurrState == 8) { ResetPlayerRequests(); return false; }
  if (!PlayerStateAvailable(self) || !scan.rivalCharactors.TryGetValue(targetID,out target) || target == null || (int)target.type != 20) return false;
  RCAIUnionActor rival = target.ai as RCAIUnionActor;
  if (!PlayerStateAvailable(rival) || rival.instPlayer.guid != targetID || rival.Camp == self.Camp) return false;
  float range = (float)DictDataManager.Instance.tableGameConfigureDao.currentData.UnionFightArriveOther;
  if (range <= .01f || float.IsNaN(range) || float.IsInfinity(range)) return false;
  Vector3 delta = target.transform.position - self.owner.transform.position;
  double now = Time.realtimeSinceStartup;
  // A small inward margin keeps float coordinates inside the unchanged server gate.
  float inside = range - .001f;
  if (delta.x * delta.x + delta.z * delta.z > inside * inside) {
   playerRequests.OutsideRange(sceneGeneration,self.instPlayer.deadNum,targetID,now); return false;
  }
  if (!playerRequests.TrySend(sceneGeneration,self.instPlayer.deadNum,targetID,rival.instPlayer.deadNum,now)) return false;
  return true;
 }
 public static void ObservePlayerClick(RCUIUnionActor click) {
  try {
   if (!Active || click == null || click.owner == null || (int)click.owner.type != 20) return;
   long target; RCAIUnionActor self = SelfPlayerActor();
   if (self != null && self.instPlayer != null && long.TryParse(click.owner.name,out target))
    playerRequests.ExplicitClick(sceneGeneration,self.instPlayer.deadNum,target,Time.realtimeSinceStartup);
  } catch (Exception) { } // Never interrupt the original selection/navigation action.
 }
 private static void ObservePlayerAdmission(RoomNetUnionFight net, object value, bool begin) {
  try {
   if (!Active || !object.ReferenceEquals(sceneRoom.netManager,net)) return;
   RCAIUnionActor self = SelfPlayerActor();
   if (self == null || self.instPlayer == null) return;
   if (begin) {
    SCUFPlayerFightBegin frame = value as SCUFPlayerFightBegin;
    if (frame != null && frame.guid == scenePlayerID) {
     playerRequests.ObserveBegin(sceneGeneration,self.instPlayer.deadNum);
     playerRequests.ObserveFightState(sceneGeneration,self.instPlayer.deadNum,(int)self.CurrState == 1 || (int)self.CurrState == 6);
    }
   } else {
    SCUFPlayerFightEnd frame = value as SCUFPlayerFightEnd;
    if (frame != null && frame.guid == scenePlayerID) playerRequests.ObserveEnd(sceneGeneration,self.instPlayer.deadNum);
   }
  } catch (Exception) { ResetPlayerRequests(); }
 }
 public static bool CanLoad() { return RoomManager.curSceneType == SceneType.UNIONFIGHT; }
 private static int loadGeneration;
 private static WWW pending;
 private static string SessionHex() {
  byte[] key = StaticData.sessionKey;
  if (key == null || key.Length != 16) return null;
  return BitConverter.ToString(key).Replace("-", "").ToLowerInvariant();
 }
 private static bool Current(RoomUnionFight room, long id, string session, int generation) {
  return generation == loadGeneration && room != null && object.ReferenceEquals(RoomManager.currentRoom,room) && CanLoad() && StaticData.playerData != null && StaticData.playerData.playerId == id && SessionHex() == session;
 }
 public static bool TryPrepareEntry(UnionFightSignUpItem item) {
  if (GameConfig.Instance.IsAssetFull || item == null || item.gotoFightSceneBtn == null || !item.gotoFightSceneBtn.activeInHierarchy || StaticData.playerData == null || SessionHex() == null) return false;
  GuildEntryProbe probe = item.gotoFightSceneBtn.GetComponent<GuildEntryProbe>();
  if (probe == null) probe = item.gotoFightSceneBtn.AddComponent<GuildEntryProbe>();
  probe.Begin(item);
  return true;
 }
 internal static IEnumerator ProbeEntry(GuildEntryProbe probe, UnionFightSignUpItem item) {
  long id = StaticData.playerData.playerId;
  string session = SessionHex();
  object room = RoomManager.currentRoom;
  WWW request = RequestMap(id, session, "/guild-war/map-entry");
  probe.Request = request;
  if (request == null) {
   probe.Complete(); BoxManager.CreateNetError("公会战场景读取失败，请稍后重试"); yield break;
  }
  float deadline = Time.realtimeSinceStartup + 3f;
  while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
  bool current = probe != null && probe.gameObject.activeInHierarchy && object.ReferenceEquals(probe.Item,item) && object.ReferenceEquals(RoomManager.currentRoom,room) && StaticData.playerData != null && StaticData.playerData.playerId == id && SessionHex() == session;
  string projection = null;
  bool accepted = current && ReadProjection(request, out projection);
  // The probe grants no entry or reward. The original transfer still validates
  // enrollment and the following loader checks the actual frozen map again.
  bool compatible = accepted && projection == "re-flat-v1";
  probe.Complete();
  if (!current) yield break;
  if (!accepted) { BoxManager.CreateNetError("公会战场景读取失败，请稍后重试"); yield break; }
  if (!compatible) { BoxManager.CreatePopupTextBox(LanguageTool.GetTextNoUpdate(1046), 0f); yield break; }
  ManagerController.Instance.TrySceneTransfer("fight_union", Vector3.zero, "UIUnionFightRunMap", null, string.Empty);
 }
 private static WWW RequestMap(long id, string session) { return RequestMap(id, session, "/guild-war/map"); }
 private static WWW RequestMap(long id, string session, string path) {
  try {
   string origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority);
   Dictionary<string,string> headers = new Dictionary<string,string>(); headers.Add("Content-Type","application/json");
   string body = "{\"playerId\":" + id.ToString(CultureInfo.InvariantCulture) + ",\"sessionHex\":\"" + session + "\"}";
   return new WWW(origin + path,Encoding.UTF8.GetBytes(body),headers);
  } catch (Exception) { return null; }
 }
 private static bool ReadProjection(WWW request, out string projection) {
  projection = null;
  try {
   if (!request.isDone || !string.IsNullOrEmpty(request.error)) return false;
   JsonData response = JsonMapper.ToObject(request.text);
   if ((int)response["code"] != 0) return false;
   projection = (string)response["data"]["mapProjection"];
   return projection == "" || projection == "re-flat-v1";
  } catch (Exception) { return false; }
 }
 private static IEnumerator OriginalLoad(RoomUnionFight room) {
  Type type = typeof(RoomUnionFight).GetNestedType("<_WaitForLoadComplete>c__Iterator98", BindingFlags.NonPublic);
  if (type == null) throw new InvalidOperationException("Original guild loader iterator missing");
  object iterator = Activator.CreateInstance(type,true);
  type.GetField("<>f__this",Flags).SetValue(iterator,room);
  type.GetField("$PC",Flags).SetValue(iterator,0);
  return (IEnumerator)iterator;
 }
 private static bool PrepareProjection() {
  try { Prepare(); return true; }
  catch (Exception) { Release(root); BoxManager.CreateNetError("公会战地图加载失败，请重试"); return false; }
 }
 private static bool BuildScene() {
  try { Build(); return true; }
  catch (Exception) {
   GameObject failed = root; Release(failed); if (failed != null) UnityEngine.Object.Destroy(failed);
   BoxManager.CreateNetError("公会战地图加载失败，请重试"); return false;
  }
 }
 // Original pooled DrawCalls can still belong to the scene that created them.
 // Keep only those standalone UI objects alive across this owned map transition.
 private static bool PreserveDrawCalls(UnityEngine.SceneManagement.Scene previous) {
  BetterList<UIDrawCall> active = UIDrawCall.activeList, inactive = UIDrawCall.inactiveList;
  if (!CheckDrawCalls(active,previous,false) || !CheckDrawCalls(inactive,previous,false)) return false;
  return CheckDrawCalls(active,previous,true) && CheckDrawCalls(inactive,previous,true);
 }
 private static bool CheckDrawCalls(BetterList<UIDrawCall> list, UnityEngine.SceneManagement.Scene previous, bool move) {
  if (list == null) return true;
  if (list.size < 0 || list.size > 512 || (list.size > 0 && (list.buffer == null || list.buffer.Length < list.size))) return false;
  for (int i = 0; i < list.size; i++) {
   UIDrawCall call = list.buffer[i]; if (call == null) continue;
   GameObject item = call.gameObject;
   if (!item.scene.Equals(previous)) continue;
   if (item.transform.parent != null) return false;
   if (move) UnityEngine.Object.DontDestroyOnLoad(item);
  }
  return true;
 }
 public static IEnumerator LoadRoom(RoomUnionFight room) {
  int generation = ++loadGeneration;
  if (pending != null) { pending.Dispose(); pending = null; }
  if (!CanLoad() || room == null || StaticData.playerData == null) yield break;
  long id = StaticData.playerData.playerId; string session = SessionHex();
  if (id <= 0 || session == null) yield break;
  // RoomManager.Switch sets currentRoom only after GetRoom returns. The
  // original Init starts this coroutine inside GetRoom, so its first move
  // must yield once before checking the actual owner. Never assign the owner
  // here or accept a stale room/player/session after that binding point.
  yield return null;
  if (!Current(room,id,session,generation)) yield break;
  WWW request = RequestMap(id,session);
  if (request == null) { BoxManager.CreateNetError("公会战地图加载失败，请重试"); yield break; }
  pending = request;
  string projection = null;
  bool accepted = false;
  try {
   float deadline = Time.realtimeSinceStartup + 3f;
   while (Current(room,id,session,generation) && !request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
   if (!Current(room,id,session,generation)) yield break;
   accepted = ReadProjection(request,out projection);
  } finally { if (object.ReferenceEquals(pending,request)) pending = null; request.Dispose(); }
  if (!accepted) { BoxManager.CreateNetError("公会战地图加载失败，请重试"); yield break; }
  if (!Current(room,id,session,generation)) yield break;
  if (projection == "") {
   // A previous procedural scene cannot masquerade as the legacy bundle scene.
   if (root != null) {
    GameObject previousRoot = root; Release(previousRoot); UnityEngine.Object.Destroy(previousRoot);
    UnityEngine.SceneManagement.Scene previousFlat = SceneManager.GetActiveScene();
    if (previousFlat.name == "map_battle_001") {
     SceneManager.SetActiveScene(SceneManager.CreateScene("HYJSGuildLegacyLoading"));
     SceneManager.UnloadScene(previousFlat.name); yield return null;
    }
   }
   if (Current(room,id,session,generation)) yield return room.StartCoroutine(OriginalLoad(room));
   yield break;
  }
  if (root != null) { GameObject previousRoot = root; Release(previousRoot); UnityEngine.Object.Destroy(previousRoot); }
  sceneRoom = room; scenePlayerID = id; sceneSession = session; sceneGeneration = generation;
  if (!PrepareProjection()) yield break;
  UnityEngine.SceneManagement.Scene previous = SceneManager.GetActiveScene();
  if (previous.name != "map_battle_001") {
   if (!PreserveDrawCalls(previous)) { BoxManager.CreateNetError("公会战界面加载失败，请重试"); yield break; }
   SceneManager.SetActiveScene(SceneManager.CreateScene("map_battle_001"));
   if (!BuildScene()) yield break;
   if (previous.name != "DontDestroyOnLoad") SceneManager.UnloadScene(previous.name);
   typeof(RoomUnionFight).GetField("isPlayedEnterScene", Flags | BindingFlags.Static).SetValue(room, false);
   yield return null;
  } else if (root == null && !BuildScene()) yield break;
  if (!Current(room,id,session,generation)) { if (sceneGeneration == generation && root != null) { GameObject obsolete = root; Release(obsolete); UnityEngine.Object.Destroy(obsolete); } yield break; }
  particleDeadline = Time.realtimeSinceStartup + 15f;
  IEnumerator routine = (IEnumerator)typeof(RoomUnionFight).GetMethod("_RegistParticle", Flags).Invoke(room, null);
  yield return room.StartCoroutine(routine);
  if (!Active || !Current(room,id,session,generation)) {
   if (sceneGeneration == generation && root != null) { GameObject obsolete = root; Release(obsolete); UnityEngine.Object.Destroy(obsolete); }
   yield break;
  }
  typeof(RoomUnionFight).GetMethod("LoadFixedTransferPoint", Flags).Invoke(room, null);
  typeof(RoomUnionFight).GetMethod("OnLoadComplete", Flags).Invoke(room, null);
 }
 public static IEnumerator RegisterParticle(RoomUnionFight room, string path) {
  if (!Active || string.IsNullOrEmpty(path)) yield break;
  if (!AssetBundleManager.Instance.IsAssetExist(path)) yield break;
  AssetBundleManager.Instance.LoadAssetBundle(path);
  float deadline = Math.Min(Time.realtimeSinceStartup + 15f, particleDeadline);
  while (Active && !AssetBundleManager.Instance.IsLoadedAssetBundle(path) && Time.realtimeSinceStartup < deadline) yield return null;
  if (Active && AssetBundleManager.Instance.IsLoadedAssetBundle(path)) GameObjectPoolManager.Instance.Regist(path, CommonMethod.GetAssetNameFromAssetBundleName(path));
 }

 public static float ApproachDistance(float range) {
  // Move one centimetre inside the server's unchanged range to avoid float
  // endpoints landing just outside it after the original MoveTowards call.
  return Active && range > 0f && !float.IsNaN(range) && !float.IsInfinity(range) ? Mathf.Max(0f,range - .01f) : range;
 }
 private static readonly FieldInfo MoveCommitTime = typeof(RoomNetBase).GetField("lastCommitTime",Flags);
 private static readonly FieldInfo MoveLastPosition = typeof(RoomNetBase).GetField("lastPosition",Flags);
 private static readonly MethodInfo MoveSync = typeof(RoomNetUnionFight).GetMethod("SolveSyncSelfToServer",Flags);
 private static void SyncArrivalPosition(RoomCharactor actor) {
  if (!Active || RoomUnionFight.IsOnHook || !NetAPI.SocketIsConnected() || actor == null ||
      !object.ReferenceEquals(sceneRoom.charactorManager.GetSelfCharactor(),actor)) return;
  RoomNetUnionFight net = sceneRoom.netManager as RoomNetUnionFight;
  if (net == null || MoveCommitTime == null || MoveLastPosition == null || MoveSync == null) return;
  // Flush a changed endpoint through the original CSMove sender before the
  // original arrival callback can ask for a fight on the same ordered socket.
  if ((Vector3)MoveLastPosition.GetValue(net) == actor.transform.position) return;
  MoveCommitTime.SetValue(net,Time.time - Mathf.Max(0f,net.sendTime) - .001f);
  MoveSync.Invoke(net,null);
 }
	public static bool TickMove (RCMoveNavigation move)
	{
		if (!Active) {
			return false;
		}
		Queue<Vector3> queue = (Queue<Vector3>)typeof(RCMoveNavigation).GetField ("corners", Flags).GetValue (move);
		move.owner.transform.position = Ground (move.owner.transform.position);
		if (queue.Count == 0) {
			return true;
		}
		Vector3 vector = Ground (queue.Peek ());
		Vector3 position = move.owner.transform.position;
		Vector3 vector2 = Vector3.MoveTowards (position, vector, move.GetCurrentSpeed () * Time.deltaTime);
		move.owner.transform.position = vector2;
		Vector3 forward = vector - position;
		if (forward.sqrMagnitude > 0.0001f) {
			move.owner.transform.rotation = Quaternion.RotateTowards (move.owner.transform.rotation, Quaternion.LookRotation (forward), 720f * Time.deltaTime);
		}
		if ((vector2 - vector).sqrMagnitude < 0.01f) {
			// AutoGoto's endpoint already includes the original interaction range.
			// Complete that final segment before its arrival callback can ask PK.
			move.owner.transform.position = vector;
			queue.Dequeue ();
			if (queue.Count == 0) SyncArrivalPosition(move.owner);
		}
		return true;
	}

	public static bool MoveTo (RCMoveNavigation move, Vector3 target)
	{
		if (!Active) {
			return false;
		}
		Queue<Vector3> obj = (Queue<Vector3>)typeof(RCMoveNavigation).GetField ("corners", Flags).GetValue (move);
		obj.Clear ();
		obj.Enqueue (Ground (target));
		return true;
	}

	public static bool Speed (RCMoveNavigation move, float speed)
	{
		if (!Active) {
			return false;
		}
		typeof(RCMoveNavigation).GetField ("moveSpeed", Flags).SetValue (move, speed);
		return true;
	}

	public static bool Sample (Vector3 source, out Vector3 target)
	{
		if (Active) {
			target = Ground (source);
			return true;
		}
		NavMeshHit hit;
		bool flag = NavMesh.SamplePosition (source, out hit, 100f, -1);
		target = (flag ? hit.position : source);
		return flag;
	}

	public static bool Reposition (RCMoveNavigation move, Vector3 target)
	{
		if (!Active) {
			return false;
		}
		move.owner.transform.position = Ground (target);
		return true;
	}

	public static bool Reset (RCMoveNavigation move)
	{
		return Reposition (move, move.owner.transform.position);
	}

	public static bool Siege (RCLODBase lod, string name)
	{
		if (!Active || (name != "battle_obj_801" && name != "battle_obj_802")) {
			return false;
		}
		if (AssetBundleManager.Instance.IsAssetExist("model/" + name)) return false;
        Materials ();
		if (lod.Model != null) {
			return true;
		}
		// The original chariot HP presentation reads a skinned body and its
		// first textured material. Keep that contract with the recovered camp
		// materials even when the missing model bundle uses procedural geometry.
		Material bodyMaterial = Resources.Load<Material>("shaders/" + name);
		if (bodyMaterial == null || bodyMaterial.mainTexture == null) {
			throw new InvalidOperationException("公会战冲车原阵营材质加载失败");
		}
		GameObject gameObject = new GameObject (name);
		gameObject.transform.SetParent (lod.owner.transform, false);
		GameObject body = Shape (gameObject.transform, "Cart", PrimitiveType.Cube, new Vector3 (0f, 1f, 0f), new Vector3 (2.3f, 1f, 3.7f), bodyMaterial);
		MeshFilter bodyFilter = body.GetComponent<MeshFilter>();
		Mesh bodyMesh = UnityEngine.Object.Instantiate(bodyFilter.sharedMesh) as Mesh;
		meshes.Add(bodyMesh);
		BoneWeight[] weights = new BoneWeight[bodyMesh.vertexCount];
		for (int i = 0; i < weights.Length; i++) { weights[i].boneIndex0 = 0; weights[i].weight0 = 1f; }
		bodyMesh.boneWeights = weights;
		bodyMesh.bindposes = new Matrix4x4[] { Matrix4x4.identity };
		MeshRenderer rigid = body.GetComponent<MeshRenderer>();
		rigid.enabled = false;
		UnityEngine.Object.Destroy(rigid);
		UnityEngine.Object.Destroy(bodyFilter);
		SkinnedMeshRenderer skin = body.AddComponent<SkinnedMeshRenderer>();
		skin.sharedMesh = bodyMesh;
		skin.bones = new Transform[] { body.transform };
		skin.rootBone = body.transform;
		skin.localBounds = bodyMesh.bounds;
		skin.sharedMaterial = bodyMaterial;
		Shape (gameObject.transform, "Ram", PrimitiveType.Cylinder, new Vector3 (0f, 1.3f, 1f), new Vector3 (0.55f, 2.2f, 0.55f), stone).transform.localRotation = Quaternion.Euler (90f, 0f, 0f);
		for (int i = -1; i <= 1; i += 2) {
			for (int j = -1; j <= 1; j += 2) {
				Shape (gameObject.transform, "Wheel", PrimitiveType.Cylinder, new Vector3 ((float)i * 1.2f, 0.6f, (float)j * 1.2f), new Vector3 (1.1f, 0.15f, 1.1f), stone).transform.localRotation = Quaternion.Euler (0f, 0f, 90f);
			}
		}
		typeof(RCLODBase).GetField ("model", Flags).SetValue (lod, gameObject);
		typeof(RCLODBase).GetField ("loadingModelName", Flags).SetValue (lod, null);
		return true;
	}

	public static bool MiniMap (MiniMapComponent component)
	{
		if (!Active) {
			return false;
		}
		if (map == null) {
			map = new Texture2D (256, 160, TextureFormat.ARGB32, false);
			for (int i = 0; i < 160; i++) {
				for (int j = 0; j < 256; j++) {
					bool flag = Math.Abs (i - 80) < 10 || Math.Abs (i - 40) < 6 || Math.Abs (i - 120) < 6;
					map.SetPixel (j, i, flag ? new Color (0.64f, 0.63f, 0.54f) : new Color (0.24f, 0.29f, 0.3f));
				}
			}
			map.Apply ();
		}
  UITexture texture = (UITexture)typeof(MiniMapComponent).GetField("mapTexture",Flags).GetValue(component);
  if (texture == null) return false;
  bool captured = false;
  foreach (MiniMapRestore restore in miniMaps) if (restore.Component == component) captured = true;
  if (!captured) miniMaps.Add(new MiniMapRestore { Component = component, Texture = texture, OriginalTexture = texture.mainTexture, Scale = typeof(MiniMapComponent).GetField("scale",Flags).GetValue(component), Offset = typeof(MiniMapComponent).GetField("offset",Flags).GetValue(component), Angle = typeof(MiniMapComponent).GetField("angle",Flags).GetValue(component) });
  texture.mainTexture = map;
		typeof(MiniMapComponent).GetField ("scale", Flags).SetValue (component, 2f);
		typeof(MiniMapComponent).GetField ("offset", Flags).SetValue (component, new Vector2 (-80f, 0f));
		typeof(MiniMapComponent).GetField ("angle", Flags).SetValue (component, 0f);
		typeof(MiniMapComponent).GetMethod("SetViewZoneSize",Flags).Invoke(component,null);
        return true;
	}
}

}
