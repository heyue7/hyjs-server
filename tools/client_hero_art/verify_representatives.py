"""Execute the actual crop/alias source with bounded UI stubs; no Unity rendering claim."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).with_name('HeroArt.cs')
STUBS = r'''
namespace UnityEngine {
 public struct Rect { public float x,y,width,height; public Rect(float x,float y,float w,float h){this.x=x;this.y=y;width=w;height=h;} }
 public struct Vector4 { public float x,y,z,w; public Vector4(float x,float y,float z,float w){this.x=x;this.y=y;this.z=z;this.w=w;} }
 public class Object { public string name;public HideFlags hideFlags;public static void Destroy(Object o){} }
 public enum HideFlags { DontSave }
 public enum TextureFormat { RGBA32 }
 public enum RenderTextureFormat { ARGB32 }
 public enum TextureWrapMode { Clamp }
 public enum FilterMode { Bilinear }
 public struct Color32 { public byte r,g,b,a;public Color32(byte r,byte g,byte b,byte a){this.r=r;this.g=g;this.b=b;this.a=a;} }
 public class Texture:Object { public int width=1024,height=1024;public Texture(){}public Texture(string n){name=n;} }
 public class Texture2D:Texture {
  public TextureWrapMode wrapMode;public FilterMode filterMode;
  public Texture2D(int w,int h,TextureFormat f,bool m){width=w;height=h;}
  public void ReadPixels(Rect r,int x,int y){throw new System.NotSupportedException("GPU is outside stub scope");}
  public Color32[] GetPixels32(){throw new System.NotSupportedException("GPU is outside stub scope");}
  public void SetPixels32(Color32[] p){} public void Apply(bool m,bool r){}
 }
 public class RenderTexture:Texture {
  public static RenderTexture active;
  public static RenderTexture GetTemporary(int w,int h,int d,RenderTextureFormat f){throw new System.NotSupportedException("GPU is outside stub scope");}
  public static void ReleaseTemporary(RenderTexture t){}
 }
 public static class Graphics { public static void Blit(Texture t,RenderTexture r){throw new System.NotSupportedException("GPU is outside stub scope");} }
 public static class Debug { public static void LogWarning(string text){} }
 public static class Mathf {public static float Clamp(float v,float min,float max){return System.Math.Max(min,System.Math.Min(max,v));}public static int RoundToInt(float n){return (int)System.Math.Round(n);} }
 public class GameObject {
  public bool activeInHierarchy=true;
  System.Collections.Generic.Dictionary<System.Type,Component> parts=new System.Collections.Generic.Dictionary<System.Type,Component>();
  public T AddComponent<T>() where T:Component,new(){var p=new T();p.gameObject=this;parts[typeof(T)]=p;return p;}
  public T GetComponent<T>() where T:Component{return parts.TryGetValue(typeof(T),out var p)?(T)p:null;}
 }
 public class Component { public GameObject gameObject; public T GetComponent<T>() where T:Component{return gameObject.GetComponent<T>();} }
 public class MonoBehaviour:Component {}
}
namespace Dict.DataModel { public class TableHero { public string id;public int IsShowInCard;} public class TableSkill {public int LevelGrowFactor;} public class TableSkillCast {} }
namespace Dict {
 public class FakeCastDao {public System.Collections.Generic.List<DataModel.TableSkillCast> GetByOneIndex(string s){throw new System.NotSupportedException("Native dictionary is outside stub scope");}}
 public class FakeSkillDao {public DataModel.TableSkill GetById(string s){throw new System.NotSupportedException("Native dictionary is outside stub scope");}}
 public class DictDataManager {public static DictDataManager Instance;public FakeCastDao tableSkillCastDao;public FakeSkillDao tableSkillDao;}
}
namespace Dict.Blo {public static class DictSkillCastBlo {public static double GetSkillExportFactor(string s){throw new System.NotSupportedException("Native dictionary is outside stub scope");}}}
namespace UI {
 public class PageManager {public static PageManager Instance;public object GetDictPage(string name){throw new System.NotSupportedException("Native page lifecycle is outside stub scope");}}
 public class SevenDayPage:UnityEngine.Component {public TopLeftDays topLeft;}
 public class TopLeftDays:UnityEngine.Component {public void ReFreshDayAct(){throw new System.NotSupportedException("Native indicator request is outside stub scope");}}
}
public class UITexture:UnityEngine.Component {
 public UnityEngine.Texture mainTexture;
 public UnityEngine.Rect uvRect=new UnityEngine.Rect(.1f,.2f,.7f,.8f);
 public UnityEngine.Vector4 drawRegion=new UnityEngine.Vector4(0,0,1,1);
 public int width=128,height=128,pixelPerfectCalls;
 public void MakePixelPerfect(){pixelPerfectCalls++;width=height=1024;}
}
public class AloneTextureKit:UnityEngine.Component {}
'''
PROGRAM = r'''
using System;
using UnityEngine;
using HYJS.HeroArt;
using Dict.DataModel;
static class Program {
 static int checks;
 static void Check(bool v,string n){if(!v)throw new Exception(n);checks++;}
 static bool Eq(float a,float b){return Math.Abs(a-b)<.00001f;}
 static bool Eq(Rect a,Rect b){return Eq(a.x,b.x)&&Eq(a.y,b.y)&&Eq(a.width,b.width)&&Eq(a.height,b.height);}
 static void Main(){
  foreach(string id in new[]{"069","070","073"}){
   Check(HeroArtFallback.MissingOriginalText("desc_hero_"+id)=="原客户端未提供该角色简介。","exact missing original biography status");
   for(int slot=1;slot<=3;slot++){
    // Native SkillCast reads require the original initialized dictionary and
    // are validated in the fixed managed candidate/phone, outside these stubs.
    Check(!String.IsNullOrEmpty(HeroArtFallback.MissingOriginalText("name_skill_"+id+"_"+slot)),"missing skill has explicit placeholder");
   }
   string painting="heropainter/hero_card_"+id;
   foreach(string request in new[]{"heroheader/hero_icon_"+id,"HeroHeader/hero_icon_"+id,"hero2d/hero_2d_"+id}){
    Check(HeroArtFallback.MapBundle(request)==painting,"alias "+request);
    Check(HeroArtFallback.MapBundle(HeroArtFallback.MapBundle(request))==painting,"idempotent alias");
    string asset=request.Substring(request.LastIndexOf('/')+1);
    Check(HeroArtFallback.MapAsset(request,asset)=="hero_card_"+id,"paired asset");
    Check(HeroArtFallback.MapAsset(request,"hero_icon_001")=="hero_icon_001","other asset unchanged");
   }
   Check(HeroArtFallback.AtlasHidden(new TableHero{id="hero_"+id,IsShowInCard=1})==0,"published atlas");
   var go=new GameObject();var tex=go.AddComponent<UITexture>();var kit=go.AddComponent<AloneTextureKit>();
   Rect original=tex.uvRect;var art=new Texture("hero_card_"+id);
   HeroArtFallback.AssignHead(tex,art);Rect face=tex.uvRect;
   Check(tex.mainTexture==art && face.width>.2f && face.width<.3f,"face crop");
   Check(face.x>=0&&face.y>=0&&face.x+face.width<=1&&face.y+face.height<=1,"bounded UV");
   Check(tex.width==128&&tex.height==128&&tex.drawRegion.x>0&&tex.drawRegion.z<1,"head stays inside original frame");
   HeroArtFallback.AssignHead(tex,new Texture("hero_icon_071"));Check(Eq(tex.uvRect,original),"pooled normal restores UV");
   Check(Eq(tex.drawRegion.x,0)&&Eq(tex.drawRegion.z,1),"pooled normal restores draw region");
   HeroArtFallback.AssignHead(tex,art);HeroArtFallback.AssignHead(tex,null);Check(Eq(tex.uvRect,original),"clear restores UV");
   HeroArtFallback.PrepareCard(kit,"hero2d/hero_2d_"+id);HeroArtFallback.AssignCard(tex,art);
   Check(Eq(tex.uvRect.width,560f/1024f)&&tex.mainTexture==art,"card crop");
   HeroArtFallback.PrepareCard(kit,painting);HeroArtFallback.AssignCard(tex,art);Check(Eq(tex.uvRect,original),"detail keeps original full painting");
   HeroArtFallback.PrepareCard(kit,"hero2d/hero_2d_"+id);HeroArtFallback.AssignCard(tex,art);
   HeroArtFallback.PrepareCard(kit,"hero2d/hero_2d_071");HeroArtFallback.AssignCard(tex,new Texture("hero_2d_071"));Check(Eq(tex.uvRect,original),"pooled normal card restores");
   HeroArtFallback.AssignRequested(tex,art,"HeroHeader/hero_icon_"+id);
   HeroArtFallback.MakeRequestedPixelPerfect(tex);
   Check(Eq(tex.uvRect,face)&&tex.width==128&&tex.height==128&&tex.pixelPerfectCalls==0,"async head preserves prefab dimensions");
   HeroArtFallback.AssignRequested(tex,art,"hero2d/hero_2d_"+id);
   Check(Eq(tex.uvRect.width,560f/1024f)&&Eq(tex.drawRegion.x,0),"async recruit card crop has original region");
   HeroArtFallback.AssignRequested(tex,art,painting);
   Check(Eq(tex.uvRect,original)&&Eq(tex.drawRegion.x,0),"async full detail restores original UV and region");
   HeroArtFallback.MakeRequestedPixelPerfect(tex);
   Check(tex.width==1024&&tex.height==1024&&tex.pixelPerfectCalls==1,"full painting retains original pixel-perfect behavior");
   tex.width=160;tex.height=112;
   HeroArtFallback.AssignRequested(tex,art,"HeroHeader/hero_icon_"+id);
   HeroArtFallback.MakeRequestedPixelPerfect(tex);
   Check(tex.width==160&&tex.height==112&&tex.pixelPerfectCalls==1,"non-square original layout retained");
   HeroArtFallback.AssignRequested(tex,new Texture("hero_icon_071"),"HeroHeader/hero_icon_071");
   HeroArtFallback.MakeRequestedPixelPerfect(tex);
   Check(Eq(tex.uvRect,original)&&Eq(tex.drawRegion.x,0)&&tex.pixelPerfectCalls==2,"unrelated async image still runs original behavior");
  }
  foreach(string path in new[]{"heroheader/hero_icon_071","hero2d/hero_2d_007","model/roa01_body","heropainter/hero_card_070","heroheader/hero_icon_069_extra","",null})
   Check(HeroArtFallback.MapBundle(path)==path,"other path unchanged");
  foreach(string key in new[]{"desc_hero_001","name_hero_069","desc_skill_068_1","name_skill_070_10","desc_skill_073_3_extra","",null})
   Check(HeroArtFallback.MissingOriginalText(key)==null,"all existing/unrelated text retains original reader");
  Check(HeroArtFallback.AtlasHidden(new TableHero{id="hero_074",IsShowInCard=1})==1,"other hidden preserved");
  Check(HeroArtFallback.AtlasHidden(new TableHero{id="hero_007",IsShowInCard=0})==0,"normal atlas preserved");
  Console.WriteLine("PASS "+checks+" bounded alias, visibility and pooled UV assertions");
 }
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    dotnet = ROOT / '.tools/native-batch/dotnet/dotnet'
    with tempfile.TemporaryDirectory(prefix='hyjs-hero-art-representatives-') as temporary:
        work = Path(temporary)
        shutil.copy2(SOURCE, work / 'HeroArt.cs')
        (work / 'Stubs.cs').write_text(STUBS)
        (work / 'Program.cs').write_text(PROGRAM)
        project = work / 'Probe.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><Nullable>disable</Nullable></PropertyGroup></Project>')
        compiled = subprocess.run(['rtk', 'proxy', str(dotnet), 'build', str(project), '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'], capture_output=True, text=True, timeout=90)
        if compiled.returncode:
            raise RuntimeError(compiled.stdout + compiled.stderr)
        run = subprocess.run(['rtk', 'proxy', str(dotnet), str(work / 'bin/Debug/net8.0/Probe.dll')], capture_output=True, text=True, timeout=15)
        report = dict(mode='actual-HeroArt-source-with-bounded-Unity-UI-stubs', sourceSHA256=hashlib.sha256(SOURCE.read_bytes()).hexdigest(), status='PASS' if run.returncode == 0 else 'FAIL', output=run.stdout + run.stderr, unityRenderingExecuted=False, deviceExecuted=False)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps(report))
        assert run.returncode == 0


if __name__ == '__main__':
    main()
