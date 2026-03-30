package main

import (
	"encoding/json"
	"log"
	"sync"
)

type Hub struct {
	game       *Game
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewHub(game *Game) *Hub {
	return &Hub{
		game:       game,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Player joined: %s", client.Name)
			h.broadcastState()
			h.broadcastPlayers()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("Player left: %s", client.Name)
			h.broadcastPlayers()
		}
	}
}

type PlayerInfo struct {
	Name string `json:"name"`
	Team Team   `json:"team"`
	Role Role   `json:"role"`
}

type OutgoingMessage struct {
	Type    string              `json:"type"`
	State   *GameStateForClient `json:"state,omitempty"`
	Players []PlayerInfo        `json:"players,omitempty"`
	Error   string              `json:"error,omitempty"`
}

func (h *Hub) handleMessage(c *Client, msg *IncomingMessage) {
	switch msg.Type {
	case "set_role":
		c.Team = Team(msg.Team)
		c.Role = Role(msg.Role)
		log.Printf("%s set role: %s %s", c.Name, c.Team, c.Role)
		h.broadcastPlayers()
		h.sendStateTo(c)

	case "start_game":
		h.game.StartGame()
		h.broadcastState()

	case "give_clue":
		if c.Role != RoleSpymaster {
			h.sendError(c, "Only spymasters can give clues.")
			return
		}
		if msg.Word == "" || msg.Count < 0 {
			h.sendError(c, "Invalid clue.")
			return
		}
		if !h.game.GiveClue(c.Team, msg.Word, msg.Count) {
			h.sendError(c, "Not your turn to give a clue.")
			return
		}
		h.broadcastState()

	case "guess":
		if c.Role != RoleOperative {
			h.sendError(c, "Only operatives can guess.")
			return
		}
		ok, _ := h.game.Guess(c.Team, msg.Index)
		if !ok {
			h.sendError(c, "Invalid guess.")
			return
		}
		h.broadcastState()

	case "end_turn":
		if !h.game.EndTurn(c.Team) {
			h.sendError(c, "Cannot end turn right now.")
			return
		}
		h.broadcastState()

	case "new_game":
		h.game.NewGame()
		h.broadcastState()
	}
}

func (h *Hub) broadcastState() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		h.sendStateTo(client)
	}
}

func (h *Hub) sendStateTo(c *Client) {
	state := h.game.StateFor(c.Role)
	msg := OutgoingMessage{
		Type:  "state",
		State: &state,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

func (h *Hub) broadcastPlayers() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	players := make([]PlayerInfo, 0, len(h.clients))
	for client := range h.clients {
		players = append(players, PlayerInfo{
			Name: client.Name,
			Team: client.Team,
			Role: client.Role,
		})
	}

	msg := OutgoingMessage{
		Type:    "players",
		Players: players,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}

	for client := range h.clients {
		select {
		case client.send <- data:
		default:
		}
	}
}

func (h *Hub) sendError(c *Client, errMsg string) {
	msg := OutgoingMessage{
		Type:  "error",
		Error: errMsg,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}
