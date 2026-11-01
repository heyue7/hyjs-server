using System;
using System.Collections.Generic;
using System.Reflection;
using Room;
using UI;
using UnityEngine;

namespace HYJS.DesertV4
{
    // A desert-only view of the already-running native map and accepted NPC
    // snapshot. It owns no gameplay state and does not send network requests.
    public sealed class MapViewer86 : MonoBehaviour
    {
        const string IconResource="HYJS.DesertV4.Evacuation.png";
        static readonly BindingFlags InstanceFlags=BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public;
        static readonly FieldInfo NativeInstance=typeof(MiniMapComponent).GetField("_instance",BindingFlags.Static|BindingFlags.NonPublic);
        static readonly FieldInfo NativeMap=typeof(MiniMapComponent).GetField("miniMap",InstanceFlags);
        static readonly FieldInfo NativeTexture=typeof(MiniMapComponent).GetField("mapTexture",InstanceFlags);
        static readonly FieldInfo NativeScale=typeof(MiniMapComponent).GetField("scale",InstanceFlags);
        static readonly FieldInfo NativeOffset=typeof(MiniMapComponent).GetField("offset",InstanceFlags);
        static MapViewer86 instance;
        static readonly TeamMap86.DiagnosticGate teamDiagnostics=new TeamMap86.DiagnosticGate();

        MiniMapComponent source;
        RoomBase ownerRoom;
        Texture2D evacuationIcon;
        Texture2D teamDot;
        readonly Dictionary<string,UITexture> smallMarkers=new Dictionary<string,UITexture>();
        readonly Dictionary<string,UITexture> largeExits=new Dictionary<string,UITexture>();
        readonly Dictionary<AMMiniMapEntity,UISprite> largeResources=new Dictionary<AMMiniMapEntity,UISprite>();
        sealed class BossMarker { public GameObject root; public UISprite icon; public UILabel label; }
        BossMarker smallBoss,largeBoss,fixedBossTime;
        AMMiniMap bossLeaseMap;
        AMMiniMapEntity bossLeaseEntity;
        string bossLeaseId;
        sealed class TeamLease { public AMMiniMap map; public AMMiniMapEntity entity; }
        readonly Dictionary<TeamMap86.Key,TeamLease> teamLeases=new Dictionary<TeamMap86.Key,TeamLease>();
        readonly Dictionary<TeamMap86.Key,UITexture> smallTeam=new Dictionary<TeamMap86.Key,UITexture>();
        readonly Dictionary<TeamMap86.Key,UITexture> largeTeam=new Dictionary<TeamMap86.Key,UITexture>();
        readonly List<TeamMap86.Visible> teamVisible=new List<TeamMap86.Visible>();
        readonly List<TeamMap86.Key> teamRemoved=new List<TeamMap86.Key>();
        GameObject view,mask,background,heading,closeButton,clipRoot,chromeRoot;
        Transform mapRoot;
        UIPanel panel,clip,chrome;
        UITexture mapImage;
        UISprite selfMarker;
        Page priorPage;
        GameObject priorBox;
        float viewport,fit,zoom;
        Vector2 pan;
        int screenWidth,screenHeight,dragFinger=-1,pinchA=-1,pinchB=-1;
        Vector2 lastDrag,lastMid;
        float lastDistance;
        float nextSmallRefresh,nextMarkerRefresh;
        bool mouseDragging,reportedMiniFailure,touchLease,restorePending;

        static bool Target()
        { return StaticData.fieldId=="field_003" && RoomManager.currentRoom is RoomField; }

        // Another overlay may be waiting to restore its input lease while
        // this independent map view still owns the screen.
        public static bool ViewOpen { get { return instance!=null && instance.view!=null; } }

        static MapViewer86 Ensure(MiniMapComponent value)
        {
            if (value==null || !Target()) return null;
            if (instance!=null && (instance.source!=value || instance.ownerRoom!=RoomManager.currentRoom)) ReleaseScene();
            if (instance==null)
            {
                instance=value.gameObject.AddComponent<MapViewer86>();
                instance.source=value; instance.ownerRoom=RoomManager.currentRoom;
            }
            return instance;
        }

        // Hooked ahead of the original OnClick; non-desert scenes always retain
        // the original method, including town and union-war handling.
        public static bool Open(object value)
        {
            if (!Target()) return false;
            try
            {
                MapViewer86 viewer=Ensure(value as MiniMapComponent);
                if (viewer==null) throw new InvalidOperationException("native mini map unavailable");
                if (viewer.restorePending)
                { viewer.RestoreInputWhenIdle(); if (viewer.restorePending) return true; }
                viewer.Show();
            }
            catch (Exception error)
            {
                if (instance!=null) instance.Close();
                Debug.LogWarning("Desert map open: "+error.GetType().Name+": "+error.Message);
                BoxManager.CreatePopupTextBox("地图正在准备，请稍后再试",0f);
            }
            return true;
        }

        // Called after the field runtime has accepted its snapshot. Discovery
        // never invokes the native singleton getter, which could create a map.
        public static void SyncMini()
        {
            if (!Target() || NativeInstance==null) return;
            try
            {
                MiniMapComponent value=NativeInstance.GetValue(null) as MiniMapComponent;
                MapViewer86 viewer=Ensure(value);
                if (viewer!=null && Time.unscaledTime>=viewer.nextSmallRefresh)
                { viewer.nextSmallRefresh=Time.unscaledTime+.25f; viewer.UpdateSmallBoss(); viewer.UpdateSmallMarkers(); viewer.UpdateSmallTeam(); }
            }
            catch (Exception error)
            {
                if (instance!=null)
                {
                    instance.ClearSmallBoss();
                    instance.ClearSmallTeam();
                    if (!instance.reportedMiniFailure)
                    { instance.reportedMiniFailure=true; Debug.LogWarning("Desert map markers: "+error.GetType().Name+": "+error.Message); }
                }
            }
        }

