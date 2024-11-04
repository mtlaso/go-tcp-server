// Package guessword manages the word guess game.
package guessword

import "sync"

type GameEngine struct {
	Word    string
	Mu      sync.RWMutex
	Tries   int
	Started bool
}

// NewGame initialises a new game engine.
func (ge *GameEngine) NewGame(word string) *GameEngine {
	ge.Mu.Lock()
	defer ge.Mu.Unlock()
	return &GameEngine{
		Word:    word,
		Tries:   0,
		Started: true,
	}
}

// CheckWord checks if the word was found.
// If found, it returns true and ends the game.
func (ge *GameEngine) CheckWord(word string) bool {
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
func (ge *GameEngine) EndGame() bool {
	ge.Mu.Lock()
	defer ge.Mu.Unlock()
	if ge.Started {
		ge.Started = false
		return true
	}

	return false
}
