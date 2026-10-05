package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ログイン(Lapountのみ)と、ログイン中アカウントで使うコマンド

func apiKey() string {
	return loadConfig().APIKey
}

// card は角丸の枠で囲んだ表示(title は枠の上に表示)
func card(col RGB, title string, rows ...string) {
	w := strWidth(title) + 4
	for _, r := range rows {
		if n := strWidth(r); n > w {
			w = n
		}
	}
	w += 2
	line := strings.Repeat("─", w)
	fmt.Println(paint(col, "╭─ ") + boldPaint(col, title) + paint(col, " "+strings.Repeat("─", max(w-strWidth(title)-3, 1))+"╮"))
	for _, r := range rows {
		fmt.Println(paint(col, "│") + " " + padRight(r, w-1) + paint(col, "│"))
	}
	fmt.Println(paint(col, "╰"+line+"╯"))
}

func accountRows(m meInfo) []string {
	return []string{
		bold(m.label()) + dim("  "+m.DisplayName),
		dim("ID  ") + m.PublicUUID,
		dim("画面 ") + baseURL() + "/" + m.PublicUUID,
	}
}

func notLoggedIn() {
	fmt.Fprintln(os.Stderr, paint(cRed, "✗")+" ログインが必要です。 "+paint(cSky, "ohax login")+dim(" (Lapount でログインします)"))
}

func requireLogin() (string, bool) {
	k := apiKey()
	if k == "" {
		notLoggedIn()
	}
	return k, k != ""
}

// call は JSON を送受信する(key があれば APIキーのヘッダーを付ける)
func call(method, url, key string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "ohax-cli/"+version)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("ohatwikeeper-api-key", key)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("おはツイKeeperに接続できませんでした: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if out != nil {
		_ = json.Unmarshal(b, out)
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return resp.StatusCode, fmt.Errorf("%s", e.Error)
	}
	return resp.StatusCode, nil
}

type meInfo struct {
	PublicUUID  string `json:"public_uuid"`
	XUsername   string `json:"x_username"`
	DisplayName string `json:"x_display_name"`
}

func (m meInfo) label() string {
	if m.XUsername != "" {
		return "@" + m.XUsername
	}
	return m.PublicUUID
}

func getMe(key string) (meInfo, error) {
	var m meInfo
	_, err := call(http.MethodGet, baseURL()+"/api/v2/user/me", key, nil, &m)
	return m, err
}

func cmdLogin(_ []string) int {
	if k := apiKey(); k != "" {
		if m, err := getMe(k); err == nil {
			card(cGreen, "ログイン済み", accountRows(m)...)
			fmt.Println()
			fmt.Println(dim("  アカウントを変えるには、先に ") + paint(cSky, "ohax logout") + dim(" してから ") + paint(cSky, "ohax login") + dim(" を実行してください"))
			return 0
		}
	}
	key, err := lapountLogin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", paint(cRed, "✗"), err)
		return 1
	}
	m, err := getMe(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s ログインに失敗しました: %v\n", paint(cRed, "✗"), err)
		return 1
	}
	if _, err := saveConfig(config{UUID: m.PublicUUID, APIKey: key}); err != nil {
		fmt.Fprintf(os.Stderr, "%s 設定を保存できませんでした: %v\n", paint(cRed, "✗"), err)
		return 1
	}
	fmt.Println()
	card(cGreen, "ログインしました", accountRows(m)...)
	fmt.Println()
	fmt.Println(dim("  使ってみる: ") + paint(cSky, "ohax list") + dim(" / ") + paint(cSky, "ohax add <ツイートURL>") + dim(" / ") + paint(cSky, "ohax all"))
	return 0
}

