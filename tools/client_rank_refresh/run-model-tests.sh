#!/bin/sh
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/hyjs-rank-model.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
cat > "$test_dir/RankModels.csproj" <<EOF
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework>
    <LangVersion>3</LangVersion><ImplicitUsings>disable</ImplicitUsings>
    <Nullable>disable</Nullable><EnableDefaultCompileItems>false</EnableDefaultCompileItems>
    <GenerateAssemblyInfo>false</GenerateAssemblyInfo><GenerateTargetFrameworkAttribute>false</GenerateTargetFrameworkAttribute>
  </PropertyGroup>
  <ItemGroup>
    <Compile Include="$here/Runtime.cs" /><Compile Include="$here/RuntimeModels.cs" />
    <Compile Include="$here/ModelTests.cs" />
  </ItemGroup>
</Project>
EOF
"${DOTNET:-dotnet}" run --project "$test_dir/RankModels.csproj" --configuration Release --verbosity quiet
