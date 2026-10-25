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
public static class GuildBattleScene
{
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
		Shape (root.transform, "BattlefieldGround", PrimitiveType.Cube, new Vector3 (40f, -0.25f, 0f), new Vector3 (155f, 0.5f, 108f), stone, true);
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
  try {
   GuardAction action = guardAction;
   if (action == null) return;
   if (!GuardOwner(action) || action.Target == null || action.AI == null || action.Guard == null || (int)action.Guard.CurrState == 2) { CancelGuard(true); return; }
   if (action.Phase == 0 && (!action.AI.AutoPathFinding || !object.ReferenceEquals(typeof(RCAIUnionSelf).GetField("arrivedCallback",Flags).GetValue(action.AI),action.Arrival))) CancelGuard(false);
   if (action.Phase == 2 && action.AI.IdleState()) CancelGuard(false);
  } catch (Exception) { guardAction = null; ++guardGeneration; }
 }
 public static void ObservePlayerBegin(RoomNetUnionFight net, object value) {
  try {
   GuardAction action = guardAction; SCUFPlayerFightBegin frame = value as SCUFPlayerFightBegin;
   if (!GuardOwner(action) || frame == null || !object.ReferenceEquals(action.Room.netManager,net) || frame.guid != action.PlayerID) return;
   if (frame.targetGuid == action.GUID && action.Sent) action.Phase = 2;
   else CancelGuard(false);
  } catch (Exception) { guardAction = null; ++guardGeneration; }
 }
 public static void ObservePlayerEnd(RoomNetUnionFight net, object value) {
  try {
   GuardAction action = guardAction; SCUFPlayerFightEnd frame = value as SCUFPlayerFightEnd;
   if (GuardOwner(action) && action.Phase == 2 && frame != null && object.ReferenceEquals(action.Room.netManager,net) && frame.guid == action.PlayerID) CancelGuard(false);
  } catch (Exception) { guardAction = null; ++guardGeneration; }
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
 private static WWW RequestMap(long id, string session) {
  try {
   string origin = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority);
   Dictionary<string,string> headers = new Dictionary<string,string>(); headers.Add("Content-Type","application/json");
   string body = "{\"playerId\":" + id.ToString(CultureInfo.InvariantCulture) + ",\"sessionHex\":\"" + session + "\"}";
   return new WWW(origin + "/guild-war/map",Encoding.UTF8.GetBytes(body),headers);
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
 public static IEnumerator LoadRoom(RoomUnionFight room) {
  int generation = ++loadGeneration;
  if (pending != null) { pending.Dispose(); pending = null; }
  if (!CanLoad() || room == null || StaticData.playerData == null) yield break;
  long id = StaticData.playerData.playerId; string session = SessionHex();
  if (id <= 0 || session == null || !Current(room,id,session,generation)) yield break;
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
			queue.Dequeue ();
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
		GameObject gameObject = new GameObject (name);
		gameObject.transform.SetParent (lod.owner.transform, false);
		Shape (gameObject.transform, "Cart", PrimitiveType.Cube, new Vector3 (0f, 1f, 0f), new Vector3 (2.3f, 1f, 3.7f), wood);
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
