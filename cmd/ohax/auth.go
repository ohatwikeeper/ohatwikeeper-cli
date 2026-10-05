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

func requireLogin() (string, bool) {
	k := apiKey()
	if k == "" {
		fmt.Fprintln(os.Stderr, paint(cRed, "✗")+" ログインが必要です。 "+paint(cSky, "ohax login")+dim(" (Lapount でログインします)"))
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

func cmdLogin(args []string) int {
	fmt.Fprintln(os.Stderr, bold("おはツイKeeperにログイン(Lapount)"))
	key, err := lapountLogin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", paint(cRed, "✗"), err)
		return 1
	}
	m, err := getMe(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s ログインに失敗しました: %v\n", paint(cRed, "✗"), err)
		return 1
	}
	p, err := saveConfig(config{UUID: m.PublicUUID, APIKey: key})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s 設定を保存できませんでした: %v\n", paint(cRed, "✗"), err)
		return 1
	}
	fmt.Printf("%s %s としてログインしました %s\n", paint(cGreen, "✓"), bold(m.label()), dim("("+m.PublicUUID+")"))
	fmt.Println(dim("  保存先: " + p))
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
	fmt.Fprintln(os.Stderr, "ブラウザで次のURLを開き、コード "+bold(st.UserCode)+" を承認してください:")
	fmt.Fprintln(os.Stderr, "  "+paint(cSky, st.VerificationURL))
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
	if p, err := configPath(); err == nil {
		_ = os.Remove(p)
	}
	fmt.Println(paint(cGreen, "✓") + " ログアウトしました(保存したAPIキーを削除)")
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
	fmt.Printf("%s %s\n", bold(m.label()), dim("("+m.PublicUUID+")"))
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
	fmt.Printf("%s 登録 %d / 既に登録済み %d / 失敗 %d\n", paint(cGreen, "✓"), r.Summary.Success, r.Summary.Duplicate, r.Summary.Error)
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
	fmt.Printf("%s %d 件削除しました\n", paint(cGreen, "✓"), r.Deleted)
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
	for _, x := range r.Records {
		t := []rune(strings.ReplaceAll(x.Text, "\n", " "))
		if len(t) > 40 {
			t = append(t[:40], '…')
		}
		fmt.Printf("%s  %s  %s\n", dim(x.Uniqid), x.Date, string(t))
	}
	return 0
}
