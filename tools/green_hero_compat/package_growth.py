"""Package the fixed green resources and first-round UI guard on a qualified APK.

Produces one verified candidate; current/rollback APK and phone stay untouched.
The shared wallet builder owns the CLI and all preexisting signing helpers.
"""
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tempfile
import time
import zipfile

ROOT=Path(__file__).resolve().parents[2]
CONFIG="assets/local-resource-overrides.json"
EXPECTED_RESOURCES={"assets/android/tabledata/tablehero","assets/android/tabledata/tablefrag","assets/android/model/lingnai01_body","assets/android/model/sicary01_body","assets/android/model/nio01_body","assets/android/smallassetmanifest"}
GUARDS={"Battle.View.BattleViewCtrl::IsCanTransfer","Battle.View.BattleViewCtrl::AskTransEnd"}


def _green_resources(directory, archive, base_sha):
    from tools import build_wallet_client as common
    manifest_path=directory/"manifest.json"
    module=json.loads(manifest_path.read_text())
    if (module.get("schema")!="green-six-compatible-asset-overlay-v1"
        or module.get("status")!="STATIC_RESOURCE_VERIFIED_RUNTIME_PENDING"
        or module.get("baseAPKSHA256")!=base_sha
        or module.get("userApprovedLocalCompatibility") is not True
        or module.get("originalEngineMethodsChanged") is not False
        or {row.get("id")for row in module.get("characters",[])}!={"hero_%03d"%index for index in range(56,62)}
        or module.get("localFragmentRule")!={"combineCount":10,"sellPrice":0,"originalRuleRecovered":False}):
        raise ValueError("Green resource overlay contract differs")
    rows=module.get("files",[])
    if len(rows)!=6 or {r.get("entry")for r in rows}!=EXPECTED_RESOURCES:
        raise ValueError("Green overlay must contain exactly six resource entries")
    replacements={}
    for row in rows:
        path=(ROOT/row["path"]).resolve()
        if (not path.is_relative_to(directory) or common.digest(path)!=row["sha256"]
            or path.stat().st_size!=row["bytes"]
            or common._hash_entry(archive,row["entry"])!=row["beforeSHA256"]):
            raise ValueError("Fixed green resource differs: "+row["entry"])
        replacements[row["entry"]]=path.read_bytes()
    config_path=ROOT/"tools/green_hero_compat/local-resource-overrides.json"
    config=json.loads(config_path.read_text())
    expected=[dict(path=r["entry"].removeprefix("assets/android/"),sha256=r["sha256"],bytes=r["bytes"])for r in rows]
    if config!={"schema":"hyjs-green-resource-overrides-v1","version":1,"files":expected}:
        raise ValueError("Java resource configuration differs from fixed overlay")
    if expected[-1]["path"]!="smallassetmanifest":raise ValueError("Resource manifest must commit last")
    java=(ROOT/"tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java").read_text()
    java_pins=[dict(path=p,sha256=s,bytes=int(n))for p,s,n in re.findall(r'new Pin\("([^"\n]+)", "([0-9a-f]{64})", ([0-9]+)L\)',java)]
    if java_pins!=expected:raise ValueError("Compiled Java whitelist differs from fixed six resources")
    # The original loader reads md5/size from the small manifest. Verify every
    # changed payload entry appears exactly once with its actual new bytes.
    small=replacements["assets/android/smallassetmanifest"].decode().splitlines()
    if not small or small[0]!="3000":raise ValueError("Small resource manifest version differs")
    parsed={}
    for line in small[1:]:
        path,digest,size=line.split("\t")
        if path in parsed:raise ValueError("Duplicate resource manifest entry")
        parsed[path]=(digest,int(size))
    if len(parsed)!=7818:raise ValueError("Original resource inventory denominator differs")
    for entry,raw in replacements.items():
        if entry.endswith("/smallassetmanifest"):continue
        if parsed.get(entry.removeprefix("assets/android/"))!=(hashlib.md5(raw).hexdigest(),len(raw)):
            raise ValueError("Resource manifest md5/size differs: "+entry)
    for model,value in module.get("modelTransplants",{}).items():
        if (model not in {"lingnai01_body","sicary01_body","nio01_body"}
            or value.get("meshMaterialTextureAndTransformObjectsUnchanged") is not True
            or len(value.get("addedAnimationClips",[]))!=11):
            raise ValueError("Approved model animation transplant contract differs")
    if len(module.get("modelTransplants",{}))!=3:raise ValueError("Three NPC animation transplants required")
    replacements[CONFIG]=config_path.read_bytes()
    return replacements,module


