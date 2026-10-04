#!/usr/bin/env bash
# 登入 Jellyfin，把 access token 存到 ~/.config/jellyfin-extra/token（權限 600），供開發測試使用。
# 用法：tools/jf-login.sh http://<Jellyfin 內網位址>:8096 <帳號>
# 密碼以互動方式輸入，不會出現在指令列、shell 歷史或檔案裡。
set -euo pipefail

if [[ $# -ne 2 ]]; then
	echo "usage: $0 <jellyfin-url> <username>" >&2
	exit 2
fi
url=${1%/}
user=$2
out=${XDG_CONFIG_HOME:-$HOME/.config}/jellyfin-extra/token

read -rsp "Password for $user: " pw
echo

body=$(jq -n --arg u "$user" --arg p "$pw" '{Username: $u, Pw: $p}')
unset pw
auth='MediaBrowser Client="jellyfin-extra-dev", Device="devbox", DeviceId="jellyfin-extra-devbox", Version="0.1"'

resp=$(curl -sS -w '\n%{http_code}' -X POST "$url/Users/AuthenticateByName" \
	-H "Authorization: $auth" -H 'Content-Type: application/json' --data-binary @- <<<"$body")
code=${resp##*$'\n'}
json=${resp%$'\n'*}
if [[ $code != 200 ]]; then
	echo "login failed: HTTP $code" >&2
	exit 1
fi

token=$(jq -r '.AccessToken // empty' <<<"$json")
if [[ -z $token ]]; then
	echo "login failed: no AccessToken in response" >&2
	exit 1
fi
mkdir -p "$(dirname "$out")"
(umask 077 && printf '%s\n' "$token" >"$out")
echo "logged in as $(jq -r '.User.Name' <<<"$json") (user id $(jq -r '.User.Id' <<<"$json")); token saved to $out"
