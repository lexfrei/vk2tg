package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	vkapi "github.com/himidori/golang-vk-api"
	tb "github.com/tucnak/telebot"
)

type conf struct {
	TGToken string
	VKToken string
	TGUser  int
}

var c conf
var wg = &sync.WaitGroup{}
var last vkapi.WallPost
var lastUpdate time.Time
var startTime time.Time

func init() {
	var err error
	c.TGToken = os.Getenv("V2T_TG_TOKEN")
	c.VKToken = os.Getenv("V2T_VK_TOKEN")
	c.TGUser, err = strconv.Atoi(os.Getenv("V2T_TG_USER"))
	if err != nil {
		log.Fatalln("Invalid TG user ID: " + os.Getenv("V2T_TG_USER"))
	}
	startTime = time.Now()
}

func main() {
	ticker := time.NewTicker(1 * time.Minute)

	posts := make(chan vkapi.WallPost)

	vkClient, err := vkapi.NewVKClientWithToken(c.VKToken, nil, true)
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

	tgBot.Handle("/status", func(m *tb.Message) {
		msg := fmt.Sprintf("I'm fine\nLast post date:\t%s\nRecived in:\t%s\nUptime:\t%s",
			time.Unix(last.Date, 0).In(time.FixedZone("UTC+3", 3*60*60)).Format(time.RFC822),
			lastUpdate.In(time.FixedZone("UTC+3", 3*60*60)).Format(time.RFC822),
			time.Since(startTime).Round(time.Second),
		)
		_, err = tgBot.Send(m.Sender, msg)
		if err != nil {
			log.Printf("Error on sending status: %s", err)
		}

	})

	go tgBot.Start()
	go sendToTG(posts, tgBot, c.TGUser)
	go watchNewPosts(ticker.C, posts, vkClient)

	wg.Add(2)
	wg.Wait()
}

func sendToTG(posts <-chan vkapi.WallPost, bot *tb.Bot, user int) {
	defer wg.Done()
	defer log.Println("Sender: done")
	for p := range posts {

		toThePost := tb.InlineButton{
			Text: "🌎 К посту",
			URL:  "https://vk.com/wall-57692133_" + strconv.Itoa(p.ID),
		}
		toMessages := tb.InlineButton{
			Text: "✍️ Написать",
			URL:  "vk.com/write" + strconv.Itoa(p.SignerID),
		}
		postInlineKeys := [][]tb.InlineButton{
			{toThePost, toMessages},
		}

		_, err := bot.Send(&tb.User{ID: user}, p.Text, &tb.ReplyMarkup{
			InlineKeyboard: postInlineKeys,
		})
		if err != nil {
			log.Printf("Can't send message: %s\n", err)
		}
		ch, err := bot.ChatByID(strconv.Itoa(c.TGUser))
		if err != nil {
			log.Printf("Error on fetching user info: %s", err)
		}
		log.Printf("Message sent to %s", ch.FirstName)
	}
}

func watchNewPosts(tick <-chan time.Time, posts chan<- vkapi.WallPost, client *vkapi.VKClient) {
	defer wg.Done()
	defer log.Println("Fetcher: done")
	for range tick {
		lastUpdate = time.Now()
		log.Println("Fetching new posts")
		w, err := client.WallGet("cosplay_second", 10, nil)
		if err != nil {
			log.Fatalln(err)
		}

		if w.Posts[0].ID == last.ID {
			log.Printf("No new posts found")
			continue
		}

		for i := len(w.Posts) - 1; i >= 0; i-- {
			log.Printf("Post %d: Processing", w.Posts[i].ID)

			if last.ID > w.Posts[i].ID {
				log.Printf("Post %d: Not a new post, skipped", w.Posts[i].ID)
				continue
			} else {
				log.Printf("Post %d: Selected as latest", w.Posts[i].ID)
				last = *w.Posts[i]
			}

			if !strings.Contains(w.Posts[i].Text, "#поиск") {
				log.Printf("Post %d: Post does not contain required substring, skipping", w.Posts[i].ID)
				continue
			}

			log.Printf("Post %d: Sending to TG", w.Posts[i].ID)
			posts <- *w.Posts[i]
		}
	}
}