        public static void ReleaseScene()
        {
            if (instance==null) return;
            MapViewer86 old=instance; instance=null; old.Close(); old.ClearSmall();
            if (old.evacuationIcon!=null) { Destroy(old.evacuationIcon); old.evacuationIcon=null; }
            if (old.teamDot!=null) { Destroy(old.teamDot); old.teamDot=null; }
            Destroy(old);
        }
        void OnDestroy()
        {
            Close(); ClearSmall();
            if (evacuationIcon!=null) { Destroy(evacuationIcon); evacuationIcon=null; }
            if (teamDot!=null) { Destroy(teamDot); teamDot=null; }
            if (instance==this) instance=null;
        }

        AMMiniMap Map()
        {
            if (source==null || NativeMap==null) return null;
            return NativeMap.GetValue(source) as AMMiniMap;
        }
        float SceneScale()
        {
            if (NativeScale==null) throw new InvalidOperationException("native map scale unavailable");
            float result=(float)NativeScale.GetValue(source);
            if (Single.IsNaN(result) || Single.IsInfinity(result) || result<=0) throw new InvalidOperationException("invalid map scale");
            return result;
        }
        Vector2 SceneOffset()
        {
            if (NativeOffset==null) throw new InvalidOperationException("native map offset unavailable");
            Vector2 value=(Vector2)NativeOffset.GetValue(source);
            if (Single.IsNaN(value.x) || Single.IsNaN(value.y) || Single.IsInfinity(value.x) || Single.IsInfinity(value.y))
                throw new InvalidOperationException("invalid map offset");
            return value;
        }
        Texture NativeMapTexture()
        {
            UITexture texture=NativeTexture==null ? null : NativeTexture.GetValue(source) as UITexture;
            return texture==null ? null : texture.mainTexture;
        }
        Texture2D Icon()
        {
            if (evacuationIcon!=null) return evacuationIcon;
            using (System.IO.Stream stream=typeof(MapViewer86).Assembly.GetManifestResourceStream(IconResource))
            {
                if (stream==null || stream.Length<=0 || stream.Length>1048576) throw new InvalidOperationException("evacuation marker unavailable");
                byte[] bytes=new byte[(int)stream.Length]; int offset=0;
                while (offset<bytes.Length) { int count=stream.Read(bytes,offset,bytes.Length-offset); if (count<=0) throw new InvalidOperationException("incomplete marker"); offset+=count; }
                Texture2D image=new Texture2D(2,2,TextureFormat.ARGB32,false);
                if (!image.LoadImage(bytes)) { Destroy(image); throw new InvalidOperationException("invalid marker image"); }
                image.filterMode=FilterMode.Bilinear; evacuationIcon=image; return image;
            }
        }
        static void Parent(Transform child,Transform parent)
        { child.parent=parent; child.localPosition=Vector3.zero; child.localScale=Vector3.one; child.localRotation=Quaternion.identity; }
        static void SetLayer(Transform target,int layer)
        { target.gameObject.layer=layer; foreach (Transform child in target) SetLayer(child,layer); }
        static UITexture NewTexture(Transform parent,string name,Texture texture,int size,int depth)
        {
            GameObject go=new GameObject(name); go.layer=parent.gameObject.layer; Parent(go.transform,parent);
            UITexture widget=go.AddComponent<UITexture>(); widget.mainTexture=texture; widget.width=size; widget.height=size;
            widget.pivot=UIWidget.Pivot.Center; widget.depth=depth; return widget;
        }
        static void RemoveTexture(UITexture widget)
        { if (widget!=null) { widget.gameObject.SetActive(false); Destroy(widget.gameObject); } }
        void ClearSmall()
        { ClearSmallBoss(); ClearSmallTeam(); foreach (UITexture item in smallMarkers.Values) RemoveTexture(item); smallMarkers.Clear(); }

