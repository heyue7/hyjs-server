"""Build only the two reviewed Finals UI CIL changes; Root packages/installs."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.p10_voice.build import verify

ENTRIES = {("ServerBattleFinals", "InitData"), ("ServerBattlePlayerUIItem", "isOwner")}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--input-sha256", required=True)
    parser.add_argument("--output-directory", required=True, type=Path)
    args = parser.parse_args()
    source = args.input.resolve(strict=True)
    expected = args.input_sha256.lower()
    if len(expected) != 64 or any(character not in "0123456789abcdef" for character in expected):
        raise ValueError("Expected input SHA256 must be exactly 64 hex characters")
    before = source.read_bytes()
    if hashlib.sha256(before).hexdigest() != expected:
        raise ValueError("Controlled Assembly input SHA256 differs")
    directory = args.output_directory.resolve()
    output = directory / "Assembly-CSharp.dll"
    if source == output or directory.exists() and any(directory.iterdir()):
        raise ValueError("Output must be a separate empty directory")
    directory.mkdir(parents=True, exist_ok=True)
    runtime = ROOT / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    try:
        with tempfile.TemporaryDirectory(prefix="hyjs-finals-ui-injector-") as temporary:
            temporary = Path(temporary)
            shutil.copyfile(Path(__file__).with_name("FinalsUIInjector.cs"), temporary / "Program.cs")
            project = temporary / "FinalsUIInjector.csproj"
            project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                               '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                               '</PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) +
                               '</HintPath></Reference></ItemGroup></Project>')
            for command in ([dotnet, "build", project, "--nologo", "--verbosity", "quiet"],
                            [dotnet, temporary / "bin/Debug/net8.0/FinalsUIInjector.dll", source, output]):
                subprocess.run(["rtk", "proxy", *map(str, command)], check=True, timeout=120)
        after = output.read_bytes()
        proof = verify(before, after, ENTRIES)
        if proof["originalEngineMethods"] != 547:
            raise AssertionError("Expected exactly all 547 original engine methods")
        # The common verifier excludes allowed bodies. Check their EH explicitly
        # and reject extra types/methods, so the exception-region claim is complete.
        import dnfile
        from dncil.cil.body.reader import read_method_body_from_bytes
        old, new = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
        if len(old.net.mdtables.TypeDef.rows) != len(new.net.mdtables.TypeDef.rows) or len(old.net.mdtables.MethodDef.rows) != len(new.net.mdtables.MethodDef.rows):
            raise AssertionError("Unexpected additional method or type")
        for owner in old.net.mdtables.TypeDef.rows:
            for entry in owner.MethodList:
                if (str(owner.TypeName), str(entry.row.Name)) not in ENTRIES:
                    continue
                original = read_method_body_from_bytes(old.get_data(entry.row.Rva))
                updated_row = new.net.mdtables.MethodDef.rows[entry.row_index - 1]
                updated = read_method_body_from_bytes(new.get_data(updated_row.Rva))
                if original.exception_handlers or updated.exception_handlers:
                    raise AssertionError("Reviewed target methods must retain their original empty EH")
        proof.update(inputSHA256=expected, outputSHA256=hashlib.sha256(after).hexdigest(),
                     targetExceptionRegionsUnchanged=True, noAddedMethodOrType=True,
                     scope="only original first-list missing-slot null and self pid+realm comparison", phoneVerified=False)
        (directory / "manifest.json").write_text(json.dumps(proof, indent=2) + "\n")
        print(json.dumps(proof))
    except BaseException:
        output.unlink(missing_ok=True)
        (directory / "manifest.json").unlink(missing_ok=True)
        raise


if __name__ == "__main__":
    main()