def build_growth_overlay(args):
    from tools import build_wallet_client as common
    from tools.green_hero_compat.build_formation_guard import verify as verify_guard
    base=args.base_apk.resolve();ui=args.ui.resolve();record=args.record.resolve()
    green=args.green_hero_overlay.resolve();guard=args.formation_guard.resolve()
    proof_path=args.base_wallet_proof.resolve()
    current=ROOT/"build/hyjs-local.apk";rollback=ROOT/"build/hyjs-rollback.apk"
    candidate=ROOT/"build/hyjs-growth-candidate.apk"
    if (base not in {current,rollback} or not re.fullmatch(r"[0-9a-f]{64}",args.base_sha256)
        or not ui.is_relative_to(ROOT/"build") or not green.is_relative_to(ROOT/"analysis")
        or not guard.is_relative_to(ROOT/"analysis") or not proof_path.is_relative_to(ROOT/"analysis")
        or not record.is_relative_to(ROOT/"analysis") or record.exists()):
        raise ValueError("Use fixed local input APK/proofs, owned UI build and new evidence record")
    sdk=Path(os.environ.get("ANDROID_SDK_ROOT",Path.home()/"Library/Android/sdk"))
    android=sdk/"build-tools/33.0.0"
    java_home=Path.home()/"Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home"
    key=ROOT/".local-signing/debug.keystore"
    if not key.is_file():raise ValueError("Existing project signing identity is required")
    env=dict(os.environ,JAVA_HOME=str(java_home),PATH=str(java_home/"bin")+os.pathsep+os.environ.get("PATH",""))
    began=time.monotonic();temporary_path=None
    packager_sha=common.digest(Path(__file__))
    report=dict(status="PENDING",mode="green-growth-resources-and-round-one-formation",baseAPK=str(base.relative_to(ROOT)),baseSHA256=args.base_sha256,phoneUI=False,installed=False,promoted=False,originalAPKRepatchedFromFlags=False,ownedTemporaryFilesCleaned=False,packagerSource="tools/green_hero_compat/package_growth.py",packagerSourceSHA256=packager_sha)
    with (ROOT/"build/.client-build.lock").open("a") as lock:
        fcntl.flock(lock,fcntl.LOCK_EX)
        try:
            current_before=common.digest(current)
            if common.digest(base)!=args.base_sha256:raise ValueError("Fixed base APK changed")
            if candidate.exists() or candidate.with_suffix(".apk.idsig").exists():raise ValueError("Preserve existing growth candidate; use an explicitly cleaned build output")
            if shutil.disk_usage(current.parent).free < base.stat().st_size*3+200*1024*1024:raise ValueError("Insufficient owned build workspace")
            proof=json.loads(proof_path.read_text())
            module=json.loads((guard/"manifest.json").read_text())
            if (module.get("schema")!="local-first-round-formation-view-guard-v1" or module.get("status")!="PASS_OFFLINE_NOT_PACKAGED"
                or set(module.get("allowedChangedMethods",[]))!=GUARDS
                or module.get("originalEngineMethodsUnchanged")!=547 or module.get("original547EngineMethodsChanged") is not False
                or module.get("actualManagedGuardStates")!=16 or module.get("rejectedSubmitsWithoutSceneOrEngine")!=12
                or module.get("replacementEntries")!={common.DLL:module.get("outputManagedSHA256")}):
                raise ValueError("Formation guard qualification contract differs")
            for name,expected in module["sourcePins"].items():
                source=(ROOT/name).resolve()
                if not source.is_relative_to(ROOT/"tools/green_hero_compat") or common.digest(source)!=expected:raise ValueError("Fixed formation guard source changed")
            replacement_dll=guard/"Assembly-CSharp.dll"
            if common.digest(replacement_dll)!=module["outputManagedSHA256"]:raise ValueError("Fixed guarded DLL changed")
            ui_proof=json.loads((ui/"manifest.json").read_text())
            common.verify_wallet_ui(ui/"classes2.dex",ui/"AndroidManifest.wallet.xml")
            if common.digest(ui/"classes2.dex")!=ui_proof.get("dexSHA256") or not ui_proof.get("iconIncluded") or common.digest(ui/"assets/local-wallet/icon.png")!=ui_proof.get("iconSHA256"):raise ValueError("Wallet UI dex/icon proof differs")
            source_names=set()
            for source in ui_proof["sources"]:
                path=(ROOT/source["path"]).resolve();source_names.add(path)
                green_java=ROOT/"tools/green_hero_compat/java"
                if (not(path.is_relative_to(ROOT/"tools/local_wallet") or path.is_relative_to(green_java) or path==ROOT/"tools/green_hero_compat/local-resource-overrides.json") or common.digest(path)!=source["sha256"]):raise ValueError("Fixed wallet/green Java source changed")
            java=ROOT/"tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java"
            if java not in source_names:raise ValueError("Wallet build lacks the fixed resource refresh source")
            activity=(ROOT/"tools/local_wallet/src/com/hyjs/wallet/WalletActivity.java").read_text()
            sync=re.search(r"LocalAssetOverlay\.synchronize\(this\)",activity)
            super_call=re.search(r"super\.onCreate\(state\)",activity)
            if not sync or not super_call or sync.start()>=super_call.start():raise ValueError("Resource refresh must execute before Unity startup")
            certificate=common._certificate(common.run([android/"apksigner","verify","--print-certs",base],env))
            if certificate!=proof["signerCertificateSHA256"]:raise ValueError("Base signing identity differs from proof")
            with tempfile.TemporaryDirectory(prefix="hyjs-green-growth-apk-",dir=ROOT/"build") as directory:
                temp=temporary_path=Path(directory)
                unsigned,aligned,signed=[temp/name for name in ["unsigned.apk","aligned.apk","signed.apk"]]
                inherited={}
                with zipfile.ZipFile(base) as incoming:
                    qualified=common.verify_wallet_upgrade_base(incoming,proof,args.base_sha256,common.wallet_base_promotion(args))
                    names=incoming.namelist()
                    if len(names)!=len(set(names)) or common._hash_entry(incoming,common.DLL)!=module["inputManagedSHA256"]:raise ValueError("Base APK entries/managed input differ")
                    checks=verify_guard(incoming.read(common.DLL),replacement_dll.read_bytes())
                    for name,value in checks.items():
                        if module.get(name)!=value:raise ValueError("Recomputed formation guard proof differs: "+name)
                    replacements,green_module=_green_resources(green,incoming,args.base_sha256)
                    replacements[common.DLL]=replacement_dll.read_bytes()
                    replacements["classes2.dex"]=(ui/"classes2.dex").read_bytes()
                    if (ui/CONFIG).read_bytes()!=replacements[CONFIG]:raise ValueError("Packaged Java resource config differs")
                    for entry in ["assets/local-wallet.json","assets/local-wallet/icon.png"]:
                        if common.digest(ui/entry)!=common._hash_entry(incoming,entry):raise ValueError("Wallet config/icon must remain unchanged")
                    allowed=set(replacements)
                    if allowed!={common.DLL,"classes2.dex",CONFIG}|EXPECTED_RESOURCES:raise ValueError("Growth overlay entry whitelist differs")
                    with zipfile.ZipFile(unsigned,"w") as outgoing:
                        outgoing.comment=incoming.comment
                        for info in incoming.infolist():
                            if common._signature(info.filename):continue
                            if info.filename in replacements:outgoing.writestr(copy.copy(info),replacements[info.filename]);continue
                            hash_value=hashlib.sha256()
                            with incoming.open(info) as source,outgoing.open(copy.copy(info),"w") as destination:
                                while block:=source.read(1024*1024):hash_value.update(block);destination.write(block)
                            inherited[info.filename]=hash_value.hexdigest()
                        for name in sorted(allowed-set(names)):
                            info=zipfile.ZipInfo(name,date_time=(1980,1,1,0,0,0));info.compress_type=zipfile.ZIP_DEFLATED;outgoing.writestr(info,replacements[name])
                common.run([android/"zipalign","-f","-p","4",unsigned,aligned],env)
                common.run([android/"apksigner","sign","--ks",key,"--ks-pass","pass:android","--key-pass","pass:android","--out",signed,aligned],env)
                if common._certificate(common.run([android/"apksigner","verify","--verbose","--print-certs",signed],env))!=certificate:raise ValueError("Candidate signing identity differs")
                common.run([android/"zipalign","-c","-p","4",signed],env)
                hashes={n:hashlib.sha256(v).hexdigest()for n,v in replacements.items()}
                with zipfile.ZipFile(signed) as verified:
                    names=verified.namelist()
                    if len(names)!=len(set(names)) or {n for n in names if not common._signature(n)}!=set(inherited)|allowed:raise ValueError("Signed candidate entry set differs")
                    for name,expected in dict(inherited,**hashes).items():
                        if common._hash_entry(verified,name)!=expected:raise ValueError("Candidate entry differs: "+name)
                if common.digest(Path(__file__))!=packager_sha:raise ValueError("Packager source changed during candidate build")
                if common.digest(current)!=current_before or common.digest(base)!=args.base_sha256:raise ValueError("Current/base APK changed during candidate build")
                qualified=dict(qualified);qualified.update(hashes)
                engine=dict(proof["bridgeProof"])
                engine.update(mode="green-growth-view-formation-guard",inputSHA256=module["inputManagedSHA256"],outputSHA256=module["outputManagedSHA256"],freshEngineComparison=True,managedAssemblyByteIdenticalToBase=False,onlyChangedEntries=sorted(GUARDS))
                report.update(baseWalletProof=str(proof_path.relative_to(ROOT)),baseWalletProofSHA256=common.digest(proof_path),ui=str(ui.relative_to(ROOT)),uiProofSHA256=common.digest(ui/"manifest.json"),greenHeroOverlay=str(green.relative_to(ROOT)),greenHeroOverlaySHA256=common.digest(green/"manifest.json"),formationGuard=str(guard.relative_to(ROOT)),formationGuardSHA256=common.digest(guard/"manifest.json"),greenHeroProof=green_module,formationGuardProof=module,bridgeProof=engine,manifestProof=proof["manifestProof"],modernIME=proof.get("modernIME"),modernIMEInheritedFromBase=True,qualifiedWalletEntries=qualified,replacementEntries=hashes,managedAssemblyByteIdenticalToBase=False,allOtherInheritedEntriesExact=True,inheritedEntryCount=len(inherited),inheritedEntrySHA256=hashlib.sha256(json.dumps(inherited,sort_keys=True,separators=(",",":")).encode()).hexdigest(),signerCertificateSHA256=certificate,binarySHA256=common.digest(signed),outputAPK=str(candidate.relative_to(ROOT)),signatureVerified=True,zipalignVerified=True,originalClassesDexSHA256=proof.get("originalClassesDexSHA256"),classesDexSHA256=inherited.get("classes.dex"),baseClassesDexByteIdentical=True,AndroidManifestByteIdentical=True,currentAPKUnchanged=True,resourceRefreshBeforeUnity=True,resourceRefreshWhitelistEntries=6,walletResourceRefreshUpgraded=True,inheritedOriginalResourceInventoryRequalified=False)
                promotion=getattr(args,"base_promotion_proof",None)
                if promotion:report.update(basePromotionProof=str(promotion.resolve().relative_to(ROOT)),basePromotionProofSHA256=common.digest(promotion))
                for name in ["awakeEndRecoveryOverlayProof","activityTitleOverlayProof","formationOverlayProof","resourceOverlayProof","awakeGateRemovalProof"]:
                    if name in proof:report[name]=proof[name]
                signed.replace(candidate)
                idsig=signed.with_suffix(".apk.idsig")
                if idsig.exists():idsig.replace(candidate.with_suffix(".apk.idsig"))
                report["status"]="PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED"
        except BaseException as error:
            report.update(status="FAILED_NOT_QUALIFIED",errorType=type(error).__name__,error=str(error)[-6000:])
            raise
        finally:
            report.update(ownedTemporaryFilesCleaned=temporary_path is None or not temporary_path.exists(),seconds=round(time.monotonic()-began,3))
            record.parent.mkdir(parents=True,exist_ok=True);record.write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(json.dumps({k:report[k]for k in ["status","binarySHA256","inheritedEntryCount","outputAPK"]}))
