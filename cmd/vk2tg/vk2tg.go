package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	vkapi "github.com/himidori/golang-vk-api"
	"github.com/kljensen/snowball/russian"
	tb "github.com/tucnak/telebot"
)

var reSplitter = regexp.MustCompile(`(?m)[^А-Яа-я]`)

var keys = []string{
	"ката",
	"кинжа",
	"клинок",
	"меч",
	"изготов",
	"сошьет",
	"сдела",
	"пересыл",
	"парик",
	"лейс",
}

func main() {
	ticker := time.NewTicker(1 * time.Minute)

	posts := make(chan vkapi.WallPost)

	vkClient, err := vkapi.NewVKClientWithToken(os.Getenv("V2T_VK_TOKEN"), nil, true)
	if err != nil {
		log.Fatalf("Can't longin to VK: %s\n", err)
	}
	log.Printf("Successfully logged to VK\n")

	tgBot, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv("V2T_TG_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatalf("Can't longin to TG: %s\n", err)
	}
	log.Printf("Successfully logged to TG as %s\n", tgBot.Me.Username)

	go watchNewPosts(ticker.C, posts, vkClient)
	go sendToTG(posts, tgBot)
	fmt.Scanln()
}

func sendToTG(posts <-chan vkapi.WallPost, bot *tb.Bot) {
	for p := range posts {

		inlineBtn1 := tb.InlineButton{
			Text: "🌎 Оригинал",
			URL:  "https://vk.com/wall-57692133_" + strconv.Itoa(p.ID),
		}
		inlineBtn2 := tb.InlineButton{
			Text: "✍️ Написать",
			URL:  "vk.com/write" + strconv.Itoa(p.SignerID),
		}
		inlineKeys := [][]tb.InlineButton{
			{inlineBtn1, inlineBtn2},
		}

		// 240336636
		// 74194657
		_, err := bot.Send(&tb.User{ID: 74194657}, p.Text, &tb.ReplyMarkup{
			InlineKeyboard: inlineKeys,
		})
		if err != nil {
			log.Printf("Can't send message: %s\n", err)
		} else {
			log.Println("Message sent to Andrey")
		}

		log.Println("Message sent to Andrey")
	}
}

func watchNewPosts(tick <-chan time.Time, posts chan<- vkapi.WallPost, client *vkapi.VKClient) {
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

			if !strings.Contains(w.Posts[i].Text, "#поиск") {
				continue
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
