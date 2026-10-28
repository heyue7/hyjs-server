using System;
using System.Collections;
using System.Reflection;
using Dict;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Preload from the same native model extension and tool IDs used by
    // PlayerAIStateFishing/Mining. The callback attaches clips and hidden tools
    // without playing an action. No synthetic asset names or reward state.
    public sealed class GatherPreload
    {
        const BindingFlags Flags=BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        readonly MonoBehaviour host;
        readonly RoomCharactor actor;
        readonly RCAIActor ai;
        readonly PlayerAIStateFishing fish;
        readonly PlayerAIStateMining mine;
        readonly FieldInfo fishLoaded, mineLoaded, fishTool, mineTool;
        bool disposed, clipRequested, clipDone, extensionReleased, fishRequested, mineRequested;
        string extensionName, fishName, mineName;
        GameObject model, ownedFish, ownedMine;
        public bool Failed { get; private set; }
        public GatherPreload(MonoBehaviour owner,RoomCharactor self)
        {
            if (owner==null || self==null) throw new ArgumentNullException("Missing player");
            host=owner; actor=self; ai=self.ai as RCAIActor;
            if (ai==null) throw new InvalidOperationException("Native actor FSM unavailable");
            FieldInfo statesField=typeof(RCAIActor).GetField("allStates",Flags);
            IDictionary states=statesField==null ? null : statesField.GetValue(ai) as IDictionary;
            fish=states==null ? null : states[3] as PlayerAIStateFishing;
            mine=states==null ? null : states[4] as PlayerAIStateMining;
            fishLoaded=typeof(PlayerAIStateFishing).GetField("loadedFishAni",Flags);
            mineLoaded=typeof(PlayerAIStateMining).GetField("loadedMineAni",Flags);
            fishTool=typeof(PlayerAIStateFishing).GetField("fishingRod",Flags);
            mineTool=typeof(PlayerAIStateMining).GetField("mineModel",Flags);
            if (fish==null || mine==null || fishLoaded==null || mineLoaded==null || fishTool==null || mineTool==null)
                throw new InvalidOperationException("Native gather preload anchors differ");
        }
        bool Alive() { return !disposed && host!=null && actor!=null && actor.lod!=null && actor.lod.Model!=null; }
        bool Loaded(FieldInfo field,object state) { return (bool)field.GetValue(state); }
        bool HasTool(FieldInfo field,object state) { return (field.GetValue(state) as GameObject)!=null; }
        public bool Ready(string kind)
        {
            if (!Alive() || actor.lod.Model!=model) return false;
            if (kind=="fish") return Loaded(fishLoaded,fish) && HasTool(fishTool,fish);
            if (kind=="mine") return Loaded(mineLoaded,mine) && HasTool(mineTool,mine);
            return false;
        }
        public void Tick()
        {
            if (!Alive() || Failed) return;
            GameObject current=actor.lod.Model;
            if (model!=null && model!=current) { Failed=true; return; }
            if (model==null)
            {
                model=current; extensionName=current.name+"_field";
                fishName=DictDataManager.Instance.tableGameConfigureDao.currentData.FishingRod;
                mineName=DictDataManager.Instance.tableGameConfigureDao.currentData.MiningHoe;
                if (String.IsNullOrEmpty(fishName) || String.IsNullOrEmpty(mineName)) { Failed=true; return; }
            }
            if ((!Loaded(fishLoaded,fish) || !Loaded(mineLoaded,mine)) && !clipRequested)
            {
                clipRequested=true;
                host.StartCoroutine(LoadClips());
            }
            if (!HasTool(fishTool,fish) && !fishRequested) { fishRequested=true; LoadTool(fishName,true); }
            if (!HasTool(mineTool,mine) && !mineRequested) { mineRequested=true; LoadTool(mineName,false); }
        }
        IEnumerator LoadClips()
        {
            IEnumerator inner=ResourceLoader.GetInstance().modelLoader.LoadModelExtend(extensionName,OnClips);
            while (true)
            {
                bool more=false; object value=null;
                try { more=inner.MoveNext(); if (more) value=inner.Current; } catch (Exception) { more=false; }
                if (!more) break;
                yield return value;
            }
            if (!clipDone) { clipDone=true; if (disposed) ReleaseExtension(); else Failed=true; }
        }
        void ReleaseExtension()
        {
            if (extensionReleased || !clipRequested || String.IsNullOrEmpty(extensionName)) return;
            extensionReleased=true; ResourceLoader.GetInstance().modelLoader.UnloadModelExtend(extensionName);
        }
        void OnClips(object value)
        {
            clipDone=true;
            if (disposed) { ReleaseExtension(); return; }
            if (!Alive() || actor.lod.Model!=model) { Failed=true; return; }
            AnimClipsAsset source=value as AnimClipsAsset; ModelRenderKit kit=model.GetComponent<ModelRenderKit>();
            if (source==null || source.clips==null || kit==null) { Failed=true; return; }
            AnimationClip fishClip=null,mineClip=null;
            foreach (AnimationClip clip in source.clips)
            {
                if (clip==null) continue;
                if (clip.name=="diaoyu") fishClip=clip;
                if (clip.name=="wakuang") mineClip=clip;
            }
            if (fishClip==null || mineClip==null) { Failed=true; return; }
            if (!Loaded(fishLoaded,fish)) { kit.AddLifeAni(fishClip); fishLoaded.SetValue(fish,true); }
            if (!Loaded(mineLoaded,mine)) { kit.AddLifeAni(mineClip); mineLoaded.SetValue(mine,true); }
        }
        void LoadTool(string name,bool isFish)
        {
            try { host.StartCoroutine(ResourceLoader.GetInstance().weaponLoader.LoadWeapon(name,delegate(object value) { OnTool(value,name,isFish); })); }
            catch (Exception) { Failed=true; }
        }
        void OnTool(object value,string name,bool isFish)
        {
            GameObject go=value as GameObject;
            if (go==null) { if (!disposed) Failed=true; return; }
            if (!Alive() || actor.lod.Model!=model)
            { ResourceLoader.GetInstance().weaponLoader.UnloadWeapon(go,name); return; }
            if (HasTool(isFish ? fishTool : mineTool,isFish ? (object)fish : (object)mine))
            { ResourceLoader.GetInstance().weaponLoader.UnloadWeapon(go,name); return; }
            ModelRenderKit kit=model.GetComponent<ModelRenderKit>(); Transform hand=kit==null ? null : kit.GetTransform("Bip001 R Hand");
            if (hand==null) { ResourceLoader.GetInstance().weaponLoader.UnloadWeapon(go,name); Failed=true; return; }
            go.transform.SetParent(hand); go.transform.localPosition=new Vector3(-.05f,.025f,-.224f); go.transform.localScale=Vector3.one;
            go.transform.localRotation=Quaternion.Euler(-2.173f,-7.8f,81f); go.name=name; go.SetActive(false);
            if (isFish) { fishTool.SetValue(fish,go); ai.fishObj=go; ownedFish=go; }
            else { mineTool.SetValue(mine,go); ai.mineObj=go; ownedMine=go; }
        }
        // Native OnLeave unloads an active tool. Re-arm only after that unload;
        // the owner calls Tick again when the actor returns to normal state.
        public void RecheckAfterAction()
        {
            if (!HasTool(fishTool,fish)) fishRequested=false;
            if (!HasTool(mineTool,mine)) mineRequested=false;
        }
        public void Dispose()
        {
            if (disposed) return; disposed=true;
            ReleaseTool(fishTool,fish,fishName,true,ownedFish);
            ReleaseTool(mineTool,mine,mineName,false,ownedMine);
            if (clipDone) ReleaseExtension();
        }
        void ReleaseTool(FieldInfo field,object state,string name,bool isFish,GameObject owned)
        {
            GameObject go=field.GetValue(state) as GameObject;
            if (go==null || owned==null || !System.Object.ReferenceEquals(go,owned) || String.IsNullOrEmpty(name)) return;
            field.SetValue(state,null);
            if (isFish && ai.fishObj==go) ai.fishObj=null;
            if (!isFish && ai.mineObj==go) ai.mineObj=null;
            ResourceLoader.GetInstance().weaponLoader.UnloadWeapon(go,name);
        }
    }
}