        UIAtlas BossAtlas()
        {
            UISprite frame=Need(source.transform,"small/ForegroundPanel/foreground2/frame").GetComponent<UISprite>();
            UIAtlas atlas=frame==null ? null : frame.atlas;
            if (atlas==null || atlas.GetSprite("UI_ditu_boss")==null) throw new InvalidOperationException("native boss map sprite unavailable");
            return atlas;
        }
        BossMarker NewBossMarker(Transform parent,int size,int depth)
        {
            GameObject root=new GameObject("DesertBoss"); root.layer=parent.gameObject.layer; Parent(root.transform,parent);
            try
            {
                GameObject iconObject=new GameObject("Icon"); iconObject.layer=root.layer; Parent(iconObject.transform,root.transform);
                UISprite icon=iconObject.AddComponent<UISprite>(); icon.atlas=BossAtlas(); icon.spriteName="UI_ditu_boss";
                icon.pivot=UIWidget.Pivot.Center; icon.width=size; icon.height=size; icon.depth=depth;
                GameObject prefab=Resources.Load("Pages/TownMapPage") as GameObject;
                UILabel template=prefab==null ? null : Need(prefab.transform,"top").GetComponentInChildren<UILabel>();
                if (template==null) throw new InvalidOperationException("native map label unavailable");
                GameObject textObject=Clone(template.transform,root.transform,"BossState");
                foreach (Transform child in textObject.transform) Destroy(child.gameObject);
                UILabel label=textObject.GetComponent<UILabel>();
                foreach (MonoBehaviour component in label.GetComponents<MonoBehaviour>())
                    if (component!=null && component.GetType().Name=="TextKit") DestroyImmediate(component);
                label.pivot=UIWidget.Pivot.Center; label.alignment=NGUIText.Alignment.Center;
                label.width=84; label.height=18; label.fontSize=size>21 ? 14 : 12; label.maxLineCount=1;
                label.overflowMethod=UILabel.Overflow.ShrinkContent; label.depth=depth+1; label.color=Color.white;
                label.effectStyle=UILabel.Effect.Outline; label.effectColor=Color.black;
                textObject.transform.localPosition=new Vector3(0,-size*.5f-8,0);
                return new BossMarker { root=root,icon=icon,label=label };
            }
            catch (Exception) { root.SetActive(false); Destroy(root); throw; }
        }
        static void SetBossState(BossMarker marker,string state,string label)
        {
            marker.icon.color=state=="active" ? Color.white : new Color(.58f,.58f,.58f,1f);
            marker.label.text=label;
        }
        static void RemoveBossMarker(BossMarker marker)
        { if (marker!=null && marker.root!=null) { marker.root.SetActive(false); Destroy(marker.root); } }
        void RestoreBossLease()
        {
            try
            {
                if (bossLeaseMap!=null && bossLeaseEntity!=null && bossLeaseMap.GetEntity(bossLeaseId)==bossLeaseEntity && !bossLeaseEntity.Show)
                    bossLeaseEntity.Show=true;
            }
            catch (Exception) { } // the native map may already be tearing down
            finally { bossLeaseMap=null; bossLeaseEntity=null; bossLeaseId=null; }
        }
        void ClearSmallBoss()
        {
            RestoreBossLease(); RemoveBossMarker(smallBoss); smallBoss=null;
            RemoveBossMarker(fixedBossTime); fixedBossTime=null;
        }
        void UpdateFixedBossTime(string state,string label)
        {
            if (state!="waiting" && state!="syncing")
            { RemoveBossMarker(fixedBossTime); fixedBossTime=null; return; }
            // The frame stays in the minimap foreground while OffsetRoot follows
            // world coordinates. Place the label inside the frame's lower edge.
            Transform transform=Need(source.transform,"small/ForegroundPanel/foreground2/frame");
            UISprite frame=transform.GetComponent<UISprite>();
            if (frame==null) throw new InvalidOperationException("native minimap frame unavailable");
            if (fixedBossTime!=null && (fixedBossTime.root==null || fixedBossTime.root.transform.parent!=transform))
            { RemoveBossMarker(fixedBossTime); fixedBossTime=null; }
            if (fixedBossTime==null)
            {
                fixedBossTime=NewBossMarker(transform,21,frame.depth+1);
                fixedBossTime.icon.gameObject.SetActive(false);
                fixedBossTime.label.transform.localPosition=Vector3.zero;
            }
            Vector3[] corners=frame.localCorners;
            fixedBossTime.root.transform.localPosition=new Vector3((corners[0].x+corners[2].x)*.5f,corners[0].y+11,0);
            fixedBossTime.label.width=Math.Max(1,Math.Min(100,(int)Math.Abs(corners[2].x-corners[0].x)-8));
            SetBossState(fixedBossTime,state,label);
        }
        void UpdateSmallBoss()
        {
            AMMiniMap map=Map(); string id,state,label; Vector3 position;
            if (map==null || !map.Initialized || map.OffsetRoot==null || !FieldBoss.MapMarkerLabel(ownerRoom,out id,out position,out state,out label))
            { ClearSmallBoss(); return; }
            if (smallBoss!=null && (smallBoss.root==null || smallBoss.root.transform.parent!=map.OffsetRoot)) ClearSmallBoss();
            AMMiniMapEntity entity=map.GetEntity(id);
            if (bossLeaseEntity!=null && (bossLeaseMap!=map || bossLeaseId!=id || bossLeaseEntity!=entity || entity.SpriteName!="UI_ditu_boss"))
                RestoreBossLease();
            if (entity!=null && entity.SpriteName=="UI_ditu_boss")
            {
                if (bossLeaseEntity==null && entity.Show) { bossLeaseMap=map; bossLeaseEntity=entity; bossLeaseId=id; }
                if (bossLeaseEntity==entity) entity.Show=false;
            }
            if (smallBoss==null) smallBoss=NewBossMarker(map.OffsetRoot,21,24);
            SetBossState(smallBoss,state,label);
            UpdateFixedBossTime(state,label);
            float scale=SceneScale(); smallBoss.root.transform.localPosition=new Vector3(position.x*scale,position.z*scale,0);
        }

