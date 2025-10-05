// file: bot.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

const (
	BaseAPI = "https://signup-backend.billions.network"
)

var (
	proxies    []string
	proxyIndex int
)

type UserData struct {
	Email             string  `json:"email"`
	Power             int     `json:"power"`
	NextDailyRewardAt *string `json:"nextDailyRewardAt"`
}

func loadProxies(filename string) {
	lines, err := readLines(filename)
	if err != nil {
		log.Printf("File '%s' tidak ditemukan atau tidak bisa dibaca. Bot akan berjalan tanpa proxy.", filename)
		return
	}
	proxies = lines
	if len(proxies) > 0 {
		log.Printf("Berhasil memuat %d proxy.", len(proxies))
	}
}

func getNextProxy() string {
	if len(proxies) == 0 {
		return ""
	}
	p := proxies[proxyIndex]
	proxyIndex = (proxyIndex + 1) % len(proxies)
	return p
}

func main() {
	loadProxies("proxy.txt")

	for {
		lines, err := readLines("cookies.txt")
		if err != nil {
			log.Fatalf("Gagal membaca cookies.txt. Jalankan 'sessionRefresher.go' terlebih dahulu: %v", err)
		}
		log.Printf("Total Akun: %d", len(lines))

		var cookies []string
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) == 2 && strings.HasPrefix(parts[1], "session_id=") {
				cookies = append(cookies, parts[1])
			}
		}

		if len(cookies) == 0 {
			log.Fatal("Tidak ada cookie yang valid di cookies.txt. Jalankan 'sessionRefresher.go'.")
		}

		for i, cookie := range cookies {
			log.Printf("=================[ Memproses Akun %d dari %d ]=================", i+1, len(cookies))
			proxyStr := getNextProxy()
			if proxyStr != "" {
				log.Printf("Menggunakan Proxy: %s", proxyStr)
			}
			processAccount(cookie, proxyStr)
			log.Println("Jeda 5 detik antar akun...")
			time.Sleep(5 * time.Second)
		}

		log.Println("Semua akun telah diproses. Menunggu 24 jam untuk siklus berikutnya...")
		time.Sleep(24 * time.Hour)
	}
}

func processAccount(sessionCookie, proxyStr string) {
	userData, err := getUserData(sessionCookie, proxyStr)
	if err != nil {
		log.Printf("Gagal mendapatkan data user (sesi/proxy mungkin tidak valid): %v", err)
		return
	}

	log.Printf("Akun: %s", maskEmail(userData.Email))
	log.Printf("Power: %d PTS", userData.Power)

	if userData.NextDailyRewardAt == nil {
		log.Println("Saatnya untuk klaim reward harian...")
		claimDailyReward(sessionCookie, proxyStr)
	} else {
		nextClaimTime, err := time.Parse(time.RFC3339, *userData.NextDailyRewardAt)
		if err != nil {
			log.Printf("Format waktu tidak valid: %v", err)
			return
		}
		if time.Now().UTC().After(nextClaimTime) {
			log.Println("Saatnya untuk klaim reward harian...")
			claimDailyReward(sessionCookie, proxyStr)
		} else {
			loc, _ := time.LoadLocation("Asia/Jakarta")
			log.Printf("Check-In: Belum Waktunya Klaim. Coba lagi pada: %s", nextClaimTime.In(loc).Format("02-01-2006 15:04:05 WIB"))
		}
	}
}

func makeRequest(method, urlStr, sessionCookie, proxyStr string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://signup.billions.network")
	req.Header.Set("Referer", "https://signup.billions.network/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", sessionCookie)
	if method == "POST" {
		req.Header.Set("Content-Length", "0")
	}

	transport := &http.Transport{}
	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			return nil, fmt.Errorf("format proxy tidak valid: %w", err)
		}
		if proxyURL.Scheme == "socks5" {
			dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("gagal membuat SOCKS5 dialer: %w", err)
			}
			transport.DialContext = dialer.DialContext
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
	return client.Do(req)
}

func getUserData(sessionCookie, proxyStr string) (*UserData, error) {
	resp, err := makeRequest("GET", BaseAPI+"/me", sessionCookie, proxyStr, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gagal, status code: %d. Sesi mungkin tidak valid", resp.StatusCode)
	}
	var userData UserData
	if err := json.NewDecoder(resp.Body).Decode(&userData); err != nil {
		return nil, err
	}
	return &userData, nil
}

func claimDailyReward(sessionCookie, proxyStr string) {
	resp, err := makeRequest("POST", BaseAPI+"/claim-daily-reward", sessionCookie, proxyStr, nil)
	if err != nil {
		log.Printf("Gagal klaim reward: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		log.Println("Check-In: Berhasil Diklaim - Reward: 25 Power PTS")
	} else {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Gagal klaim reward: Status %d - %s", resp.StatusCode, string(bodyBytes))
	}
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	local := parts[0]
	if len(local) > 6 {
		return fmt.Sprintf("%s***%s@%s", local[:3], local[len(local)-3:], parts[1])
	}
	return fmt.Sprintf("%s***@%s", local[:1], parts[1])
}
