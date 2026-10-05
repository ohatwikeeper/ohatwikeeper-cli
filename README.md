# ohax — おはツイKeeper 公式CLI

[おはツイKeeper](https://ohatwikeeper.com/)の公式CLI。自分や他のユーザーの記録(プロフィール・推移グラフ・投稿グラス・アワード・ギャラリー)を、ターミナルの`ohax`コマンドでフルカラー表示する。Go標準ライブラリのみ、依存パッケージなし。

truecolor(256色の端末でも可)で描画し、アバターや投稿画像のサムネイルも端末の中に表示する。表示幅は端末に合わせて自動で調整する。

## インストール

npm(Linux / macOS / Windows):

```bash
npm i -g @ohatwikeeper/cli
```

OS/CPUに合ったビルド済みバイナリが入る(Linux/macOSでは初回の起動時にnodeのシムがネイティブバイナリに置き換わり、以後はnodeを経由しない)。更新は同じコマンドで行う。

Goがある場合:

```bash
go install github.com/lapius7/ohatwikeeper-cli/cmd/ohax@latest
```

Goがない場合(Linux / macOS):

```bash
curl -fsSL https://ohatwikeeper.com/cli/install.sh | bash
```

OS/CPUに合ったビルド済みバイナリを`~/.local/bin/ohax`に置く(`OHAX_INSTALL_DIR`で変更可)。Windowsは`https://ohatwikeeper.com/cli/dl/ohax-windows-amd64.exe`を直接ダウンロードする。

詳しい使い方: https://ohatwikeeper.com/cli

## 使い方

```bash
ohax tiskf                  # プロフィール(アバター・統計・連続投稿・直近30日・最近の投稿)
ohax graph tiskf            # 推移グラフ(いいね・インプレッション・リポスト・返信)
ohax grass tiskf            # 投稿グラス(日ごとの投稿カレンダー)
ohax awards tiskf           # アワードと次の目標
ohax gallery tiskf          # 画像ギャラリー(サムネイル付き)
ohax all tiskf              # 上の5つをまとめて表示
ohax rss tiskf              # RSSフィード(XML)
ohax                        # ヘルプ
```

ユーザーの指定には、public_uuidのほか共有URLもそのまま渡せる。URLにページ名が含まれていればそのページを表示する。

```bash
ohax https://tiskf.ohax.pw/graph
ohax ohatwikeeper.com/tiskf/awards
```

### オプション

```bash
ohax graph --days 90        # 直近90日だけ
ohax grass --weeks 12       # 直近12週間だけ(既定は画面幅に合わせる)
ohax gallery --limit 30     # 30件(既定12件)
ohax gallery --no-images    # サムネイルなしの一覧
ohax --width 80 awards      # 表示幅を指定
```

### 既定ユーザー

```bash
ohax use tiskf              # 保存(存在確認してから保存する)
ohax profile                # 以後はユーザー省略でOK
ohax grass
ohax whoami                 # 保存中のユーザー
ohax use --clear            # 削除
```

保存先は`$XDG_CONFIG_HOME/ohax/config.json`(macOSは`~/Library/Application Support/ohax/`、Windowsは`%AppData%\ohax\`)。環境変数`OHAX_UUID`があればそちらが優先される。

### ブラウザ

```bash
ohax open graph             # 推移グラフをブラウザで開く
ohax url awards tiskf       # ブラウザ用URLを表示するだけ
```

### 色

出力先が端末なら色付き、パイプ・リダイレクト先や`NO_COLOR`設定時は自動で色なし(サムネイルも出さず、画像はURLで表示)。`-n`/`--no-color`で強制的に色なし、`--color`でパイプ先でも色付き。色の段階は`COLORTERM`等から判定し、`OHAX_COLOR=truecolor|256|none`で上書きできる。

## 仕組み

- データはおはツイKeeperの公開API(`https://ohatwikeeper.com/api/v2/public/users/<uuid>/...`)から取り、画面はすべてこのCLIで描画している。サーバーの`curl`向けテキストカードは使っていない
- 画像のサムネイルはpbs.twimg.comの小さいJPEGを取得し、「▀」1文字に上下2ピクセルずつ描いている。ギャラリーの「画像を開く」リンクは公開APIが返す`image_proxy_url`(img.ohatwikeeper.com)
- `OHAX_BASE_URL`で接続先を差し替えられる(開発用)

## 配布物のビルド

`./build.sh`で全OS/CPU向けにクロスコンパイルし、ohatwikeeper.comの`cli/dl/`(と`cli/install.sh`)に配置する。`OHAX_DIST_DIR=dist ./build.sh`なら手元に出すだけ。

npm版は`v*`タグをpushするとGitHub Actions(`.github/workflows/release.yml`)が`npm/build.mjs`でビルドしてnpmに公開し、GitHub Releaseにもバイナリを添付する(npmのTrusted Publisherで認証するのでトークンは不要)。手元での確認は`node npm/build.mjs 0.0.0-dev --pack`。