        Texture2D TeamDot()
        {
            if (teamDot!=null) return teamDot;
            // Alpha silhouette copied from the original 6x6 minimap player
            // circle; neutral RGB lets the four map colors remain distinct.
            byte[] alpha={0,144,234,234,144,0, 144,255,255,255,255,144,
                255,255,255,255,255,255, 255,255,255,255,255,255,
                144,255,255,255,255,144, 0,144,234,234,144,0};
            Color32[] pixels=new Color32[36];
            for (int i=0;i<pixels.Length;i++) pixels[i]=new Color32(255,255,255,alpha[i]);
            teamDot=new Texture2D(6,6,TextureFormat.ARGB32,false);
            teamDot.SetPixels32(pixels); teamDot.Apply(); teamDot.filterMode=FilterMode.Bilinear;
            return teamDot;
        }
        static Color TeamColor(int slot)
        {
            if (slot==0) return new Color(0.25f,.86f,.95f,1f);
            if (slot==1) return new Color(.38f,.95f,.47f,1f);
            if (slot==2) return new Color(.76f,.48f,1f,1f);
            return new Color(1f,.82f,.25f,1f);
        }
        static bool NativePlayerSprite(AMMiniMapEntity entity)
        { return entity!=null && (entity.SpriteName=="UI_zhujiemian_ditu_03" || entity.SpriteName=="UI_zhujiemian_ditu_08"); }
        TeamMap86.Visible FindTeam(TeamMap86.Key key)
        { foreach (TeamMap86.Visible item in teamVisible) if (item.key.Equals(key)) return item; return null; }
        void RestoreTeamLease(TeamMap86.Key key)
        {
            TeamLease lease;
            if (!teamLeases.TryGetValue(key,out lease)) return;
            // Restore only the same native entity we actually hid. A removed
            // or replaced entity belongs to the native map lifecycle.
            try
            {
                if (lease.map!=null && lease.entity!=null && lease.map.GetEntity(key.playerId.ToString())==lease.entity && !lease.entity.Show)
                    lease.entity.Show=true;
            }
            catch (Exception) { } // the native map may already be tearing down
            finally { teamLeases.Remove(key); }
        }
        void ClearSmallTeam()
        {
            teamRemoved.Clear(); foreach (TeamMap86.Key key in teamLeases.Keys) teamRemoved.Add(key);
            foreach (TeamMap86.Key key in teamRemoved) RestoreTeamLease(key);
            foreach (UITexture marker in smallTeam.Values) RemoveTexture(marker);
            smallTeam.Clear(); teamVisible.Clear(); teamRemoved.Clear();
        }
        void UpdateSmallTeam()
        {
            AMMiniMap map=Map();
            if (map==null || !map.Initialized || map.OffsetRoot==null) { ClearSmallTeam(); return; }
            TeamMap86.Collect(ownerRoom,teamVisible);
            teamRemoved.Clear();
            foreach (KeyValuePair<TeamMap86.Key,UITexture> item in smallTeam)
            {
                AMMiniMapEntity current=map.GetEntity(item.Key.playerId.ToString());
                TeamLease lease;
                if (FindTeam(item.Key)==null || (current!=null && !NativePlayerSprite(current)) ||
                    (teamLeases.TryGetValue(item.Key,out lease) && (lease.map!=map || current!=lease.entity)))
                    teamRemoved.Add(item.Key);
            }
            foreach (TeamMap86.Key key in teamRemoved)
            {
                RestoreTeamLease(key); UITexture old;
                if (smallTeam.TryGetValue(key,out old)) { RemoveTexture(old); smallTeam.Remove(key); }
            }
            float scale=SceneScale();
            foreach (TeamMap86.Visible item in teamVisible)
            {
                AMMiniMapEntity entity=map.GetEntity(item.actorKey);
                if (entity!=null && !NativePlayerSprite(entity)) continue; // native red-name marker retains priority
                TeamLease lease; UITexture marker;
                if (entity!=null && !teamLeases.TryGetValue(item.key,out lease))
                {
                    if (!entity.Show)
                    {
                        if (smallTeam.TryGetValue(item.key,out marker)) { RemoveTexture(marker); smallTeam.Remove(item.key); }
                        continue; // another native owner already hid it
                    }
                    teamLeases[item.key]=new TeamLease { map=map,entity=entity };
                }
                if (!smallTeam.TryGetValue(item.key,out marker) || marker==null)
                {
                    marker=NewTexture(map.OffsetRoot,"Team_"+item.key.playerId,TeamDot(),8,20);
                    smallTeam[item.key]=marker;
                }
                if (entity!=null) entity.Show=false;
                marker.color=TeamColor(item.color);
                Vector3 pos=item.position;
                marker.transform.localPosition=new Vector3(pos.x*scale,pos.z*scale,0);
            }
        }

        void UpdateSmallMarkers()
        {
            AMMiniMap map=Map(); if (map==null || !map.Initialized || map.OffsetRoot==null) return;
            Dictionary<string,Vector3> points=FieldRuntime.ExtractionMapPositions();
            var removed=new List<string>();
            foreach (string id in smallMarkers.Keys) if (!points.ContainsKey(id)) removed.Add(id);
            foreach (string id in removed) { RemoveTexture(smallMarkers[id]); smallMarkers.Remove(id); }
            float scale=SceneScale();
            foreach (KeyValuePair<string,Vector3> entry in points)
            {
                UITexture widget;
                if (!smallMarkers.TryGetValue(entry.Key,out widget) || widget==null)
                {
                    widget=NewTexture(map.OffsetRoot,"DesertExit_"+entry.Key,Icon(),21,20);
                    smallMarkers[entry.Key]=widget;
                }
                Vector3 pos=entry.Value;
                // This is the same x/z conversion used by GetMiniMapPosition.
                widget.transform.localPosition=new Vector3(pos.x*scale,pos.z*scale,0);
            }
        }

