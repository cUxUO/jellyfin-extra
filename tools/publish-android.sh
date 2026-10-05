#!/usr/bin/env bash
# 把已建置的 Android release 發布成 GitHub Release（repo 見 android/gradle.properties 的 updateRepo），供 app 自動更新。
# tag v<版本> 打在目前的 commit 上，所以要先提交並推送，建置也要用這個 commit。
# 在主機執行（需要 gh 並已 gh auth login、python3）；APK 先在容器內建置：
#   cd android && ./gradlew assembleRelease
# 用法：tools/publish-android.sh [版本說明檔]
# 版本號取自建置結果（app/build.gradle.kts 的 versionName），tag 是 v<版本>。
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
out=$root/android/app/build/outputs/apk/release
repo=$(sed -n 's/^updateRepo=//p' "$root/android/gradle.properties")
notes=${1:-}

if ! git -C "$root" diff --quiet HEAD -- android; then
	echo "android/ 有未提交的變更，請先提交、推送再重新建置" >&2
	exit 1
fi
commit=$(git -C "$root" rev-parse HEAD)
if ! git -C "$root" branch -r --contains "$commit" | grep -q .; then
	echo "目前的 commit 還沒推送到遠端" >&2
	exit 1
fi

meta=$out/output-metadata.json
if [[ ! -f $meta ]]; then
	echo "找不到 $meta，請先在容器內 ./gradlew assembleRelease" >&2
	exit 1
fi
version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["elements"][0]["versionName"])' "$meta")
tag=v$version

dist=$root/build/release/$version
rm -rf "$dist"
mkdir -p "$dist"
while read -r abi file; do
	src=$out/$file
	# 確認有簽章（APK Signing Block）；release 只會用發布金鑰簽，沒有金鑰時 Gradle 直接報錯
	if ! grep -qa 'APK Sig Block 42' "$src"; then
		echo "$file 沒有簽章" >&2
		exit 1
	fi
	cp "$src" "$dist/JellyfinExtra-$version-$abi.apk"
done < <(python3 -c 'import json,sys
for e in json.load(open(sys.argv[1]))["elements"]: print(e["filters"][0]["value"], e["outputFile"])' "$meta")

echo "發布 $tag 到 $repo："
ls -l "$dist"
args=(release create "$tag" --repo "$repo" --target "$commit" --title "Jellyfin Extra $version")
if [[ -n $notes ]]; then
	args+=(--notes-file "$notes")
else
	args+=(--notes "Jellyfin Extra $version")
fi
gh "${args[@]}" "$dist"/*.apk
echo "完成：https://github.com/$repo/releases/tag/$tag"
