// fakejf 是開發用的假 Jellyfin：只實作 xcode 用到的三個 API，用本機檔案當片源。
// 用於沒有真實 Jellyfin 帳號時的端對端測試，不要部署到正式環境。
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// item 是 items.json 的一筆：片源檔案與 Jellyfin 會回傳的 MediaSources。
type item struct {
	File         string          `json:"file"`
	MediaSources json.RawMessage `json:"MediaSources"`
}

func main() {
	listen := flag.String("listen", "127.0.0.1:18096", "listen address")
	itemsPath := flag.String("items", "items.json", "items file")
	token := flag.String("token", "devtoken", "accepted token")
	flag.Parse()

	b, err := os.ReadFile(*itemsPath)
	if err != nil {
		log.Fatal(err)
	}
	var items map[string]item
	if err := json.Unmarshal(b, &items); err != nil {
		log.Fatal(err)
	}
	base := filepath.Dir(*itemsPath)

	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != `MediaBrowser Token="`+*token+`"` {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /Users/Me", authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Id":"00000000000000000000000000000001","Name":"dev"}`))
	}))
	mux.HandleFunc("GET /Items/{id}/PlaybackInfo", authed(func(w http.ResponseWriter, r *http.Request) {
		it, ok := items[r.PathValue("id")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"MediaSources":`))
		w.Write(it.MediaSources)
		w.Write([]byte(`}`))
	}))
	mux.HandleFunc("GET /Videos/{id}/stream", authed(func(w http.ResponseWriter, r *http.Request) {
		it, ok := items[r.PathValue("id")]
		if !ok || r.URL.Query().Get("static") != "true" {
			http.NotFound(w, r)
			return
		}
		f := it.File
		if !filepath.IsAbs(f) {
			f = filepath.Join(base, f)
		}
		http.ServeFile(w, r, f) // 支援 Range
	}))

	log.Printf("fakejf on %s with %d items", *listen, len(items))
	log.Fatal(http.ListenAndServe(*listen, mux))
}
