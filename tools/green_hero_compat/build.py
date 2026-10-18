"""Build bounded green-character compatibility resources without packaging or device use.

Original APK/table bytes stay immutable. Three combat-ready models reuse original
basic attacks. Three NPC appearances retain all original mesh/material/texture data
and receive same-path legacy combat curves from monster_070.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import zipfile
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
PAIRS = {"hero_056":"monster_068", "hero_057":"monster_070", "hero_058":"monster_070", "hero_059":"monster_070", "hero_060":"monster_067", "hero_061":"monster_070"}
FIELDS = ["SkillId1", "BeFoundBone", "RushFlyTime", "RushFallTime", "JumpBackFlyTime", "JumpBackFallTime", "ImposState"]
TRANSPLANTS = ["lingnai01_body", "sicary01_body", "nio01_body"]
REQUIRED = {"attack01", "standfight", "runfight", "standweak", "hit", "dizzy", "floata", "falla", "fallb", "fallc", "jumpb"}


def sha(raw): return hashlib.sha256(raw).hexdigest()

def hierarchy(env):
    objects = {o.path_id:o.read_typetree() for o in env.objects if o.type.name in {"GameObject", "Transform"}}
    transforms = {o.path_id:o.read_typetree() for o in env.objects if o.type.name == "Transform"}
    result = {}
    def visit(index, path):
        value = transforms[index]
        result[path] = value
        for child in value["m_Children"]:
            ident = child["m_PathID"]
            name = objects[transforms[ident]["m_GameObject"]["m_PathID"]]["m_Name"]
            visit(ident, (path+"/" if path else "")+name)
    for index, value in transforms.items():
        if value["m_Father"]["m_PathID"] == 0: visit(index, "")
    return result


def curves(tree):
    return [(key, row) for key,value in tree.items() if key.endswith("Curves") and isinstance(value,list) for row in value if isinstance(row,dict) and "path" in row]


def transplant(raw, donor_raw):
    env, donor = UnityPy.load(raw), UnityPy.load(donor_raw)
    paths = hierarchy(env)
    animations = [o for o in env.objects if o.type.name == "Animation"]
    assert len(animations) == 1
    animation = animations[0]
    tree = animation.read_typetree()
    template = next(o for o in env.objects if o.type.name == "AnimationClip")
    serialized = template.assets_file
    initial = {o.path_id:sha(o.get_raw_data()) for o in env.objects}
    evidence = []
    for donor_clip in donor.objects:
        if donor_clip.type.name != "AnimationClip": continue
        clip = donor_clip.read_typetree()
        if clip["m_Name"] not in REQUIRED: continue
        assert clip["m_Legacy"] and not clip["m_CompressedRotationCurves"] and not clip["m_PPtrCurves"] and not clip["m_Events"]
        missing = sorted({row["path"] for _,row in curves(clip)}-paths.keys())
        # Missing curves are donor decoration or unskinned footsteps only.
        assert all(path.endswith("/Bone001") or path.endswith("/Bip001 Footsteps") for path in missing), missing
        dropped = []
        for key in [key for key in clip if key.endswith("Curves") and isinstance(clip[key],list)]:
            kept = []
            for row in clip[key]:
                if "path" not in row: kept.append(row); continue
                path = row["path"]
                if path not in paths:
                    dropped.append(path); continue
                if key == "m_PositionCurves" and path not in {"root", "root/Bip001/Bip001 Pelvis"}:
                    # Keep target bind translations, so costume/limb lengths stay intact.
                    for sample in row["curve"]["m_Curve"]:
                        sample["value"] = dict(paths[path]["m_LocalPosition"])
                        sample["inSlope"] = dict(x=0.0,y=0.0,z=0.0)
                        sample["outSlope"] = dict(x=0.0,y=0.0,z=0.0)
                if key == "m_ScaleCurves":
                    for sample in row["curve"]["m_Curve"]:
                        sample["value"] = dict(paths[path]["m_LocalScale"])
                        sample["inSlope"] = dict(x=0.0,y=0.0,z=0.0)
                        sample["outSlope"] = dict(x=0.0,y=0.0,z=0.0)
                kept.append(row)
            clip[key] = kept
        ident = max(serialized.objects)+1
        target = copy.copy(template)
        target.path_id = ident
        target.data = None
        serialized.objects[ident] = target
        target.save_typetree(clip)
        tree["m_Animations"].append({"m_FileID":0,"m_PathID":ident})
        evidence.append(dict(name=clip["m_Name"], donorPathID=donor_clip.path_id, addedPathID=ident, droppedNonSkeletonPaths=sorted(set(dropped)), retainedCurvePaths=sorted({row["path"] for _,row in curves(clip)}), clipDataSHA256=sha(target.data)))
    assert {item["name"] for item in evidence} == REQUIRED
    animation.save_typetree(tree)
    encoded = env.file.save(packer="lz4")
    actual = UnityPy.load(encoded)
    byid = {o.path_id:o for o in actual.objects}
    for ident, digest in initial.items():
        if ident != animation.path_id: assert sha(byid[ident].get_raw_data()) == digest
    assert len(byid) == len(initial)+len(REQUIRED)
    added = {item["addedPathID"] for item in evidence}
    loaded_anim = byid[animation.path_id].read_typetree()
    assert added <= {row["m_PathID"] for row in loaded_anim["m_Animations"]}
    for item in evidence:
        d = byid[item["addedPathID"]].read_typetree()
        assert {row["path"] for _,row in curves(d)} <= paths.keys()
        assert sha(byid[item["addedPathID"]].get_raw_data()) == item["clipDataSHA256"]
    return encoded, dict(originalObjects=len(initial), unchangedOriginalObjects=len(initial)-1, modifiedOriginalObjects=[dict(pathID=animation.path_id,type="Animation",change="append eleven actual legacy clips")], addedAnimationClips=evidence, meshMaterialTextureAndTransformObjectsUnchanged=True, runtimeRenderingVerified=False)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk",type=Path,required=True)
    parser.add_argument("--output-directory",type=Path,required=True)
    args=parser.parse_args()
    args.output_directory=args.output_directory.resolve()
    args.base_apk=args.base_apk.resolve()
    assert not args.output_directory.exists(), "Use fresh output directory"
    args.output_directory.mkdir(parents=True)
    output=args.output_directory
    manifest=dict(schema="green-six-compatible-asset-overlay-v1",status="STATIC_RESOURCE_VERIFIED_RUNTIME_PENDING",baseAPK=str(args.base_apk),baseAPKSHA256=sha(args.base_apk.read_bytes()),userApprovedLocalCompatibility=True,originalSkillRestorationClaim=False,files=[],characters=[],originalEngineMethodsChanged=False)
    with zipfile.ZipFile(args.base_apk) as apk:
        changes={}
        original=apk.read("assets/android/tabledata/tablehero")
        env=UnityPy.load(original)
        asset=next(o for o in env.objects if o.type.name=="TextAsset")
        data=asset.read_typetree()
        text=data["m_Script"]
        if isinstance(text,bytes): text=text.decode()
        lines=text.splitlines()
        keys=lines[1].split("\t")
        rows={line.split("\t")[0]:dict(zip(keys,line.split("\t"))) for line in lines[5:] if line}
        for ident, donor in PAIRS.items():
            row=rows[ident]; source=rows[donor]
            assert row["quality"]=="12" and all(not row[key] for key in FIELDS)
            before=dict(row)
            for key in FIELDS: row[key]=source[key]
            manifest["characters"].append(dict(id=ident, originalModel=row["resName"], donor=donor, skill=row["SkillId1"], changes={key:dict(before=before[key],after=row[key]) for key in FIELDS}, baseAttributesUnchanged=True, ultimateAdded=False))
        for index in range(5,len(lines)):
            ident=lines[index].split("\t")[0]
            if ident in PAIRS: lines[index]="\t".join(rows[ident].get(key,"") for key in keys)
        derived="\n".join(lines)+"\n"
        data["m_Script"]=derived
        asset.save_typetree(data)
        modified=env.file.save(packer="lz4")
        reread=next(o.read_typetree()["m_Script"] for o in UnityPy.load(modified).objects if o.type.name=="TextAsset")
        assert reread==derived
        changes["assets/android/tabledata/tablehero"]=modified
        (output/"TableHero.tsv").write_text(derived)
        manifest["serverDerivedTableSHA256"]=sha(derived.encode())
        # Six green-card fragment rows are a user-approved local fallback.
        frag_name="assets/android/tabledata/tablefrag"
        frag_env=UnityPy.load(apk.read(frag_name))
        frag_asset=next(o for o in frag_env.objects if o.type.name=="TextAsset")
        frag_data=frag_asset.read_typetree()
        frag_text=frag_data["m_Script"]
        if isinstance(frag_text,bytes):frag_text=frag_text.decode()
        frag_lines=frag_text.splitlines()
        for ident in PAIRS:
            assert not any(line.startswith("frag_"+ident+"\t") for line in frag_lines[5:])
            frag_lines.append("\t".join(["frag_"+ident,ident,"10","0"]))
        frag_derived="\n".join(frag_lines)+"\n"
        frag_data["m_Script"]=frag_derived
        frag_asset.save_typetree(frag_data)
        changes[frag_name]=frag_env.file.save(packer="lz4")
        assert next(o.read_typetree()["m_Script"] for o in UnityPy.load(changes[frag_name]).objects if o.type.name=="TextAsset")==frag_derived
        (output/"TableFrag.tsv").write_text(frag_derived)
        manifest["localFragmentRule"]=dict(combineCount=10,sellPrice=0,originalRuleRecovered=False)
        models={}
        donor=apk.read("assets/android/model/monster_070")
        for model in TRANSPLANTS:
            name="assets/android/model/"+model
            changes[name], models[model]=transplant(apk.read(name),donor)
        manifest["modelTransplants"]=models
        raw=apk.read("assets/android/smallassetmanifest")
        lines=raw.decode().splitlines()
        for index in range(1,len(lines)):
            path,digest,size=lines[index].split("\t")
            if "assets/android/"+path in changes:
                value=changes["assets/android/"+path]
                lines[index]="\t".join([path,hashlib.md5(value).hexdigest(),str(len(value))])
        changes["assets/android/smallassetmanifest"]=("\n".join(lines)+"\n").encode()
        for name,value in changes.items():
            path=output/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(value)
            manifest["files"].append(dict(entry=name,path=str(path.relative_to(ROOT)),beforeSHA256=sha(apk.read(name)),sha256=sha(value),bytes=len(value)))
    (output/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
    print(json.dumps(dict(status=manifest["status"],characters=len(manifest["characters"]),changedEntries=len(manifest["files"]),manifest=str(output/"manifest.json"))))

if __name__=="__main__":main()
