// Original TownPage master availability overlay. No scene transfer or battle hook.
// Unity 5.3 / C# 3; authority is the authenticated server state endpoint.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.GuildBattle {
public static class GuildWarAvailability {
 internal const BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;

 // Prefix UI.TownPage.Update: leave the complete native Update body intact.
 public static void EnsureTownAvailability(TownPage page) {
  if (page == null) return;
  GuildWarAvailabilityOwner owner = page.GetComponent<GuildWarAvailabilityOwner>();
  if (owner == null) owner = page.gameObject.AddComponent<GuildWarAvailabilityOwner>();
  owner.Bind(page);
 }

 // Prefix both original town entry methods: false returns before their body.
 public static bool CanEnterFromTown(TownPage page) {
  if (page == null) return false;
  EnsureTownAvailability(page);
  GuildWarAvailabilityOwner owner = page.GetComponent<GuildWarAvailabilityOwner>();
  if (owner != null && owner.CanEnter()) return true;
  if (owner != null && owner.IsKnownClosed()) BoxManager.CreateNetError("公会战暂未开放");
  else BoxManager.CreateNetError("公会战状态读取失败，请稍后重试");
  return false;
 }

 internal static bool Identity(out long player, out string session, out string origin) {
  player = 0; session = null; origin = null;
  try {
   if (StaticData.playerData == null || StaticData.playerData.playerId <= 0) return false;
   byte[] key = StaticData.sessionKey;
   if (key == null || key.Length != 16) return false;
   Uri uri = new Uri(ServerConfig.GetServerUrl());
   if (uri.Scheme != Uri.UriSchemeHttp && uri.Scheme != Uri.UriSchemeHttps) return false;
   player = StaticData.playerData.playerId;
   session = BitConverter.ToString(key).Replace("-", "").ToLowerInvariant();
   origin = uri.GetLeftPart(UriPartial.Authority);
   return true;
  } catch (Exception) { return false; }
 }
}

// Owned by the TownPage object, independent of procedural battle scene lifetime.
public sealed class GuildWarAvailabilityOwner : MonoBehaviour {
 private TownPage page;
 private GameObject button;
 private long playerID;
 private string sessionHex;
 private string serverOrigin;
 private bool? available;
 private float verifiedAt;
 private float nextPollAt;
 private WWW request;
 private bool busy;
 private bool hiddenByOwner;
 private bool originalActiveSelf;
 private int generation;

 internal void Bind(TownPage value) {
  if (page != value) { Cancel(); page = value; button = null; hiddenByOwner = false; }
  SynchronizeIdentity();
  if (button == null && page != null) {
   try {
    FieldInfo field = typeof(TownPage).GetField("unionFightBtn", GuildWarAvailability.Flags);
    if (field != null) button = field.GetValue(page) as GameObject;
   } catch (Exception) { button = null; }
  }
  RefreshIfDue();
 }

 private bool SynchronizeIdentity() {
  long id; string session; string origin;
  bool valid = GuildWarAvailability.Identity(out id, out session, out origin);
  if (!valid || id != playerID || session != sessionHex || origin != serverOrigin) {
   Cancel();
   playerID = valid ? id : 0;
   sessionHex = valid ? session : null;
   serverOrigin = valid ? origin : null;
   nextPollAt = 0f;
  }
  return valid;
 }

 private bool Fresh() {
  float age = Time.realtimeSinceStartup - verifiedAt;
  return available.HasValue && age >= 0f && age < 5f;
 }
 internal bool CanEnter() {
  if (!SynchronizeIdentity()) return false;
  RefreshIfDue();
  return Fresh() && available.Value;
 }
 internal bool IsKnownClosed() { return Fresh() && !available.Value; }

 private void RefreshIfDue() {
  if (busy || playerID <= 0 || sessionHex == null || serverOrigin == null || !isActiveAndEnabled) return;
  float now = Time.realtimeSinceStartup;
  if (now < nextPollAt) return;
  // Include failures in the five-second poll budget; a click cannot retry-loop.
  nextPollAt = now + 5f;
  busy = true;
  StartCoroutine(ReadState(generation, playerID, sessionHex, serverOrigin));
 }

