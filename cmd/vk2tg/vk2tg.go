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
	Period  time.Duration
	Paused  bool
	WG      *sync.WaitGroup
	Silent  bool
}

type stat struct {
	LastPost   vkapi.WallPost
	LastUpdate time.Time
	StartTime  time.Time
}

var c conf
var st stat

func init() {
	var err error

	c.Period = time.Minute
	c.Paused = false
	c.WG = &sync.WaitGroup{}
	c.Silent = false

	st.StartTime = time.Now()

	c.TGToken = os.Getenv("V2T_TG_TOKEN")
	c.VKToken = os.Getenv("V2T_VK_TOKEN")
	c.TGUser, err = strconv.Atoi(os.Getenv("V2T_TG_USER"))
	if err != nil {
		log.Fatalln("Invalid TG user ID: " + os.Getenv("V2T_TG_USER"))
	}
}

func main() {
	ticker := time.NewTicker(c.Period)

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
		msg := fmt.Sprintf("I'm fine\nLast post date:\t%s\nRecived in:\t%s\nUptime:\t%s\nPaused:\t%t\nSound:\t%t",
			time.Unix(st.LastPost.Date, 0).In(time.FixedZone("UTC+3", 3*60*60)).Format(time.RFC822),
			st.LastUpdate.In(time.FixedZone("UTC+3", 3*60*60)).Format(time.RFC822),
			time.Since(st.StartTime).Round(time.Second),
			c.Paused,
			!c.Silent,
		)
		_, err = tgBot.Send(m.Sender, msg)
		if err != nil {
			log.Printf("Error on sending status: %s", err)
		}
	})

	tgBot.Handle("/pause", func(m *tb.Message) {
		ch, err := tgBot.ChatByID(strconv.Itoa(c.TGUser))
		if err != nil {
			log.Printf("Error on fetching user info: %s", err)
		}
		c.Paused = !c.Paused
		if c.Paused {
			defer log.Printf("Paused by user: %s", ch.FirstName)
			ticker.Stop()
			_, err = tgBot.Send(m.Sender, "Paused! Send /pause to continue")
			if err != nil {
				log.Printf("Error on sending status: %s", err)
			}
		} else {
			defer log.Printf("Unpaused by user: %s", ch.FirstName)
			ticker.Reset(c.Period)
			_, err = tgBot.Send(m.Sender, "Unpaused! Send /pause to stop")
			if err != nil {
				log.Printf("Error on sending status: %s", err)
			}
		}
	})

	tgBot.Handle("/mute", func(m *tb.Message) {
		ch, err := tgBot.ChatByID(strconv.Itoa(c.TGUser))
		if err != nil {
			log.Printf("Error on fetching user info: %s", err)
		}
		c.Silent = !c.Silent
		if c.Silent {
			defer log.Printf("Muted by user: %s", ch.FirstName)
			ticker.Stop()
			_, err = tgBot.Send(m.Sender, "Muted! Send /mute to go loud")
			if err != nil {
				log.Printf("Error on sending status: %s", err)
			}
		} else {
			defer log.Printf("Unmuted by user: %s", ch.FirstName)
			ticker.Reset(c.Period)
			_, err = tgBot.Send(m.Sender, "Unmuted! Send /mute to go silent")
			if err != nil {
				log.Printf("Error on sending status: %s", err)
			}
		}
	})

	go tgBot.Start()
	go sendPostToTG(posts, tgBot, c.TGUser)
	go watchNewPosts(ticker.C, posts, vkClient)

	c.WG.Add(2)
	c.WG.Wait()
}

func sendPostToTG(posts <-chan vkapi.WallPost, bot *tb.Bot, user int) {
	defer c.WG.Done()
	defer log.Println("Sender: done")
	for p := range posts {
		for _, a := range p.Attachments {
			log.Println(a.Photo.Photo604)
		}
		_, err := bot.Send(
			&tb.User{ID: user},
			p.Text,
			&tb.SendOptions{
				ReplyTo: &tb.Message{},
				ReplyMarkup: &tb.ReplyMarkup{
					InlineKeyboard: [][]tb.InlineButton{
						{
							tb.InlineButton{
								Text: "🌎 К посту",
								URL:  "https://vk.com/wall-57692133_" + strconv.Itoa(p.ID),
							},
							tb.InlineButton{
								Text: "✍️ Написать",
								URL:  "vk.com/write" + strconv.Itoa(p.SignerID),
							},
						},
					},
				},
				DisableNotification: c.Silent,
			},
		)
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
	defer c.WG.Done()
	defer log.Println("Fetcher: done")
	for range tick {
		st.LastUpdate = time.Now()
		log.Println("Fetching new posts")
		w, err := client.WallGet("cosplay_second", 10, nil)
		if err != nil {
			log.Printf("failed to fetch posts: %s", err)
			continue
		}

		if w.Posts[0].ID == st.LastPost.ID {
			log.Printf("No new posts found")
			continue
		}

		for i := len(w.Posts) - 1; i >= 0; i-- {
			log.Printf("Post %d: Processing", w.Posts[i].ID)

			if st.LastPost.ID > w.Posts[i].ID {
				log.Printf("Post %d: Not a new post, skipped", w.Posts[i].ID)
				continue
			} else {
				log.Printf("Post %d: Selected as latest", w.Posts[i].ID)
				st.LastPost = *w.Posts[i]
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
