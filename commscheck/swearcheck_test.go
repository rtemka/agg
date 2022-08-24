package main

import (
	"math/rand"
	"testing"
	"time"
)

func badText() string {
	i := rand.Intn(len(swearing) - 1)
	return "I think you are " + swearing[i]
}

func TestBanned(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	t.Run("allowed_comments", func(t *testing.T) {

		allowed := []Comment{
			{
				Text: "good comment",
			},
			{
				Text: "good comment",
			},
		}

		for i := range allowed {
			if Banned(allowed[i]) {
				t.Errorf("Banned() = comment ban %t, want %t", Banned(allowed[i]), false)
			}
		}

	})

	t.Run("banned_comments", func(t *testing.T) {
		banned := []Comment{
			{
				Text: badText(),
			},
			{
				Text: badText(),
			},
			{
				Text: badText(),
			},
		}

		for i := range banned {
			if !Banned(banned[i]) {
				t.Errorf("SwearCheck() = comment ban %t, want %t", Banned(banned[i]), true)
			}
		}

	})

}
