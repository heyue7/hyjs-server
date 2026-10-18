"""One Awake difficulty-row signed APK candidate; never promote or install."""
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import time
import zipfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.build_wallet_client import DLL,BRIDGE,_certificate,_hash_entry,_signature,digest,run
from tools.awake_diff_rows.build import BASE_APK,BASE_DLL,verify
from tools.local_asset_ready.build import SCOPE,SCOPE_SHA

HERE=ROOT/'analysis/p7/awake-diff-rows-r1-20261017'
BASE=ROOT/'build/hyjs-local.apk'
ROLLBACK=ROOT/'build/hyjs-rollback.apk'
PROOF=ROOT/'analysis/growth-first-20261016/function-pet-equip-five-r1-20261017/apk-candidate-build-r1.json'
PROMOTION=ROOT/'analysis/growth-first-20261016/function-pet-equip-five-r1-20261017/apk-promotion-r1.json'
OVERLAY=HERE/'overlay-r1'
OUTPUT=ROOT/'build/hyjs-awake-diff-rows-candidate-20261017.apk'
RECORD=HERE/'apk-candidate-build-r2.json'

def verify_base(archive,proof,expected):
    # P9 qualifies an independent candidate, then records its separate promotion.
    # Validate both real records without rewriting their historical statuses.
    promotion=json.loads(PROMOTION.read_bytes())
    if (proof.get('status')!='PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED'
            or proof.get('binarySHA256')!=expected
            or not proof.get('signatureVerified') or not proof.get('zipalignVerified')
            or promotion.get('currentSHA256')!=expected
            or promotion.get('current')!='build/hyjs-local.apk'
            or promotion.get('sameAsVerifiedInstalledAPK') is not True):
        raise ValueError('Exact P9 candidate qualification and promotion required')
    engine=proof.get('bridgeProof',{})
    entries=proof.get('qualifiedWalletEntries',{})
    required={DLL,BRIDGE,'AndroidManifest.xml','classes2.dex','assets/local-wallet.json','assets/local-wallet/icon.png'}
    if (engine.get('originalEngineMethods')!=547
            or not engine.get('originalEngineInstructionBytesAndExceptionRegionsUnchanged')
            or not required.issubset(entries)
            or entries[DLL]!=BASE_DLL or entries[DLL]!=engine.get('outputSHA256')
            or entries[BRIDGE]!=engine.get('bridgeSHA256')):
        raise ValueError('Inherited Engine/managed/wallet proof differs')
    for name,value in entries.items():
        if name not in archive.namelist() or _hash_entry(archive,name)!=value:
            raise ValueError('Inherited qualified entry differs: '+name)
    return entries