        static Transform Need(Transform root,string path)
        { Transform result=root.Find(path); if (result==null) throw new InvalidOperationException("native map prefab missing "+path); return result; }
        static GameObject Clone(Transform template,Transform parent,string name)
        {
            GameObject go=(GameObject)Instantiate(template.gameObject); go.name=name; Parent(go.transform,parent); SetLayer(go.transform,parent.gameObject.layer);
            foreach (UIButtonMessage message in go.GetComponentsInChildren<UIButtonMessage>(true)) DestroyImmediate(message);
            foreach (UIEventListener listener in go.GetComponentsInChildren<UIEventListener>(true)) DestroyImmediate(listener);
            foreach (UIButton button in go.GetComponentsInChildren<UIButton>(true)) button.onClick.Clear();
            foreach (UITweener tween in go.GetComponentsInChildren<UITweener>(true)) tween.enabled=false;
            go.SetActive(true); return go;
        }
        static int AboveExistingPanels()
        {
            int order=0;
            foreach (UnityEngine.Object obj in FindObjectsOfType(typeof(UIPanel)))
            { UIPanel item=obj as UIPanel; if (item!=null && item.gameObject.activeInHierarchy) order=Math.Max(order,item.sortingOrder); }
            return order+1;
        }
        static void SizeCollider(GameObject go,UIWidget widget,int width,int height)
        {
            widget.width=width; widget.height=height;
            BoxCollider collider=go.GetComponent<BoxCollider>();
            if (collider==null) collider=go.AddComponent<BoxCollider>();
            collider.center=Vector3.zero; collider.size=new Vector3(width,height,1);
        }
        GameObject NativeMiniMapBorder(Transform parent)
        {
            UISprite original=Need(source.transform,"small/ForegroundPanel/foreground2/frame").GetComponent<UISprite>();
            UIAtlas sourceAtlas=original==null ? null : original.atlas;
            UISpriteData pixels=sourceAtlas==null ? null : sourceAtlas.GetSprite("UI_zhujiemian_ditu_02");
            if (pixels==null || pixels.width!=183 || pixels.height!=146 || sourceAtlas.spriteMaterial==null)
                throw new InvalidOperationException("original minimap frame unavailable");
            // Copy metadata into a view-owned atlas: the original texture and
            // material stay shared, while its original small-map data stays untouched.
            GameObject atlasObject=new GameObject("OriginalMiniMapFrameAtlas"); atlasObject.layer=parent.gameObject.layer; Parent(atlasObject.transform,parent);
            UIAtlas atlas=atlasObject.AddComponent<UIAtlas>();
            atlas.spriteMaterial=sourceAtlas.spriteMaterial; atlas.pixelSize=sourceAtlas.pixelSize;
            UISpriteData border=new UISpriteData(); border.CopyFrom(pixels); border.name="DesertMapFrame86";
            border.SetBorder(18,18,18,18); atlas.spriteList=new List<UISpriteData>{border};
            GameObject frameObject=new GameObject("OriginalMiniMapFrame"); frameObject.layer=parent.gameObject.layer; Parent(frameObject.transform,parent);
            UISprite frame=frameObject.AddComponent<UISprite>(); frame.atlas=atlas; frame.spriteName=border.name;
            frame.type=UIBasicSprite.Type.Sliced; frame.fillCenter=false; frame.pivot=UIWidget.Pivot.Center; frame.depth=2;
            return frameObject;
        }
        void ReportTeamDiagnostic()
        {
            if (teamDiagnostics.Full) return;
            try
            {
                TeamMap86.Diagnostics state=TeamMap86.DiagnosticState();
                state.viewMembers=teamVisible.Count; state.smallMarkers=smallTeam.Count; state.leases=teamLeases.Count;
                if (teamDiagnostics.Allow(state)) Debug.LogWarning("TeamMap86 "+state.ToString());
            }
            catch (Exception) { } // diagnostic sampling must not affect input or rendering
        }
        void Show()
        {
            if (view!=null) return;
            if (ownerRoom!=RoomManager.currentRoom || BoxManager.Anthor==null || NativeMapTexture()==null || Map()==null)
                throw new InvalidOperationException("native map is not ready");
            Texture nativeTexture=NativeMapTexture();
            if (nativeTexture.width!=512 || nativeTexture.height!=512) throw new InvalidOperationException("unexpected native desert map dimensions");
            GameObject prefab=Resources.Load("Pages/TownMapPage") as GameObject;
            if (prefab==null) throw new InvalidOperationException("native town map prefab unavailable");
            priorPage=PageManager.instance==null ? null : PageManager.instance.currentPage;
            priorBox=BoxManager.m_kBox;
            touchLease=TouchKit.GetEnabled();
            restorePending=false;
            if (touchLease) TouchKit.SetEnabled(false);
            view=new GameObject("DesertMapViewer86"); view.layer=BoxManager.Anthor.gameObject.layer; Parent(view.transform,BoxManager.Anthor);
            try
            {
                panel=view.AddComponent<UIPanel>(); panel.depth=UIPanel.nextUnusedDepth; panel.sortingOrder=AboveExistingPanels();
                mask=Clone(Need(prefab.transform,"mask"),view.transform,"Mask");
                chromeRoot=new GameObject("MapChrome"); chromeRoot.layer=view.layer; Parent(chromeRoot.transform,view.transform);
                chrome=chromeRoot.AddComponent<UIPanel>(); chrome.depth=panel.depth+2; chrome.sortingOrder=panel.sortingOrder+2;
                background=NativeMiniMapBorder(chromeRoot.transform);
                heading=Clone(Need(prefab.transform,"top"),chromeRoot.transform,"Heading");
                closeButton=Clone(Need(prefab.transform,"closeAnthor/close"),chromeRoot.transform,"Close");
                UISprite maskSprite=mask.GetComponent<UISprite>(),bg=background.GetComponent<UISprite>(),top=heading.GetComponent<UISprite>(),close=closeButton.GetComponent<UISprite>();
                if (maskSprite==null || bg==null || top==null || close==null) throw new InvalidOperationException("native map chrome unavailable");
                maskSprite.depth=0; bg.depth=2; bg.pivot=UIWidget.Pivot.Center; top.depth=40; close.depth=60;
                UILabel title=heading.GetComponentInChildren<UILabel>(); if (title!=null) { title.depth=41; title.text="地图"; }
                UIEventListener.Get(closeButton).onClick=delegate(GameObject ignored) { Close(); };
                UIEventListener.Get(mask).onClick=delegate(GameObject ignored) { };
                clipRoot=new GameObject("MapViewport"); clipRoot.layer=view.layer; Parent(clipRoot.transform,view.transform);
                clip=clipRoot.AddComponent<UIPanel>(); clip.depth=panel.depth+1; clip.sortingOrder=panel.sortingOrder+1;
                clip.clipping=UIDrawCall.Clipping.SoftClip; clip.clipSoftness=Vector2.zero;
                GameObject mapObject=new GameObject("MapContent"); mapObject.layer=view.layer; Parent(mapObject.transform,clipRoot.transform); mapRoot=mapObject.transform;
                mapImage=NewTexture(mapRoot,"OriginalMap",nativeTexture,512,1);
                GameObject player=Clone(Need(prefab.transform,"left/player"),mapRoot,"Self");
                selfMarker=player.GetComponent<UISprite>(); if (selfMarker==null) throw new InvalidOperationException("native player marker unavailable");
                selfMarker.depth=30;
                Resize(true); UpdateSelfMarker(); UpdateLargeMarkers(); nextMarkerRefresh=Time.unscaledTime+.25f;
            }
            catch (Exception) { Close(); throw; }
            ReportTeamDiagnostic();
        }
        void Close()
        {
            ClearSmallTeam();
            foreach (UITexture item in largeExits.Values) RemoveTexture(item); largeExits.Clear();
            foreach (UITexture item in largeTeam.Values) RemoveTexture(item); largeTeam.Clear();
            foreach (UISprite item in largeResources.Values) if (item!=null) Destroy(item.gameObject);
            largeResources.Clear();
            RemoveBossMarker(largeBoss); largeBoss=null;
            if (view!=null) { view.SetActive(false); Destroy(view); }
            view=null; mask=null; background=null; heading=null; closeButton=null; clipRoot=null; chromeRoot=null; mapRoot=null; panel=null; clip=null; chrome=null; mapImage=null; selfMarker=null;
            dragFinger=pinchA=pinchB=-1; mouseDragging=false;
            // A later page or dialog owns its own input lifecycle until it
            // returns to the same state in which the map acquired its lease.
            if (touchLease) restorePending=true;
            touchLease=false; RestoreInputWhenIdle();
        }
        void RestoreInputWhenIdle()
        {
            if (!restorePending) { priorPage=null; priorBox=null; return; }
            if (!Target() || ownerRoom!=RoomManager.currentRoom)
            { restorePending=false; priorPage=null; priorBox=null; return; }
            if (PlayerActionPanel86.ViewOpen) return;
            if (PageManager.instance==null || PageManager.instance.currentPage!=priorPage || BoxManager.m_kBox!=priorBox) return;
            if (!TouchKit.GetEnabled()) TouchKit.SetEnabled(true);
            restorePending=false; priorPage=null; priorBox=null;
        }
        void Resize(bool first)
        {
            UIRoot ui=view.GetComponentInParent<UIRoot>();
            if (ui==null || ui.activeHeight<=0 || Screen.height<=0) throw new InvalidOperationException("native UI scale unavailable");
            float height=ui.activeHeight, width=height*Screen.width/(float)Screen.height;
            float next=Mathf.Min(width-48f,height-110f);
            if (next<100f) throw new InvalidOperationException("map viewport too small");
            float previousFit=fit; viewport=next; fit=viewport/512f;
            if (first || previousFit<=0) { zoom=fit; pan=Vector2.zero; }
            else { float ratio=fit/previousFit; zoom=Mathf.Clamp(zoom*ratio,fit,fit*4f); pan*=ratio; }
            clip.baseClipRegion=new Vector4(0,0,viewport,viewport);
            SizeCollider(mask,mask.GetComponent<UIWidget>(),Mathf.CeilToInt(width)+10,Mathf.CeilToInt(height)+10);
            UIWidget bg=background.GetComponent<UIWidget>(); bg.width=Mathf.CeilToInt(viewport+36); bg.height=Mathf.CeilToInt(viewport+36);
            // NGUI pivot changes preserve corners by moving the cloned widget.
            // Recenter after sizing the original frame around this viewport.
            background.transform.localPosition=Vector3.zero;
            heading.transform.localPosition=new Vector3(0,viewport*.5f+23f,0);
            closeButton.transform.localPosition=new Vector3(viewport*.5f+1f,viewport*.5f+23f,0);
            screenWidth=Screen.width; screenHeight=Screen.height; Constrain();
        }
        void Constrain()
        {
            float limit=Mathf.Max(0,(512f*zoom-viewport)*.5f);
            pan.x=Mathf.Clamp(pan.x,-limit,limit); pan.y=Mathf.Clamp(pan.y,-limit,limit);
            if (mapRoot!=null) { mapRoot.localPosition=new Vector3(pan.x,pan.y,0); mapRoot.localScale=new Vector3(zoom,zoom,1); }
            // Keep original glyph sizes steady throughout a pinch, before the next snapshot refresh.
            Vector3 markerScale=Vector3.one/zoom;
            foreach (UITexture marker in largeExits.Values) if (marker!=null) marker.transform.localScale=markerScale;
            foreach (UISprite marker in largeResources.Values) if (marker!=null) marker.transform.localScale=markerScale;
            foreach (UITexture marker in largeTeam.Values) if (marker!=null) marker.transform.localScale=markerScale;
            if (largeBoss!=null && largeBoss.root!=null) largeBoss.root.transform.localScale=markerScale;
        }
        Vector2 ScreenToView(Vector2 position)
        {
            UICamera uiCamera=UICamera.FindCameraForLayer(view.layer);
            Camera camera=uiCamera!=null ? uiCamera.cachedCamera : null;
            if (camera==null) throw new InvalidOperationException("native UI camera unavailable");
            float distance=camera.WorldToScreenPoint(clipRoot.transform.position).z;
            Vector3 world=camera.ScreenToWorldPoint(new Vector3(position.x,position.y,distance));
            Vector3 local=clipRoot.transform.InverseTransformPoint(world); return new Vector2(local.x,local.y);
        }
        bool Inside(Vector2 point)
        { return Math.Abs(point.x)<=viewport*.5f && Math.Abs(point.y)<=viewport*.5f; }
        void ZoomAt(Vector2 anchor,float factor)
        {
            if (factor<=0 || Single.IsNaN(factor) || Single.IsInfinity(factor)) return;
            float next=Mathf.Clamp(zoom*factor,fit,fit*4f);
            Vector2 location=(anchor-pan)/zoom; pan=anchor-location*next; zoom=next; Constrain();
        }
        void Gestures()
        {
            if (Input.touchCount>=2)
            {
                Touch a=Input.GetTouch(0), b=Input.GetTouch(1); Vector2 pa=ScreenToView(a.position),pb=ScreenToView(b.position);
                if (pinchA!=a.fingerId || pinchB!=b.fingerId)
                {
                    dragFinger=-1;
                    if (!Inside(pa) || !Inside(pb)) { pinchA=pinchB=-1; return; }
                    pinchA=a.fingerId; pinchB=b.fingerId; lastMid=(pa+pb)*.5f; lastDistance=Vector2.Distance(pa,pb); return;
                }
                Vector2 mid=(pa+pb)*.5f; float distance=Vector2.Distance(pa,pb);
                pan+=mid-lastMid; if (lastDistance>1f) ZoomAt(mid,distance/lastDistance); else Constrain();
                lastMid=mid; lastDistance=distance; return;
            }
            if (pinchA!=-1) { pinchA=pinchB=-1; dragFinger=-1; }
            if (Input.touchCount==1)
            {
                Touch touch=Input.GetTouch(0); Vector2 point=ScreenToView(touch.position);
                if (touch.phase==TouchPhase.Began && Inside(point)) { dragFinger=touch.fingerId; lastDrag=point; }
                else if (dragFinger==touch.fingerId && touch.phase==TouchPhase.Moved) { pan+=point-lastDrag; lastDrag=point; Constrain(); }
                else if (touch.phase==TouchPhase.Ended || touch.phase==TouchPhase.Canceled) dragFinger=-1;
                return;
            }
            dragFinger=-1;
            Vector2 mouse=ScreenToView(Input.mousePosition);
            if (Input.GetMouseButtonDown(0) && Inside(mouse)) { mouseDragging=true; lastDrag=mouse; }
            if (mouseDragging && Input.GetMouseButton(0)) { pan+=mouse-lastDrag; lastDrag=mouse; Constrain(); }
            if (Input.GetMouseButtonUp(0)) mouseDragging=false;
        }
        Vector3 RelativePosition(Vector2 mapPosition,Vector2 offset,float scale)
        { return new Vector3(mapPosition.x-offset.x*scale,mapPosition.y-offset.y*scale,0); }
        void UpdateSelfMarker()
        {
            RoomCharactor self=ownerRoom==null || ownerRoom.charactorManager==null ? null : ownerRoom.charactorManager.GetSelfCharactor();
            if (self==null || selfMarker==null) return;
            float scale=SceneScale(); Vector2 offset=SceneOffset();
            Vector3 selfPosition=self.transform.localPosition;
            selfMarker.transform.localPosition=RelativePosition(new Vector2(selfPosition.x*scale,selfPosition.z*scale),offset,scale);
            selfMarker.transform.localRotation=Quaternion.Euler(0,180,self.transform.localEulerAngles.y+75f);
            selfMarker.transform.localScale=Vector3.one/zoom;
        }
        bool LargeTeamVisible(TeamMap86.Visible item,AMMiniMap map)
        {
            if (item==null) return false;
            AMMiniMapEntity entity=map.GetEntity(item.actorKey);
            if (entity==null) return true; // the same-room actor may precede its native map entity
            TeamLease lease;
            return NativePlayerSprite(entity) && (entity.Show ||
                (teamLeases.TryGetValue(item.key,out lease) && lease.map==map && lease.entity==entity));
        }
        void UpdateLargeTeam()
        {
            AMMiniMap map=Map(); if (map==null || mapRoot==null) return;
            teamRemoved.Clear();
            foreach (TeamMap86.Key key in largeTeam.Keys)
            { TeamMap86.Visible item=FindTeam(key); if (!LargeTeamVisible(item,map)) teamRemoved.Add(key); }
            foreach (TeamMap86.Key key in teamRemoved) { RemoveTexture(largeTeam[key]); largeTeam.Remove(key); }
            float scale=SceneScale(); Vector2 offset=SceneOffset();
            foreach (TeamMap86.Visible item in teamVisible)
            {
                if (!LargeTeamVisible(item,map)) continue;
                UITexture marker;
                if (!largeTeam.TryGetValue(item.key,out marker) || marker==null)
                {
                    marker=NewTexture(mapRoot,"Team_"+item.key.playerId,TeamDot(),10,25);
                    largeTeam[item.key]=marker;
                }
                marker.color=TeamColor(item.color);
                Vector3 pos=item.position;
                marker.transform.localPosition=RelativePosition(new Vector2(pos.x*scale,pos.z*scale),offset,scale);
                marker.transform.localScale=Vector3.one/zoom;
            }
        }
        void UpdateLargeMarkers()
        {
            AMMiniMap map=Map();
            if (map==null) { RemoveBossMarker(largeBoss); largeBoss=null; return; }
            float scale=SceneScale(); Vector2 offset=SceneOffset();
            string bossId,bossState,bossLabel; Vector3 bossPosition;
            if (!FieldBoss.MapMarkerLabel(ownerRoom,out bossId,out bossPosition,out bossState,out bossLabel))
            { RemoveBossMarker(largeBoss); largeBoss=null; }
            else
            {
                if (largeBoss==null || largeBoss.root==null) largeBoss=NewBossMarker(mapRoot,26,24);
                SetBossState(largeBoss,bossState,bossLabel);
                largeBoss.root.transform.localPosition=RelativePosition(new Vector2(bossPosition.x*scale,bossPosition.z*scale),offset,scale);
                largeBoss.root.transform.localScale=Vector3.one/zoom;
            }

            Dictionary<string,Vector3> exits=FieldRuntime.ExtractionMapPositions(); var oldExits=new List<string>();
            foreach (string id in largeExits.Keys) if (!exits.ContainsKey(id)) oldExits.Add(id);
            foreach (string id in oldExits) { RemoveTexture(largeExits[id]); largeExits.Remove(id); }
            foreach (KeyValuePair<string,Vector3> item in exits)
            {
                UITexture marker;
                if (!largeExits.TryGetValue(item.Key,out marker) || marker==null)
                { marker=NewTexture(mapRoot,"Exit_"+item.Key,Icon(),24,20); largeExits[item.Key]=marker; }
                marker.transform.localPosition=RelativePosition(new Vector2(item.Value.x*scale,item.Value.z*scale),offset,scale);
                marker.transform.localScale=Vector3.one/zoom;
            }
            Transform frame=Need(source.transform,"small/ForegroundPanel/foreground2/frame"); UISprite original=frame.GetComponent<UISprite>();
            if (original==null || original.atlas==null) throw new InvalidOperationException("native minimap atlas unavailable");
            var seen=new HashSet<AMMiniMapEntity>();
            for (int i=0;i<map.EntityCount;i++)
            {
                AMMiniMapEntity entity=map.GetEntity(i); if (entity==null || !entity.Show) continue;
                string sprite=entity.SpriteName; if (sprite!="UI_ditu_wakuang" && sprite!="UI_ditu_yugou") continue;
                seen.Add(entity); UISprite marker;
                if (!largeResources.TryGetValue(entity,out marker) || marker==null)
                {
                    GameObject go=new GameObject("Resource"); go.layer=mapRoot.gameObject.layer; Parent(go.transform,mapRoot);
                    marker=go.AddComponent<UISprite>(); marker.atlas=original.atlas; marker.pivot=UIWidget.Pivot.Center; marker.width=21; marker.height=21; marker.depth=10;
                    largeResources[entity]=marker;
                }
                marker.spriteName=sprite; marker.transform.localPosition=RelativePosition(entity.Position,offset,scale); marker.transform.localScale=Vector3.one/zoom;
            }
            var removed=new List<AMMiniMapEntity>();
            foreach (KeyValuePair<AMMiniMapEntity,UISprite> item in largeResources) if (!seen.Contains(item.Key)) removed.Add(item.Key);
            foreach (AMMiniMapEntity key in removed) { if (largeResources[key]!=null) Destroy(largeResources[key].gameObject); largeResources.Remove(key); }
        }
        void Update()
        {
            if (!Target() || ownerRoom!=RoomManager.currentRoom) { ReleaseScene(); return; }
            if (view==null) { RestoreInputWhenIdle(); return; }
            if (PageManager.instance==null || PageManager.instance.currentPage!=priorPage || BoxManager.m_kBox!=priorBox)
            { Close(); return; }
            try
            {
                if (screenWidth!=Screen.width || screenHeight!=Screen.height) Resize(false);
                Gestures(); UpdateSelfMarker(); UpdateLargeTeam();
                if (Time.unscaledTime>=nextMarkerRefresh)
                { nextMarkerRefresh=Time.unscaledTime+.25f; UpdateLargeMarkers(); }
            }
            catch (Exception error)
            { Debug.LogWarning("Desert map view: "+error.GetType().Name+": "+error.Message); Close(); }
        }
    }
}
