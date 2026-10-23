using System;
using System.Collections;
using System.Collections.Generic;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.Recycle
{
    // Reuse the original checkbox, selected slots and confirm/decompose flow.
    public sealed class SelectionControl : MonoBehaviour
    {
        RecycleSelectPanel panel;
        WWW pending;
        int generation;
        bool reading;
        public const int Maximum = 100;

        internal static bool IsReading(RecycleSelectPanel target)
        {
            SelectionControl control = target == null ? null : target.GetComponent<SelectionControl>();
            return control != null && control.reading;
        }
        readonly List<UIButton> confirms = new List<UIButton>();
        readonly List<Collider> blockedItems = new List<Collider>();
        readonly List<bool> previousColliderState = new List<bool>();

        public static void Select(RecycleSelectPanel target)
        {
            if (target == null) return;
            SelectionControl control = target.GetComponent<SelectionControl>();
            if (control == null) control = target.gameObject.AddComponent<SelectionControl>();
            if (!HYJS.Recycle61.Flow.AllowSelection()) return;
            control.Begin(target);
        }

        UIToggle Toggle()
        {
            RecycleBreakupSelectPetPanel pet = panel as RecycleBreakupSelectPetPanel;
            if (pet != null) return pet.checkbox;
            RecycleBreakupSelectHeroPanel hero = panel as RecycleBreakupSelectHeroPanel;
            return hero == null ? null : hero.checkbox;
        }

        void SetConfirm(bool enabled)
        {
            if (confirms.Count == 0)
                foreach (UIButton button in panel.GetComponentsInChildren<UIButton>(true))
                    if (button.name == "ConfirmBtn") confirms.Add(button);
            foreach (UIButton button in confirms) if (button != null) button.isEnabled=enabled;
            // Keep manual row taps from changing the original cache during the asynchronous read.
            // The original checkbox remains usable to cancel this request.
            if (!enabled && blockedItems.Count == 0 && panel.simpleGrid != null)
                foreach (GameObject child in panel.simpleGrid.childs)
                {
                    if (child == null) continue;
                    foreach (Collider collider in child.GetComponentsInChildren<Collider>(true))
                    {
                        blockedItems.Add(collider); previousColliderState.Add(collider.enabled); collider.enabled=false;
                    }
                }
            if (enabled)
            {
                for (int index=0; index<blockedItems.Count; index++)
                    if (blockedItems[index] != null) blockedItems[index].enabled=previousColliderState[index];
                blockedItems.Clear(); previousColliderState.Clear();
            }
        }

        void Clear()
        {
            panel.ClearSelectCache();
            StaticData.ClearRecycleBreakupSelect();
            if (panel.simpleGrid != null)
                foreach (GameObject child in panel.simpleGrid.childs)
                {
                    if (child == null) continue;
                    RecycleSelectItemBase item=child.GetComponent<RecycleSelectItemBase>();
                    if (item != null) item.Select(false);
                }
        }

        void Begin(RecycleSelectPanel target)
        {
            generation++;
            StopAllCoroutines();
            if (pending != null) { pending.Dispose(); pending=null; }
            panel=target;
            reading=false; SetConfirm(true);
            UIToggle toggle=Toggle();
            if (toggle == null || !toggle.value) { Clear(); return; }
            if (StaticData.playerData == null || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return;
            reading=true; SetConfirm(false);
            StartCoroutine(ReadSelection(generation,StaticData.playerData.playerId,Session()));
        }

        static string Session() { return StaticData.sessionKey == null ? "" : BitConverter.ToString(StaticData.sessionKey).Replace("-","").ToLowerInvariant(); }

        bool Current(int current,long owner,string session)
        {
            return generation==current && panel != null && panel.gameObject.activeInHierarchy &&
                StaticData.playerData != null && StaticData.playerData.playerId==owner && Session()==session;
        }

        IEnumerator ReadSelection(int current,long owner,string session)
        {
            string kind=panel is RecycleBreakupSelectEquipPanel ? "equipment" : panel is RecycleBreakupSelectPetPanel ? "pet" : "hero";
            string origin=null;
            try { origin=new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); } catch(Exception) { }
            if(origin==null) { reading=false; SetConfirm(true); BoxManager.CreatePopupTextBox("连接未就绪，请重新勾选一键选择",0f); yield break; }
            string body="{\"playerId\":"+owner+",\"sessionHex\":\""+session+"\",\"kind\":\""+kind+"\",\"limit\":"+Maximum+"}";
            Dictionary<string,string> headers=new Dictionary<string,string>(); headers.Add("Content-Type","application/json");
            WWW request=new WWW(origin+"/recycle/selection",Encoding.UTF8.GetBytes(body),headers);
            pending=request;
            float deadline=Time.realtimeSinceStartup+20f;
            while(!request.isDone && Time.realtimeSinceStartup<deadline) yield return null;
            string message="一键选择失败，请重新勾选重试";
            try
            {
                if(Current(current,owner,session) && request.isDone && String.IsNullOrEmpty(request.error))
                {
                    JsonData reply=JsonMapper.ToObject(request.text);
                    if((int)reply["code"]==0)
                    {
                        JsonData guids=reply["data"]["guids"];
                        if(guids.Count<=Maximum)
                        {
                            // Validate the entire server selection before touching either original cache.
                            var visible = new Dictionary<string, RecycleSelectItemBase>();
                            var eligible = new HashSet<string>();
                            foreach (object candidate in panel.GetCanSelectItems())
                            {
                                var field = candidate == null ? null : candidate.GetType().GetField("guid");
                                string id = field == null ? null : field.GetValue(candidate) as string;
                                if (!String.IsNullOrEmpty(id)) eligible.Add(id);
                            }
                            foreach (GameObject child in panel.simpleGrid.childs)
                            {
                                if (child == null) continue;
                                RecycleSelectItemBase item = child.GetComponent<RecycleSelectItemBase>();
                                if (item == null) continue;
                                string id = item.GetItemGUID();
                                if (String.IsNullOrEmpty(id) || !eligible.Contains(id)) continue;
                                if (visible.ContainsKey(id)) throw new InvalidOperationException("Duplicate visible selection");
                                visible.Add(id, item);
                            }
                            var requested = new List<string>();
                            for (int index = 0; index < guids.Count; index++) requested.Add((string)guids[index]);
                            var selected = ValidateSelection(requested, visible.Keys, Maximum,
                                StaticData.recycleBreakupSelect == null ? 0 : StaticData.recycleBreakupSelect.Length);
                            // Keep both complete old caches until the entire reply has passed validation.
                            Clear();
                            // Original panel cache and original slot array commit only after complete validation.
                            for (int index = 0; index < selected.Count; index++)
                            {
                                string guid = selected[index];
                                StaticData.recycleBreakupSelect[index] = guid;
                                panel.AddSelectCache(guid);
                                visible[guid].Select(true);
                            }
                            // Keep the confirmed selection visible without changing any candidate or slot.
                            if (selected.Count > 0)
                            {
                                var ordered = new List<GameObject>();
                                foreach (GameObject child in panel.simpleGrid.childs)
                                {
                                    if (child == null) continue;
                                    RecycleSelectItemBase item = child.GetComponent<RecycleSelectItemBase>();
                                    if (item != null && selected.Contains(item.GetItemGUID())) ordered.Add(child);
                                }
                                foreach (GameObject child in panel.simpleGrid.childs)
                                {
                                    if (child == null) { ordered.Add(child); continue; }
                                    RecycleSelectItemBase item = child.GetComponent<RecycleSelectItemBase>();
                                    if (item == null || !selected.Contains(item.GetItemGUID())) ordered.Add(child);
                                }
                                panel.simpleGrid.childs.Clear();
                                panel.simpleGrid.childs.AddRange(ordered);
                                panel.simpleGrid.Reposition();
                                UIScrollView scroll = panel.scrollView == null ? null : panel.scrollView.GetComponent<UIScrollView>();
                                if (scroll != null) scroll.ResetPosition();
                            }
                            if(guids.Count==0) message="没有可分解的闲置材料";
                            else message="";
                        }
                    }
                    else if(((IDictionary)reply).Contains("msg")) message=(string)reply["msg"];
                    else if(((IDictionary)reply).Contains("message")) message=(string)reply["message"];
                }
            }
            catch(Exception) { }
            request.Dispose(); if(pending==request) pending=null;
            if(!Current(current,owner,session)) yield break;
            reading=false; SetConfirm(true);
            // Read-only selection failures leave the previous complete selection untouched.
            if(!String.IsNullOrEmpty(message)) BoxManager.CreatePopupTextBox(message,0f);
        }

        internal static List<string> ValidateSelection(List<string> requested, ICollection<string> visible, int maximum, int slots)
        {
            if (maximum < 1 || maximum > Maximum || slots < maximum || requested == null || requested.Count > maximum)
                throw new InvalidOperationException("Invalid original selection slots");
            var result = new List<string>(); var unique = new HashSet<string>();
            foreach (string guid in requested)
            {
                if (String.IsNullOrEmpty(guid) || !unique.Add(guid) || !visible.Contains(guid))
                    throw new InvalidOperationException("Selection list changed");
                result.Add(guid);
            }
            return result;
        }

        void OnDisable() { reading=false; generation++; StopAllCoroutines(); if(pending!=null) {pending.Dispose();pending=null;} if(panel!=null) SetConfirm(true); }
        void OnDestroy() { if(pending!=null) pending.Dispose(); }
    }
}