// lapountLogin はサーバー経由の device code で Lapount ログインし、APIキーを受け取る
func lapountLogin() (string, error) {
	var st struct {
		Code            string `json:"code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
	}
	if _, err := call(http.MethodPost, baseURL()+"/app-api/auth/cli/lapount/start", "", map[string]string{}, &st); err != nil {
		return "", fmt.Errorf("Lapountログインを開始できませんでした: %w", err)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, bold("Lapount でログイン"))
	fmt.Fprintln(os.Stderr, "  1. ブラウザで次のURLを開く")
	fmt.Fprintln(os.Stderr, "     "+paint(cSky, st.VerificationURL))
	fmt.Fprintln(os.Stderr, "  2. コード "+boldPaint(cGreen, st.UserCode)+" を確認して「承認」")
	fmt.Fprintln(os.Stderr, dim("  承認を待っています…(約"+fmt.Sprint(max(st.ExpiresIn, 60)/60)+"分で期限切れ)"))
	iv := time.Duration(max(st.Interval, 2)) * time.Second
	deadline := time.Now().Add(time.Duration(max(st.ExpiresIn, 60)) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(iv)
		var r struct {
			Status string `json:"status"`
			APIKey string `json:"api_key"`
		}
		_, err := call(http.MethodPost, baseURL()+"/app-api/auth/cli/lapount/poll", "", map[string]string{"code": st.Code}, &r)
		if err != nil {
			return "", err
		}
		if r.Status == "ok" && r.APIKey != "" {
			return r.APIKey, nil
		}
	}
	return "", fmt.Errorf("承認の待ち時間が過ぎました。もう一度 ohax login を実行してください")
}

func cmdLogout() int {
	if apiKey() == "" {
		fmt.Println(dim("ログインしていません"))
		return 0
	}
	if p, err := configPath(); err == nil {
		_ = os.Remove(p)
	}
	fmt.Println(paint(cGreen, "✓") + " ログアウトしました " + dim("(保存したログイン情報を削除)"))
	return 0
}

func cmdWhoami() int {
	key, ok := requireLogin()
	if !ok {
		return 1
	}
	m, err := getMe(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
		return 1
	}
	card(cSky, "ログイン中", accountRows(m)...)
	return 0
}

func cmdAdd(urls []string) int {
	key, ok := requireLogin()
	if !ok {
		return 1
	}
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "使い方: ohax add <ツイートURL>...")
		return 2
	}
	var r struct {
		Summary struct{ Success, Duplicate, Error int } `json:"summary"`
		Results struct {
			Error []struct{ URL, Message string } `json:"error"`
		} `json:"results"`
	}
	if _, err := call(http.MethodPost, baseURL()+"/api/v2/user/records/bulk", key, map[string]any{"urls": urls}, &r); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
		return 1
	}
	fmt.Printf("%s 登録 %s  %s 登録済み %s  %s 失敗 %s\n", paint(cGreen, "✓"), bold(fmt.Sprint(r.Summary.Success)), dim("│"), bold(fmt.Sprint(r.Summary.Duplicate)), dim("│"), bold(fmt.Sprint(r.Summary.Error)))
	for _, e := range r.Results.Error {
		fmt.Fprintf(os.Stderr, "  %s %s %s\n", paint(cRed, "✗"), e.URL, dim(e.Message))
	}
	if r.Summary.Error > 0 {
		return 1
	}
	return 0
}

func cmdRemove(args []string) int {
	key, ok := requireLogin()
	if !ok {
		return 1
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "使い方: ohax rm <記録ID または ツイートURL>...")
		return 2
	}
	var ids, urls []string
	for _, a := range args {
		if strings.HasPrefix(a, "http") {
			urls = append(urls, a)
		} else {
			ids = append(ids, a)
		}
	}
	var r struct {
		Deleted int `json:"deleted"`
	}
	if _, err := call(http.MethodDelete, baseURL()+"/api/v2/user/records/bulk", key, map[string]any{"ids": ids, "urls": urls}, &r); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
		return 1
	}
	fmt.Printf("%s %s 件削除しました\n", paint(cGreen, "✓"), bold(fmt.Sprint(r.Deleted)))
	return 0
}

func cmdList() int {
	key, ok := requireLogin()
	if !ok {
		return 1
	}
	var r struct {
		Records []struct {
			Uniqid string `json:"uniqid"`
			URL    string `json:"url"`
			Date   string `json:"date"`
			Text   string `json:"text"`
		} `json:"records"`
	}
	if _, err := call(http.MethodGet, baseURL()+"/api/v2/user/records", key, nil, &r); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
		return 1
	}
	if len(r.Records) == 0 {
		fmt.Println(dim("記録はまだありません。 ") + paint(cSky, "ohax add <ツイートURL>") + dim(" で登録できます"))
		return 0
	}
	fmt.Println(bold("自分の記録") + dim(fmt.Sprintf("  %d件", len(r.Records))))
	fmt.Println(dim(padRight("ID", 12) + padRight("日付", 12) + "本文"))
	for _, x := range r.Records {
		t := []rune(strings.ReplaceAll(x.Text, "\n", " "))
		if len(t) > 36 {
			t = append(t[:36], '…')
		}
		fmt.Println(paint(cSky, padRight(x.Uniqid, 12)) + padRight(x.Date, 12) + string(t))
	}
	return 0
}
