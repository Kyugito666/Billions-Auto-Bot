// file: sessionRefresher.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdp/cdp/network"
	"github.com/chromedp/chromedp"
)

func main() {
	lines, _ := readLines("cookies.txt")
	fmt.Println("Pilih akun yang ingin di-refresh sesinya:")
	if len(lines) > 0 {
		for i, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) == 2 {
				fmt.Printf("%d: %s\n", i+1, parts[0])
			}
		}
	}
	fmt.Printf("%d: Tambah Akun Baru\n", len(lines)+1)

	var choice int
	fmt.Print("Masukkan pilihan -> ")
	_, err := fmt.Scan(&choice)
	if err != nil || choice < 1 || choice > len(lines)+1 {
		log.Fatal("Pilihan tidak valid.")
	}

	var profileName string
	if choice > len(lines) {
		fmt.Print("Masukkan nama untuk profil baru (contoh: akun_satu) -> ")
		fmt.Scan(&profileName)
	} else {
		profileName = strings.Split(lines[choice-1], "|")[0]
	}

	log.Printf("Menggunakan profil: %s", profileName)
	profilePath, err := filepath.Abs(filepath.Join("profiles", profileName))
	if err != nil {
		log.Fatalf("Gagal membuat path profil: %v", err)
	}
	log.Printf("Menyimpan data browser di: %s", profilePath)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("start-maximized", true),
		chromedp.UserDataDir(profilePath),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	var sessionCookie string
	log.Println("Browser akan terbuka. Silakan login...")
	log.Println("Setelah login dan dashboard terlihat, program akan otomatis mengambil sesi.")

	err = chromedp.Run(ctx,
		chromedp.Navigate(`https://signup.billions.network?rc=J3I4P2TG`),
		chromedp.WaitVisible(`#__next > div.sc-a8d8f459-0.jFrzOz > div > div > div > p.sc-a8d8f459-7.bNRZCf`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			for _, cookie := range cookies {
				if cookie.Name == "session_id" {
					sessionCookie = "session_id=" + cookie.Value
					return nil
				}
			}
			return fmt.Errorf("cookie session_id tidak ditemukan")
		}),
	)

	if err != nil {
		log.Fatalf("Gagal menyelesaikan alur login, mungkin timeout atau selector berubah: %v", err)
	}

	if sessionCookie != "" {
		updateCookieFile(profileName, sessionCookie, choice-1, len(lines))
		log.Printf("Berhasil me-refresh dan menyimpan sesi untuk profil '%s'", profileName)
	} else {
		log.Println("Gagal mendapatkan session. Silakan coba lagi.")
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

func updateCookieFile(profileName, newCookie string, index, totalLines int) {
	lines, err := readLines("cookies.txt")
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Gagal membaca cookies.txt: %v", err)
	}
	newLine := fmt.Sprintf("%s|%s", profileName, newCookie)
	if index >= totalLines {
		lines = append(lines, newLine)
	} else {
		lines[index] = newLine
	}
	err = os.WriteFile("cookies.txt", []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		log.Fatalf("Gagal menulis ke cookies.txt: %v", err)
	}
}
