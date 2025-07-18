package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // izinkan semua origin
		},
	}
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex

	// regex menangkap: angka[:angka]@s.whatsapp.net
	sWhatsAppPattern = regexp.MustCompile(`\b[\d:]+@s\.whatsapp\.net\b`)
)

// WebSocket handler
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	log.Println("🟢 Client connected")
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()
		conn.Close()
		log.Println("🔴 Client disconnected")
	}()

	for {
		if _, _, err := conn.NextReader(); err != nil {
			break
		}
	}
}

// POST handler dengan validasi `from`
func postHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[WEBHOOK] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("[WEBHOOK] Body: %s", string(body))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// ❌ Abaikan jika tidak ada "message"
	msg, hasMessage := payload["message"].(map[string]interface{})
	if !hasMessage {
		log.Println("❌ Tidak ada objek 'message'")
		fmt.Fprint(w, "ignored: no message object")
		return
	}

	// ❌ Abaikan jika message.text kosong
	if text, ok := msg["text"].(string); !ok || strings.TrimSpace(text) == "" {
		log.Println("❌ message.text kosong")
		fmt.Fprint(w, "ignored: empty message.text")
		return
	}

	fromRaw, ok := payload["from"].(string)
	if !ok || fromRaw == "" {
		log.Println("❌ Tidak ada 'from' yang valid")
		fmt.Fprint(w, "ignored: no valid from")
		return
	}

	// ❌ Abaikan jika mengandung @g.us
	if strings.Contains(fromRaw, "@g.us") {
		log.Println("❌ Diblokir karena mengandung @g.us:", fromRaw)
		fmt.Fprint(w, "ignored: group detected")
		return
	}

	// ✅ Ambil yang cocok angka(:xx)?@s.whatsapp.net
	matches := sWhatsAppPattern.FindAllString(fromRaw, -1)
	if len(matches) == 0 {
		log.Println("❌ Tidak ditemukan @s.whatsapp.net valid:", fromRaw)
		fmt.Fprint(w, "ignored: invalid format")
		return
	}

	// 🔧 Bersihkan jika ada :xx dan hapus duplikasi @s.whatsapp.net
	raw := strings.Split(matches[0], ":")[0]
	cleanFrom := strings.TrimSuffix(raw, "@s.whatsapp.net") + "@s.whatsapp.net"
	payload["from"] = cleanFrom

	// Marshal ulang dan kirim ke WebSocket
	newBody, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to serialize filtered body", http.StatusInternalServerError)
		return
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()
	for conn := range clients {
		err := conn.WriteMessage(websocket.TextMessage, newBody)
		if err != nil {
			log.Println("❌ Failed to send to client:", err)
			conn.Close()
			delete(clients, conn)
		}
	}

	fmt.Fprint(w, "ok")
}

// Entry point
func main() {
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/webhook", postHandler)

	log.Println("🚀 Listening on :8080 (WebSocket: /ws, Webhook: /webhook)")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
