// ohax — おはツイKeeper(ohatwikeeper.com)の公式CLI。記録(プロフィール・推移グラフ・
// 投稿グラス・アワード・ギャラリー)をターミナルにフルカラーで表示する。
//
// データはおはツイKeeperの公開API(/api/v2/public/users/{uuid}/*)から取り、
// 画面はすべてこのCLIで描画する。Go標準ライブラリのみ。
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

var version = "dev"

const defaultTimeout = 20 * time.Second

// セクション名 → ブラウザ版のURLパス。プロフィールは空パス。
var sections = []struct{ name, path, label string }{
	{"profile", "", "プロフィール"},
	{"graph", "graph", "推移グラフ"},
	{"grass", "grass", "投稿グラス"},
	{"awards", "awards", "アワード"},
	{"gallery", "gallery", "ギャラリー"},
}

// rssは生XMLなので"all"には含めない
var extraSections = map[string]string{"rss": "rss"}

var uuidRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type options struct {
	colorForce int // 1: --color, -1: --no-color, 0: 自動
	noImages   bool
	limit      int
	weeks      int
	days       int
	width      int
}

func main() {
	// go install で入れた場合は -ldflags が無いので、モジュールのバージョン(v0.1.0等)を使う
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
	}
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var opts options
	var pos []string
	intFlag := func(name string, i *int, dst *int) bool {
		a := args[*i]
		val := ""
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			val = v
		} else if *i+1 < len(args) {
			*i++
			val = args[*i]
		}
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "✗ %s には正の整数を指定してください\n", name)
			return false
		}
		*dst = n
		return true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, _, _ := strings.Cut(a, "=")
		switch {
		case a == "-n" || a == "--no-color" || a == "--nocolor":
			opts.colorForce = -1
		case a == "--color":
			opts.colorForce = 1
		case a == "--no-images" || a == "--no-image":
			opts.noImages = true
		case name == "--limit" || name == "-l":
			if !intFlag(name, &i, &opts.limit) {
				return 2
			}
		case name == "--weeks" || name == "-w":
			if !intFlag(name, &i, &opts.weeks) {
				return 2
			}
		case name == "--days" || name == "-d":
			if !intFlag(name, &i, &opts.days) {
				return 2
			}
		case name == "--width":
			if !intFlag(name, &i, &opts.width) {
				return 2
			}
		case a == "-h" || a == "--help":
			setupColor(opts.colorForce)
			usage(os.Stdout)
			return 0
		case a == "-v" || a == "--version":
			fmt.Println("ohax", version)
			fmt.Print(lapiusFooter())
			return 0
		case a == "--clear": // ohax use --clear
			pos = append(pos, a)
		case a == "--":
			pos = append(pos, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			fmt.Fprintf(os.Stderr, "✗ 不明なオプション: %s (ohax --help で一覧を見られます)\n", a)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	setupColor(opts.colorForce)

	// 引数なしはヘルプ(既定ユーザーのプロフィールは ohax profile で見る)
	if len(pos) == 0 {
		usage(os.Stdout)
		return 0
	}
	cmd := pos[0]
	switch cmd {
	case "help":
		usage(os.Stdout)
		return 0
	case "version":
		fmt.Println("ohax", version)
		fmt.Print(lapiusFooter())
		return 0
	case "use":
		return cmdUse(pos[1:])
	case "whoami":
		return cmdWhoami()
	case "all":
		return withTarget(pos[1:], "", func(uuid, _ string) int {
			v := newView(uuid, opts)
			for _, fn := range []func(view) error{renderProfile, renderGraph, renderGrass, renderAwards, renderGallery} {
				if err := fn(v); err != nil {
					return fail(err, uuid)
				}
			}
			return 0
		})
	case "open", "url":
		rest := pos[1:]
		section := ""
		if len(rest) > 0 && isSection(rest[0]) {
			section, rest = rest[0], rest[1:]
		}
		return withTarget(rest, section, func(uuid, sec string) int {
			u := browserURL(uuid, sec)
			if cmd == "url" {
				fmt.Println(u)
				return 0
			}
			if err := openBrowser(u); err != nil {
				fmt.Fprintf(os.Stderr, "%s ブラウザを開けませんでした: %v\n  %s\n", paint(cRed, "✗"), err, u)
				return 1
			}
			fmt.Println(paint(cSky, "↗") + " " + link(u, u))
			return 0
		})
	}

	section := ""
	rest := pos
	if isSection(cmd) {
		section, rest = cmd, pos[1:]
	}
	return withTarget(rest, section, func(uuid, sec string) int {
		v := newView(uuid, opts)
		var err error
		switch sec {
		case "graph":
			err = renderGraph(v)
		case "grass":
			err = renderGrass(v)
		case "awards":
			err = renderAwards(v)
		case "gallery":
			err = renderGallery(v)
		case "rss":
			err = printRSS(uuid)
		default:
			err = renderProfile(v)
		}
		if err != nil {
			return fail(err, uuid)
		}
		return 0
	})
}

func newView(uuid string, opts options) view {
	v := view{
		uuid:      uuid,
		w:         termWidth(opts.width),
		images:    colorLevel > 0 && !opts.noImages,
		limit:     opts.limit,
		weeks:     opts.weeks,
		days:      opts.days,
		isDefault: uuid == defaultUUID(),
	}
	if v.limit == 0 {
		v.limit = 12
		if !v.images {
			v.limit = 20
		}
	}
	return v
}

func fail(err error, uuid string) int {
	if errors.Is(err, errNotFound) {
		fmt.Fprintf(os.Stderr, "%s ユーザー「%s」は見つかりませんでした。\n", paint(cRed, "✗"), uuid)
		fmt.Fprintln(os.Stderr, dim("  UUIDが正しいか、公開設定がオンになっているか確かめてください。"))
		return 1
	}
	fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
	return 1
}

// withTarget は引数(UUIDかURL)→環境変数→保存済み設定の順で対象ユーザーを決める。
// URLにセクションが含まれていて、コマンドでセクションが指定されていなければそれを使う。
func withTarget(rest []string, section string, fn func(uuid, section string) int) int {
	if len(rest) > 1 {
		fmt.Fprintf(os.Stderr, "%s 引数が多すぎます: %s\n", paint(cRed, "✗"), strings.Join(rest[1:], " "))
		return 2
	}
	var uuid string
	if len(rest) == 1 {
		u, sec, err := parseTarget(rest[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
			return 2
		}
		uuid = u
		if section == "" {
			section = sec
		}
	} else {
		uuid = defaultUUID()
		if uuid == "" {
			fmt.Fprintf(os.Stderr, "%s ユーザーが指定されていません。\n", paint(cRed, "✗"))
			fmt.Fprintln(os.Stderr, dim("  ohax <public_uuid> のように指定するか、ohax use <public_uuid> で既定ユーザーを保存してください。"))
			return 2
		}
	}
	return fn(uuid, section)
}

// parseTarget は "tiskf" / "https://tiskf.ohax.pw/graph" / "ohax.pw/tiskf" /
// "ohatwikeeper.com/tiskf/awards" / "tiskf.ohatwikeeper.com" を受け付ける。
func parseTarget(s string) (uuid, section string, err error) {
	s = strings.TrimSpace(s)
	if uuidRe.MatchString(s) {
		return s, "", nil
	}
	t := s
	if i := strings.Index(t, "://"); i >= 0 {
		t = t[i+3:]
	}
	if i := strings.IndexAny(t, "?#"); i >= 0 {
		t = t[:i]
	}
	host, path, _ := strings.Cut(t, "/")
	host = strings.ToLower(host)
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })

	switch {
	case host == "ohax.pw" || host == "ohatwikeeper.com" || host == "www.ohatwikeeper.com":
		if len(parts) == 0 {
			break
		}
		uuid = parts[0]
		parts = parts[1:]
	case strings.HasSuffix(host, ".ohax.pw"):
		uuid = strings.TrimSuffix(host, ".ohax.pw")
	case strings.HasSuffix(host, ".ohatwikeeper.com"):
		uuid = strings.TrimSuffix(host, ".ohatwikeeper.com")
	}
	if uuid == "" || !uuidRe.MatchString(uuid) {
		return "", "", fmt.Errorf("「%s」はpublic_uuidまたはおはツイKeeperのURLとして読み取れません", s)
	}
	if len(parts) > 0 && isSection(parts[0]) {
		section = parts[0]
	}
	return uuid, section, nil
}

