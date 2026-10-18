"""锁外编译P13固定服务/Go-race，原协议专库锁内仅运行；源码全SHA验证。"""
import argparse,hashlib,json,subprocess,tempfile,shutil
from pathlib import Path

def prepare(source):
    if not source.name.startswith('hyjs-p13-field-'):raise AssertionError('fixed P13 source guard')
    directory=Path(tempfile.mkdtemp(prefix='hyjs-p13-prebuilt-'))
    class Checks:
        native_root=None
        def assertEqual(self,a,b,*why):
            if a!=b:raise AssertionError(why)
        def assertIn(self,a,b,*why):
            if a not in b:raise AssertionError(why)
    self=Checks();self.temp=type('Temp',(),{'name':str(directory)})();self.binary=str(directory/'hyjs-native');SOURCE=source
    self.native_root = Path(self.temp.name) / 'native-source'
    shutil.copytree(SOURCE / 'hyjs-server', self.native_root / 'hyjs-server')
    main_path = self.native_root / 'hyjs-server/main.go'
    main = main_path.read_text()
    main = main.replace('"hyjs-server/config"', '"hyjs-server/config"\n"hyjs-server/constUtil"\n"strings"', 1)
    marker = 'defer cancel()\n'
    self.assertEqual(main.count(marker), 1)
    main = main.replace(marker, marker + '''
    // 仅本测试编译副本：受专属库名称保护的服务器内部可控时钟。
    if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_") { return errors.New("native field isolation guard") }
    ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time {
    raw, err := os.ReadFile(os.Getenv("HYJS_P13_FIELD_NATIVE_CLOCK"))
    if err != nil { panic(err) }
    at, err := strconv.ParseInt(string(raw), 10, 64)
    if err != nil { panic(err) }
    return time.UnixMilli(at)
    })
''', 1)
    main = main.replace('errs := make(chan error, 6)', 'errs := make(chan error, 7)').replace('remaining := 6', 'remaining := 7')
    if 'errs <- s.ServeP13UnionFightField(ctx)' not in main:
        main = main.replace('go func() { errs <- s.ServeP13UnionFight(ctx) }()', 'go func() { errs <- s.ServeP13UnionFight(ctx) }()\ngo func() { errs <- s.ServeP13UnionFightField(ctx) }()', 1)
    main_path.write_text(main)
    scene_path = self.native_root / 'hyjs-server/service/p10SceneService.go'
    scene = scene_path.read_text()
    if 'if handled, err := s.SyncP13UnionFightField(ctx, id, movement);' not in scene:
        scene = scene.replace('if handled, err := s.SyncP13GuildRoom(ctx, id, movement);', 'if handled, err := s.SyncP13UnionFightField(ctx, id, movement); handled || err != nil { return err }\nif handled, err := s.SyncP13GuildRoom(ctx, id, movement);', 1)
    if 'if s.DisconnectP13UnionFightField(ctx, id, session)' not in scene:
        scene = scene.replace('if s.DisconnectP13GuildRoom(ctx, id, session)', 'if s.DisconnectP13UnionFightField(ctx, id, session) { return }\nif s.DisconnectP13GuildRoom(ctx, id, session)', 1)
    scene_path.write_text(scene)
    handler_path = self.native_root / 'hyjs-server/handler/gameHandler.go'
    handler = handler_path.read_text().replace('b.GetTarget() != constUtil.P13GuildRoomTarget {', 'b.GetTarget() != constUtil.P13GuildRoomTarget && b.GetTarget() != constUtil.P13UnionFightFieldTarget {', 1)
    self.assertIn('b.GetTarget() != constUtil.P13UnionFightFieldTarget', handler, '场景原转场白名单未接入')
    handler_path.write_text(handler)
    result = subprocess.run(['rtk', 'proxy', 'gofmt', '-w', str(main_path), str(scene_path), str(handler_path)], capture_output=True, text=True, timeout=30)
    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
    result = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', self.binary, '.'], cwd=self.native_root / 'hyjs-server', capture_output=True, text=True, timeout=180)
    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
    files={}
    for base in ['hyjs-server','analysis/tables']:
        for path in sorted((source/base).rglob('*')):
            if path.is_file():files[str(path.relative_to(source))]=hashlib.sha256(path.read_bytes()).hexdigest()
    go_binary=directory/'service-race.test'
    result=subprocess.run(['rtk','proxy','go','test','-race','-c','-o',str(go_binary),'./service'],cwd=source/'hyjs-server',capture_output=True,text=True,timeout=240)
    if result.returncode:raise AssertionError(result.stdout+result.stderr)
    manifest={'files':files,'native_sha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'race_sha256':hashlib.sha256(go_binary.read_bytes()).hexdigest()}
    (directory/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
    return directory

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--source-root',type=Path,required=True);args=parser.parse_args()
    print(prepare(args.source_root))
