using System;
using System.Collections.Generic;
using System.Reflection;
using UI;
using Dict.Blo;
using Dict.DataModel;
using UnityEngine;

namespace HYJS.PetEquip48 {
public static class PetEquipFeedback {
 const BindingFlags Fields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
 static T Get<T>(object value, string name) { return (T)value.GetType().GetField(name, Fields).GetValue(value); }
 static void Set(object value, string name, object data) { value.GetType().GetField(name, Fields).SetValue(value, data); }
 static bool Donor(UIPet other, UIPet target) {
  if (other == null || other.guid == target.guid || other.id != target.id || other.owner != null || other.level != 1 || other.exp != 0 || other.breachlevel != 0 || other.intimacyLevel != 0 || other.intimacyExp != 0) return false;
  foreach (UIPetEquip equip in other.GetAllPetEquips()) {
   if (equip != null && (equip.strLevel != 0 || equip.advanLevel != 0 || equip.awakeLevel != 0)) return false;
  }
  return true;
 }
 static int Display(long value) { return (int)Math.Max(0L, Math.Min((long)int.MaxValue, value)); }
 static void Icon(PetEquipAwakeTab tab, string id, long have, int need) {
  GameObject go = UnityEngine.Object.Instantiate(tab.iconPref) as GameObject;
  tab.grid.AddChild(go.transform); go.transform.localPosition = Vector3.zero; go.transform.localScale = Vector3.one; go.SetActive(true);
  CommonIcon icon = go.GetComponent<CommonIcon>();
  icon.SetData(id, !id.StartsWith("pet_"), -1, null); icon.UpdateNumLabelWithColor(Display(have), need);
 }
 public static void AwakeCosts(PetEquipAwakeTab tab) {
  UIPetEquip equip = Get<UIPetEquip>(tab, "petEquip");
  UIPet target = StaticData.playerData.GetUIPet(equip.petGuid);
  TablePetEquipAwake row = DictPetEquipBlo.GetPetEquipAwake(equip.equipId, equip.awakeLevel + 1);
  tab.awakeContent.SetActive(row != null); tab.maxContent.SetActive(row == null);
  Set(tab, "isEnough", false); Set(tab, "isHaveZeroPet", false);
  if (row == null || target == null) return;
  NGUITools.DestroyChildren(tab.grid.transform);
  bool enough = true;
  foreach (string part in row.cost.Split(';')) {
   if (part.Length == 0) continue;
   string[] cost = part.Split(','); int need = int.Parse(cost[1]);
   long have = CommonMethod.GetObjectHaveNum(cost[0]);
   Icon(tab, cost[0], have, need); if (have < need) enough = false;
  }
  int donors = 0;
  if (row.selfCostNum > 0) {
   foreach (UIPet other in StaticData.playerData.InstPlayerData.GetAllPets()) if (Donor(other, target)) donors++;
   Icon(tab, target.id, donors, row.selfCostNum);
  }
  bool valid = row.selfCostNum >= 0 && donors >= row.selfCostNum;
  Set(tab, "isEnough", enough && valid); Set(tab, "isHaveZeroPet", valid);
  tab.grid.enabled = true; tab.grid.Reposition();
 }
 public static bool CanStrength(PetEquipStrengthTab tab, int number) {
  UIPetEquip equip = Get<UIPetEquip>(tab, "petEquip");
  PetEquipImprove page = Get<PetEquipImprove>(tab, "page");
  int cap = Math.Min(equip.maxStrLevel, page.GetCurrPet().level);
  TablePetEquipAdvance next = DictPetEquipBlo.GetPetEquipAdvance(equip.equipId, equip.advanLevel + 1);
  if (next != null) cap = Math.Min(cap, next.needStrengthLevel);
  if (equip.strLevel >= cap) { BoxManager.CreatePopupTextBox("已达当前强化上限，请提升精灵等级或进阶装备", 0f); return false; }
  int cost = number == 1 ? Get<int>(tab, "costOnce") : (int)tab.GetType().GetMethod("GetFiveCost", Fields).Invoke(tab, null);
  if (StaticData.playerData.coin < (long)cost) { BoxManager.CreatePopupTextBox(TableTextBlo.GetText("coin_lack"), 0f); return false; }
  return true;
 }
}
}
