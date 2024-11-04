package words

import (
	"crypto/rand"
	"embed"
	"math/big"
	"strings"
)

//go:embed words.txt
var f embed.FS

// GenerateWord returns a random word.
func GenerateWord() (string, error) {

	data, err := f.ReadFile("words.txt")
	if err != nil {
		return "", err
	}

	words := strings.Split(string(data), "\n")
	randNum, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", err
	}

	return words[int(randNum.Int64())], nil
}
