package vk2tg

import (
	"log"
	"strings"
	"time"

	"github.com/kljensen/snowball/russian"
	vkapi "github.com/lexfrei/golang-vk-api"
)

func WatchNewPosts(tick <-chan time.Time, posts chan<- vkapi.WallPost, client *vkapi.VKClient) {
	select {
	case <-tick:
		w, err := client.WallGet("cosplay_second", 10, nil)
		// TODO: Handle too many posts
		var last int = 0
		if err != nil {
			log.Fatalln(err)
		}

		for i := len(w.Posts) - 1; i >= 0; i-- {
			if last > w.Posts[i].ID {
				break
			} else {
				last = w.Posts[i].ID
			}

			var contains bool = false
			wrds := reSplitter.Split(w.Posts[i].Text, -1)
			for _, wrd := range wrds {
				for _, key := range keys {
					if strings.Contains(russian.Stem(wrd, false), key) {
						contains = true
					}
				}
				if contains {
					break
				}
			}

			posts <- *w.Posts[i]
		}
	}
}
