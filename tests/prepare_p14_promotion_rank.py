"""Build a fixed P14 Rank native binary before the SQL window."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile


def prepare(source):
    source = source.resolve()
    if not source.name.startswith("hyjs-p14-promotion-rank-"):
        raise AssertionError("fixed promotion source guard")
    directory = Path(tempfile.mkdtemp(prefix="hyjs-p14-rank-prebuilt-"))
    try:
        server = directory / "server"
        shutil.copytree(source / "hyjs-server", server)
        main_file = server / "main.go"
        main = main_file.read_text()
        main = main.replace('"hyjs-server/config"', '"hyjs-server/config"\n"hyjs-server/constUtil"\n"strings"', 1)
        marker = "defer cancel()\n"
        assert main.count(marker) == 1
        main = main.replace(marker, marker + '''
        // Only this compiled test copy binds an internal clock. No HTTP value can set it.
        if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_") { return errors.New("promotion native isolation guard") }
        ctx = context.WithValue(ctx, constUtil.P14CrossClockContextKey{}, func() time.Time {
            raw, err := os.ReadFile(os.Getenv("HYJS_P14_PROMOTION_CLOCK")); if err != nil { panic(err) }
            at, err := strconv.ParseInt(string(raw),10,64); if err != nil { panic(err) }
            if at == 0 { return time.Now() }; return time.UnixMilli(at)
        })
''', 1)
        main_file.write_text(main)
        handler_file = server / "handler/gameHandler.go"
        handler = handler_file.read_text()
        assert 'case *pb.CSKFRankInfo:' in handler and 'case *pb.CSKuafuMatchPlayer:' in handler
        subprocess.run(["rtk","proxy","gofmt","-w",str(main_file),str(handler_file)], check=True, capture_output=True, text=True, timeout=30)
        binary = directory / "hyjs-native"
        result = subprocess.run(["rtk","proxy","go","build","-o",str(binary),"."],cwd=server,capture_output=True,text=True,timeout=240)
        if result.returncode:
            raise AssertionError(result.stdout + result.stderr)
        files = {}
        for base in ["hyjs-server","analysis/tables"]:
            for path in sorted((source / base).rglob("*")):
                if path.is_file() and path.name != "app.log":
                    files[str(path.relative_to(source))] = hashlib.sha256(path.read_bytes()).hexdigest()
        proof = {"files":files,"nativeSHA256":hashlib.sha256(binary.read_bytes()).hexdigest(),"derivedMainSHA256":hashlib.sha256(main_file.read_bytes()).hexdigest(),"derivedHandlerSHA256":hashlib.sha256(handler_file.read_bytes()).hexdigest(),"clockScope":"isolated test compiled main only; no request field/header binding"}
        (directory / "manifest.json").write_text(json.dumps(proof,indent=2)+"\n")
        shutil.rmtree(server)
        return directory
    except BaseException:
        shutil.rmtree(directory,ignore_errors=True)
        raise


if __name__ == "__main__":
    parser=argparse.ArgumentParser()
    parser.add_argument("--source-root",type=Path,required=True)
    print(prepare(parser.parse_args().source_root))