 private IEnumerator ReadState(int token, long id, string session, string origin) {
  WWW owned = null;
  try {
   Dictionary<string,string> headers = new Dictionary<string,string>();
   headers.Add("Content-Type", "application/json");
   headers.Add("Cache-Control", "no-store");
   string body = "{\"playerId\":" + id.ToString(CultureInfo.InvariantCulture) + ",\"sessionHex\":\"" + session + "\"}";
   owned = new WWW(origin + "/guild-war/state", Encoding.UTF8.GetBytes(body), headers);
  } catch (Exception) { owned = null; }
  request = owned;
  float deadline = Time.realtimeSinceStartup + 3f;
  while (owned != null && !owned.isDone && Time.realtimeSinceStartup < deadline) {
   if (!SameIdentity(token, id, session, origin)) { Complete(owned); yield break; }
   yield return null;
  }
  if (SameIdentity(token, id, session, origin)) {
   bool enabled;
   if (Parse(owned, out enabled)) { available = enabled; verifiedAt = Time.realtimeSinceStartup; }
   else { available = null; verifiedAt = 0f; }
  }
  Complete(owned);
 }

 private bool SameIdentity(int token, long id, string session, string origin) {
  long current; string key; string authority;
  return token == generation && page != null && isActiveAndEnabled &&
   GuildWarAvailability.Identity(out current, out key, out authority) &&
   current == id && key == session && authority == origin;
 }
 private static bool Parse(WWW value, out bool enabled) {
  enabled = false;
  try {
   if (value == null || !value.isDone || !string.IsNullOrEmpty(value.error)) return false;
   JsonData result = JsonMapper.ToObject(value.text);
   if ((int)result["code"] != 0) return false;
   JsonData flag = result["data"]["warEnabled"];
   if (!flag.IsBoolean) return false;
   enabled = (bool)flag;
   return true;
  } catch (Exception) { return false; }
 }
 private void Complete(WWW owned) {
  if (owned != null) owned.Dispose();
  if (object.ReferenceEquals(request, owned)) { request = null; busy = false; }
 }
 private void Cancel() {
  generation++;
  StopAllCoroutines();
  if (request != null) request.Dispose();
  request = null; busy = false; available = null; verifiedAt = 0f;
 }

 private void LateUpdate() {
  if (page == null) return;
  SynchronizeIdentity();
  RefreshIfDue();
  if (button == null) return;
  if (!Fresh() || !available.Value) {
   if (button.activeSelf) {
    originalActiveSelf = button.activeSelf;
    hiddenByOwner = true;
    button.SetActive(false);
   }
   return;
  }
  RestoreNativeDecision();
 }

 private void RestoreNativeDecision() {
  if (!hiddenByOwner) return;
  // The overlay's old activeSelf cannot prove current native eligibility.
  // Re-run the original preview decision instead of forcing the button on.
  if (button.activeSelf) { hiddenByOwner = false; return; }
  if (!originalActiveSelf || page == null) return;
  try {
   FieldInfo preview = typeof(TownPage).GetField("scUFInfoPreView", GuildWarAvailability.Flags);
   FieldInfo count = typeof(TownPage).GetField("fightCount", GuildWarAvailability.Flags);
   MethodInfo refresh = typeof(TownPage).GetMethod("MsgUFCheckState", GuildWarAvailability.Flags);
   if (preview == null || count == null || refresh == null) return;
   object value = preview.GetValue(page);
   if (value == null) return;
   refresh.Invoke(page, new object[] { value, count.GetValue(page) });
   hiddenByOwner = false;
  } catch (Exception) { if (button != null) button.SetActive(false); } // Keep hidden until a genuine native preview decides.
 }
 private void OnDisable() { Cancel(); }
 private void OnDestroy() { Cancel(); page = null; button = null; }
}
}
