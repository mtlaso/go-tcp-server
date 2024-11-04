package game

import "sync"

type GuessWordGameEngine struct {
	Word    string
	Mu      sync.RWMutex
	Tries   int
	Started bool
}

// NewGame initialises a new game engine.
func (ge *GuessWordGameEngine) NewGame(word string) *GuessWordGameEngine {
	ge.Mu.Lock()
	defer ge.Mu.Unlock()
	return &GuessWordGameEngine{
		Word:    word,
		Tries:   0,
		Started: true,
	}
}

// CheckWord checks if the word was found.
// If found, it returns true and ends the game.
func (ge *GuessWordGameEngine) CheckWord(word string) bool {
	ge.Mu.Lock()
	defer func() {
		ge.Tries++
		ge.Mu.Unlock()
	}()

	if !ge.Started {
		return false
	}
	if word == ge.Word {
		ge.Started = false
		return true
	}

	return false
}

// EndGame end a game that started.
//
// Return true if the game just ended.
func (ge *GuessWordGameEngine) EndGame() bool {
	ge.Mu.Lock()
	defer ge.Mu.Unlock()
	if ge.Started {
		ge.Started = false
		return true
	}

	return false
}
