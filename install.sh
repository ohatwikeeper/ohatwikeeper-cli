#!/usr/bin/env bash
# ohax (おはツイKeeper 公式CLI) のワンライナーインストーラー。
#
#   curl -fsSL https://ohatwikeeper.com/cli/install.sh | bash
#
# Goは不要。GitHub Releases(ohatwikeeper/ohatwikeeper-cli)からOS・CPUに合ったビルド済みバイナリを取得して
# ~/.local/bin/ohax に置く(OHAX_INSTALL_DIRで変更可)。
set -euo pipefail

BASE_URL="${OHAX_DL_URL:-https://github.com/ohatwikeeper/ohatwikeeper-cli/releases/latest/download}"
INSTALL_DIR="${OHAX_INSTALL_DIR:-$HOME/.local/bin}"

if [ -t 1 ]; then
  BOLD=$'\033[1m'; DIM=$'\033[2m'; RESET=$'\033[0m'
  RED=$'\033[31m'; GREEN=$'\033[32m'; CYAN=$'\033[36m'; YELLOW=$'\033[33m'
else
  BOLD=""; DIM=""; RESET=""; RED=""; GREEN=""; CYAN=""; YELLOW=""
fi
info() { printf "%s→%s %s\n" "$CYAN" "$RESET" "$1"; }
ok()   { printf "%s✓%s %s\n" "$GREEN" "$RESET" "$1"; }
warn() { printf "%s!%s %s\n" "$YELLOW" "$RESET" "$1"; }
err()  { printf "%s✗%s %s\n" "$RED" "$RESET" "$1" >&2; }

printf "%sohax%s — おはツイKeeper 公式CLI インストーラー\n\n" "$BOLD" "$RESET"

case "$(uname -s)" in
  Linux) OS=linux ;;
  Darwin) OS=darwin ;;
  *) err "未対応のOSです: $(uname -s)(Windowsは npm i -g @ohatwikeeper/cli か、GitHub Releases から ohax-win32-x64.tar.gz を取得してください)"; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) ARCH=x64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) err "未対応のCPUです: $(uname -m)"; exit 1 ;;
esac

info "ohax (${OS}/${ARCH}) をダウンロード中"

mkdir -p "$INSTALL_DIR"
TMP="$(mktemp)"
TMPD="$(mktemp -d)"
trap 'rm -rf "$TMP" "$TMPD"' EXIT
if ! curl -fsSL "${BASE_URL}/ohax-${OS}-${ARCH}.tar.gz" -o "$TMP"; then
  err "ダウンロードに失敗しました"
  exit 1
fi
tar -xzf "$TMP" -C "$TMPD"
BIN="$(find "$TMPD" -type f -name ohax | head -n1)"
[ -n "$BIN" ] || { err "アーカイブに ohax が見つかりません"; exit 1; }
mv "$BIN" "$TMP"
chmod +x "$TMP"
mv "$TMP" "${INSTALL_DIR}/ohax"
trap - EXIT
ok "インストール先: ${DIM}${INSTALL_DIR}/ohax${RESET}"
ok "バージョン: ${BOLD}$("${INSTALL_DIR}/ohax" version 2>/dev/null | sed 's/^ohax //')${RESET}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    printf "\n"
    warn "PATHに ${INSTALL_DIR} が通っていません"
    printf "  シェルの設定ファイル(~/.bashrc, ~/.zshrc 等)に以下を追記してください:\n"
    printf "  %sexport PATH=\"\$PATH:%s\"%s\n" "$DIM" "$INSTALL_DIR" "$RESET"
    ;;
esac

printf "\n%s🎉 ohax のインストールが完了しました！%s\n\n" "$BOLD" "$RESET"
printf "次のステップ:\n"
printf "  %s1.%s %sohax login%s              ★推奨: Lapount でログイン(自分の記録の閲覧・登録・削除ができます)\n" "$BOLD" "$RESET" "$CYAN" "$RESET"
printf "  %s2.%s %sohax all%s                プロフィール〜ギャラリーをまとめて表示\n" "$BOLD" "$RESET" "$CYAN" "$RESET"
printf "\n%s詳細:%s https://ohatwikeeper.com/cli\n" "$DIM" "$RESET"