def main():
    if OUTPUT.exists() or RECORD.exists():raise ValueError('Preserve prior candidate and attempt evidence')
    module=json.loads((OVERLAY/'manifest.json').read_bytes());proof=json.loads(PROOF.read_bytes())
    dll=(OVERLAY/'Assembly-CSharp.dll').read_bytes()
    if hashlib.sha256(dll).hexdigest()!=module['outputManagedSHA256'] or module['inputManagedSHA256']!=BASE_DLL:raise ValueError('Frozen Awake difficulty-row DLL differs')
    if digest(BASE)!=BASE_APK or digest(SCOPE)!=SCOPE_SHA:raise ValueError('Qualified APK/scope differs')
    current_before=digest(BASE);rollback_before=digest(ROLLBACK)
    android=Path(os.environ.get('ANDROID_SDK_ROOT',Path.home()/'Library/Android/sdk'))/'build-tools/33.0.0'
    java=Path.home()/'Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home'
    key=ROOT/'.local-signing/debug.keystore'
    for p in [android/'zipalign',android/'apksigner',java/'bin/java',key]:
        if not p.is_file():raise ValueError('Existing signing tool missing: '+str(p))
    env=dict(os.environ,JAVA_HOME=str(java),PATH=str(java/'bin')+os.pathsep+os.environ.get('PATH',''))
    report=dict(status='PENDING',mode='awake-diff-rows-ui-candidate-only',baseAPK=str(BASE.relative_to(ROOT)),baseSHA256=current_before,
        baseProof=str(PROOF.relative_to(ROOT)),baseProofSHA256=digest(PROOF),managedOverlay=str(OVERLAY.relative_to(ROOT)),
        basePromotionProof=str(PROMOTION.relative_to(ROOT)),basePromotionProofSHA256=digest(PROMOTION),
        managedOverlaySHA256=digest(OVERLAY/'manifest.json'),sourceSHA256=digest(Path(__file__)),installed=False,phoneUI=False,
        currentAPKModified=False,rollbackAPKModified=False,adbUsed=False)
    began=time.monotonic();temporary=None
    with (ROOT/'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        try:
            if shutil.disk_usage(BASE.parent).free<BASE.stat().st_size*3+250*1024*1024:raise ValueError('Insufficient free space for owned intermediates')
            certificate=_certificate(run([android/'apksigner','verify','--verbose','--print-certs',BASE],env))
            if certificate!=proof['signerCertificateSHA256']:raise ValueError('Base signing identity differs')
            inherited={};compression={};selected={r['entry'] for r in json.loads(SCOPE.read_bytes())['resources']}
            with tempfile.TemporaryDirectory(prefix='hyjs-awake-diff-rows-apk-',dir=ROOT/'build') as directory:
                temporary=Path(directory)
                unsigned,aligned,signed=[temporary/n for n in ['unsigned.apk','aligned.apk','signed.apk']]
                with zipfile.ZipFile(BASE) as incoming:
                    qualified=dict(verify_base(incoming,proof,current_before))
                    names=incoming.namelist()
                    if len(names)!=len(set(names)):raise ValueError('Duplicate input ZIP entries')
                    checks,_,_=verify(incoming.read(DLL),dll)
                    if any(module.get(k)!=v for k,v in checks.items()):raise ValueError('Fresh Engine/method comparison differs')
                    if len(selected)!=2680 or any(incoming.getinfo(n).compress_type!=zipfile.ZIP_STORED for n in selected):raise ValueError('Original 501 STORED closure differs')
                    with zipfile.ZipFile(unsigned,'w') as outgoing:
                        outgoing.comment=incoming.comment
                        for info in incoming.infolist():
                            if _signature(info.filename):continue
                            compression[info.filename]=info.compress_type
                            if info.filename==DLL:
                                outgoing.writestr(copy.copy(info),dll);continue
                            h=hashlib.sha256()
                            with incoming.open(info) as source,outgoing.open(copy.copy(info),'w') as destination:
                                while chunk:=source.read(1024*1024):h.update(chunk);destination.write(chunk)
                            inherited[info.filename]=h.hexdigest()
                run([android/'zipalign','-f','-p','4',unsigned,aligned],env)
                run([android/'apksigner','sign','--ks',key,'--ks-pass','pass:android','--key-pass','pass:android','--out',signed,aligned],env)
                if _certificate(run([android/'apksigner','verify','--verbose','--print-certs',signed],env))!=certificate:raise ValueError('Candidate signer differs')
                run([android/'zipalign','-c','-p','4',signed],env)
                expected=dict(inherited,**{DLL:module['outputManagedSHA256']})
                with zipfile.ZipFile(signed) as checked:
                    names=checked.namelist()
                    if len(names)!=len(set(names)) or {n for n in names if not _signature(n)}!=set(expected):raise ValueError('Candidate entry set differs')
                    for name,value in expected.items():
                        if _hash_entry(checked,name)!=value or checked.getinfo(name).compress_type!=compression[name]:raise ValueError('Inherited payload/compression differs: '+name)
                    with zipfile.ZipFile(BASE) as old_manifest:
                        if checked.read('AndroidManifest.xml')!=old_manifest.read('AndroidManifest.xml'):raise ValueError('Package/manifest changed')
                if digest(BASE)!=current_before or digest(ROLLBACK)!=rollback_before or digest(PROOF)!=report['baseProofSHA256'] or digest(PROMOTION)!=report['basePromotionProofSHA256']:raise ValueError('Base/rollback/proof changed during build')
                qualified[DLL]=module['outputManagedSHA256']
                engine=dict(proof['bridgeProof']);engine.update(mode='awake-diff-rows-ui-candidate',inputSHA256=BASE_DLL,outputSHA256=module['outputManagedSHA256'],freshEngineComparison=True,originalEngineMethods=547,originalEngineInstructionBytesAndExceptionRegionsUnchanged=True,onlyChangedEntries=checks['onlyChangedMethods'])
                report.update(status='PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED',binarySHA256=digest(signed),outputAPK=str(OUTPUT.relative_to(ROOT)),
                    signatureVerified=True,zipalignVerified=True,signerCertificateSHA256=certificate,bridgeProof=engine,
                    managedMethodProof=checks,awakeDiffRowsOverlayProof=module,petEquipFiveOverlayProof=proof['petEquipFiveOverlayProof'],qualifiedWalletEntries=qualified,replacementEntries={DLL:module['outputManagedSHA256']},
                    manifestProof=proof['manifestProof'],modernIME=proof.get('modernIME'),modernIMEInheritedFromBase=True,
                    awakeGateRemovalProof=proof['awakeGateRemovalProof'],allOtherInheritedEntriesExact=True,inheritedEntryCount=len(inherited),
                    inheritedEntrySHA256=hashlib.sha256(json.dumps(inherited,sort_keys=True,separators=(',',':')).encode()).hexdigest(),
                    allNonSignatureCompressionMethodsPreserved=True,storedClosureEntries=len(selected),resourceScopeSHA256=SCOPE_SHA,
                    unchangedManifestAndPackage=True,baseClassesDexByteIdentical=True,packageSizeBytes=signed.stat().st_size,
                    directRollbackAPK=str(BASE.relative_to(ROOT)),directRollbackSHA256=current_before,
                    preservedExistingRollbackAPK=str(ROLLBACK.relative_to(ROOT)),preservedExistingRollbackSHA256=rollback_before)
                signed.replace(OUTPUT)
                idsig=signed.with_suffix('.apk.idsig')
                if idsig.exists():idsig.replace(OUTPUT.with_suffix('.apk.idsig'))
        except BaseException as error:
            report.update(status='FAILED_NOT_QUALIFIED',errorType=type(error).__name__,error=str(error)[-4000:]);raise
        finally:
            report.update(ownedTemporaryFilesCleaned=temporary is None or not temporary.exists(),seconds=round(time.monotonic()-began,3))
            RECORD.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','outputAPK','binarySHA256','inheritedEntryCount','storedClosureEntries','ownedTemporaryFilesCleaned','seconds']}))

if __name__=='__main__':main()