func isSection(s string) bool {
	if _, ok := extraSections[s]; ok {
		return true
	}
	for _, sec := range sections {
		if sec.name == s {
			return true
		}
	}
	return false
}

func pathOf(section string) string {
	if p, ok := extraSections[section]; ok {
		return p
	}
	for _, sec := range sections {
		if sec.name == section {
			return sec.path
		}
	}
	return ""
}

func baseURL() string {
	if b := os.Getenv("OHAX_BASE_URL"); b != "" {
		return strings.TrimRight(b, "/")
	}
	return "https://ohatwikeeper.com"
}

func browserURL(uuid, section string) string {
	u := baseURL() + "/" + uuid
	if p := pathOf(section); p != "" {
		u += "/" + p
	}
	return u
}

// printRSS はRSSフィード(XML)をそのまま出力する
func printRSS(uuid string) error {
	u := baseURL() + "/" + uuid + "/rss"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ohax-cli/"+version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("おはツイKeeperに接続できませんでした: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("RSSを取得できませんでした (HTTP %d)", resp.StatusCode)
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]|\x1b\\]8;[^\x1b\x07]*(?:\x1b\\\\|\x07)")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func openBrowser(u string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", u)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	return c.Start()
}

func usage(w io.Writer) {
	h := func(s string) string { return boldPaint(cSun, s) }
	c := func(s string) string { return paint(cSky, s) }
	lines := []string{
		"",
		" " + gradientText("☀ ohax", sunrise) + "  " + dim("おはツイKeeper 公式CLI ("+version+")"),
		"",
		h(" 見る"),
		"   " + c("ohax <user>") + "                 プロフィール",
		"   " + c("ohax graph   [<user>]") + "       推移グラフ(いいね・表示・リポスト・返信)",
		"   " + c("ohax grass   [<user>]") + "       投稿グラス(日ごとの投稿カレンダー)",
		"   " + c("ohax awards  [<user>]") + "       アワードと次の目標",
		"   " + c("ohax gallery [<user>]") + "       画像ギャラリー(サムネイル付き)",
		"   " + c("ohax all     [<user>]") + "       上の5つをまとめて表示",
		"   " + c("ohax profile [<user>]") + "       プロフィール(既定ユーザーならこちら)",
		"   " + c("ohax rss     [<user>]") + "       RSSフィード(XML)",
		"",
		h(" 既定ユーザー"),
		"   " + c("ohax use <user>") + "             保存する(以後<user>を省略できる)",
		"   " + c("ohax use --clear") + "            削除する",
		"   " + c("ohax whoami") + "                 保存中のユーザーを表示",
		"",
		h(" ブラウザ"),
		"   " + c("ohax open [<page>] [<user>]") + " ブラウザで開く(例: ohax open graph)",
		"   " + c("ohax url  [<page>] [<user>]") + " URLを表示する",
		"",
		h(" オプション"),
		"   " + c("-d, --days <N>") + "              graph: 直近N日だけ表示",
		"   " + c("-w, --weeks <N>") + "             grass: 表示する週数(既定は画面幅に合わせる)",
		"   " + c("-l, --limit <N>") + "             gallery: 表示する件数(既定12)",
		"   " + c("    --no-images") + "             アバター・サムネイルを表示しない",
		"   " + c("    --width <N>") + "             表示幅を指定する",
		"   " + c("-n, --no-color") + "              色なしで表示(パイプ先やNO_COLOR設定時は自動)",
		"   " + c("    --color") + "                 パイプ先でも色付きで出力",
		"   " + c("-v, --version") + "               バージョン",
		"",
		" " + dim("<user> には public_uuid(例: tiskf)か、共有URLをそのまま渡せます:"),
		" " + dim("  https://tiskf.ohax.pw/graph ・ ohatwikeeper.com/tiskf/awards"),
		"",
		" " + dim("環境変数: OHAX_UUID(既定ユーザー) / NO_COLOR / OHAX_COLOR=truecolor|256|none"),
		" " + dim("詳しくは https://ohatwikeeper.com/cli"),
		"",
	}
	fmt.Fprintln(w, strings.Join(lines, "\n"))
	fmt.Fprint(w, lapiusFooter())
}
