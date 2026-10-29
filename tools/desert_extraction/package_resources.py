"""Strict two-entry desert resource composition for package_native."""
import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[2]
BASE_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
BUNDLE_SHA = '2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1'
BUNDLE_BYTES = 10846378
CATALOG_ENTRY = 'assets/android/smallassetmanifest'
LOGICAL = b'map/map_desertfield_001'
BUNDLE_ENTRY = 'assets/android/hyjs/desert53/' + BUNDLE_SHA + '/map_desertfield_001'

def sha(data):
    return hashlib.sha256(data).hexdigest()

def sha_file(path):
    h=hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''): h.update(block)
    return h.hexdigest()

def require(ok, reason):
    if not ok: raise ValueError(reason)

def compose_catalog(before, bundle):
    require(sha(bundle)==BUNDLE_SHA and len(bundle)==BUNDLE_BYTES, 'Desert bundle identity differs')
    lines=before.splitlines(keepends=True)
    require(bool(lines) and lines[0].strip()==b'3000', 'Resource catalog format differs')
    require(not any(line.split(b'\t')[0]==LOGICAL for line in lines), 'Desert logical catalog entry already exists')
    row=LOGICAL+b'\t'+hashlib.md5(bundle).hexdigest().encode()+b'\t'+str(len(bundle)).encode()+b'\n'
    return before+(b'\n' if not before.endswith(b'\n') else b'')+row

def apply_overlay(base, directory, payload, pins):
    """Validate all evidence before adding exactly one catalog and one map."""
    base,directory=Path(base).resolve(),Path(directory).resolve()
    require(directory.is_relative_to(ROOT), 'Desert overlay outside project')
    require(sha_file(base)==BASE_SHA, 'Desert resource base differs')
    manifest=directory/'resource-manifest.json'
    report=json.loads(manifest.read_text())
    require(report.get('schema')=='hyjs-desert-v4-map-resource-overlay-v1' and report.get('baseAPKSHA256')==BASE_SHA, 'Unqualified desert resource overlay')
    require(report.get('bundleSHA256')==BUNDLE_SHA and report.get('bundleBytes')==BUNDLE_BYTES, 'Desert map pin differs')
    files=report.get('payloadFiles',{})
    require(set(files)=={CATALOG_ENTRY,BUNDLE_ENTRY} and set(report.get('replacementEntries',{}))==set(files), 'Desert resource scope differs')
    local={}
    for name,relative in files.items():
        f=(ROOT/relative).resolve(); require(f.is_relative_to(directory) and f.is_file(), 'Desert payload outside overlay')
        data=f.read_bytes(); require(sha(data)==report['replacementEntries'][name], 'Desert payload SHA differs: '+name)
        require(name not in payload, 'Overlapping desert payload: '+name); local[name]=data
    require(bool(report.get('sourcePins')), 'Desert resource source pins missing')
    checked={}
    for relative,h in report['sourcePins'].items():
        f=(ROOT/relative).resolve(); require(f.is_relative_to(ROOT) and f.is_file() and sha(f.read_bytes())==h, 'Desert source pin differs: '+relative)
        require(f not in pins or pins[f]==h, 'Conflicting desert source pin: '+relative); checked[f]=h
    with zipfile.ZipFile(base) as z:
        names=z.namelist(); require(len(names)==len(set(names)) and CATALOG_ENTRY in names and BUNDLE_ENTRY not in names, 'Desert base entry inventory differs')
        old=z.read(CATALOG_ENTRY); expected=compose_catalog(old,local[BUNDLE_ENTRY])
        require(sha(old)==report['originalCatalogSHA256'] and sha(expected)==report['updatedCatalogSHA256'] and local[CATALOG_ENTRY]==expected, 'Desert catalog append differs')
    pins.update(checked); pins[manifest]=sha(manifest.read_bytes()); payload.update(local)
    return {'schema':report['schema'],'bundleSHA256':BUNDLE_SHA,'bundleBytes':BUNDLE_BYTES,
            'originalCatalogSHA256':report['originalCatalogSHA256'],'updatedCatalogSHA256':report['updatedCatalogSHA256'],
            'onlyExactAppendAndMapEntry':True,'sourcePinsVerified':True,'deviceExecuted':False}
