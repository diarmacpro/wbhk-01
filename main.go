package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
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
)

// Handler WebSocket untuk klien
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

	// Tidak mendengarkan pesan dari klien
	for {
		if _, _, err := conn.NextReader(); err != nil {
			break
		}
	}
}

// Handler untuk menerima POST dan broadcast ke WebSocket
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

	// Kirim ke semua client WebSocket
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for conn := range clients {
		err := conn.WriteMessage(websocket.TextMessage, body)
		if err != nil {
			log.Println("Failed to send to client:", err)
			conn.Close()
			delete(clients, conn)
		}
	}

	fmt.Fprint(w, "ok")
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/webhook", postHandler)

	log.Println("Listening on :8080 (WebSocket: /ws, Webhook: /webhook)")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
