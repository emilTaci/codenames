package main

import (
	"math/rand/v2"
	"sync"
)

type Team string

const (
	TeamRed  Team = "red"
	TeamBlue Team = "blue"
)

type CardColor string

const (
	ColorRed      CardColor = "red"
	ColorBlue     CardColor = "blue"
	ColorNeutral  CardColor = "neutral"
	ColorAssassin CardColor = "assassin"
)

type Role string

const (
	RoleSpymaster Role = "spymaster"
	RoleOperative Role = "operative"
)

type Phase string

const (
	PhaseLobby    Phase = "lobby"
	PhaseClue     Phase = "clue"
	PhaseGuess    Phase = "guess"
	PhaseGameOver Phase = "gameover"
)

const BoardSize = 25

type Card struct {
	Word     string    `json:"word"`
	Color    CardColor `json:"color"`
	Revealed bool      `json:"revealed"`
}

type Clue struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type Game struct {
	mu sync.RWMutex

	Board         [BoardSize]Card `json:"board"`
	CurrentTeam   Team            `json:"currentTeam"`
	Phase         Phase           `json:"phase"`
	CurrentClue   *Clue           `json:"currentClue"`
	GuessesLeft   int             `json:"guessesLeft"`
	RedRemaining  int             `json:"redRemaining"`
	BlueRemaining int             `json:"blueRemaining"`
	Winner        Team            `json:"winner"`
	Log           []string        `json:"log"`
}

func NewGame() *Game {
	g := &Game{}
	g.reset()
	return g
}

func (g *Game) reset() {
	words := pickWords(BoardSize)

	colors := make([]CardColor, BoardSize)
	for i := 0; i < 9; i++ {
		colors[i] = ColorRed
	}
	for i := 9; i < 17; i++ {
		colors[i] = ColorBlue
	}
	for i := 17; i < 24; i++ {
		colors[i] = ColorNeutral
	}
	colors[24] = ColorAssassin

	rand.Shuffle(len(colors), func(i, j int) {
		colors[i], colors[j] = colors[j], colors[i]
	})

	for i := 0; i < BoardSize; i++ {
		g.Board[i] = Card{
			Word:     words[i],
			Color:    colors[i],
			Revealed: false,
		}
	}

	g.CurrentTeam = TeamRed
	g.Phase = PhaseLobby
	g.CurrentClue = nil
	g.GuessesLeft = 0
	g.RedRemaining = 9
	g.BlueRemaining = 8
	g.Winner = ""
	g.Log = []string{}
}

func (g *Game) StartGame() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Phase = PhaseClue
	g.Log = append(g.Log, "Game started! Red team's turn to give a clue.")
}

func (g *Game) NewGame() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reset()
	g.Phase = PhaseClue
	g.Log = append(g.Log, "New game started! Red team's turn to give a clue.")
}

func (g *Game) GiveClue(team Team, word string, count int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Phase != PhaseClue || g.CurrentTeam != team {
		return false
	}

	g.CurrentClue = &Clue{Word: word, Count: count}
	g.GuessesLeft = count + 1
	g.Phase = PhaseGuess
	g.Log = append(g.Log, string(team)+" spymaster: "+word+" "+itoa(count))
	return true
}

func (g *Game) Guess(team Team, index int) (ok bool, turnOver bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Phase != PhaseGuess || g.CurrentTeam != team {
		return false, false
	}
	if index < 0 || index >= BoardSize || g.Board[index].Revealed {
		return false, false
	}

	card := &g.Board[index]
	card.Revealed = true
	g.Log = append(g.Log, string(team)+" operative guessed: "+card.Word)

	switch card.Color {
	case ColorAssassin:
		g.Phase = PhaseGameOver
		if team == TeamRed {
			g.Winner = TeamBlue
		} else {
			g.Winner = TeamRed
		}
		g.Log = append(g.Log, "ASSASSIN! "+string(g.Winner)+" team wins!")
		return true, true

	case ColorRed:
		g.RedRemaining--
		if g.RedRemaining == 0 {
			g.Phase = PhaseGameOver
			g.Winner = TeamRed
			g.Log = append(g.Log, "Red team found all their words! Red wins!")
			return true, true
		}
		if team != TeamRed {
			g.switchTurn()
			return true, true
		}

	case ColorBlue:
		g.BlueRemaining--
		if g.BlueRemaining == 0 {
			g.Phase = PhaseGameOver
			g.Winner = TeamBlue
			g.Log = append(g.Log, "Blue team found all their words! Blue wins!")
			return true, true
		}
		if team != TeamBlue {
			g.switchTurn()
			return true, true
		}

	case ColorNeutral:
		g.switchTurn()
		return true, true
	}

	g.GuessesLeft--
	if g.GuessesLeft <= 0 {
		g.switchTurn()
		return true, true
	}

	return true, false
}

func (g *Game) EndTurn(team Team) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Phase != PhaseGuess || g.CurrentTeam != team {
		return false
	}

	g.Log = append(g.Log, string(team)+" team ended their turn.")
	g.switchTurn()
	return true
}

func (g *Game) switchTurn() {
	if g.CurrentTeam == TeamRed {
		g.CurrentTeam = TeamBlue
	} else {
		g.CurrentTeam = TeamRed
	}
	g.Phase = PhaseClue
	g.CurrentClue = nil
	g.GuessesLeft = 0
	g.Log = append(g.Log, string(g.CurrentTeam)+" team's turn to give a clue.")
}

type GameStateForClient struct {
	Board         [BoardSize]CardForClient `json:"board"`
	CurrentTeam   Team                     `json:"currentTeam"`
	Phase         Phase                    `json:"phase"`
	CurrentClue   *Clue                    `json:"currentClue"`
	GuessesLeft   int                      `json:"guessesLeft"`
	RedRemaining  int                      `json:"redRemaining"`
	BlueRemaining int                      `json:"blueRemaining"`
	Winner        Team                     `json:"winner"`
	Log           []string                 `json:"log"`
}

type CardForClient struct {
	Word     string    `json:"word"`
	Color    CardColor `json:"color"`
	Revealed bool      `json:"revealed"`
}

func (g *Game) StateFor(role Role) GameStateForClient {
	g.mu.RLock()
	defer g.mu.RUnlock()

	isSpymaster := role == RoleSpymaster
	gameOver := g.Phase == PhaseGameOver

	var board [BoardSize]CardForClient
	for i, c := range g.Board {
		board[i] = CardForClient{
			Word:     c.Word,
			Revealed: c.Revealed,
		}
		if c.Revealed || isSpymaster || gameOver {
			board[i].Color = c.Color
		}
	}

	return GameStateForClient{
		Board:         board,
		CurrentTeam:   g.CurrentTeam,
		Phase:         g.Phase,
		CurrentClue:   g.CurrentClue,
		GuessesLeft:   g.GuessesLeft,
		RedRemaining:  g.RedRemaining,
		BlueRemaining: g.BlueRemaining,
		Winner:        g.Winner,
		Log:           g.Log,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
