using System.Collections.Generic;
using ProtoBuf.Message;
using NetWork.Packets;
using UI;
using UnityEngine;

namespace HYJS.NewbieSign
{
 public static class NewbieSignGuard
 {
  public static bool CanOpen(Packet packet, bool success)
  {
   // Existing transport failure handling stays authoritative and never opens a page.
   if (!success) return false;
   SCLoginRewardInfo info = packet == null ? null : packet.kBody as SCLoginRewardInfo;
   if (info != null && info.isShow == 1 && info.items != null && info.items.Count > 0) return true;
   bool completed = info != null && info.items != null && info.items.Count > 0;
   if (completed) foreach (LoginRewardItem item in info.items)
    if (item == null || item.state != 2) { completed = false; break; }
   TownPage town = PageManager.Instance.currentPage as TownPage;
   if (town != null && town.tenDaySignBtn != null)
   {
    town.tenDaySignBtn.SetActive(false);
    ShowRight menu = town.GetComponentInChildren<ShowRight>();
    if (menu != null) menu.ResetIconPos(3);
   }
   BoxManager.CreatePopupTextBox(completed ? "本期萌新福利已全部领取。" : "当前萌新福利未开放或已结束。", 0f);
   return false;
  }

  public static bool CanRender(TenDaySignPage page, List<LoginRewardItem> items)
  {
   // Real claimed rows remain readable; this guard never filters a nonempty server list.
   if (items != null && items.Count > 0) return true;
   if (page == null) return false;
   if (page.simpleGrid != null && page.simpleGrid.childs != null)
   {
    foreach (GameObject child in page.simpleGrid.childs)
     if (child != null) { child.SetActive(false); Object.Destroy(child); }
    page.simpleGrid.childs.Clear();
    page.simpleGrid.Reposition();
   }
   if (page.itemPref != null) page.itemPref.SetActive(false);
   BoxManager.CreatePopupTextBox("当前没有可显示的萌新福利，请稍后重新打开。", 0f);
   page.Close();
   return false;
  }
 }
}
