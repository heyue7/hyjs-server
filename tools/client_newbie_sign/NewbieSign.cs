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
#if HYJS_NEWBIE_QA
   DiagnoseOpen(packet,success,"manual");
#endif
   // Existing transport failure handling stays authoritative and never opens a page.
   if (!success) return Decision(false);
   SCLoginRewardInfo info = packet == null ? null : packet.kBody as SCLoginRewardInfo;
   if (info != null && info.isShow == 1 && info.items != null && info.items.Count > 0) return Decision(true);
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
   return Decision(false);
  }

  public static bool CanRender(TenDaySignPage page, List<LoginRewardItem> items)
  {
#if HYJS_NEWBIE_QA
   DiagnosePage(page,items==null?-1:items.Count,"render-enter");
#endif
   // Real claimed rows remain readable; this guard never filters a nonempty server list.
   if (items != null && items.Count > 0) return Decision(true);
   if (page == null) return Decision(false);
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
   return Decision(false);
  }
  private static bool Decision(bool value)
  {
#if HYJS_NEWBIE_QA
   Write("decision="+value);
#endif
   return value;
  }
#if HYJS_NEWBIE_QA
  private static void Write(string text)
  {
   try { using(AndroidJavaClass log=new AndroidJavaClass("android.util.Log")) log.CallStatic<int>("i","HYJS.NewbieSignQA",text); }
   catch(System.Exception) { }
  }
  private static void DiagnoseOpen(Packet packet,bool success,string origin)
  {
   try { SCLoginRewardInfo info=packet==null?null:packet.kBody as SCLoginRewardInfo;
    Write(origin+" success="+success+" show="+(info==null?-1:info.isShow)+" count="+(info==null||info.items==null?-1:info.items.Count)); }
   catch(System.Exception){Write(origin+" inspect-error");}
  }
  private static void DiagnosePage(TenDaySignPage page,int count,string stage)
  {
   try { Write(stage+" count="+count+" page="+(page!=null)+" active="+(page!=null&&page.gameObject.activeSelf)+" grid="+(page!=null&&page.simpleGrid!=null)+" childs="+(page==null||page.simpleGrid==null||page.simpleGrid.childs==null?-1:page.simpleGrid.childs.Count)+" prefab="+(page!=null&&page.itemPref!=null)+" prefabActive="+(page!=null&&page.itemPref!=null&&page.itemPref.activeSelf)); }
   catch(System.Exception){Write(stage+" inspect-error");}
  }
  public static void DiagnoseAuto(Packet packet,bool success){DiagnoseOpen(packet,success,"auto");}
  public static void RenderExit(TenDaySignPage page){DiagnosePage(page,-1,"render-exit");}
#endif

 }
}
